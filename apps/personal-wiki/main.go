// personal-wiki: a single-user reference wiki — jot notes, browse by tag,
// link related pages with [[Title]].
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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/bketelsen/bespoke/apps/personal-wiki/views"
	"github.com/bketelsen/bespoke/pkg/auth"
	"github.com/bketelsen/bespoke/pkg/db"
	"github.com/bketelsen/bespoke/pkg/web"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

//go:embed skills
var skillFS embed.FS

const sqliteTime = "2006-01-02 15:04:05"

// linkPattern matches [[Title]] wiki-links in page bodies.
var linkPattern = regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)

func main() {
	migrations, _ := fs.Sub(migrationFS, "migrations")
	sqldb, err := db.Open("personal-wiki", migrations)
	if err != nil {
		log.Fatal(err)
	}

	web.Run("personal-wiki", func(mux *http.ServeMux) {
		registerTools(mux, sqldb) // before EnableChat so chat sees them
		skills, _ := fs.Sub(skillFS, "skills")
		if err := web.Skills(mux, skills); err != nil { // ADR-0026: page-authoring, wiki-gardening
			log.Fatal(err)
		}
		web.EnableChat(mux, "personal-wiki", func(ctx context.Context, user auth.User) (string, error) {
			return chatContext(ctx, sqldb, user.Login)
		})

		web.DashboardCard(mux, func(ctx context.Context, user auth.User) (templ.Component, error) {
			return dashCard(ctx, sqldb, user.Login)
		})

		web.Intent(mux, "Personal Wiki", web.IntentDef{
			Name:   "add-page",
			Title:  "Add to Wiki",
			Prompt: "This becomes a wiki page (title guessed from the first line).",
			Handler: func(ctx context.Context, user auth.User, text string) (string, error) {
				title, body := splitTitleBody(text)
				id, err := upsertPage(ctx, sqldb, user.Login, title, body, nil)
				if err != nil {
					return "", err
				}
				web.Changed(user.Login)
				return fmt.Sprintf("/page/%d", id), nil
			},
		})

		mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			if err := ensureIndex(r.Context(), sqldb, user.Login); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			alpha := r.URL.Query().Get("sort") == "alpha"
			q := r.URL.Query().Get("q")
			pages, err := loadPages(r.Context(), sqldb, user.Login, alpha, q)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			recent, err := loadRecentlyViewed(r.Context(), sqldb, user.Login, 5)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			var indexID int64
			if err := sqldb.QueryRowContext(r.Context(),
				"SELECT id FROM pages WHERE login = ? AND title = ?", user.Login, indexTitle).Scan(&indexID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			index, err := loadPage(r.Context(), sqldb, user.Login, indexID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			graph, err := loadGraph(r.Context(), sqldb, user.Login)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			views.Home(user, index, orphanPages(graph), pages, recent, alpha, q).Render(r.Context(), w)
		})

		mux.HandleFunc("GET /new", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			views.New(user).Render(r.Context(), w)
		})

		mux.HandleFunc("POST /pages", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			title := strings.TrimSpace(r.FormValue("title"))
			if title == "" {
				http.Error(w, "title is required", http.StatusBadRequest)
				return
			}
			tags := parseTags(r.FormValue("tags"))
			id, err := upsertPage(r.Context(), sqldb, user.Login, title, r.FormValue("body"), tags)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			web.Changed(user.Login)
			http.Redirect(w, r, fmt.Sprintf("/page/%d", id), http.StatusSeeOther)
		})

		mux.HandleFunc("GET /page/{id}", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			page, err := loadPage(r.Context(), sqldb, user.Login, id)
			if err == sql.ErrNoRows {
				http.NotFound(w, r)
				return
			} else if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := touchViewed(r.Context(), sqldb, user.Login, id); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			web.Changed(user.Login) // recently-viewed / dashboard resurfacing
			backlinks, err := loadBacklinks(r.Context(), sqldb, user.Login, page.Title, id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			graph, err := loadGraph(r.Context(), sqldb, user.Login)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			views.View(user, page, breadcrumb(graph, page.Title), backlinks).Render(r.Context(), w)
		})

		mux.HandleFunc("GET /page/{id}/edit", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			page, err := loadPage(r.Context(), sqldb, user.Login, id)
			if err == sql.ErrNoRows {
				http.NotFound(w, r)
				return
			} else if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			views.Edit(user, page).Render(r.Context(), w)
		})

		mux.HandleFunc("POST /page/{id}/edit", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			title := strings.TrimSpace(r.FormValue("title"))
			if title == "" {
				http.Error(w, "title is required", http.StatusBadRequest)
				return
			}
			tags := parseTags(r.FormValue("tags"))
			if err := updatePage(r.Context(), sqldb, user.Login, id, title, r.FormValue("body"), tags); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			web.Changed(user.Login)
			http.Redirect(w, r, fmt.Sprintf("/page/%d", id), http.StatusSeeOther)
		})

		mux.HandleFunc("POST /page/{id}/delete", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			id := r.PathValue("id")
			// The Index is the hierarchy root — it cannot be deleted.
			if _, err := sqldb.ExecContext(r.Context(),
				"DELETE FROM pages WHERE id = ? AND login = ? AND title != ?", id, user.Login, indexTitle); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			web.Changed(user.Login)
			http.Redirect(w, r, "/", http.StatusSeeOther)
		})

		mux.HandleFunc("GET /tag/{tag}", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			tag := r.PathValue("tag")
			pages, err := loadPagesByTag(r.Context(), sqldb, user.Login, tag)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			views.Tag(user, tag, pages).Render(r.Context(), w)
		})
	})
}

