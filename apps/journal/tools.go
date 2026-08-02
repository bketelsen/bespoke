// LLM tools (ADR-0021). The journal is append-only by spec (README
// non-goals) — so tools are add, list, delete; no update exists on purpose.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/bketelsen/bespoke/pkg/auth"
	"github.com/bketelsen/bespoke/pkg/web"
)

func obj(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func registerTools(mux *http.ServeMux, sqldb *sql.DB) {
	web.Tool(mux, web.ToolDef{
		Name:        "add_entry",
		Description: "Add a journal entry (the journal is append-only; entries are never edited).",
		Schema: obj(map[string]any{
			"body": map[string]any{"type": "string", "description": "the entry text; markdown allowed"},
		}, "body"),
		Handler: func(ctx context.Context, user auth.User, args json.RawMessage) (string, error) {
			var a struct {
				Body string `json:"body"`
			}
			if err := json.Unmarshal(args, &a); err != nil || strings.TrimSpace(a.Body) == "" {
				return "", fmt.Errorf("body is required")
			}
			if _, err := sqldb.ExecContext(ctx,
				"INSERT INTO entries (login, body) VALUES (?, ?)", user.Login, strings.TrimSpace(a.Body)); err != nil {
				return "", err
			}
			return "Entry added.", nil
		},
	})

	web.Tool(mux, web.ToolDef{
		Name:        "list_entries",
		Description: "List the user's journal entries with ids and local timestamps, newest first.",
		Schema: obj(map[string]any{
			"days": map[string]any{"type": "integer", "description": "how many days back (default 7)"},
		}),
		Handler: func(ctx context.Context, user auth.User, args json.RawMessage) (string, error) {
			var a struct {
				Days int `json:"days"`
			}
			json.Unmarshal(args, &a)
			if a.Days <= 0 || a.Days > 365 {
				a.Days = 7
			}
			rows, err := sqldb.QueryContext(ctx,
				"SELECT id, created_at, body FROM entries WHERE login = ? AND created_at >= datetime('now', ?) ORDER BY created_at DESC, id DESC LIMIT 100",
				user.Login, fmt.Sprintf("-%d days", a.Days))
			if err != nil {
				return "", err
			}
			defer rows.Close()
			var b strings.Builder
			n := 0
			for rows.Next() {
				var id int64
				var created, body string
				if err := rows.Scan(&id, &created, &body); err != nil {
					return "", err
				}
				fmt.Fprintf(&b, "#%d [%s] %s\n", id, localStamp(created), body)
				n++
			}
			if n == 0 {
				return fmt.Sprintf("No entries in the last %d days.", a.Days), nil
			}
			return b.String(), rows.Err()
		},
	})

	web.Tool(mux, web.ToolDef{
		Name:        "delete_entry",
		Description: "Delete a journal entry by id — the accident hatch only. Destructive; only on an explicit user request.",
		Schema: obj(map[string]any{
			"id": map[string]any{"type": "integer"},
		}, "id"),
		Handler: func(ctx context.Context, user auth.User, args json.RawMessage) (string, error) {
			var a struct {
				ID int64 `json:"id"`
			}
			if err := json.Unmarshal(args, &a); err != nil || a.ID == 0 {
				return "", fmt.Errorf("id is required")
			}
			res, err := sqldb.ExecContext(ctx,
				"DELETE FROM entries WHERE id = ? AND login = ?", a.ID, user.Login)
			if err != nil {
				return "", err
			}
			if n, _ := res.RowsAffected(); n == 0 {
				return "", fmt.Errorf("no entry #%d", a.ID)
			}
			return fmt.Sprintf("Deleted entry #%d.", a.ID), nil
		},
	})
}
