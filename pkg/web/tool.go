package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"

	"github.com/bketelsen/bespoke/pkg/auth"
)

// ToolDef is a user-scoped action the app exposes to LLMs (ADR-0021):
// agentic chat calls these through the gateway, and the platform MCP server
// exposes them to external clients as <slug>_<name>. Handlers must scope
// everything by the given user — same discipline as HTTP handlers.
type ToolDef struct {
	Name        string
	Description string
	Schema      map[string]any // JSON Schema for the arguments object
	Handler     func(ctx context.Context, user auth.User, args json.RawMessage) (string, error)
}

// wireTool is a tool as advertised over /_tools and sent to the gateway.
type wireTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Schema      map[string]any `json:"schema"`
	URL         string         `json:"url,omitempty"`
}

var (
	toolsMu    sync.Mutex
	toolDefs   []wireTool
	toolsOnce  sync.Once
	toolMounts *http.ServeMux // captured for the /_tools listing mount
)

// Tool registers an LLM-callable action: POST /_tools/<name> executes it
// (auth'd like everything), and GET /_tools lists all definitions for the
// aggregators. Call inside web.Run's register.
func Tool(mux *http.ServeMux, def ToolDef) {
	toolsMu.Lock()
	toolDefs = append(toolDefs, wireTool{Name: def.Name, Description: def.Description, Schema: def.Schema})
	toolsMu.Unlock()

	toolsOnce.Do(func() {
		mux.HandleFunc("GET /_tools", func(w http.ResponseWriter, r *http.Request) {
			toolsMu.Lock()
			defer toolsMu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(toolDefs)
		})
	})

	mux.HandleFunc("POST /_tools/"+def.Name, func(w http.ResponseWriter, r *http.Request) {
		user := auth.FromContext(r.Context())
		args, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		result, err := def.Handler(r.Context(), user, args)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(result))
	})
}

// localTools returns this app's tool definitions with callable URLs for the
// gateway (it calls back over HTTP with the user's identity, uniformly for
// local and cross-app tools).
func localTools() []wireTool {
	toolsMu.Lock()
	defer toolsMu.Unlock()
	out := make([]wireTool, len(toolDefs))
	copy(out, toolDefs)
	base := selfBase.Load()
	if base != nil {
		for i := range out {
			out[i].URL = *base + "/_tools/" + out[i].Name
		}
	}
	return out
}
