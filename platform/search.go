// Global search fan-out (ADR-0028): query each app's /_search in parallel
// with the caller's identity forwarded, group results by app. Any miss
// (absent endpoint, error, slow, oversized) is simply skipped — a broken
// app can't break search. Mirrors cards.go.
package main

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/bketelsen/bespoke/internal/manifest"
	"github.com/bketelsen/bespoke/pkg/web"
)

var searchClient = &http.Client{Timeout: 900 * time.Millisecond}

// SearchGroup is one app's hits for a query, in the dashboard's grouped view.
type SearchGroup struct {
	Slug    string
	Name    string
	Results []web.SearchResult
}

// aggregateSearch fans out q to every app's /_search, forwarding identity.
// Groups are returned only for apps that produced at least one result, in
// the input order (fan-out yields no cross-app ranking).
func aggregateSearch(ctx context.Context, login, name, q string, apps []manifest.App) []SearchGroup {
	host := cmp.Or(os.Getenv("BESPOKE_BIND_IP"), "127.0.0.1")

	type slot struct {
		group SearchGroup
		ok    bool
	}
	slots := make([]slot, len(apps))
	var wg sync.WaitGroup
	for i, app := range apps {
		wg.Add(1)
		go func(i int, app manifest.App) {
			defer wg.Done()
			u := fmt.Sprintf("http://%s:%d/_search?q=%s", host, app.Port, url.QueryEscape(q))
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
			if err != nil {
				return
			}
			req.Header.Set("Tailscale-User-Login", login)
			req.Header.Set("Tailscale-User-Name", name)
			resp, err := searchClient.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return
			}
			var payload struct {
				Results []web.SearchResult `json:"results"`
			}
			if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<10)).Decode(&payload); err != nil {
				return
			}
			if len(payload.Results) == 0 {
				return
			}
			slots[i] = slot{SearchGroup{Slug: app.Slug, Name: app.Name, Results: payload.Results}, true}
		}(i, app)
	}
	wg.Wait()

	var groups []SearchGroup
	for _, s := range slots {
		if s.ok {
			groups = append(groups, s.group)
		}
	}
	return groups
}
