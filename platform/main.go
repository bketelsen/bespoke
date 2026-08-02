// platformd v0: the dashboard/registry (docs/design/architecture.md).
// Scans apps/*/app.toml on every request — the manifests are the registry.
package main

import (
	"cmp"
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/a-h/templ"
	"github.com/bketelsen/bespoke/internal/manifest"
	"github.com/bketelsen/bespoke/pkg/auth"
	"github.com/bketelsen/bespoke/pkg/db"
	"github.com/bketelsen/bespoke/pkg/llm"
	"github.com/bketelsen/bespoke/pkg/web"
	"github.com/bketelsen/bespoke/platform/views"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

func main() {
	domain := cmp.Or(os.Getenv("BESPOKE_DOMAIN"), "bespoke.example.com")
	root := cmp.Or(os.Getenv("BESPOKE_ROOT"), ".")

	migrations, _ := fs.Sub(migrationFS, "migrations")
	sqldb, err := db.Open("platformd", migrations)
	if err != nil {
		log.Fatal(err)
	}

	// Internal listener for the LLM gateway: not routed by Caddy, reachable
	// only on-host (dev) or from the edge-blocked port range (ADR-0011).
	internal := flag.String("internal",
		cmp.Or(os.Getenv("BESPOKE_INTERNAL_LISTEN"), "127.0.0.1:4001"),
		"internal gateway listen address")

	gw := newLLMGateway()
	gw.briefs = sqldb // gateway injects per-user briefs (ADR-0019)
	go gw.start()
	agw := newAudioGateway()

	web.Serve("platformd", 4000, func(mux *http.ServeMux) {
		go serveInternal(*internal, gw, agw) // after flag.Parse (inside Serve)

		// The all-apps chat (ADR-0020): context aggregated from every
		// chat-enabled app over the app contract — never their databases.
		// Agentic across ALL apps' tools (ADR-0021), namespaced per app.
		web.EnableChatWithTools(mux, "dashboard",
			func(ctx context.Context, user auth.User) (string, error) {
				apps, _, err := manifest.LoadAll(root)
				if err != nil {
					return "", err
				}
				return aggregateContexts(ctx, user.Login, user.Name, apps), nil
			},
			func(ctx context.Context, user auth.User) []llm.Tool {
				var tools []llm.Tool
				for _, t := range allAppTools(ctx, root) {
					tools = append(tools, llm.Tool{
						Name:        t.Slug + "_" + t.Name,
						Description: fmt.Sprintf("[%s] %s", t.Slug, t.Description),
						Schema:      t.Schema,
						URL:         t.URL,
					})
				}
				return tools
			})

		// External LLM clients: the platform MCP endpoint (ADR-0021).
		mux.Handle("/mcp", mcpHandler(root))

		// Live card grid (ADR-0022): re-rendered when any app reports a
		// change for this user via the plane's /notify.
		dev := os.Getenv("BESPOKE_DEV_USER") != ""
		web.Live(mux, func(ctx context.Context, user auth.User) (templ.Component, error) {
			apps, _, err := manifest.LoadAll(root)
			if err != nil {
				return nil, err
			}
			cards := fetchCards(ctx, user.Login, user.Name, apps)
			return views.CardGrid(dev, domain, apps, cards), nil
		})

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
			user := auth.FromContext(r.Context())
			cards := fetchCards(r.Context(), user.Login, user.Name, apps)
			views.Dashboard(user, dev, domain, apps, cards, warnings).Render(r.Context(), w)
		})

		mux.HandleFunc("GET /settings", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			var name, brief string
			sqldb.QueryRowContext(r.Context(),
				"SELECT name, brief FROM briefs WHERE login = ?", user.Login).Scan(&name, &brief)
			views.Settings(user, name, brief, r.URL.Query().Get("saved") == "1").Render(r.Context(), w)
		})

		mux.HandleFunc("POST /settings", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			name := strings.TrimSpace(r.FormValue("name"))
			brief := strings.TrimSpace(r.FormValue("brief"))
			if len(brief) > 4000 {
				brief = brief[:4000]
			}
			if _, err := sqldb.ExecContext(r.Context(), `
				INSERT INTO briefs (login, name, brief, updated_at) VALUES (?, ?, ?, datetime('now'))
				ON CONFLICT(login) DO UPDATE SET name=excluded.name, brief=excluded.brief, updated_at=excluded.updated_at`,
				user.Login, name, brief); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			http.Redirect(w, r, "/settings?saved=1", http.StatusSeeOther)
		})
	})
}
