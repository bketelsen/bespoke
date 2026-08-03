// LLM tools (ADR-0021): create/search/get pages, scoped to the calling user.
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
		Name:        "create_page",
		Description: "Create a wiki page with a title, markdown body, and optional tags. Fails if the title already exists.",
		Schema: obj(map[string]any{
			"title": map[string]any{"type": "string"},
			"body":  map[string]any{"type": "string"},
			"tags":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		}, "title"),
		Handler: func(ctx context.Context, user auth.User, args json.RawMessage) (string, error) {
			var a struct {
				Title string   `json:"title"`
				Body  string   `json:"body"`
				Tags  []string `json:"tags"`
			}
			if err := json.Unmarshal(args, &a); err != nil || strings.TrimSpace(a.Title) == "" {
				return "", fmt.Errorf("title is required")
			}
			id, err := upsertPage(ctx, sqldb, user.Login, strings.TrimSpace(a.Title), a.Body, a.Tags)
			if err != nil {
				return "", err
			}
			web.Changed(user.Login)
			return fmt.Sprintf("Created page #%d: %s", id, a.Title), nil
		},
	})

	web.Tool(mux, web.ToolDef{
		Name:        "search_pages",
		Description: "Search the wiki by title or body text substring. Returns matching page ids, titles, and tags.",
		Schema: obj(map[string]any{
			"query": map[string]any{"type": "string"},
		}, "query"),
		Handler: func(ctx context.Context, user auth.User, args json.RawMessage) (string, error) {
			var a struct {
				Query string `json:"query"`
			}
			if err := json.Unmarshal(args, &a); err != nil || strings.TrimSpace(a.Query) == "" {
				return "", fmt.Errorf("query is required")
			}
			pages, err := loadPages(ctx, sqldb, user.Login, false, a.Query)
			if err != nil {
				return "", err
			}
			if len(pages) == 0 {
				return "No matching pages.", nil
			}
			var b strings.Builder
			for _, p := range pages {
				fmt.Fprintf(&b, "#%d %s", p.ID, p.Title)
				if len(p.Tags) > 0 {
					fmt.Fprintf(&b, " [%s]", strings.Join(p.Tags, ", "))
				}
				b.WriteString("\n")
			}
			return b.String(), nil
		},
	})

	web.Tool(mux, web.ToolDef{
		Name:        "get_page",
		Description: "Fetch a wiki page's full body and tags by exact title.",
		Schema: obj(map[string]any{
			"title": map[string]any{"type": "string"},
		}, "title"),
		Handler: func(ctx context.Context, user auth.User, args json.RawMessage) (string, error) {
			var a struct {
				Title string `json:"title"`
			}
			if err := json.Unmarshal(args, &a); err != nil || strings.TrimSpace(a.Title) == "" {
				return "", fmt.Errorf("title is required")
			}
			var id int64
			err := sqldb.QueryRowContext(ctx,
				"SELECT id FROM pages WHERE login = ? AND title = ?", user.Login, strings.TrimSpace(a.Title)).Scan(&id)
			if err == sql.ErrNoRows {
				return "", fmt.Errorf("no page titled %q", a.Title)
			} else if err != nil {
				return "", err
			}
			page, err := loadPage(ctx, sqldb, user.Login, id)
			if err != nil {
				return "", err
			}
			var b strings.Builder
			fmt.Fprintf(&b, "#%d %s\n", page.ID, page.Title)
			if len(page.Tags) > 0 {
				fmt.Fprintf(&b, "Tags: %s\n", strings.Join(page.Tags, ", "))
			}
			b.WriteString("\n")
			b.WriteString(page.Body)
			return b.String(), nil
		},
	})
}
