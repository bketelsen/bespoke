package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bketelsen/bespoke/pkg/audio"
	"github.com/bketelsen/bespoke/pkg/auth"
	"github.com/bketelsen/bespoke/pkg/llm"
)

// chatEnabled flips the AppShell's chat chrome on; one app per process, so
// package state is fine.
var chatEnabled atomic.Bool

// ChatProvider returns the current user's relevant app data as text — the
// context the in-app chat answers from (ADR-0015). Keep it recent and
// bounded (e.g. last 30 days), not a full dump.
type ChatProvider func(ctx context.Context, user auth.User) (string, error)

type chatMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

// ToolSource supplies the tools a chat may use (ADR-0021). Apps get their
// own registered tools automatically via EnableChat; platformd's dashboard
// chat aggregates every app's.
type ToolSource func(ctx context.Context, user auth.User) []llm.Tool

// EnableChat mounts POST /_chat and turns on the AppShell chat panel. Call
// once inside web.Run's register function:
//
//	web.EnableChat(mux, "journal", func(ctx, user) (string, error) { … })
//
// The chat is agentic over the app's own web.Tool registrations.
func EnableChat(mux *http.ServeMux, slug string, provider ChatProvider) {
	EnableChatWithTools(mux, slug, provider, func(ctx context.Context, user auth.User) []llm.Tool {
		wire := localTools()
		tools := make([]llm.Tool, 0, len(wire))
		for _, t := range wire {
			tools = append(tools, llm.Tool{Name: t.Name, Description: t.Description, Schema: t.Schema, URL: t.URL})
		}
		return tools
	})
}

// EnableChatWithTools is EnableChat with an explicit tool source. Extra
// llm options are applied to every chat completion — platformd's dashboard
// chat uses this for llm.WithBuiltins (ADR-0024); apps normally pass none.
func EnableChatWithTools(mux *http.ServeMux, slug string, provider ChatProvider, toolSource ToolSource, extra ...llm.Option) {
	chatEnabled.Store(true)
	ai := llm.New(slug)
	voice := audio.New(slug)

	// Voice input for the chat panel: browser WAV → local whisper → text
	// the user can edit before sending.
	mux.HandleFunc("POST /_chat/transcribe", func(w http.ResponseWriter, r *http.Request) {
		text, err := voice.Transcribe(r.Context(),
			http.MaxBytesReader(w, r.Body, 25<<20),
			audio.WithMIME(r.Header.Get("Content-Type")))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"text": strings.TrimSpace(text)})
	})

	// Speak toggle in the chat panel (ADR-0015 chrome): synthesize a reply
	// via the audio service's local TTS. Degrades to an error the panel
	// surfaces quietly when no TTS backend is configured.
	mux.HandleFunc("POST /_chat/speak", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil || strings.TrimSpace(req.Text) == "" {
			http.Error(w, "bad request: need {text}", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
		defer cancel()
		rc, ct, err := voice.Speak(ctx, req.Text)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer rc.Close()
		w.Header().Set("Content-Type", ct)
		io.Copy(w, rc)
	})

	// The app's chat context, raw (ADR-0020): platformd aggregates these for
	// the dashboard's all-apps chat. Same auth as everything; identity is
	// forwarded per request like dashboard cards.
	mux.HandleFunc("GET /_chat/context", func(w http.ResponseWriter, r *http.Request) {
		user := auth.FromContext(r.Context())
		text, err := provider(r.Context(), user)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(text))
	})

	mux.HandleFunc("POST /_chat", func(w http.ResponseWriter, r *http.Request) {
		user := auth.FromContext(r.Context())
		var req struct {
			Message string        `json:"message"`
			History []chatMessage `json:"history"`
			Speak   bool          `json:"speak"` // reply will be read aloud
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Message) == "" {
			http.Error(w, "bad request: need {message}", http.StatusBadRequest)
			return
		}

		appContext, err := provider(r.Context(), user)
		if err != nil {
			http.Error(w, "context: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if len(req.History) > 20 { // keep prompts bounded
			req.History = req.History[len(req.History)-20:]
		}
		var b strings.Builder
		for _, m := range req.History {
			fmt.Fprintf(&b, "%s: %s\n", m.Role, m.Text)
		}
		fmt.Fprintf(&b, "user: %s", req.Message)

		tools := toolSource(r.Context(), user)

		style := "Write plain conversational prose — the chat renders raw text, so never use " +
			"markdown, bullets, headings, or formatting symbols."
		if len(tools) > 0 {
			style += " You have tools to CREATE, UPDATE, COMPLETE, and DELETE the user's data — " +
				"use them when asked instead of explaining how; then confirm plainly what you did. " +
				"For destructive actions (delete), act only on an explicit, unambiguous request."
		}
		if req.Speak {
			style = "Your answer will be READ ALOUD by text-to-speech. Write exactly what should " +
				"be spoken: plain conversational sentences only — absolutely no markdown, bullets, " +
				"symbols, or abbreviations that read poorly. Humanize dates and times " +
				"('yesterday evening', 'August first', 'about two weeks ago' — never raw " +
				"timestamps). Prefer one short, natural paragraph a friend would say."
		}
		system := fmt.Sprintf(
			"You are the assistant inside the %q app on Bespoke, the owner's personal platform. "+
				"It is now %s (the owner's local time; timestamps in the app data are local too "+
				"unless marked otherwise). Answer briefly and concretely from the app data below; "+
				"when asked about trends, reason over dates. If the data can't answer, say what's "+
				"missing. %s\n\n--- app data ---\n%s",
			slug, time.Now().Format("Monday, January 2 2006, 3:04 PM (MST)"), style, appContext)

		opts := []llm.Option{llm.WithSystem(system), llm.WithUser(user.Login)}
		if len(tools) > 0 {
			opts = append(opts, llm.WithTools(tools))
		}
		opts = append(opts, extra...)
		text, err := ai.Complete(r.Context(), b.String(), opts...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"text": text})
	})
}
