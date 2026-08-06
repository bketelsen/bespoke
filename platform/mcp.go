// The MCP surface (ADR-0021, converging the ADR-0018 registry's LLM face):
// one Streamable HTTP endpoint at the apex /mcp exposing every app's tools
// to external LLM clients, namespaced <slug>_<tool>, executing as the
// connected tailnet user. Mounted behind auth like every page — Caddy
// stamps identity on the way in.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/bketelsen/bespoke/internal/manifest"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"cmp"
)

type appTool struct {
	Slug        string
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Schema      map[string]any `json:"schema"`
	URL         string
	Automation  string `json:"automation"`
}

var (
	toolCacheMu sync.Mutex
	toolCache   []appTool
	toolCacheAt time.Time
)

// allAppTools fetches every app's /_tools listing (10s cache; misses are
// skipped — an app without tools just contributes none).
func allAppTools(ctx context.Context, root string) []appTool {
	toolCacheMu.Lock()
	defer toolCacheMu.Unlock()
	if time.Since(toolCacheAt) < 10*time.Second {
		return toolCache
	}
	host := cmp.Or(os.Getenv("BESPOKE_BIND_IP"), "127.0.0.1")
	apps, _, err := manifest.LoadAll(root)
	if err != nil {
		return toolCache
	}
	var out []appTool
	for _, app := range apps {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			fmt.Sprintf("http://%s:%d/_tools", host, app.Port), nil)
		if err != nil {
			continue
		}
		// The listing is definitions only; any identity satisfies auth.
		req.Header.Set("Tailscale-User-Login", "platform@internal")
		resp, err := contextClient.Do(req)
		if err != nil {
			continue
		}
		var defs []appTool
		err = json.NewDecoder(io.LimitReader(resp.Body, 256<<10)).Decode(&defs)
		resp.Body.Close()
		if err != nil {
			continue
		}
		for _, d := range defs {
			d.Slug = app.Slug
			d.URL = fmt.Sprintf("http://%s:%d/_tools/%s", host, app.Port, d.Name)
			out = append(out, d)
		}
	}
	toolCache, toolCacheAt = out, time.Now()
	return out
}

// mcpHandler serves the platform MCP endpoint. A server is built per
// request, scoped to the caller's tailnet identity — tools execute as them.
func mcpHandler(root, domain string) http.Handler {
	return mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		login := r.Header.Get("Tailscale-User-Login")
		if login == "" {
			return nil // 400; only reachable through the identity-stamping edge anyway
		}
		s := mcp.NewServer(&mcp.Implementation{Name: "bespoke", Version: "1.0.0"}, nil)
		for _, t := range allAppTools(r.Context(), root) {
			t := t
			schema := t.Schema
			if schema == nil {
				schema = map[string]any{"type": "object"}
			}
			s.AddTool(&mcp.Tool{
				Name:        t.Slug + "_" + t.Name,
				Description: fmt.Sprintf("[%s] %s", t.Slug, t.Description),
				InputSchema: schema,
			}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				args := []byte("{}")
				if req.Params.Arguments != nil {
					args = req.Params.Arguments
				}
				hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.URL, bytes.NewReader(args))
				if err != nil {
					return nil, err
				}
				hreq.Header.Set("Content-Type", "application/json")
				hreq.Header.Set("Tailscale-User-Login", login)
				resp, err := toolClient.Do(hreq)
				if err != nil {
					return nil, err
				}
				defer resp.Body.Close()
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
				result := &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: string(body)}},
				}
				if resp.StatusCode != http.StatusOK {
					result.IsError = true
				}
				return result, nil
			})
		}
		s.AddTool(&mcp.Tool{
			Name:        "search",
			Description: "Search across all your apps' data. Returns matches grouped by app with deep links.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"q": map[string]any{"type": "string", "description": "search query"},
				},
				"required": []string{"q"},
			},
		}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var args struct {
				Q string `json:"q"`
			}
			if req.Params.Arguments != nil {
				_ = json.Unmarshal(req.Params.Arguments, &args)
			}
			apps, _, err := manifest.LoadAll(root)
			if err != nil {
				return nil, err
			}
			dev := os.Getenv("BESPOKE_DEV_USER") != ""
			out := formatSearchGroups(aggregateSearch(ctx, login, login, args.Q, dev, domain, apps))
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: out}}}, nil
		})
		return s
	}, nil)
}
