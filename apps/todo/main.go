// todo: tasks with due dates, priorities, and one level of subtasks.
// Spec: README.md (one-shot 2/3, 2026-08-01).
package main

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/bketelsen/bespoke/apps/todo/views"
	"github.com/bketelsen/bespoke/pkg/auth"
	"github.com/bketelsen/bespoke/pkg/db"
	"github.com/bketelsen/bespoke/pkg/web"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

func main() {
	migrations, _ := fs.Sub(migrationFS, "migrations")
	sqldb, err := db.Open("todo", migrations)
	if err != nil {
		log.Fatal(err)
	}

	web.Run("todo", func(mux *http.ServeMux) {
		registerTools(mux, sqldb) // before EnableChat so chat sees them
		web.EnableChat(mux, "todo", func(ctx context.Context, user auth.User) (string, error) {
			return chatContext(ctx, sqldb, user.Login)
		})
		web.DashboardCard(mux, func(ctx context.Context, user auth.User) (templ.Component, error) {
			return dashCard(ctx, sqldb, user.Login)
		})

		// Live tasks region (ADR-0022): patched on any task change.
		web.Live(mux, func(ctx context.Context, user auth.User) (templ.Component, error) {
			tasks, err := loadTasks(ctx, sqldb, user.Login, false)
			if err != nil {
				return nil, err
			}
			return views.TasksLive(tasks), nil
		})

		web.Intent(mux, "Todo", web.IntentDef{
			Name:   "create-task",
			Title:  "Create Todo",
			Prompt: "This becomes a task (low priority, no due date — edit it after if needed).",
			Handler: func(ctx context.Context, user auth.User, text string) (string, error) {
				_, err := sqldb.ExecContext(ctx,
					"INSERT INTO tasks (login, description) VALUES (?, ?)", user.Login, text)
				if err == nil {
					web.Changed(user.Login)
				}
				return "/", err
			},
		})

		mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			hideCompleted := r.URL.Query().Get("completed") == "hidden"
			tasks, err := loadTasks(r.Context(), sqldb, user.Login, hideCompleted)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			views.Home(user, tasks, hideCompleted, r.URL.Query().Get("did")).Render(r.Context(), w)
		})

		mux.HandleFunc("POST /tasks", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			if err := createTask(r, sqldb, user.Login, 0); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			web.Changed(user.Login)
			http.Redirect(w, r, "/", http.StatusSeeOther)
		})

		mux.HandleFunc("POST /tasks/{id}/sub", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			var parent int64
			var isSub int
			err := sqldb.QueryRowContext(r.Context(),
				"SELECT id, parent_id IS NOT NULL FROM tasks WHERE id = ? AND login = ?",
				r.PathValue("id"), user.Login).Scan(&parent, &isSub)
			if err != nil || isSub == 1 { // one level deep only (README rules)
				http.Error(w, "invalid parent", http.StatusBadRequest)
				return
			}
			if err := createTask(r, sqldb, user.Login, parent); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			web.Changed(user.Login)
			http.Redirect(w, r, "/", http.StatusSeeOther)
		})

		mux.HandleFunc("POST /tasks/{id}/toggle", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			completed, err := toggleTask(r.Context(), sqldb, user.Login, r.PathValue("id"))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			web.Changed(user.Login)
			dest := "/"
			if completed != "" {
				// Event → intent (ADR-0018): the view offers "Journal it?".
				dest = "/?did=" + url.QueryEscape(completed)
			}
			http.Redirect(w, r, dest, http.StatusSeeOther)
		})

		mux.HandleFunc("GET /tasks/{id}/edit", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			var t views.Task
			err := sqldb.QueryRowContext(r.Context(),
				"SELECT id, description, COALESCE(due,''), priority FROM tasks WHERE id = ? AND login = ?",
				r.PathValue("id"), user.Login).Scan(&t.ID, &t.Description, &t.Due, &t.Priority)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			views.Edit(user, t).Render(r.Context(), w)
		})

		mux.HandleFunc("POST /tasks/{id}/edit", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			desc := strings.TrimSpace(r.FormValue("description"))
			if desc == "" {
				http.Error(w, "description required", http.StatusBadRequest)
				return
			}
			priority := r.FormValue("priority")
			if priority != "M" && priority != "H" {
				priority = "L"
			}
			due := strings.TrimSpace(r.FormValue("due"))
			if due != "" {
				if _, err := time.Parse("2006-01-02", due); err != nil {
					http.Error(w, "bad due date", http.StatusBadRequest)
					return
				}
			}
			if _, err := sqldb.ExecContext(r.Context(),
				"UPDATE tasks SET description = ?, due = NULLIF(?, ''), priority = ? WHERE id = ? AND login = ?",
				desc, due, priority, r.PathValue("id"), user.Login); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			web.Changed(user.Login)
			http.Redirect(w, r, "/", http.StatusSeeOther)
		})

		mux.HandleFunc("POST /tasks/{id}/delete", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			if _, err := sqldb.ExecContext(r.Context(),
				"DELETE FROM tasks WHERE login = ? AND (id = ? OR parent_id = ?)",
				user.Login, r.PathValue("id"), r.PathValue("id")); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			web.Changed(user.Login)
			http.Redirect(w, r, "/", http.StatusSeeOther)
		})
	})
}

