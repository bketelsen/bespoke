// print-projects: a Bespoke app — manifest, routes, views, migrations, nothing
// else (CLAUDE.md).
package main

import (
	"context"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/bketelsen/bespoke/apps/print-projects/views"
	"github.com/bketelsen/bespoke/pkg/auth"
	"github.com/bketelsen/bespoke/pkg/db"
	"github.com/bketelsen/bespoke/pkg/web"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

func main() {
	migrations, _ := fs.Sub(migrationFS, "migrations")
	sqldb, err := db.Open("print-projects", migrations)
	if err != nil {
		log.Fatal(err)
	}

	web.Run("print-projects", func(mux *http.ServeMux) {
		registerTools(mux, sqldb)
		web.EnableChat(mux, "print-projects", func(ctx context.Context, user auth.User) (string, error) {
			return chatContext(ctx, sqldb, user.Login)
		})
		web.DashboardCard(mux, func(ctx context.Context, user auth.User) (templ.Component, error) {
			var waiting, projects, recent int
			err := sqldb.QueryRowContext(ctx, `SELECT
				count(*),
				coalesce(sum(CASE WHEN NOT EXISTS (SELECT 1 FROM print_history h WHERE h.project_id = p.id AND h.login = p.login) THEN 1 ELSE 0 END),0),
				(SELECT count(*) FROM print_history WHERE login = ? AND printed_on >= date('now','-30 days'))
				FROM projects p WHERE login = ?`, user.Login, user.Login).Scan(&projects, &waiting, &recent)
			if err != nil {
				return nil, err
			}
			return views.DashCard(waiting, projects, recent), nil
		})
		web.Live(mux, func(ctx context.Context, user auth.User) (templ.Component, error) {
			projects, err := loadProjects(ctx, sqldb, user.Login)
			if err != nil {
				return nil, err
			}
			return views.ProjectsLive(projects, time.Now().Format("2006-01-02")), nil
		})
		web.Intent(mux, "Print Projects", web.IntentDef{
			Name: "create-project", Title: "Create Print Project",
			Prompt: "This text becomes the project name; add its URL and notes afterward if needed.",
			Handler: func(ctx context.Context, user auth.User, text string) (string, error) {
				name := strings.TrimSpace(text)
				if name == "" {
					return "", fmt.Errorf("project name required")
				}
				_, err := sqldb.ExecContext(ctx, "INSERT INTO projects (login, name) VALUES (?, ?)", user.Login, name)
				if err == nil {
					web.Changed(user.Login)
				}
				return "/", err
			},
		})

		mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			projects, err := loadProjects(r.Context(), sqldb, user.Login)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			views.Home(user, projects, time.Now().Format("2006-01-02")).Render(r.Context(), w)
		})
		mux.HandleFunc("POST /projects", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			name := strings.TrimSpace(r.FormValue("name"))
			if name == "" {
				http.Error(w, "project name required", http.StatusBadRequest)
				return
			}
			url := strings.TrimSpace(r.FormValue("source_url"))
			if !validURL(url) {
				http.Error(w, "URL must start with http:// or https://", http.StatusBadRequest)
				return
			}
			_, err := sqldb.ExecContext(r.Context(), "INSERT INTO projects (login,name,source_url,notes) VALUES (?,?,?,?)", user.Login, name, url, strings.TrimSpace(r.FormValue("notes")))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			web.Changed(user.Login)
			http.Redirect(w, r, "/", http.StatusSeeOther)
		})
		mux.HandleFunc("POST /projects/{id}/prints", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			r.Body = http.MaxBytesReader(w, r.Body, 9<<20)
			if err := r.ParseMultipartForm(8 << 20); err != nil {
				http.Error(w, "upload must be 8 MiB or smaller", http.StatusBadRequest)
				return
			}
			date := r.FormValue("printed_on")
			if _, err := time.Parse("2006-01-02", date); err != nil {
				http.Error(w, "print date must be YYYY-MM-DD", http.StatusBadRequest)
				return
			}
			var image []byte
			var imageType string
			file, _, err := r.FormFile("image")
			if err == nil {
				defer file.Close()
				image, err = io.ReadAll(io.LimitReader(file, (8<<20)+1))
				if err != nil || len(image) > 8<<20 {
					http.Error(w, "image must be 8 MiB or smaller", http.StatusBadRequest)
					return
				}
				imageType = http.DetectContentType(image)
				if !allowedImage(imageType) {
					http.Error(w, "image must be JPEG, PNG, WebP, or GIF", http.StatusBadRequest)
					return
				}
			} else if err != http.ErrMissingFile {
				http.Error(w, "invalid image", http.StatusBadRequest)
				return
			}
			res, err := sqldb.ExecContext(r.Context(), `INSERT INTO print_history (project_id,login,printed_on,notes,image,image_type)
				SELECT id,login,?,?,?,? FROM projects WHERE id = ? AND login = ?`, date, strings.TrimSpace(r.FormValue("notes")), nullableBytes(image), imageType, r.PathValue("id"), user.Login)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if n, _ := res.RowsAffected(); n == 0 {
				http.NotFound(w, r)
				return
			}
			web.Changed(user.Login)
			http.Redirect(w, r, "/", http.StatusSeeOther)
		})
		mux.HandleFunc("POST /projects/{id}/edit", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			name := strings.TrimSpace(r.FormValue("name"))
			url := strings.TrimSpace(r.FormValue("source_url"))
			if name == "" {
				http.Error(w, "project name required", http.StatusBadRequest)
				return
			}
			if !validURL(url) {
				http.Error(w, "URL must start with http:// or https://", http.StatusBadRequest)
				return
			}
			res, err := sqldb.ExecContext(r.Context(), "UPDATE projects SET name=?,source_url=?,notes=? WHERE id=? AND login=?", name, url, strings.TrimSpace(r.FormValue("notes")), r.PathValue("id"), user.Login)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if n, _ := res.RowsAffected(); n == 0 {
				http.NotFound(w, r)
				return
			}
			web.Changed(user.Login)
			http.Redirect(w, r, "/", http.StatusSeeOther)
		})
		mux.HandleFunc("GET /prints/{id}/image", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			var image []byte
			var typ string
			if err := sqldb.QueryRowContext(r.Context(), "SELECT image,image_type FROM print_history WHERE id = ? AND login = ? AND image IS NOT NULL", r.PathValue("id"), user.Login).Scan(&image, &typ); err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", typ)
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Cache-Control", "private, max-age=86400")
			w.Write(image)
		})
		mux.HandleFunc("POST /projects/{id}/delete", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			_, err := sqldb.ExecContext(r.Context(), "DELETE FROM projects WHERE id = ? AND login = ?", r.PathValue("id"), user.Login)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			web.Changed(user.Login)
			http.Redirect(w, r, "/", http.StatusSeeOther)
		})
	})
}

func validURL(s string) bool {
	return s == "" || strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://")
}
func allowedImage(s string) bool {
	return s == "image/jpeg" || s == "image/png" || s == "image/webp" || s == "image/gif"
}
func nullableBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}
