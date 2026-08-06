package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/bketelsen/bespoke/pkg/auth"
	"github.com/bketelsen/bespoke/pkg/events"
	"github.com/bketelsen/bespoke/pkg/ui"
	"github.com/starfederation/datastar-go/datastar"
)

func notifications(mux *http.ServeMux) {
	c := events.New("")
	mux.HandleFunc("GET /_notifications", func(w http.ResponseWriter, r *http.Request) {
		u := auth.FromContext(r.Context())
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit == 0 {
			limit = 30
		}
		p, err := c.List(r.Context(), u.Login, r.URL.Query().Get("after"), limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		jsonResponseLocal(w, p)
	})
	mux.HandleFunc("POST /_notifications/read-all", func(w http.ResponseWriter, r *http.Request) { proxyMutation(w, r, c, "", "read-all") })
	mux.HandleFunc("POST /_notifications/{id}/{action}", func(w http.ResponseWriter, r *http.Request) {
		a := r.PathValue("action")
		if a != "read" && a != "dismiss" {
			http.NotFound(w, r)
			return
		}
		proxyMutation(w, r, c, r.PathValue("id"), a)
	})
	mux.HandleFunc("GET /_notifications/live", func(w http.ResponseWriter, r *http.Request) {
		u := auth.FromContext(r.Context())
		sse := datastar.NewSSE(w, r)
		patch := func(ctx context.Context, n *events.Notification) error {
			p, err := c.List(ctx, u.Login, "", 30)
			if err != nil {
				return err
			}
			count, err := c.Unread(ctx, u.Login)
			if err != nil {
				return err
			}
			if err = sse.PatchElementTempl(ui.NotificationCount(count)); err != nil {
				return err
			}
			if err = sse.PatchElementTempl(ui.NotificationList(p.Notifications)); err != nil {
				return err
			}
			return sse.PatchElementTempl(ui.NotificationToasts(n))
		}
		if err := patch(r.Context(), nil); err != nil {
			return
		}
		_ = c.Stream(r.Context(), u.Login, func(n events.Notification) error { return patch(r.Context(), &n) })
	})
}
func proxyMutation(w http.ResponseWriter, r *http.Request, c *events.Client, id, action string) {
	u := auth.FromContext(r.Context())
	status, err := c.Mutate(r.Context(), u.Login, id, action)
	if err != nil {
		if status == 404 {
			http.NotFound(w, r)
		} else {
			http.Error(w, err.Error(), http.StatusBadGateway)
		}
		return
	}
	w.WriteHeader(204)
}
func jsonResponseLocal(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
