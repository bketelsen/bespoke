// Package web: app-contract search endpoint (ADR-0028). GET /_search?q=
// returns the caller's user-scoped matches; platformd fans out to every
// app and groups the results.
package web

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/bketelsen/bespoke/pkg/auth"
)

// SearchResult is one hit an app returns for a query. Only Title is
// required; URL is app-relative and best-effort deep (a bare "/" home link
// is acceptable). See docs/specs/app-search.md.
type SearchResult struct {
	Title     string `json:"title"`
	Snippet   string `json:"snippet,omitempty"`
	URL       string `json:"url,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

// SearchProvider returns the user's results matching q. It MUST scope to the
// user and MUST be a cheap DB query — never an LLM call (platformd waits
// behind a short timeout).
type SearchProvider func(ctx context.Context, user auth.User, q string) ([]SearchResult, error)

// Search mounts GET /_search, the optional app-contract endpoint the
// dashboard search box and the platform `search` tool fan out to. Call
// inside web.Run's register.
func Search(mux *http.ServeMux, provider SearchProvider) {
	mux.HandleFunc("GET /_search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		results, err := provider(r.Context(), auth.FromContext(r.Context()), q)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if results == nil {
			results = []SearchResult{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"results": results})
	})
}
