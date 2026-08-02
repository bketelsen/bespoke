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

func schema(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func registerTools(mux *http.ServeMux, db *sql.DB) {
	web.Tool(mux, web.ToolDef{Name: "list_projects", Description: "List the user's 3D print projects and print history.", Schema: schema(map[string]any{}), Handler: func(ctx context.Context, user auth.User, args json.RawMessage) (string, error) {
		return chatContext(ctx, db, user.Login)
	}})
	web.Tool(mux, web.ToolDef{Name: "create_project", Description: "Create a 3D print project. A project without history means the user wants to print it.", Schema: schema(map[string]any{
		"name": map[string]any{"type": "string"}, "source_url": map[string]any{"type": "string", "description": "optional http(s) URL"}, "notes": map[string]any{"type": "string"},
	}, "name"), Handler: func(ctx context.Context, user auth.User, args json.RawMessage) (string, error) {
		var a struct {
			Name      string `json:"name"`
			SourceURL string `json:"source_url"`
			Notes     string `json:"notes"`
		}
		if err := json.Unmarshal(args, &a); err != nil || strings.TrimSpace(a.Name) == "" {
			return "", fmt.Errorf("name is required")
		}
		if !validURL(a.SourceURL) {
			return "", fmt.Errorf("source_url must start with http:// or https://")
		}
		res, err := db.ExecContext(ctx, "INSERT INTO projects(login,name,source_url,notes) VALUES(?,?,?,?)", user.Login, strings.TrimSpace(a.Name), strings.TrimSpace(a.SourceURL), strings.TrimSpace(a.Notes))
		if err != nil {
			return "", err
		}
		id, _ := res.LastInsertId()
		web.Changed(user.Login)
		return fmt.Sprintf("Created print project #%d: %s", id, a.Name), nil
	}})
	web.Tool(mux, web.ToolDef{Name: "update_project", Description: "Update a print project's name, source URL, or notes.", Schema: schema(map[string]any{
		"id": map[string]any{"type": "integer"}, "name": map[string]any{"type": "string"}, "source_url": map[string]any{"type": "string"}, "notes": map[string]any{"type": "string"},
	}, "id"), Handler: func(ctx context.Context, user auth.User, args json.RawMessage) (string, error) {
		var a struct {
			ID        int64   `json:"id"`
			Name      *string `json:"name"`
			SourceURL *string `json:"source_url"`
			Notes     *string `json:"notes"`
		}
		if err := json.Unmarshal(args, &a); err != nil || a.ID == 0 {
			return "", fmt.Errorf("id is required")
		}
		sets, vals := []string{}, []any{}
		if a.Name != nil {
			name := strings.TrimSpace(*a.Name)
			if name == "" {
				return "", fmt.Errorf("name cannot be empty")
			}
			sets = append(sets, "name=?")
			vals = append(vals, name)
		}
		if a.SourceURL != nil {
			sourceURL := strings.TrimSpace(*a.SourceURL)
			if !validURL(sourceURL) {
				return "", fmt.Errorf("source_url must start with http:// or https://")
			}
			sets = append(sets, "source_url=?")
			vals = append(vals, sourceURL)
		}
		if a.Notes != nil {
			sets = append(sets, "notes=?")
			vals = append(vals, strings.TrimSpace(*a.Notes))
		}
		if len(sets) == 0 {
			return "", fmt.Errorf("nothing to update")
		}
		vals = append(vals, a.ID, user.Login)
		res, err := db.ExecContext(ctx, "UPDATE projects SET "+strings.Join(sets, ",")+" WHERE id=? AND login=?", vals...)
		if err != nil {
			return "", err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return "", fmt.Errorf("no project #%d", a.ID)
		}
		web.Changed(user.Login)
		return fmt.Sprintf("Updated print project #%d.", a.ID), nil
	}})
	web.Tool(mux, web.ToolDef{Name: "add_print", Description: "Add a print-history record to a project. Images can only be added through the app form.", Schema: schema(map[string]any{
		"project_id": map[string]any{"type": "integer"}, "printed_on": map[string]any{"type": "string", "description": "YYYY-MM-DD; defaults to today"}, "notes": map[string]any{"type": "string"},
	}, "project_id"), Handler: func(ctx context.Context, user auth.User, args json.RawMessage) (string, error) {
		var a struct {
			ProjectID int64  `json:"project_id"`
			PrintedOn string `json:"printed_on"`
			Notes     string `json:"notes"`
		}
		if err := json.Unmarshal(args, &a); err != nil || a.ProjectID == 0 {
			return "", fmt.Errorf("project_id is required")
		}
		if a.PrintedOn == "" {
			a.PrintedOn = time.Now().Format("2006-01-02")
		}
		if _, err := time.Parse("2006-01-02", a.PrintedOn); err != nil {
			return "", fmt.Errorf("printed_on must be YYYY-MM-DD")
		}
		res, err := db.ExecContext(ctx, `INSERT INTO print_history(project_id,login,printed_on,notes) SELECT id,login,?,? FROM projects WHERE id=? AND login=?`, a.PrintedOn, strings.TrimSpace(a.Notes), a.ProjectID, user.Login)
		if err != nil {
			return "", err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return "", fmt.Errorf("no project #%d", a.ProjectID)
		}
		web.Changed(user.Login)
		return fmt.Sprintf("Added a %s print to project #%d.", a.PrintedOn, a.ProjectID), nil
	}})
	web.Tool(mux, web.ToolDef{Name: "delete_project", Description: "Delete a project and all its print history. Destructive — only on an explicit user request.", Schema: schema(map[string]any{"id": map[string]any{"type": "integer"}}, "id"), Handler: func(ctx context.Context, user auth.User, args json.RawMessage) (string, error) {
		var a struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(args, &a); err != nil || a.ID == 0 {
			return "", fmt.Errorf("id is required")
		}
		res, err := db.ExecContext(ctx, "DELETE FROM projects WHERE id=? AND login=?", a.ID, user.Login)
		if err != nil {
			return "", err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return "", fmt.Errorf("no project #%d", a.ID)
		}
		web.Changed(user.Login)
		return fmt.Sprintf("Deleted print project #%d.", a.ID), nil
	}})
}
