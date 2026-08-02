// The LLM gateway (docs/design/llm-gateway.md, ADR-0009): platformd owns the
// single Copilot SDK client and serves plain completions to apps on the
// internal listener. Sessions are inference-only — tools, skills, config
// discovery, and the session store are all disabled, and any permission
// request is denied.
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bketelsen/bespoke/pkg/web"
	copilot "github.com/github/copilot-sdk/go"
	"github.com/github/copilot-sdk/go/rpc"
)

type llmGateway struct {
	model  string
	briefs *sql.DB // platformd's db; per-user brief injection (ADR-0019)

	mu     sync.RWMutex
	client *copilot.Client
	status string // "" = healthy; otherwise the dashboard warning text
}

// briefFor returns the user's self-provided brief as a system-prompt
// section, or "" when none is stored.
func (g *llmGateway) briefFor(ctx context.Context, login string) string {
	if g.briefs == nil || login == "" {
		return ""
	}
	var name, brief string
	if err := g.briefs.QueryRowContext(ctx,
		"SELECT name, brief FROM briefs WHERE login = ?", login).Scan(&name, &brief); err != nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("About the user (self-provided brief — follow its rules):\n")
	if name != "" {
		fmt.Fprintf(&b, "Call them %s.\n", name)
	}
	if brief != "" {
		b.WriteString(brief + "\n")
	}
	if name == "" && brief == "" {
		return ""
	}
	return b.String()
}

func newLLMGateway() *llmGateway {
	return &llmGateway{
		model:  os.Getenv("BESPOKE_LLM_MODEL"), // empty = CLI default
		status: "LLM gateway starting…",
	}
}

// start launches the Copilot CLI runtime and the auth health loop. Runs in a
// goroutine: platformd serves dashboards even when the gateway is down.
func (g *llmGateway) start() {
	// A scratch working directory keeps the runtime from discovering repo
	// instructions (AGENTS.md etc.) and mixing them into app inference.
	wd := filepath.Join(os.TempDir(), "bespoke-llm")
	_ = os.MkdirAll(wd, 0o755) // failure surfaces in client.Start below

	client := copilot.NewClient(&copilot.ClientOptions{WorkingDirectory: wd})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := client.Start(ctx); err != nil {
		g.setStatus(fmt.Sprintf("LLM gateway down: copilot runtime failed to start: %v", err))
		return
	}
	g.mu.Lock()
	g.client = client
	g.mu.Unlock()

	check := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		st, err := client.GetAuthStatus(ctx)
		switch {
		case err != nil:
			g.setStatus(fmt.Sprintf("LLM gateway unhealthy: %v", err))
		case !st.IsAuthenticated:
			g.setStatus("LLM gateway: copilot is not authenticated — run `copilot` and sign in")
		default:
			g.setStatus("")
		}
	}
	check()
	go func() {
		for range time.Tick(5 * time.Minute) {
			check()
		}
	}()
}

func (g *llmGateway) setStatus(s string) {
	g.mu.Lock()
	g.status = s
	g.mu.Unlock()
	if s != "" {
		log.Println("llm:", s)
	}
}

// warning returns the dashboard warning text, empty when healthy.
func (g *llmGateway) warning() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.status
}

var denyAll = func(_ copilot.PermissionRequest, _ copilot.PermissionInvocation) (rpc.PermissionDecision, error) {
	return &rpc.PermissionDecisionDeniedInteractivelyByUser{}, nil
}

// complete runs one locked-down session: inference-only by default, or
// agentic over exactly the provided tools (ADR-0021) — builtins stay
// unavailable either way.
func (g *llmGateway) complete(ctx context.Context, system, prompt string, tools []copilot.Tool) (string, error) {
	g.mu.RLock()
	client, status := g.client, g.status
	g.mu.RUnlock()
	if client == nil || status != "" {
		return "", fmt.Errorf("gateway unavailable: %s", status)
	}

	allowed := make([]string, 0, len(tools)) // empty → inference only
	for _, t := range tools {
		allowed = append(allowed, t.Name)
	}
	cfg := &copilot.SessionConfig{
		ClientName:              "bespoke",
		Model:                   g.model,
		Tools:                   tools,
		AvailableTools:          allowed,
		OnPermissionRequest:     denyAll,
		EnableSkills:            copilot.Bool(false),
		EnableConfigDiscovery:   copilot.Bool(false),
		EnableSessionStore:      copilot.Bool(false),
		EnableFileHooks:         copilot.Bool(false),
		EnableHostGitOperations: copilot.Bool(false),
		SkipEmbeddingRetrieval:  copilot.Bool(true),
	}
	if system != "" {
		prompt = "System instructions (follow strictly):\n" + system + "\n\n---\n\n" + prompt
	}

	sess, err := client.CreateSession(ctx, cfg)
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	defer func() {
		dctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = client.DeleteSession(dctx, sess.SessionID) // best-effort cleanup
	}()

	ev, err := sess.SendAndWait(ctx, copilot.MessageOptions{Prompt: prompt})
	if err != nil {
		return "", err
	}
	if ev == nil {
		return "", fmt.Errorf("no assistant response")
	}
	msg, ok := ev.Data.(*copilot.AssistantMessageData)
	if !ok {
		return "", fmt.Errorf("unexpected final event %T", ev.Data)
	}
	return msg.Content, nil
}

