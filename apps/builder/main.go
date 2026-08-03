// builder: the app that builds new apps (ADR-0023,
// docs/design/builder-plane.md). It owns the interview, the spec gate, and
// run state; the actual building and deploying happen outside this process,
// behind the spool.
package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/bketelsen/bespoke/apps/builder/views"
	"github.com/bketelsen/bespoke/pkg/auth"
	"github.com/bketelsen/bespoke/pkg/db"
	"github.com/bketelsen/bespoke/pkg/llm"
	"github.com/bketelsen/bespoke/pkg/web"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

func main() {
	migrations, _ := fs.Sub(migrationFS, "migrations")
	sqldb, err := db.Open("builder", migrations)
	if err != nil {
		log.Fatal(err)
	}
	ai := llm.New("builder")
	mon := newMonitor(sqldb)
	mon.resume()

	// startRun creates a run from an idea and does the first interview turn
	// synchronously (~2s) so the run page opens with a question waiting.
	startRun := func(ctx context.Context, login, idea string) (string, error) {
		id := fmt.Sprintf("r%d", time.Now().UnixMilli())
		if _, err := sqldb.ExecContext(ctx,
			"INSERT INTO runs (id, login, idea) VALUES (?,?,?)", id, login, idea); err != nil {
			return "", err
		}
		if _, err := sqldb.ExecContext(ctx,
			"INSERT INTO messages (run_id, role, body) VALUES (?,?,?)", id, "user", idea); err != nil {
			return "", err
		}
		reply, err := nextTurn(ctx, ai, sqldb, id, login)
		if err != nil {
			// The run exists; the user can retry from its page.
			reply = "(the designer is unavailable right now — say anything to retry: " + err.Error() + ")"
		}
		if err := storeReply(ctx, sqldb, id, login, reply); err != nil {
			return "", err
		}
		web.Changed(login)
		return id, nil
	}

	web.Run("builder", func(mux *http.ServeMux) {
		registerTools(mux, sqldb, startRun)
		web.EnableChat(mux, "builder", func(ctx context.Context, user auth.User) (string, error) {
			return chatContext(ctx, sqldb, user.Login)
		})
		web.DashboardCard(mux, func(ctx context.Context, user auth.User) (templ.Component, error) {
			var active, activeStatus string
			_ = sqldb.QueryRowContext(ctx, `SELECT idea, status FROM runs WHERE login = ?
				AND status NOT IN ('live','failed') ORDER BY created_at DESC LIMIT 1`,
				user.Login).Scan(&active, &activeStatus)
			var shipped int
			_ = sqldb.QueryRowContext(ctx,
				"SELECT count(*) FROM runs WHERE login = ? AND status = 'live'", user.Login).Scan(&shipped)
			return views.DashCard(active, activeStatus, shipped), nil
		})
		web.Live(mux, func(ctx context.Context, user auth.User) (templ.Component, error) {
			runs, err := loadRuns(ctx, sqldb, user.Login)
			if err != nil {
				return nil, err
			}
			return views.HomeLive(runs), nil
		})
		web.Intent(mux, "Builder", web.IntentDef{
			Name: "build-app", Title: "Build an App",
			Prompt: "This text becomes the app idea; the Builder interviews you before anything is built.",
			Handler: func(ctx context.Context, user auth.User, text string) (string, error) {
				idea := strings.TrimSpace(text)
				if idea == "" {
					return "", fmt.Errorf("app idea required")
				}
				id, err := startRun(ctx, user.Login, idea)
				return "/runs/" + id, err
			},
		})

		mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			runs, err := loadRuns(r.Context(), sqldb, user.Login)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			views.Home(user, runs).Render(r.Context(), w)
		})

		mux.HandleFunc("POST /runs", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			idea := strings.TrimSpace(r.FormValue("idea"))
			if idea == "" {
				http.Error(w, "app idea required", http.StatusBadRequest)
				return
			}
			id, err := startRun(r.Context(), user.Login, idea)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			http.Redirect(w, r, "/runs/"+id, http.StatusSeeOther)
		})

		mux.HandleFunc("GET /runs/{id}", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			d, err := loadRun(r.Context(), sqldb, user.Login, r.PathValue("id"))
			if err != nil {
				http.NotFound(w, r)
				return
			}
			views.RunPage(user, d, appURL(d)).Render(r.Context(), w)
		})

		mux.HandleFunc("POST /runs/{id}/say", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			id := r.PathValue("id")
			body := strings.TrimSpace(r.FormValue("body"))
			var status string
			if err := sqldb.QueryRowContext(r.Context(),
				"SELECT status FROM runs WHERE id = ? AND login = ?", id, user.Login).Scan(&status); err != nil {
				http.NotFound(w, r)
				return
			}
			if (status != "interviewing" && status != "ready") || body == "" {
				http.Redirect(w, r, "/runs/"+id, http.StatusSeeOther)
				return
			}
			if _, err := sqldb.ExecContext(r.Context(),
				"INSERT INTO messages (run_id, role, body) VALUES (?,?,?)", id, "user", body); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			// Feedback on a ready spec reopens the interview; the run goes
			// back through the spec gate when the revision lands.
			if status == "ready" {
				if _, err := sqldb.ExecContext(r.Context(), `UPDATE runs SET status = 'interviewing',
					updated_at = datetime('now') WHERE id = ?`, id); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}
			reply, err := nextTurn(r.Context(), ai, sqldb, id, user.Login)
			if err != nil {
				reply = "(the designer errored — say anything to retry: " + err.Error() + ")"
			}
			if err := storeReply(r.Context(), sqldb, id, user.Login, reply); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			web.Changed(user.Login)
			http.Redirect(w, r, "/runs/"+id, http.StatusSeeOther)
		})

		mux.HandleFunc("POST /runs/{id}/approve", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			id := r.PathValue("id")
			var status, slug, idea, spec string
			if err := sqldb.QueryRowContext(r.Context(),
				"SELECT status, slug, idea, spec FROM runs WHERE id = ? AND login = ?",
				id, user.Login).Scan(&status, &slug, &idea, &spec); err != nil {
				http.NotFound(w, r)
				return
			}
			if status != "ready" {
				http.Redirect(w, r, "/runs/"+id, http.StatusSeeOther)
				return
			}
			if err := writeBuildRequest(id, slug, idea, spec); err != nil {
				http.Error(w, "queue build: "+err.Error(), http.StatusInternalServerError)
				return
			}
			if _, err := sqldb.ExecContext(r.Context(), `UPDATE runs SET status = 'building',
				detail = '', updated_at = datetime('now') WHERE id = ?`, id); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			mon.ensure(id)
			web.Changed(user.Login)
			http.Redirect(w, r, "/runs/"+id, http.StatusSeeOther)
		})

		// Per-run live updates: a small Datastar SSE loop that re-renders the
		// run fragment every 2s while the page is open, patching only when
		// something changed. The home page uses the standard /_live hub; this
		// page needs per-run context the hub doesn't carry.
		mux.HandleFunc("GET /runs/{id}/live", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			id := r.PathValue("id")
			sse := web.NewSSE(w, r)
			tick := time.NewTicker(2 * time.Second)
			defer tick.Stop()
			var last string
			for {
				select {
				case <-r.Context().Done():
					return
				case <-tick.C:
					d, err := loadRun(r.Context(), sqldb, user.Login, id)
					if err != nil {
						return
					}
					html, err := templ.ToGoHTML(r.Context(), views.RunLive(d, appURL(d)))
					if err != nil || string(html) == last {
						continue
					}
					last = string(html)
					if err := sse.PatchElementTempl(views.RunLive(d, appURL(d))); err != nil {
						return
					}
				}
			}
		})
	})
}

func appURL(d views.RunDetail) string {
	if d.Status != "live" || d.Slug == "" || os.Getenv("BESPOKE_DOMAIN") == "" {
		return ""
	}
	return "https://" + d.Slug + "." + os.Getenv("BESPOKE_DOMAIN")
}
