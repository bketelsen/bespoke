// family-walks: a Bespoke app — manifest, routes, views, migrations, nothing
// else (CLAUDE.md).
// Spec: README.md (design-app interview, 2026-08-02).
package main

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/bketelsen/bespoke/apps/family-walks/views"
	"github.com/bketelsen/bespoke/pkg/auth"
	"github.com/bketelsen/bespoke/pkg/db"
	"github.com/bketelsen/bespoke/pkg/web"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

const dateFormat = "2006-01-02"

func main() {
	migrations, _ := fs.Sub(migrationFS, "migrations")
	sqldb, err := db.Open("family-walks", migrations)
	if err != nil {
		log.Fatal(err)
	}

	web.Run("family-walks", func(mux *http.ServeMux) {
		registerTools(mux, sqldb) // before EnableChat so chat sees them
		web.EnableChat(mux, "family-walks", func(ctx context.Context, user auth.User) (string, error) {
			return chatContext(ctx, sqldb, user.Login)
		})
		web.DashboardCard(mux, func(ctx context.Context, user auth.User) (templ.Component, error) {
			today := time.Now().Format(dateFormat)
			walked, logged, err := todayStatus(ctx, sqldb, user.Login, today)
			if err != nil {
				return nil, err
			}
			streak, err := currentStreak(ctx, sqldb, user.Login, today)
			if err != nil {
				return nil, err
			}
			return views.DashCard(logged, walked, streak), nil
		})

		web.Live(mux, func(ctx context.Context, user auth.User) (templ.Component, error) {
			today := time.Now().Format(dateFormat)
			walked, logged, err := todayStatus(ctx, sqldb, user.Login, today)
			if err != nil {
				return nil, err
			}
			streak, err := currentStreak(ctx, sqldb, user.Login, today)
			if err != nil {
				return nil, err
			}
			return views.TodayLive(today, walked, logged, streak, milestoneMessage(streak)), nil
		})

		web.Intent(mux, "Family Walks", web.IntentDef{
			Name:   "log-walk",
			Title:  "Log tonight's walk",
			Prompt: "Did the family take a walk tonight? Reply yes or no (optionally with a note).",
			Handler: func(ctx context.Context, user auth.User, text string) (string, error) {
				walked := parseYesNo(text)
				today := time.Now().Format(dateFormat)
				if err := setWalkDay(ctx, sqldb, user.Login, today, walked, ""); err != nil {
					return "", err
				}
				web.Changed(user.Login)
				return "/", nil
			},
		})

		mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			today := time.Now().Format(dateFormat)
			walked, logged, err := todayStatus(r.Context(), sqldb, user.Login, today)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			streak, err := currentStreak(r.Context(), sqldb, user.Login, today)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			views.Home(user, today, walked, logged, streak, milestoneMessage(streak)).Render(r.Context(), w)
		})

		mux.HandleFunc("POST /today", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			today := time.Now().Format(dateFormat)
			walked := r.FormValue("walked") == "yes"
			if err := setWalkDay(r.Context(), sqldb, user.Login, today, walked, ""); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			web.Changed(user.Login)
			http.Redirect(w, r, "/", http.StatusSeeOther)
		})

		mux.HandleFunc("GET /history", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			days, err := loadHistory(r.Context(), sqldb, user.Login)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			views.History(user, days).Render(r.Context(), w)
		})

		mux.HandleFunc("GET /history/{date}", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			date := r.PathValue("date")
			if _, err := time.Parse(dateFormat, date); err != nil {
				http.Error(w, "invalid date", http.StatusBadRequest)
				return
			}
			day, err := loadDay(r.Context(), sqldb, user.Login, date)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			views.EditDay(user, *day).Render(r.Context(), w)
		})

		mux.HandleFunc("POST /history/{date}", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			date := r.PathValue("date")
			if _, err := time.Parse(dateFormat, date); err != nil {
				http.Error(w, "invalid date", http.StatusBadRequest)
				return
			}
			walked := r.FormValue("walked") == "yes"
			note := r.FormValue("note")
			if err := setWalkDay(r.Context(), sqldb, user.Login, date, walked, note); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			web.Changed(user.Login)
			http.Redirect(w, r, "/history", http.StatusSeeOther)
		})
	})
}