func createTask(r *http.Request, sqldb *sql.DB, login string, parent int64) error {
	desc := strings.TrimSpace(r.FormValue("description"))
	if desc == "" {
		return fmt.Errorf("description required")
	}
	priority := r.FormValue("priority")
	if priority != "M" && priority != "H" {
		priority = "L" // default per spec
	}
	due := strings.TrimSpace(r.FormValue("due"))
	if due != "" {
		if _, err := time.Parse("2006-01-02", due); err != nil {
			return fmt.Errorf("bad due date")
		}
	}
	var parentID any
	if parent != 0 {
		parentID = parent
	}
	_, err := sqldb.ExecContext(r.Context(),
		"INSERT INTO tasks (login, parent_id, description, due, priority) VALUES (?, ?, ?, NULLIF(?, ''), ?)",
		login, parentID, desc, due, priority)
	return err
}

func doneSQL(done int) string {
	if done == 1 {
		return "done = 1, completed_at = datetime('now')"
	}
	return "done = 0, completed_at = NULL"
}

// toggleTask flips done and applies both cascades (README rules): parents
// push their state down to subtasks; subtasks pull the parent up (all done)
// or back open (any reopened). Returns the task's description when the
// toggle COMPLETED it (fuel for the journal follow-up), else "".
func toggleTask(ctx context.Context, sqldb *sql.DB, login, id string) (string, error) {
	tx, err := sqldb.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var taskID int64
	var parent sql.NullInt64
	var done int
	var desc string
	if err := tx.QueryRowContext(ctx,
		"SELECT id, parent_id, done, description FROM tasks WHERE id = ? AND login = ?", id, login).
		Scan(&taskID, &parent, &done, &desc); err != nil {
		return "", err
	}
	newDone := 1 - done

	if _, err := tx.ExecContext(ctx,
		"UPDATE tasks SET "+doneSQL(newDone)+" WHERE id = ? AND login = ?", taskID, login); err != nil {
		return "", err
	}

	if !parent.Valid {
		// Parent toggled directly: cascade down to all subtasks.
		if _, err := tx.ExecContext(ctx,
			"UPDATE tasks SET "+doneSQL(newDone)+" WHERE parent_id = ? AND login = ?", taskID, login); err != nil {
			return "", err
		}
	} else {
		// Subtask toggled: cascade up.
		var open int
		if err := tx.QueryRowContext(ctx,
			"SELECT count(*) FROM tasks WHERE parent_id = ? AND login = ? AND done = 0",
			parent.Int64, login).Scan(&open); err != nil {
			return "", err
		}
		parentDone := 0
		if open == 0 {
			parentDone = 1
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE tasks SET "+doneSQL(parentDone)+" WHERE id = ? AND login = ?", parent.Int64, login); err != nil {
			return "", err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	if newDone == 1 {
		return desc, nil
	}
	return "", nil
}

func loadTasks(ctx context.Context, sqldb *sql.DB, login string, hideCompleted bool) ([]views.Task, error) {
	rows, err := sqldb.QueryContext(ctx, `
		SELECT id, parent_id, description, COALESCE(due,''), priority, done
		FROM tasks WHERE login = ?
		ORDER BY done, CASE WHEN due IS NULL OR due = '' THEN 1 ELSE 0 END, due,
			CASE priority WHEN 'H' THEN 0 WHEN 'M' THEN 1 ELSE 2 END, created_at, id`, login)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var parents []views.Task
	subs := map[int64][]views.Task{}
	for rows.Next() {
		var t views.Task
		var parent sql.NullInt64
		var done int
		if err := rows.Scan(&t.ID, &parent, &t.Description, &t.Due, &t.Priority, &done); err != nil {
			return nil, err
		}
		t.Done = done == 1
		t.DueLabel, t.Overdue = dueLabel(t.Due)
		if parent.Valid {
			subs[parent.Int64] = append(subs[parent.Int64], t)
		} else {
			parents = append(parents, t)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var out []views.Task
	for _, p := range parents {
		p.Subtasks = subs[p.ID]
		if hideCompleted {
			if p.Done {
				continue
			}
			var open []views.Task
			for _, s := range p.Subtasks {
				if !s.Done {
					open = append(open, s)
				}
			}
			p.Subtasks = open
		}
		out = append(out, p)
	}
	return out, nil
}

// dueLabel humanizes a YYYY-MM-DD due date against local today.
func dueLabel(due string) (string, bool) {
	if due == "" {
		return "", false
	}
	d, err := time.ParseInLocation("2006-01-02", due, time.Local)
	if err != nil {
		return due, false
	}
	today, _ := time.ParseInLocation("2006-01-02", time.Now().Format("2006-01-02"), time.Local)
	switch days := int(d.Sub(today).Hours() / 24); {
	case days < 0:
		return "overdue · " + d.Format("Jan 2"), true
	case days == 0:
		return "today", false
	case days == 1:
		return "tomorrow", false
	case days < 7:
		return d.Format("Monday"), false
	default:
		return d.Format("Jan 2"), false
	}
}

func chatContext(ctx context.Context, sqldb *sql.DB, login string) (string, error) {
	rows, err := sqldb.QueryContext(ctx, `
		SELECT description, COALESCE(due,''), priority, done, parent_id IS NOT NULL
		FROM tasks WHERE login = ? ORDER BY done, due, id`, login)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var b strings.Builder
	b.WriteString("Tasks (dues are local dates; today is " + time.Now().Format("2006-01-02") + "):\n")
	n := 0
	for rows.Next() {
		var desc, due, prio string
		var done, isSub bool
		if err := rows.Scan(&desc, &due, &prio, &done, &isSub); err != nil {
			return "", err
		}
		state := "open"
		if done {
			state = "done"
		}
		indent := ""
		if isSub {
			indent = "  subtask: "
		}
		dueNote := ""
		if due != "" {
			dueNote = ", due " + due
		}
		fmt.Fprintf(&b, "%s[%s] %s (priority %s%s)\n", indent, state, desc, prio, dueNote)
		n++
	}
	if n == 0 {
		b.WriteString("(no tasks yet)\n")
	}
	return b.String(), rows.Err()
}

// dashCard: Due Today (incl. overdue) / Due This Week / High Priority —
// open top-level tasks, deduplicated in that order (README).
func dashCard(ctx context.Context, sqldb *sql.DB, login string) (templ.Component, error) {
	today := time.Now().Format("2006-01-02")
	week := time.Now().AddDate(0, 0, 7).Format("2006-01-02")

	rows, err := sqldb.QueryContext(ctx, `
		SELECT description, COALESCE(due,''), priority FROM tasks
		WHERE login = ? AND done = 0 AND parent_id IS NULL
		ORDER BY CASE WHEN due IS NULL OR due = '' THEN 1 ELSE 0 END, due,
			CASE priority WHEN 'H' THEN 0 WHEN 'M' THEN 1 ELSE 2 END, id`, login)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dueToday, dueWeek, highPri []views.CardRow
	for rows.Next() {
		var desc, due, prio string
		if err := rows.Scan(&desc, &due, &prio); err != nil {
			return nil, err
		}
		row := views.CardRow{Description: desc, Priority: prio}
		row.DueLabel, row.Overdue = dueLabel(due)
		// Dedup by construction: each task lands in exactly one list,
		// first match wins in spec order (README).
		switch {
		case due != "" && due <= today:
			dueToday = append(dueToday, row)
		case due != "" && due <= week:
			dueWeek = append(dueWeek, row)
		case prio == "H":
			highPri = append(highPri, row)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return views.DashCard(dueToday, dueWeek, highPri), nil
}