// completeRequest is the wire format shared with pkg/llm.
type completeRequest struct {
	App    string     `json:"app"`
	System string     `json:"system,omitempty"`
	Prompt string     `json:"prompt"`
	Login  string     `json:"login,omitempty"` // set via llm.WithUser → brief injection
	Tools  []wireTool `json:"tools,omitempty"` // agentic mode (ADR-0021)
}

type wireTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Schema      map[string]any `json:"schema"`
	URL         string         `json:"url"`
}

// toolClient executes model-requested tool calls back against apps.
var toolClient = &http.Client{Timeout: 30 * time.Second}

// copilotTools wraps wire tools as SDK tools whose handlers POST to the
// owning app with the user's identity — the same trusted-forwarding pattern
// as cards and contexts. Tools run as the user, never as "the platform".
func copilotTools(tools []wireTool, login string) []copilot.Tool {
	out := make([]copilot.Tool, 0, len(tools))
	for _, t := range tools {
		t := t
		out = append(out, copilot.Tool{
			Name:           t.Name,
			Description:    t.Description,
			Parameters:     t.Schema,
			SkipPermission: true, // ours by construction; builtins stay denied
			Handler: func(inv copilot.ToolInvocation) (copilot.ToolResult, error) {
				args, err := json.Marshal(inv.Arguments)
				if err != nil {
					return copilot.ToolResult{}, err
				}
				req, err := http.NewRequest(http.MethodPost, t.URL, bytes.NewReader(args))
				if err != nil {
					return copilot.ToolResult{}, err
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Tailscale-User-Login", login)
				resp, err := toolClient.Do(req)
				if err != nil {
					return copilot.ToolResult{}, err
				}
				defer resp.Body.Close()
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
				if resp.StatusCode != http.StatusOK {
					// Tool-level failures go back to the model as text so it
					// can recover or explain, not as session errors.
					return copilot.ToolResult{
						TextResultForLLM: fmt.Sprintf("tool error (%s): %s", resp.Status, strings.TrimSpace(string(body))),
						ResultType:       "failure",
					}, nil
				}
				log.Printf("llm-tool app-url=%s user=%s ok", t.URL, login)
				return copilot.ToolResult{TextResultForLLM: string(body), ResultType: "success"}, nil
			},
		})
	}
	return out
}

// serveInternal runs the internal-services listener (the 4001 plane,
// ADR-0012) — never routed by Caddy.
func serveInternal(addr string, llm *llmGateway, audio *audioGateway) {
	mux := http.NewServeMux()
	llm.register(mux)
	audio.register(mux)
	// Change notifications from apps (ADR-0022): wake the dashboard's
	// /_live subscribers so cards refresh on any app's mutation.
	mux.HandleFunc("POST /notify", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Login string `json:"login"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&b); err != nil || b.Login == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		web.Notify(b.Login)
		w.WriteHeader(http.StatusNoContent)
	})
	log.Printf("internal services listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		llm.setStatus(fmt.Sprintf("internal listener down: %v", err))
	}
}

func (g *llmGateway) register(mux *http.ServeMux) {
	mux.HandleFunc("GET /llm/healthz", func(w http.ResponseWriter, r *http.Request) {
		if s := g.warning(); s != "" {
			http.Error(w, s, http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /llm/complete", func(w http.ResponseWriter, r *http.Request) {
		var req completeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Prompt == "" {
			http.Error(w, "bad request: need {app, prompt}", http.StatusBadRequest)
			return
		}
		timeout := 120 * time.Second
		if len(req.Tools) > 0 {
			timeout = 300 * time.Second // agentic loops take multiple turns
			if req.Login == "" {
				http.Error(w, "tools require a tagged user (llm.WithUser)", http.StatusBadRequest)
				return
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		system := req.System
		if brief := g.briefFor(ctx, req.Login); brief != "" {
			system = brief + "\n" + system
		}
		start := time.Now()
		text, err := g.complete(ctx, system, req.Prompt, copilotTools(req.Tools, req.Login))
		log.Printf("llm app=%s prompt=%dB tools=%d out=%dB dur=%s err=%v",
			req.App, len(req.Prompt), len(req.Tools), len(text), time.Since(start).Round(time.Millisecond), err)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"text": text})
	})
}
