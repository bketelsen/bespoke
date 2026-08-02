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

// EnableChat mounts POST /_chat and turns on the AppShell chat panel. Call
// once inside web.Run's register function:
//
//	web.EnableChat(mux, "journal", func(ctx, user) (string, error) { … })
func EnableChat(mux *http.ServeMux, slug string, provider ChatProvider) {
	chatEnabled.Store(true)
	ai := llm.New(slug)
	voice := audio.New(slug)

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

	mux.HandleFunc("POST /_chat", func(w http.ResponseWriter, r *http.Request) {
		user := auth.FromContext(r.Context())
		var req struct {
			Message string        `json:"message"`
			History []chatMessage `json:"history"`
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

		system := fmt.Sprintf(
			"You are the assistant inside the %q app on Bespoke, the owner's personal platform. "+
				"Today is %s. Answer briefly and concretely from the app data below; when asked about "+
				"trends, reason over dates. If the data can't answer, say what's missing.\n\n--- app data ---\n%s",
			slug, time.Now().Format("Monday, January 2 2006"), appContext)

		text, err := ai.Complete(r.Context(), b.String(), llm.WithSystem(system))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"text": text})
	})
}
