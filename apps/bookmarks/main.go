// bookmarks: a dashboard of saved links, opened when Brian wants to jump
// straight to a site without hunting for it.
// Spec: README.md.
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
	"github.com/bketelsen/bespoke/apps/bookmarks/views"
	"github.com/bketelsen/bespoke/pkg/auth"
	"github.com/bketelsen/bespoke/pkg/db"
	"github.com/bketelsen/bespoke/pkg/web"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

func main() {
	migrations, _ := fs.Sub(migrationFS, "migrations")
	sqldb, err := db.Open("bookmarks", migrations)
	if err != nil {
		log.Fatal(err)
	}

	web.Run("bookmarks", func(mux *http.ServeMux) {
		registerTools(mux, sqldb) // before EnableChat so chat sees them
		web.EnableChat(mux, "bookmarks", func(ctx context.Context, user auth.User) (string, error) {
			return chatContext(ctx, sqldb, user.Login)
		})
		web.DashboardCard(mux, func(ctx context.Context, user auth.User) (templ.Component, error) {
			return dashCard(ctx, sqldb, user.Login)
		})

		// Live bookmark list region (ADR-0022): patched on any change.
		web.Live(mux, func(ctx context.Context, user auth.User) (templ.Component, error) {
			bms, err := loadBookmarks(ctx, sqldb, user.Login, "")
			if err != nil {
				return nil, err
			}
			return views.BookmarksLive(bms, ""), nil
		})

		web.Intent(mux, "Bookmarks", web.IntentDef{
			Name:   "save-link",
			Title:  "Save Link",
			Prompt: "Paste the URL to save (title is auto-fetched).",
			Handler: func(ctx context.Context, user auth.User, text string) (string, error) {
				u := strings.TrimSpace(text)
				if _, err := addBookmark(ctx, sqldb, user.Login, u, "", ""); err != nil {
					return "", err
				}
				web.Changed(user.Login)
				return "/", nil
			},
		})

		mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			tag := r.URL.Query().Get("tag")
			bms, err := loadBookmarks(r.Context(), sqldb, user.Login, tag)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			allTags, err := loadTags(r.Context(), sqldb, user.Login)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			views.Home(user, bms, allTags, tag).Render(r.Context(), w)
		})

		mux.HandleFunc("GET /add", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			views.Add(user, "", "", "").Render(r.Context(), w)
		})

		mux.HandleFunc("POST /add", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			rawURL := strings.TrimSpace(r.FormValue("url"))
			title := strings.TrimSpace(r.FormValue("title"))
			tags := normalizeTags(r.FormValue("tags"))
			if rawURL == "" {
				views.Add(user, rawURL, title, tags).Render(r.Context(), w)
				return
			}
			if _, err := addBookmark(r.Context(), sqldb, user.Login, rawURL, title, tags); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			web.Changed(user.Login)
			http.Redirect(w, r, "/", http.StatusSeeOther)
		})

		// Fetch a page title for a URL, used by the add form's auto-fetch.
		mux.HandleFunc("GET /fetch-title", func(w http.ResponseWriter, r *http.Request) {
			rawURL := strings.TrimSpace(r.URL.Query().Get("url"))
			title := ""
			if rawURL != "" {
				title = fetchTitle(rawURL)
			}
			_, _ = fmt.Fprint(w, title)
		})

		mux.HandleFunc("GET /bookmarks/{id}/edit", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			var b views.Bookmark
			err := sqldb.QueryRowContext(r.Context(),
				"SELECT id, url, title, tags FROM bookmarks WHERE id = ? AND login = ?",
				r.PathValue("id"), user.Login).Scan(&b.ID, &b.URL, &b.Title, &b.Tags)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			views.Edit(user, b).Render(r.Context(), w)
		})

		mux.HandleFunc("POST /bookmarks/{id}/edit", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			rawURL := strings.TrimSpace(r.FormValue("url"))
			title := strings.TrimSpace(r.FormValue("title"))
			tags := normalizeTags(r.FormValue("tags"))
			if rawURL == "" || title == "" {
				http.Error(w, "url and title required", http.StatusBadRequest)
				return
			}
			if _, err := sqldb.ExecContext(r.Context(),
				"UPDATE bookmarks SET url = ?, title = ?, tags = ? WHERE id = ? AND login = ?",
				rawURL, title, tags, r.PathValue("id"), user.Login); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			web.Changed(user.Login)
			http.Redirect(w, r, "/", http.StatusSeeOther)
		})

		mux.HandleFunc("POST /bookmarks/{id}/delete", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			if _, err := sqldb.ExecContext(r.Context(),
				"DELETE FROM bookmarks WHERE id = ? AND login = ?", r.PathValue("id"), user.Login); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			web.Changed(user.Login)
			http.Redirect(w, r, "/", http.StatusSeeOther)
		})
	})
}

