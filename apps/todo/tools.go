// LLM tools (ADR-0021): full CRUD on tasks, scoped to the calling user.
// These serve agentic chat and the platform MCP surface alike.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

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
		Name:        "list_tasks",
		Description: "List the user's tasks with ids, status, due dates, priorities, and subtasks.",
		Schema: obj(map[string]any{
			"include_done": map[string]any{"type": "boolean", "description": "include completed tasks (default false)"},
		}),
		Handler: func(ctx context.Context, user auth.User, args json.RawMessage) (string, error) {
			var a struct {
				IncludeDone bool `json:"include_done"`
			}
			if len(args) > 0 {
				if err := json.Unmarshal(args, &a); err != nil {
					return "", fmt.Errorf("bad arguments: %w", err)
				}
			}
			tasks, err := loadTasks(ctx, sqldb, user.Login, !a.IncludeDone)
			if err != nil {
				return "", err
			}
			if len(tasks) == 0 {
				return "No tasks.", nil
			}
			var b strings.Builder
			for _, t := range tasks {
				fmt.Fprintf(&b, "#%d [%s] %s", t.ID, map[bool]string{true: "done", false: "open"}[t.Done], t.Description)
				if t.Due != "" {
					fmt.Fprintf(&b, " (due %s)", t.Due)
				}
				fmt.Fprintf(&b, " priority=%s\n", t.Priority)
				for _, s := range t.Subtasks {
					fmt.Fprintf(&b, "  #%d [%s] %s\n", s.ID, map[bool]string{true: "done", false: "open"}[s.Done], s.Description)
				}
			}
			return b.String(), nil
		},
	})

	web.Tool(mux, web.ToolDef{
		Name:        "create_task",
		Description: "Create a task. Optionally a subtask of a top-level task via parent_id.",
		Schema: obj(map[string]any{
			"description": map[string]any{"type": "string"},
			"due":         map[string]any{"type": "string", "description": "YYYY-MM-DD, optional"},
			"priority":    map[string]any{"type": "string", "enum": []string{"L", "M", "H"}, "description": "default L"},
			"parent_id":   map[string]any{"type": "integer", "description": "make this a subtask of the given top-level task"},
		}, "description"),
		Handler: func(ctx context.Context, user auth.User, args json.RawMessage) (string, error) {
			var a struct {
				Description string `json:"description"`
				Due         string `json:"due"`
				Priority    string `json:"priority"`
				ParentID    int64  `json:"parent_id"`
			}
			if err := json.Unmarshal(args, &a); err != nil || strings.TrimSpace(a.Description) == "" {
				return "", fmt.Errorf("description is required")
			}
			if a.Priority != "M" && a.Priority != "H" {
				a.Priority = "L"
			}
			if a.Due != "" {
				if _, err := time.Parse("2006-01-02", a.Due); err != nil {
					return "", fmt.Errorf("due must be YYYY-MM-DD")
				}
			}
			var parent any
			if a.ParentID != 0 {
				var isSub int
				if err := sqldb.QueryRowContext(ctx,
					"SELECT parent_id IS NOT NULL FROM tasks WHERE id = ? AND login = ?",
					a.ParentID, user.Login).Scan(&isSub); err != nil || isSub == 1 {
					return "", fmt.Errorf("parent_id must be one of the user's top-level tasks")
				}
				parent = a.ParentID
			}
			res, err := sqldb.ExecContext(ctx,
				"INSERT INTO tasks (login, parent_id, description, due, priority) VALUES (?, ?, ?, NULLIF(?, ''), ?)",
				user.Login, parent, strings.TrimSpace(a.Description), a.Due, a.Priority)
			if err != nil {
				return "", err
			}
			id, _ := res.LastInsertId()
			web.Changed(user.Login)
			return fmt.Sprintf("Created task #%d: %s", id, a.Description), nil
		},
	})

	web.Tool(mux, web.ToolDef{
		Name:        "update_task",
		Description: "Update a task's description, due date, and/or priority.",
		Schema: obj(map[string]any{
			"id":          map[string]any{"type": "integer"},
			"description": map[string]any{"type": "string"},
			"due":         map[string]any{"type": "string", "description": "YYYY-MM-DD, or empty string to clear"},
			"priority":    map[string]any{"type": "string", "enum": []string{"L", "M", "H"}},
		}, "id"),
		Handler: func(ctx context.Context, user auth.User, args json.RawMessage) (string, error) {
			var a struct {
				ID          int64   `json:"id"`
				Description *string `json:"description"`
				Due         *string `json:"due"`
				Priority    *string `json:"priority"`
			}
			if err := json.Unmarshal(args, &a); err != nil || a.ID == 0 {
				return "", fmt.Errorf("id is required")
			}
			sets, vals := []string{}, []any{}
			if a.Description != nil && strings.TrimSpace(*a.Description) != "" {
				sets, vals = append(sets, "description = ?"), append(vals, strings.TrimSpace(*a.Description))
			}
			if a.Due != nil {
				if *a.Due != "" {
					if _, err := time.Parse("2006-01-02", *a.Due); err != nil {
						return "", fmt.Errorf("due must be YYYY-MM-DD or empty to clear")
					}
				}
				sets, vals = append(sets, "due = NULLIF(?, '')"), append(vals, *a.Due)
			}
			if a.Priority != nil && (*a.Priority == "L" || *a.Priority == "M" || *a.Priority == "H") {
				sets, vals = append(sets, "priority = ?"), append(vals, *a.Priority)
			}
			if len(sets) == 0 {
				return "", fmt.Errorf("nothing to update")
			}
			vals = append(vals, a.ID, user.Login)
			res, err := sqldb.ExecContext(ctx,
				"UPDATE tasks SET "+strings.Join(sets, ", ")+" WHERE id = ? AND login = ?", vals...)
			if err != nil {
				return "", err
			}
			if n, _ := res.RowsAffected(); n == 0 {
				return "", fmt.Errorf("no task #%d", a.ID)
			}
			web.Changed(user.Login)
			return fmt.Sprintf("Updated task #%d.", a.ID), nil
		},
	})

	web.Tool(mux, web.ToolDef{
		Name:        "set_task_done",
		Description: "Complete or reopen a task. Cascades: completing all subtasks completes the parent; toggling a parent applies to its subtasks.",
		Schema: obj(map[string]any{
			"id":   map[string]any{"type": "integer"},
			"done": map[string]any{"type": "boolean"},
		}, "id", "done"),
		Handler: func(ctx context.Context, user auth.User, args json.RawMessage) (string, error) {
			var a struct {
				ID   int64 `json:"id"`
				Done bool  `json:"done"`
			}
			if err := json.Unmarshal(args, &a); err != nil || a.ID == 0 {
				return "", fmt.Errorf("id and done are required")
			}
			var current int
			if err := sqldb.QueryRowContext(ctx,
				"SELECT done FROM tasks WHERE id = ? AND login = ?", a.ID, user.Login).Scan(&current); err != nil {
				return "", fmt.Errorf("no task #%d", a.ID)
			}
			want := 0
			if a.Done {
				want = 1
			}
			if current == want {
				return fmt.Sprintf("Task #%d already in that state.", a.ID), nil
			}
			desc, err := toggleTask(ctx, sqldb, user.Login, fmt.Sprint(a.ID))
			if err != nil {
				return "", err
			}
			web.Changed(user.Login)
			if desc != "" {
				return fmt.Sprintf("Completed task #%d: %s", a.ID, desc), nil
			}
			return fmt.Sprintf("Reopened task #%d.", a.ID), nil
		},
	})

	web.Tool(mux, web.ToolDef{
		Name:        "delete_task",
		Description: "Delete a task (and its subtasks). Destructive — only on an explicit user request.",
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
				"DELETE FROM tasks WHERE login = ? AND (id = ? OR parent_id = ?)",
				user.Login, a.ID, a.ID)
			if err != nil {
				return "", err
			}
			if n, _ := res.RowsAffected(); n == 0 {
				return "", fmt.Errorf("no task #%d", a.ID)
			}
			web.Changed(user.Login)
			return fmt.Sprintf("Deleted task #%d.", a.ID), nil
		},
	})
}
