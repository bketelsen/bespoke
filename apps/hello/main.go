// hello: the Phase 1 proof app. Done when this renders your Tailscale login
// name through Caddy on the edge host (docs/plans/roadmap.md, Phase 1).
// Deliberately stdlib-only; it moves onto pkg/* in Phase 2.
package main

import (
	"cmp"
	"flag"
	"fmt"
	"html"
	"log"
	"net/http"
	"os"
)

func main() {
	listen := flag.String("listen", cmp.Or(os.Getenv("BESPOKE_LISTEN"), "127.0.0.1:4101"), "listen address")
	flag.Parse()

	http.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	http.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		login := r.Header.Get("Tailscale-User-Login")
		if login == "" {
			http.Error(w, "no identity header; expected to be reached via the edge proxy", http.StatusUnauthorized)
			return
		}
		name := r.Header.Get("Tailscale-User-Name")
		fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>Hello</title>
<h1>Hello, %s 👋</h1><p>Authenticated as <code>%s</code> — the whole loop works.</p>`,
			html.EscapeString(cmp.Or(name, login)), html.EscapeString(login))
	})

	log.Printf("hello listening on %s", *listen)
	log.Fatal(http.ListenAndServe(*listen, nil))
}