// splitTitleBody derives a title/body pair from free text (intents): first
// line (capped) is the title, the rest is the body.
func splitTitleBody(text string) (string, string) {
	text = strings.TrimSpace(text)
	line := text
	rest := ""
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		line, rest = text[:idx], strings.TrimSpace(text[idx+1:])
	}
	line = strings.TrimSpace(line)
	if len(line) > 80 {
		line = line[:80]
	}
	if line == "" {
		line = time.Now().Format("Jan 2 2006 15:04")
	}
	if rest == "" {
		rest = text
	}
	return line, rest
}

func parseTags(raw string) []string {
	var tags []string
	seen := map[string]bool{}
	for _, t := range strings.Split(raw, ",") {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		tags = append(tags, t)
	}
	sort.Strings(tags)
	return tags
}

func setTags(ctx context.Context, sqldb *sql.DB, pageID int64, tags []string) error {
	if _, err := sqldb.ExecContext(ctx, "DELETE FROM page_tags WHERE page_id = ?", pageID); err != nil {
		return err
	}
	for _, t := range tags {
		if _, err := sqldb.ExecContext(ctx,
			"INSERT INTO page_tags (page_id, tag) VALUES (?, ?)", pageID, t); err != nil {
			return err
		}
	}
	return nil
}

// wikiLinkTitles extracts unique [[Title]] references from a body.
func wikiLinkTitles(body string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range linkPattern.FindAllStringSubmatch(body, -1) {
		title := strings.TrimSpace(m[1])
		if title == "" || seen[title] {
			continue
		}
		seen[title] = true
		out = append(out, title)
	}
	return out
}

// ensureStubs creates blank stub pages for any [[Title]] reference in body
// that doesn't already exist for this user.
func ensureStubs(ctx context.Context, sqldb *sql.DB, login, body string) error {
	for _, title := range wikiLinkTitles(body) {
		var exists int
		err := sqldb.QueryRowContext(ctx,
			"SELECT 1 FROM pages WHERE login = ? AND title = ?", login, title).Scan(&exists)
		if err == nil {
			continue // already exists
		}
		if err != sql.ErrNoRows {
			return err
		}
		if _, err := sqldb.ExecContext(ctx,
			"INSERT INTO pages (login, title, body) VALUES (?, ?, '')", login, title); err != nil {
			return err
		}
	}
	return nil
}

// upsertPage creates a page (or errors on duplicate title) and auto-creates
// stub pages for any [[Title]] link in the body.
func upsertPage(ctx context.Context, sqldb *sql.DB, login, title, body string, tags []string) (int64, error) {
	res, err := sqldb.ExecContext(ctx,
		"INSERT INTO pages (login, title, body) VALUES (?, ?, ?)", login, title, body)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	if err := setTags(ctx, sqldb, id, tags); err != nil {
		return 0, err
	}
	if err := ensureStubs(ctx, sqldb, login, body); err != nil {
		return 0, err
	}
	return id, nil
}

