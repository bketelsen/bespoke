// platformd v0: the dashboard/registry (docs/design/architecture.md).
// Scans apps/*/app.toml on every request — the manifests are the registry.
package main

import (
	"cmp"
	"flag"
	"html/template"
	"log"
	"net/http"
	"os"

	"github.com/bketelsen/bespoke/internal/manifest"
)

var page = template.Must(template.New("dashboard").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Bespoke</title>
<style>
  /* Phase 1 placeholder styling; replaced by the design system in Phase 3
     (ADR-0010). */
  :root { color-scheme: light dark; font-family: system-ui, sans-serif; }
  body { max-width: 42rem; margin: 3rem auto; padding: 0 1rem; }
  header { display: flex; justify-content: space-between; align-items: baseline; }
  ul { list-style: none; padding: 0; display: grid; gap: .75rem; }
  li { border: 1px solid color-mix(in oklab, currentColor 25%, transparent);
       border-radius: .5rem; padding: 1rem; }
  li a { font-weight: 600; text-decoration: none; }
  .desc { opacity: .75; margin-top: .25rem; }
  .warn { color: #b45309; font-size: .875rem; }
</style>
</head>
<body>
<header><h1>Bespoke</h1><span>{{.User}}</span></header>
{{if not .Apps}}<p>No apps yet.</p>{{end}}
<ul>
{{range .Apps}}
  <li>
    <a href="https://{{.Slug}}.{{$.Domain}}/">{{.Name}}</a>
    <div class="desc">{{.Description}}</div>
  </li>
{{end}}
</ul>
{{range .Warnings}}<p class="warn">⚠ {{.}}</p>{{end}}
</body>
</html>
`))

func main() {
	listen := flag.String("listen", cmp.Or(os.Getenv("BESPOKE_LISTEN"), "127.0.0.1:4000"), "listen address")
	flag.Parse()
	domain := cmp.Or(os.Getenv("BESPOKE_DOMAIN"), "bespoke.example.com")
	root := cmp.Or(os.Getenv("BESPOKE_ROOT"), ".")

	http.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	http.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		user := r.Header.Get("Tailscale-User-Login")
		if user == "" {
			http.Error(w, "no identity header; expected to be reached via the edge proxy", http.StatusUnauthorized)
			return
		}
		apps, warnings, err := manifest.LoadAll(root)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := page.Execute(w, map[string]any{
			"User": user, "Domain": domain, "Apps": apps, "Warnings": warnings,
		}); err != nil {
			log.Printf("render: %v", err)
		}
	})

	log.Printf("platformd listening on %s (domain %s, root %s)", *listen, domain, root)
	log.Fatal(http.ListenAndServe(*listen, nil))
}