// setWalkDay upserts a single WalkDay record for the given user/date.
func setWalkDay(ctx context.Context, sqldb *sql.DB, login, date string, walked bool, note string) error {
	w := 0
	if walked {
		w = 1
	}
	_, err := sqldb.ExecContext(ctx, `
		INSERT INTO walk_days (login, date, walked, note, created_at, updated_at)
		VALUES (?, ?, ?, NULLIF(?, ''), datetime('now'), datetime('now'))
		ON CONFLICT (login, date) DO UPDATE SET
			walked = excluded.walked,
			note = excluded.note,
			updated_at = datetime('now')`,
		login, date, w, note)
	return err
}

// todayStatus reports whether the given date has been logged, and if so,
// what value.
func todayStatus(ctx context.Context, sqldb *sql.DB, login, date string) (walked bool, logged bool, err error) {
	var w int
	err = sqldb.QueryRowContext(ctx,
		"SELECT walked FROM walk_days WHERE login = ? AND date = ?", login, date).Scan(&w)
	if err == sql.ErrNoRows {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return w == 1, true, nil
}

// currentStreak counts contiguous logged "yes" days looking back from
// today, skipping unlogged days entirely, stopping at the first logged
// "no" (README streak rules).
func currentStreak(ctx context.Context, sqldb *sql.DB, login, today string) (int, error) {
	rows, err := sqldb.QueryContext(ctx,
		"SELECT walked FROM walk_days WHERE login = ? AND date <= ? ORDER BY date DESC",
		login, today)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	streak := 0
	for rows.Next() {
		var walked int
		if err := rows.Scan(&walked); err != nil {
			return 0, err
		}
		if walked == 0 {
			break
		}
		streak++
	}
	return streak, rows.Err()
}

// milestoneMessage returns a light celebration line at streak milestones,
// or "" otherwise (README milestones: 3, 7, 14, 30, 60, 100, every 100 after).
func milestoneMessage(streak int) string {
	milestones := map[int]bool{3: true, 7: true, 14: true, 30: true, 60: true, 100: true}
	if milestones[streak] || (streak > 100 && streak%100 == 0) {
		return fmt.Sprintf("🎉 %d nights in a row!", streak)
	}
	return ""
}

// parseYesNo maps free text ("yes", "no", "we did", ...) to a walked bool,
// defaulting to true for anything not clearly negative.
func parseYesNo(text string) bool {
	for _, no := range []string{"no", "didn't", "did not"} {
		if containsFold(text, no) {
			return false
		}
	}
	return true
}

func containsFold(s, substr string) bool {
	sl, subl := []rune(s), []rune(substr)
	if len(subl) == 0 || len(sl) < len(subl) {
		return false
	}
	toLower := func(rs []rune) []rune {
		out := make([]rune, len(rs))
		for i, r := range rs {
			if r >= 'A' && r <= 'Z' {
				r += 'a' - 'A'
			}
			out[i] = r
		}
		return out
	}
	sl, subl = toLower(sl), toLower(subl)
	for i := 0; i+len(subl) <= len(sl); i++ {
		match := true
		for j := range subl {
			if sl[i+j] != subl[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func loadHistory(ctx context.Context, sqldb *sql.DB, login string) ([]views.Day, error) {
	rows, err := sqldb.QueryContext(ctx,
		"SELECT date, walked, COALESCE(note, '') FROM walk_days WHERE login = ? ORDER BY date DESC LIMIT 60",
		login)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var days []views.Day
	for rows.Next() {
		var d views.Day
		var w int
		if err := rows.Scan(&d.Date, &w, &d.Note); err != nil {
			return nil, err
		}
		d.Walked = w == 1
		d.Logged = true
		days = append(days, d)
	}
	return days, rows.Err()
}

func loadDay(ctx context.Context, sqldb *sql.DB, login, date string) (*views.Day, error) {
	d := views.Day{Date: date}
	var w int
	err := sqldb.QueryRowContext(ctx,
		"SELECT walked, COALESCE(note, '') FROM walk_days WHERE login = ? AND date = ?",
		login, date).Scan(&w, &d.Note)
	if err == sql.ErrNoRows {
		return &d, nil
	}
	if err != nil {
		return nil, err
	}
	d.Walked = w == 1
	d.Logged = true
	return &d, nil
}
