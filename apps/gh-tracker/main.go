// gh-tracker: open PR/issue titles across up to 7 configured GitHub repos.
// Spec: README.md (approved spec, 2026-08-02).
package main

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/a-h/templ"
	"github.com/bketelsen/bespoke/apps/gh-tracker/views"
	"github.com/bketelsen/bespoke/pkg/auth"
	"github.com/bketelsen/bespoke/pkg/db"
	"github.com/bketelsen/bespoke/pkg/web"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

func main() {
	migrations, _ := fs.Sub(migrationFS, "migrations")
	sqldb, err := db.Open("gh-tracker", migrations)
	if err != nil {
		log.Fatal(err)
	}

	web.Run("gh-tracker", func(mux *http.ServeMux) {
		web.DashboardCard(mux, func(ctx context.Context, user auth.User) (templ.Component, error) {
			count, err := openCounts(ctx, sqldb, user.Login)
			if err != nil {
				return nil, err
			}
			return views.DashCard(count), nil
		})

		// Live projects region (ADR-0022): patched on any project/token change
		// or background refresh.
		web.Live(mux, func(ctx context.Context, user auth.User) (templ.Component, error) {
			groups, err := loadProjectGroups(ctx, sqldb, user.Login)
			if err != nil {
				return nil, err
			}
			return views.GroupsLive(toView(groups)), nil
		})

		mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			groups, err := loadProjectGroups(r.Context(), sqldb, user.Login)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			views.Home(user, toView(groups)).Render(r.Context(), w)
		})

		mux.HandleFunc("GET /settings", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			projects, err := loadSettingsProjects(r.Context(), sqldb, user.Login)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			token, err := githubToken(r.Context(), sqldb, user.Login)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			views.Settings(user, projects, token != "", "").Render(r.Context(), w)
		})

		mux.HandleFunc("POST /settings/projects", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			ownerRepo := strings.TrimSpace(r.FormValue("owner_repo"))
			if err := addProject(r.Context(), sqldb, user.Login, ownerRepo); err != nil {
				renderSettingsError(w, r, sqldb, user, err.Error())
				return
			}
			web.Changed(user.Login)
			http.Redirect(w, r, "/settings", http.StatusSeeOther)
		})

		mux.HandleFunc("POST /settings/projects/{id}/delete", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			if _, err := sqldb.ExecContext(r.Context(),
				"DELETE FROM projects WHERE id = ? AND login = ?", r.PathValue("id"), user.Login); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			web.Changed(user.Login)
			http.Redirect(w, r, "/settings", http.StatusSeeOther)
		})

		mux.HandleFunc("POST /settings/token", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			token := strings.TrimSpace(r.FormValue("github_token"))
			if token != "" { // blank keeps the existing saved token (README rule)
				if _, err := sqldb.ExecContext(r.Context(), `
					INSERT INTO settings (login, github_token) VALUES (?, ?)
					ON CONFLICT (login) DO UPDATE SET github_token = excluded.github_token`,
					user.Login, token); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}
			web.Changed(user.Login)
			http.Redirect(w, r, "/settings", http.StatusSeeOther)
		})
	})
}

func loadSettingsProjects(ctx context.Context, sqldb *sql.DB, login string) ([]views.SettingsProject, error) {
	rows, err := sqldb.QueryContext(ctx,
		"SELECT id, owner_repo FROM projects WHERE login = ? ORDER BY position, id", login)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []views.SettingsProject
	for rows.Next() {
		var p views.SettingsProject
		if err := rows.Scan(&p.ID, &p.OwnerRepo); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// addProject validates and inserts a new tracked repo, enforcing the
// 7-project cap and the owner/repo shape (README rules).
func addProject(ctx context.Context, sqldb *sql.DB, login, ownerRepo string) error {
	parts := strings.Split(ownerRepo, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return fmt.Errorf("enter a repo as owner/repo")
	}
	var count int
	if err := sqldb.QueryRowContext(ctx, "SELECT count(*) FROM projects WHERE login = ?", login).Scan(&count); err != nil {
		return err
	}
	if count >= maxProjects {
		return fmt.Errorf("max %d projects — remove one first", maxProjects)
	}
	_, err := sqldb.ExecContext(ctx,
		"INSERT INTO projects (login, owner_repo, position) VALUES (?, ?, ?)", login, ownerRepo, count)
	return err
}

func renderSettingsError(w http.ResponseWriter, r *http.Request, sqldb *sql.DB, user auth.User, msg string) {
	projects, err := loadSettingsProjects(r.Context(), sqldb, user.Login)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	token, err := githubToken(r.Context(), sqldb, user.Login)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusBadRequest)
	views.Settings(user, projects, token != "", msg).Render(r.Context(), w)
}