func updatePage(ctx context.Context, sqldb *sql.DB, login string, id int64, title, body string, tags []string) error {
	// The Index anchors the hierarchy; its body is editable but its title
	// is load-bearing (breadcrumbs, ensureIndex) and cannot change.
	var current string
	if err := sqldb.QueryRowContext(ctx,
		"SELECT title FROM pages WHERE id = ? AND login = ?", id, login).Scan(&current); err != nil {
		return err
	}
	if current == indexTitle && title != indexTitle {
		return fmt.Errorf("the %s page cannot be renamed", indexTitle)
	}
	res, err := sqldb.ExecContext(ctx,
		"UPDATE pages SET title = ?, body = ?, updated_at = datetime('now') WHERE id = ? AND login = ?",
		title, body, id, login)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("no page #%d", id)
	}
	if err := setTags(ctx, sqldb, id, tags); err != nil {
		return err
	}
	return ensureStubs(ctx, sqldb, login, body)
}

func touchViewed(ctx context.Context, sqldb *sql.DB, login string, id int64) error {
	_, err := sqldb.ExecContext(ctx,
		"UPDATE pages SET last_viewed_at = datetime('now') WHERE id = ? AND login = ?", id, login)
	return err
}

func loadTagsFor(ctx context.Context, sqldb *sql.DB, pageID int64) ([]string, error) {
	rows, err := sqldb.QueryContext(ctx, "SELECT tag FROM page_tags WHERE page_id = ? ORDER BY tag", pageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

func loadPages(ctx context.Context, sqldb *sql.DB, login string, alpha bool, q string) ([]views.PageSummary, error) {
	order := "created_at DESC"
	if alpha {
		order = "title COLLATE NOCASE ASC"
	}
	q = strings.TrimSpace(q)
	var rows *sql.Rows
	var err error
	if q == "" {
		rows, err = sqldb.QueryContext(ctx,
			"SELECT id, title, updated_at FROM pages WHERE login = ? ORDER BY "+order, login)
	} else {
		like := "%" + q + "%"
		rows, err = sqldb.QueryContext(ctx,
			"SELECT id, title, updated_at FROM pages WHERE login = ? AND (title LIKE ? OR body LIKE ?) ORDER BY "+order,
			login, like, like)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pages []views.PageSummary
	for rows.Next() {
		var p views.PageSummary
		var updated string
		if err := rows.Scan(&p.ID, &p.Title, &updated); err != nil {
			return nil, err
		}
		p.UpdatedLabel = localStamp(updated)
		tags, err := loadTagsFor(ctx, sqldb, p.ID)
		if err != nil {
			return nil, err
		}
		p.Tags = tags
		pages = append(pages, p)
	}
	return pages, rows.Err()
}

func loadPagesByTag(ctx context.Context, sqldb *sql.DB, login, tag string) ([]views.PageSummary, error) {
	rows, err := sqldb.QueryContext(ctx,
		`SELECT p.id, p.title, p.updated_at FROM pages p
		 JOIN page_tags pt ON pt.page_id = p.id
		 WHERE p.login = ? AND pt.tag = ? ORDER BY p.title COLLATE NOCASE ASC`, login, tag)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pages []views.PageSummary
	for rows.Next() {
		var p views.PageSummary
		var updated string
		if err := rows.Scan(&p.ID, &p.Title, &updated); err != nil {
			return nil, err
		}
		p.UpdatedLabel = localStamp(updated)
		tags, err := loadTagsFor(ctx, sqldb, p.ID)
		if err != nil {
			return nil, err
		}
		p.Tags = tags
		pages = append(pages, p)
	}
	return pages, rows.Err()
}

func loadRecentlyViewed(ctx context.Context, sqldb *sql.DB, login string, limit int) ([]views.PageSummary, error) {
	rows, err := sqldb.QueryContext(ctx,
		"SELECT id, title, last_viewed_at FROM pages WHERE login = ? AND last_viewed_at IS NOT NULL ORDER BY last_viewed_at DESC LIMIT ?",
		login, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pages []views.PageSummary
	for rows.Next() {
		var p views.PageSummary
		var viewed string
		if err := rows.Scan(&p.ID, &p.Title, &viewed); err != nil {
			return nil, err
		}
		p.UpdatedLabel = localStamp(viewed)
		pages = append(pages, p)
	}
	return pages, rows.Err()
}

func loadPage(ctx context.Context, sqldb *sql.DB, login string, id int64) (views.Page, error) {
	var p views.Page
	var created, updated string
	var lastViewed sql.NullString
	err := sqldb.QueryRowContext(ctx,
		"SELECT id, title, body, created_at, updated_at, last_viewed_at FROM pages WHERE id = ? AND login = ?",
		id, login).Scan(&p.ID, &p.Title, &p.Body, &created, &updated, &lastViewed)
	if err != nil {
		return p, err
	}
	p.CreatedLabel = localStamp(created)
	p.UpdatedLabel = localStamp(updated)
	if lastViewed.Valid {
		p.LastViewedLabel = localStamp(lastViewed.String)
	}
	tags, err := loadTagsFor(ctx, sqldb, id)
	if err != nil {
		return p, err
	}
	p.Tags = tags
	p.TagsRaw = strings.Join(tags, ", ")

	// Resolve [[Title]] links to existing page ids for this user, in one pass.
	links := map[string]int64{}
	for _, title := range wikiLinkTitles(p.Body) {
		var linkedID int64
		if err := sqldb.QueryRowContext(ctx,
			"SELECT id FROM pages WHERE login = ? AND title = ?", login, title).Scan(&linkedID); err == nil {
			links[title] = linkedID
		}
	}
	p.RenderedBody = renderLinks(p.Body, links)
	return p, nil
}

// renderLinks turns [[Title]] into markdown links to /page/:id (resolved) or
// stays literal if somehow unresolved (shouldn't happen — stubs are
// auto-created).
func renderLinks(body string, links map[string]int64) string {
	return linkPattern.ReplaceAllStringFunc(body, func(m string) string {
		title := strings.TrimSpace(m[2 : len(m)-2])
		if id, ok := links[title]; ok {
			return fmt.Sprintf("[%s](/page/%d)", title, id)
		}
		return title
	})
}

func loadBacklinks(ctx context.Context, sqldb *sql.DB, login, title string, excludeID int64) ([]views.PageSummary, error) {
	rows, err := sqldb.QueryContext(ctx,
		"SELECT id, title, body FROM pages WHERE login = ? AND id != ?", login, excludeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pages []views.PageSummary
	needle := "[[" + title + "]]"
	for rows.Next() {
		var id int64
		var t, body string
		if err := rows.Scan(&id, &t, &body); err != nil {
			return nil, err
		}
		if strings.Contains(body, needle) {
			pages = append(pages, views.PageSummary{ID: id, Title: t})
		}
	}
	return pages, rows.Err()
}

// dashCard backs the dashboard card (ADR-0017): cheap queries only.
func dashCard(ctx context.Context, sqldb *sql.DB, login string) (templ.Component, error) {
	var recentCount int
	if err := sqldb.QueryRowContext(ctx,
		"SELECT count(*) FROM pages WHERE login = ? AND last_viewed_at >= datetime('now', '-7 days')",
		login).Scan(&recentCount); err != nil {
		return nil, err
	}
	var total int
	if err := sqldb.QueryRowContext(ctx,
		"SELECT count(*) FROM pages WHERE login = ?", login).Scan(&total); err != nil {
		return nil, err
	}
	return views.DashCard(recentCount, total), nil
}

// localStamp renders a stored UTC timestamp in local time.
func localStamp(created string) string {
	t, err := time.ParseInLocation(sqliteTime, created, time.UTC)
	if err != nil {
		return created
	}
	return t.Local().Format("Mon Jan 2 2006, 3:04 PM")
}

// chatContext feeds the in-app chat (ADR-0015): the wiki's hierarchy as an
// indented tree (children = [[links]], rooted at Index) plus orphans, so
// the model authors into the structure instead of a flat pile of titles.
func chatContext(ctx context.Context, sqldb *sql.DB, login string) (string, error) {
	if err := ensureIndex(ctx, sqldb, login); err != nil {
		return "", err
	}
	graph, err := loadGraph(ctx, sqldb, login)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("Wiki hierarchy — indentation shows parent → child via [[links]], rooted at the required Index page. " +
		"Titles only; use get_page for content. New pages must be linked from a parent to join the hierarchy.\n\n")
	b.WriteString(hierarchyText(graph))
	return b.String(), nil
}
