package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/bketelsen/bespoke/apps/builder/views"
)

const sqliteTime = "2006-01-02 15:04:05"

func localStamp(created string) string {
	t, err := time.ParseInLocation(sqliteTime, created, time.UTC)
	if err != nil {
		return created
	}
	return t.Local().Format("Mon Jan 2 2006, 3:04 PM")
}

func loadRuns(ctx context.Context, db *sql.DB, login string) ([]views.Run, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, idea, slug, status, detail, updated_at
		FROM runs WHERE login = ? ORDER BY created_at DESC`, login)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []views.Run
	for rows.Next() {
		var r views.Run
		var updated string
		if err := rows.Scan(&r.ID, &r.Idea, &r.Slug, &r.Status, &r.Detail, &updated); err != nil {
			return nil, err
		}
		r.Updated = localStamp(updated)
		out = append(out, r)
	}
	return out, rows.Err()
}

func loadRun(ctx context.Context, db *sql.DB, login, id string) (views.RunDetail, error) {
	var d views.RunDetail
	var updated string
	err := db.QueryRowContext(ctx, `SELECT id, idea, slug, spec, status, detail, updated_at
		FROM runs WHERE id = ? AND login = ?`, id, login).
		Scan(&d.ID, &d.Idea, &d.Slug, &d.Spec, &d.Status, &d.Detail, &updated)
	if err != nil {
		return d, err
	}
	d.Updated = localStamp(updated)

	msgs, err := db.QueryContext(ctx,
		"SELECT role, body FROM messages WHERE run_id = ? ORDER BY id", id)
	if err != nil {
		return d, err
	}
	defer msgs.Close()
	for msgs.Next() {
		var m views.Message
		if err := msgs.Scan(&m.Role, &m.Body); err != nil {
			return d, err
		}
		d.Messages = append(d.Messages, m)
	}
	if err := msgs.Err(); err != nil {
		return d, err
	}

	evs, err := db.QueryContext(ctx,
		"SELECT ts, kind, body FROM run_events WHERE run_id = ? ORDER BY seq", id)
	if err != nil {
		return d, err
	}
	defer evs.Close()
	for evs.Next() {
		var e views.Event
		if err := evs.Scan(&e.TS, &e.Kind, &e.Body); err != nil {
			return d, err
		}
		if t, err := time.Parse(time.RFC3339, e.TS); err == nil {
			e.TS = t.Local().Format("3:04:05 PM")
		}
		d.Events = append(d.Events, e)
	}
	return d, evs.Err()
}

// storeReply saves an assistant turn. When the reply is the finished spec
// (protocol in interview.go) the run is promoted to 'ready'; a malformed or
// taken slug bounces the interview back one turn instead of failing the run.
func storeReply(ctx context.Context, db *sql.DB, runID, login, reply string) error {
	slug, spec, done := parseSpec(reply)
	if done {
		if err := validSlug(slug); err != nil {
			bounce := fmt.Sprintf("That slug won't work (%v) — pick another short kebab-case name?", err)
			_, ierr := db.ExecContext(ctx,
				"INSERT INTO messages (run_id, role, body) VALUES (?,?,?)", runID, "assistant", bounce)
			return ierr
		}
		if _, err := db.ExecContext(ctx, `UPDATE runs SET slug = ?, spec = ?, status = 'ready',
			updated_at = datetime('now') WHERE id = ?`, slug, spec, runID); err != nil {
			return err
		}
		_, err := db.ExecContext(ctx, "INSERT INTO messages (run_id, role, body) VALUES (?,?,?)",
			runID, "assistant", "Spec is ready below — approve it and I'll build "+slug+".")
		return err
	}
	_, err := db.ExecContext(ctx,
		"INSERT INTO messages (run_id, role, body) VALUES (?,?,?)", runID, "assistant", reply)
	return err
}

// chatContext feeds the in-app chat (ADR-0015): every run with status and
// recent progress, so "how's my app doing?" has an answer.
func chatContext(ctx context.Context, db *sql.DB, login string) (string, error) {
	runs, err := loadRuns(ctx, db, login)
	if err != nil {
		return "", err
	}
	if len(runs) == 0 {
		return "No build runs yet. The user starts one by typing an app idea on the Builder home page.", nil
	}
	var b strings.Builder
	for _, r := range runs {
		fmt.Fprintf(&b, "Run %s [%s] idea: %s", r.ID, r.Status, r.Idea)
		if r.Slug != "" {
			fmt.Fprintf(&b, " (app slug: %s)", r.Slug)
		}
		if r.Detail != "" {
			fmt.Fprintf(&b, " — %s", r.Detail)
		}
		fmt.Fprintf(&b, " (updated %s)\n", r.Updated)
	}
	return b.String(), nil
}
