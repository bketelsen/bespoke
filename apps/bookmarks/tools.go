// LLM tools (ADR-0021): add and list bookmarks, scoped to the calling user.
// These serve agentic chat and the platform MCP surface alike.
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
		Name:        "add_bookmark",
		Description: "Save a bookmark by URL. Title is auto-fetched from the page.",
		Schema: obj(map[string]any{
			"url":  map[string]any{"type": "string", "description": "the URL to save"},
			"tags": map[string]any{"type": "string", "description": "optional comma-separated tags"},
		}, "url"),
		Handler: func(ctx context.Context, user auth.User, args json.RawMessage) (string, error) {
			var a struct {
				URL  string `json:"url"`
				Tags string `json:"tags"`
			}
			if err := json.Unmarshal(args, &a); err != nil || strings.TrimSpace(a.URL) == "" {
				return "", fmt.Errorf("url is required")
			}
			id, err := addBookmark(ctx, sqldb, user.Login, strings.TrimSpace(a.URL), "", a.Tags)
			if err != nil {
				return "", err
			}
			web.Changed(user.Login)
			return fmt.Sprintf("Saved bookmark #%d.", id), nil
		},
	})

	web.Tool(mux, web.ToolDef{
		Name:        "list_bookmarks",
		Description: "List the user's saved bookmarks, optionally filtered by tag.",
		Schema: obj(map[string]any{
			"tag": map[string]any{"type": "string", "description": "optional tag to filter by"},
		}),
		Handler: func(ctx context.Context, user auth.User, args json.RawMessage) (string, error) {
			var a struct {
				Tag string `json:"tag"`
			}
			if len(args) > 0 {
				if err := json.Unmarshal(args, &a); err != nil {
					return "", fmt.Errorf("bad arguments: %w", err)
				}
			}
			bms, err := loadBookmarks(ctx, sqldb, user.Login, strings.ToLower(strings.TrimSpace(a.Tag)))
			if err != nil {
				return "", err
			}
			if len(bms) == 0 {
				return "No bookmarks.", nil
			}
			var b strings.Builder
			for _, bm := range bms {
				fmt.Fprintf(&b, "#%d %s — %s", bm.ID, bm.Title, bm.URL)
				if len(bm.Tags) > 0 {
					fmt.Fprintf(&b, " [%s]", strings.Join(bm.Tags, ", "))
				}
				b.WriteString("\n")
			}
			return b.String(), nil
		},
	})
}
