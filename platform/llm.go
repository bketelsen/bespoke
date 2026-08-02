// The LLM gateway (docs/design/llm-gateway.md, ADR-0009): platformd owns the
// single Copilot SDK client and serves plain completions to apps on the
// internal listener. Sessions are inference-only — tools, skills, config
// discovery, and the session store are all disabled, and any permission
// request is denied.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
	os.MkdirAll(wd, 0o755)

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

// complete runs one inference-only session: create, send, extract the final
// assistant message, delete.
func (g *llmGateway) complete(ctx context.Context, system, prompt string) (string, error) {
	g.mu.RLock()
	client, status := g.client, g.status
	g.mu.RUnlock()
	if client == nil || status != "" {
		return "", fmt.Errorf("gateway unavailable: %s", status)
	}

	cfg := &copilot.SessionConfig{
		ClientName:              "bespoke",
		Model:                   g.model,
		AvailableTools:          []string{}, // inference only; denyAll backstops
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
		client.DeleteSession(dctx, sess.SessionID)
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
	App    string `json:"app"`
	System string `json:"system,omitempty"`
	Prompt string `json:"prompt"`
	Login  string `json:"login,omitempty"` // set via llm.WithUser → brief injection
}

// serveInternal runs the internal-services listener (the 4001 plane,
// ADR-0012) — never routed by Caddy.
func serveInternal(addr string, llm *llmGateway, audio *audioGateway) {
	mux := http.NewServeMux()
	llm.register(mux)
	audio.register(mux)
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
		ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
		defer cancel()
		system := req.System
		if brief := g.briefFor(ctx, req.Login); brief != "" {
			system = brief + "\n" + system
		}
		start := time.Now()
		text, err := g.complete(ctx, system, req.Prompt)
		log.Printf("llm app=%s prompt=%dB out=%dB dur=%s err=%v",
			req.App, len(req.Prompt), len(text), time.Since(start).Round(time.Millisecond), err)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"text": text})
	})
}
