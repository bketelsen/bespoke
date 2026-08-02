package web

import (
	"context"
	"net/http"

	"github.com/a-h/templ"
	"github.com/bketelsen/bespoke/pkg/auth"
)

// CardProvider renders the app's live dashboard-card fragment for a user
// (ADR-0017): content only — no AppShell, no forms; cheap queries, never
// LLM calls (the dashboard waits behind a short timeout).
type CardProvider func(ctx context.Context, user auth.User) (templ.Component, error)

// DashboardCard mounts GET /_card, the optional app-contract endpoint the
// dashboard pulls per-user summaries from. Call inside web.Run's register:
//
//	web.DashboardCard(mux, func(ctx, user) (templ.Component, error) { … })
func DashboardCard(mux *http.ServeMux, provider CardProvider) {
	mux.HandleFunc("GET /_card", func(w http.ResponseWriter, r *http.Request) {
		c, err := provider(r.Context(), auth.FromContext(r.Context()))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		c.Render(r.Context(), w)
	})
}
