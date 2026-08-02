// journal: one stream for everything — notes, work log, reflections.
// Spec: README.md (design-app interview, 2026-08-01).
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
	"time"

	"github.com/a-h/templ"
	"github.com/bketelsen/bespoke/apps/journal/views"
	"github.com/bketelsen/bespoke/pkg/audio"
	"github.com/bketelsen/bespoke/pkg/auth"
	"github.com/bketelsen/bespoke/pkg/db"
	"github.com/bketelsen/bespoke/pkg/llm"
	"github.com/bketelsen/bespoke/pkg/web"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

const sqliteTime = "2006-01-02 15:04:05"

func main() {
	migrations, _ := fs.Sub(migrationFS, "migrations")
	sqldb, err := db.Open("journal", migrations)
	if err != nil {
		log.Fatal(err)
	}
	ai := llm.New("journal")
	voice := audio.New("journal")

	web.Run("journal", func(mux *http.ServeMux) {
		registerTools(mux, sqldb) // before EnableChat so chat sees them
		web.EnableChat(mux, "journal", func(ctx context.Context, user auth.User) (string, error) {
			return chatContext(ctx, sqldb, user.Login)
		})

		web.DashboardCard(mux, func(ctx context.Context, user auth.User) (templ.Component, error) {
			return dashCard(ctx, sqldb, user.Login)
		})

		web.Intent(mux, "Journal", web.IntentDef{
			Name:   "add-entry",
			Title:  "Add to Journal",
			Prompt: "This becomes a journal entry.",
			Handler: func(ctx context.Context, user auth.User, text string) (string, error) {
				_, err := sqldb.ExecContext(ctx,
					"INSERT INTO entries (login, body) VALUES (?, ?)", user.Login, text)
				return "/", err
			},
		})

		mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			days, err := loadDays(sqldb, user.Login)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			views.Stream(user, days).Render(r.Context(), w)
		})

		mux.HandleFunc("POST /entries", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			body := strings.TrimSpace(r.FormValue("body"))
			if body != "" {
				if _, err := sqldb.Exec("INSERT INTO entries (login, body) VALUES (?, ?)", user.Login, body); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}
			http.Redirect(w, r, "/", http.StatusSeeOther)
		})

		mux.HandleFunc("POST /entries/work", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			body := workMarkdown(r.FormValue("project"), r.FormValue("task"), r.FormValue("time"))
			if body != "" {
				if _, err := sqldb.Exec("INSERT INTO entries (login, body) VALUES (?, ?)", user.Login, body); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}
			http.Redirect(w, r, "/", http.StatusSeeOther)
		})

		mux.HandleFunc("POST /entries/reflection", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			body := reflectionMarkdown(r.FormValue("general"), r.FormValue("work"), r.FormValue("family"))
			if body != "" {
				if _, err := sqldb.Exec("INSERT INTO entries (login, body) VALUES (?, ?)", user.Login, body); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}
			http.Redirect(w, r, "/", http.StatusSeeOther)
		})

		mux.HandleFunc("POST /entries/voice", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			text, err := voice.Transcribe(r.Context(),
				http.MaxBytesReader(w, r.Body, 25<<20),
				audio.WithMIME(r.Header.Get("Content-Type")))
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			if text = strings.TrimSpace(text); text == "" {
				http.Error(w, "empty transcription", http.StatusUnprocessableEntity)
				return
			}
			if _, err := sqldb.Exec("INSERT INTO entries (login, body) VALUES (?, ?)", user.Login, text); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent) // recorder.js reloads the page
		})

		mux.HandleFunc("POST /entries/{id}/delete", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			if _, err := sqldb.Exec("DELETE FROM entries WHERE id = ? AND login = ?", r.PathValue("id"), user.Login); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			http.Redirect(w, r, "/", http.StatusSeeOther)
		})

		mux.HandleFunc("GET /week", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			sum, count, err := loadWeek(sqldb, user.Login)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			views.Week(user, sum, count).Render(r.Context(), w)
		})

		mux.HandleFunc("POST /week/summarize", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			if err := summarizeWeek(r, sqldb, ai, user.Login); err != nil {
				http.Error(w, "summary failed: "+err.Error(), http.StatusBadGateway)
				return
			}
			http.Redirect(w, r, "/week", http.StatusSeeOther)
		})
	})
}

