// platformd v0: the dashboard/registry (docs/design/architecture.md).
// Scans apps/*/app.toml on every request — the manifests are the registry.
package main

import (
	"cmp"
	"flag"
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

	// Internal listener for the LLM gateway: not routed by Caddy, reachable
	// only on-host (dev) or from the edge-blocked port range (ADR-0011).
	internal := flag.String("internal",
		cmp.Or(os.Getenv("BESPOKE_INTERNAL_LISTEN"), "127.0.0.1:4001"),
		"internal gateway listen address")

	gw := newLLMGateway()
	go gw.start()
	agw := newAudioGateway()

	web.Serve("platformd", 4000, func(mux *http.ServeMux) {
		go serveInternal(*internal, gw, agw) // after flag.Parse (inside Serve)

		mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			apps, warnings, err := manifest.LoadAll(root)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if s := gw.warning(); s != "" {
				warnings = append(warnings, s)
			}
			if s := agw.warning(); s != "" {
				warnings = append(warnings, s)
			}
			dev := os.Getenv("BESPOKE_DEV_USER") != ""
			cards := fetchCards(r, apps)
			views.Dashboard(auth.FromContext(r.Context()), dev, domain, apps, cards, warnings).Render(r.Context(), w)
		})
	})
}
