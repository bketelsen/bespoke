// platformd v0: the dashboard/registry (docs/design/architecture.md).
// Scans apps/*/app.toml on every request — the manifests are the registry.
package main

import (
	"cmp"
	"net/http"
	"os"

	"github.com/bketelsen/bespoke/internal/manifest"
	"github.com/bketelsen/bespoke/pkg/auth"
	"github.com/bketelsen/bespoke/pkg/web"
	"github.com/bketelsen/bespoke/platform/views"
)

func main() {
	domain := cmp.Or(os.Getenv("BESPOKE_DOMAIN"), "bespoke.example.com")
	root := cmp.Or(os.Getenv("BESPOKE_ROOT"), ".")

	web.Serve("platformd", 4000, func(mux *http.ServeMux) {
		mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			apps, warnings, err := manifest.LoadAll(root)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			views.Dashboard(auth.FromContext(r.Context()), domain, apps, warnings).Render(r.Context(), w)
		})
	})
}