// workMarkdown formats a work-log capture; empty fields are omitted, all
// fields empty → "" (nothing saved).
func workMarkdown(project, task, timeSpent string) string {
	project, task, timeSpent = strings.TrimSpace(project), strings.TrimSpace(task), strings.TrimSpace(timeSpent)
	if project == "" && task == "" && timeSpent == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("### Work log\n")
	if project != "" {
		fmt.Fprintf(&b, "- **Project:** %s\n", project)
	}
	if task != "" {
		fmt.Fprintf(&b, "- **Task:** %s\n", task)
	}
	if timeSpent != "" {
		fmt.Fprintf(&b, "- **Time:** %s\n", timeSpent)
	}
	return strings.TrimRight(b.String(), "\n")
}

// reflectionMarkdown formats an evening reflection; same omit-empty rules.
func reflectionMarkdown(general, work, family string) string {
	general, work, family = strings.TrimSpace(general), strings.TrimSpace(work), strings.TrimSpace(family)
	if general == "" && work == "" && family == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("### Evening reflection\n")
	if general != "" {
		fmt.Fprintf(&b, "**General:** %s\n\n", general)
	}
	if work != "" {
		fmt.Fprintf(&b, "**Work:** %s\n\n", work)
	}
	if family != "" {
		fmt.Fprintf(&b, "**Family:** %s\n\n", family)
	}
	return strings.TrimRight(b.String(), "\n")
}

// dashCard backs the dashboard card (ADR-0017): cheap queries only.
func dashCard(ctx context.Context, sqldb *sql.DB, login string) (templ.Component, error) {
	var today int
	if err := sqldb.QueryRowContext(ctx,
		"SELECT count(*) FROM entries WHERE login = ? AND date(created_at, 'localtime') = date('now', 'localtime')",
		login).Scan(&today); err != nil {
		return nil, err
	}
	var body, created string
	err := sqldb.QueryRowContext(ctx,
		"SELECT body, created_at FROM entries WHERE login = ? ORDER BY created_at DESC, id DESC LIMIT 1",
		login).Scan(&body, &created)
	latestTime, snippet := "", ""
	switch {
	case err == sql.ErrNoRows:
	case err != nil:
		return nil, err
	default:
		if t, perr := time.ParseInLocation(sqliteTime, created, time.UTC); perr == nil {
			latestTime = t.Local().Format("15:04")
		}
		snippet = body
		if len(snippet) > 120 {
			snippet = snippet[:120] + "…"
		}
	}
	return views.DashCard(today, latestTime, snippet), nil
}

// localStamp renders a stored UTC timestamp in local time for LLM
// consumption — feeding raw UTC to the model makes it narrate evening
// entries as "just after midnight".
func localStamp(created string) string {
	t, err := time.ParseInLocation(sqliteTime, created, time.UTC)
	if err != nil {
		return created
	}
	return t.Local().Format("Mon Jan 2 2006, 3:04 PM")
}

// chatContext feeds the in-app chat (ADR-0015): the last 30 days of entries
// plus the latest weekly summary — enough for mood/productivity questions.
func chatContext(ctx context.Context, sqldb *sql.DB, login string) (string, error) {
	rows, err := sqldb.QueryContext(ctx,
		"SELECT created_at, body FROM entries WHERE login = ? AND created_at >= datetime('now', '-30 days') ORDER BY created_at",
		login)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	zone, _ := time.Now().Zone()
	var b strings.Builder
	fmt.Fprintf(&b, "Journal entries, last 30 days (timestamps in the owner's local time, %s):\n", zone)
	n := 0
	for rows.Next() {
		var created, body string
		if err := rows.Scan(&created, &body); err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "[%s] %s\n", localStamp(created), body)
		n++
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if n == 0 {
		b.WriteString("(no entries yet)\n")
	}

	var summary, created string
	err = sqldb.QueryRowContext(ctx,
		"SELECT body, created_at FROM summaries WHERE login = ? ORDER BY created_at DESC, id DESC LIMIT 1",
		login).Scan(&summary, &created)
	if err == nil {
		fmt.Fprintf(&b, "\nLatest weekly summary (generated %s):\n%s\n", localStamp(created), summary)
	} else if err != sql.ErrNoRows {
		return "", err
	}
	return b.String(), nil
}

