package web

import (
	"context"
	"net/http"
	"strings"

	"github.com/bketelsen/bespoke/pkg/auth"
	"github.com/bketelsen/bespoke/pkg/ui"
)

// IntentDef wires a manifest-declared intent (docs/adr/0018-cross-app-intents.md)
// to its handler. Handler returns the in-app URL to land on after execution
// (the created thing, or "/").
type IntentDef struct {
	Name    string // must match an [[intents]] name in app.toml
	Title   string // confirm-page heading, e.g. "Create Todo"
	Prompt  string // label over the editable payload, e.g. "Task description"
	Handler func(ctx context.Context, user auth.User, text string) (redirect string, err error)
}

// Intent mounts GET /_intents/<name> (prefilled, editable confirm page) and
// POST /_intents/<name> (execute → redirect). Other apps link here via the
// registry; the confirm step doubles as the edit step.
func Intent(mux *http.ServeMux, appTitle string, def IntentDef) {
	mux.HandleFunc("GET /_intents/"+def.Name, func(w http.ResponseWriter, r *http.Request) {
		user := auth.FromContext(r.Context())
		text := strings.TrimSpace(r.URL.Query().Get("text"))
		ui.IntentConfirm(user, appTitle, def.Title, def.Prompt, "/_intents/"+def.Name, text).
			Render(r.Context(), w)
	})
	mux.HandleFunc("POST /_intents/"+def.Name, func(w http.ResponseWriter, r *http.Request) {
		user := auth.FromContext(r.Context())
		text := strings.TrimSpace(r.FormValue("text"))
		if text == "" {
			http.Error(w, "nothing to do: empty text", http.StatusBadRequest)
			return
		}
		dest, err := def.Handler(r.Context(), user, text)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if dest == "" {
			dest = "/"
		}
		http.Redirect(w, r, dest, http.StatusSeeOther)
	})
}