// normalizeTags trims and lowercases a comma-separated tag list, dropping
// empties, and collapses to a canonical "a,b,c" storage form.
func normalizeTags(raw string) string {
	parts := strings.Split(raw, ",")
	var out []string
	seen := map[string]bool{}
	for _, p := range parts {
		t := strings.ToLower(strings.TrimSpace(p))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return strings.Join(out, ",")
}

// addBookmark inserts a bookmark, auto-fetching the title when the caller
// didn't supply one (paste-URL flow and chat/intent flow).
func addBookmark(ctx context.Context, sqldb *sql.DB, login, rawURL, title, tags string) (int64, error) {
	if rawURL == "" {
		return 0, fmt.Errorf("url is required")
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}
	if title == "" {
		title = fetchTitle(rawURL)
	}
	if title == "" {
		title = rawURL
	}
	res, err := sqldb.ExecContext(ctx,
		"INSERT INTO bookmarks (login, url, title, tags) VALUES (?, ?, ?, ?)",
		login, rawURL, title, normalizeTags(tags))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func loadBookmarks(ctx context.Context, sqldb *sql.DB, login, tag string) ([]views.Bookmark, error) {
	query := "SELECT id, url, title, tags, created_at FROM bookmarks WHERE login = ?"
	args := []any{login}
	if tag != "" {
		query += " AND (',' || tags || ',') LIKE ?"
		args = append(args, "%,"+tag+",%")
	}
	query += " ORDER BY created_at DESC, id DESC"
	rows, err := sqldb.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []views.Bookmark
	for rows.Next() {
		var b views.Bookmark
		var tagsCSV string
		if err := rows.Scan(&b.ID, &b.URL, &b.Title, &tagsCSV, &b.CreatedAt); err != nil {
			return nil, err
		}
		b.Tags = splitTags(tagsCSV)
		out = append(out, b)
	}
	return out, rows.Err()
}

func loadTags(ctx context.Context, sqldb *sql.DB, login string) ([]string, error) {
	rows, err := sqldb.QueryContext(ctx, "SELECT tags FROM bookmarks WHERE login = ? AND tags != ''", login)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := map[string]bool{}
	var out []string
	for rows.Next() {
		var tagsCSV string
		if err := rows.Scan(&tagsCSV); err != nil {
			return nil, err
		}
		for _, t := range splitTags(tagsCSV) {
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	return out, rows.Err()
}

func splitTags(csv string) []string {
	if csv == "" {
		return nil
	}
	return strings.Split(csv, ",")
}

func chatContext(ctx context.Context, sqldb *sql.DB, login string) (string, error) {
	bms, err := loadBookmarks(ctx, sqldb, login, "")
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("Saved bookmarks (most recent first):\n")
	if len(bms) == 0 {
		b.WriteString("(none yet)\n")
	}
	for _, bm := range bms {
		fmt.Fprintf(&b, "#%d %s — %s", bm.ID, bm.Title, bm.URL)
		if len(bm.Tags) > 0 {
			fmt.Fprintf(&b, " [%s]", strings.Join(bm.Tags, ", "))
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

// dashCard: most recent 3-5 bookmarks (README).
func dashCard(ctx context.Context, sqldb *sql.DB, login string) (templ.Component, error) {
	bms, err := loadBookmarks(ctx, sqldb, login, "")
	if err != nil {
		return nil, err
	}
	if len(bms) > 5 {
		bms = bms[:5]
	}
	return views.DashCard(bms), nil
}