func loadDays(sqldb *sql.DB, login string) ([]views.Day, error) {
	rows, err := sqldb.Query(
		"SELECT id, body, created_at FROM entries WHERE login = ? ORDER BY created_at DESC, id DESC LIMIT 500", login)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var days []views.Day
	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	for rows.Next() {
		var e views.Entry
		var created string
		if err := rows.Scan(&e.ID, &e.Body, &created); err != nil {
			return nil, err
		}
		t, err := time.ParseInLocation(sqliteTime, created, time.UTC)
		if err != nil {
			return nil, err
		}
		local := t.Local()
		e.Time = local.Format("15:04")
		var label string
		switch local.Format("2006-01-02") {
		case today:
			label = "Today"
		case yesterday:
			label = "Yesterday"
		default:
			label = local.Format("Mon, Jan 2 2006")
		}
		if len(days) == 0 || days[len(days)-1].Label != label {
			days = append(days, views.Day{Label: label})
		}
		days[len(days)-1].Entries = append(days[len(days)-1].Entries, e)
	}
	return days, rows.Err()
}

func loadWeek(sqldb *sql.DB, login string) (*views.Summary, int, error) {
	var count int
	if err := sqldb.QueryRow(
		"SELECT count(*) FROM entries WHERE login = ? AND created_at >= datetime('now', '-7 days')", login).Scan(&count); err != nil {
		return nil, 0, err
	}
	var s views.Summary
	var created string
	err := sqldb.QueryRow(
		"SELECT body, range_start, range_end, created_at FROM summaries WHERE login = ? ORDER BY created_at DESC, id DESC LIMIT 1",
		login).Scan(&s.Body, &s.RangeStart, &s.RangeEnd, &created)
	switch {
	case err == sql.ErrNoRows:
		return nil, count, nil
	case err != nil:
		return nil, 0, err
	}
	if t, perr := time.ParseInLocation(sqliteTime, created, time.UTC); perr == nil {
		s.Generated = t.Local().Format("Mon, Jan 2 15:04")
	}
	return &s, count, nil
}

func summarizeWeek(r *http.Request, sqldb *sql.DB, ai *llm.Client, login string) error {
	rows, err := sqldb.Query(
		"SELECT body, created_at FROM entries WHERE login = ? AND created_at >= datetime('now', '-7 days') ORDER BY created_at", login)
	if err != nil {
		return err
	}
	defer rows.Close()
	var b strings.Builder
	n := 0
	for rows.Next() {
		var body, created string
		if err := rows.Scan(&body, &created); err != nil {
			return err
		}
		fmt.Fprintf(&b, "[%s] %s\n", localStamp(created), body)
		n++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("no entries in the last 7 days")
	}

	text, err := ai.Complete(r.Context(), b.String(),
		llm.WithUser(login),
		llm.WithSystem(
			"You summarize one person's private journal from the past week. "+
				"Write a short, warm summary (max ~150 words): what happened, recurring threads, overall mood. "+
				"Second person ('you'), no preamble, no bullet-point dump."))
	if err != nil {
		return err
	}

	end := time.Now()
	start := end.AddDate(0, 0, -7)
	_, err = sqldb.Exec(
		"INSERT INTO summaries (login, range_start, range_end, body) VALUES (?, ?, ?, ?)",
		login, start.Format("Jan 2"), end.Format("Jan 2"), text)
	return err
}
