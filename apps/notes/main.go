// notes is a synthetic showcase app: an append-only stream whose selections
// can become todos through the platform intent chrome.
package main

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/bketelsen/bespoke/apps/notes/views"
	"github.com/bketelsen/bespoke/pkg/auth"
	"github.com/bketelsen/bespoke/pkg/db"
	"github.com/bketelsen/bespoke/pkg/web"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

func loadNotes(ctx context.Context, sqldb *sql.DB, login string, limit int) ([]views.Note, error) {
	rows, err := sqldb.QueryContext(ctx, "SELECT body, created_at FROM notes WHERE login=? ORDER BY id DESC LIMIT ?", login, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var notes []views.Note
	for rows.Next() {
		var n views.Note
		if err := rows.Scan(&n.Body, &n.CreatedAt); err != nil {
			return nil, err
		}
		if stamp, err := time.ParseInLocation("2006-01-02 15:04:05", n.CreatedAt, time.UTC); err == nil {
			n.CreatedAt = stamp.Local().Format("Jan 2, 2006 at 3:04 PM")
		}
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

func addNote(ctx context.Context, sqldb *sql.DB, login, body string) error {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil
	}
	_, err := sqldb.ExecContext(ctx, "INSERT INTO notes(login, body) VALUES(?, ?)", login, body)
	if err == nil {
		web.Changed(login)
	}
	return err
}

func main() {
	migrations, _ := fs.Sub(migrationFS, "migrations")
	sqldb, err := db.Open("notes", migrations)
	if err != nil {
		log.Fatal(err)
	}

	web.Run("notes", func(mux *http.ServeMux) {
		registerTools(mux, sqldb)
		web.EnableChat(mux, "notes", func(ctx context.Context, user auth.User) (string, error) {
			notes, err := loadNotes(ctx, sqldb, user.Login, 50)
			if err != nil {
				return "", err
			}
			var b strings.Builder
			for _, n := range notes {
				b.WriteString(n.CreatedAt + " — " + n.Body + "\n")
			}
			return b.String(), nil
		})
		web.Live(mux, func(ctx context.Context, user auth.User) (templ.Component, error) {
			notes, err := loadNotes(ctx, sqldb, user.Login, 100)
			if err != nil {
				return nil, err
			}
			return views.NotesLive(notes), nil
		})
		web.DashboardCard(mux, func(ctx context.Context, user auth.User) (templ.Component, error) {
			notes, err := loadNotes(ctx, sqldb, user.Login, 3)
			if err != nil {
				return nil, err
			}
			return views.DashCard(notes), nil
		})
		web.Intent(mux, "Notes", web.IntentDef{
			Name: "add-note", Title: "Save as Note", Prompt: "Review the note before saving.",
			Handler: func(ctx context.Context, user auth.User, text string) (string, error) {
				return "/", addNote(ctx, sqldb, user.Login, text)
			},
		})
		mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			notes, err := loadNotes(r.Context(), sqldb, user.Login, 100)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			views.Home(user, notes).Render(r.Context(), w)
		})
		mux.HandleFunc("POST /notes", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			if err := addNote(r.Context(), sqldb, user.Login, r.FormValue("body")); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			http.Redirect(w, r, "/", http.StatusSeeOther)
		})
	})
}
