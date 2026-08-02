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

func schema(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func registerTools(mux *http.ServeMux, db *sql.DB, startRun func(ctx context.Context, login, idea string) (string, error)) {
	web.Tool(mux, web.ToolDef{
		Name:        "list_runs",
		Description: "List the user's app build runs with status.",
		Schema:      schema(map[string]any{}),
		Handler: func(ctx context.Context, user auth.User, args json.RawMessage) (string, error) {
			return chatContext(ctx, db, user.Login)
		}})
	web.Tool(mux, web.ToolDef{
		Name:        "get_run",
		Description: "Get one build run: spec, status, and recent build/deploy events.",
		Schema:      schema(map[string]any{"id": map[string]any{"type": "string"}}, "id"),
		Handler: func(ctx context.Context, user auth.User, args json.RawMessage) (string, error) {
			var a struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(args, &a); err != nil || a.ID == "" {
				return "", fmt.Errorf("id is required")
			}
			d, err := loadRun(ctx, db, user.Login, a.ID)
			if err != nil {
				return "", fmt.Errorf("no run %s", a.ID)
			}
			var b strings.Builder
			fmt.Fprintf(&b, "Run %s [%s] slug=%s idea: %s\n", d.ID, d.Status, d.Slug, d.Idea)
			if d.Detail != "" {
				fmt.Fprintf(&b, "Detail: %s\n", d.Detail)
			}
			if d.Spec != "" {
				fmt.Fprintf(&b, "\nSpec:\n%s\n", d.Spec)
			}
			events := d.Events
			if len(events) > 20 {
				events = events[len(events)-20:]
			}
			for _, e := range events {
				fmt.Fprintf(&b, "- %s [%s] %s\n", e.TS, e.Kind, e.Body)
			}
			return b.String(), nil
		}})
	web.Tool(mux, web.ToolDef{
		Name: "start_build",
		Description: "Start designing a new app from an idea. Begins an interview the user " +
			"finishes in the Builder app — only on an explicit user request.",
		Schema: schema(map[string]any{"idea": map[string]any{"type": "string"}}, "idea"),
		Handler: func(ctx context.Context, user auth.User, args json.RawMessage) (string, error) {
			var a struct {
				Idea string `json:"idea"`
			}
			if err := json.Unmarshal(args, &a); err != nil || strings.TrimSpace(a.Idea) == "" {
				return "", fmt.Errorf("idea is required")
			}
			id, err := startRun(ctx, user.Login, strings.TrimSpace(a.Idea))
			if err != nil {
				return "", err
			}
			return "Started run " + id + ". The user continues the design interview at /runs/" + id + " in the Builder app.", nil
		}})
}
