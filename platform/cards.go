// Dashboard card fetching (ADR-0017): pull each app's /_card fragment in
// parallel with the caller's identity forwarded; any miss falls back to the
// manifest description.
package main

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bketelsen/bespoke/internal/manifest"
)

var cardClient = &http.Client{Timeout: 900 * time.Millisecond}

var contextClient = &http.Client{Timeout: 1500 * time.Millisecond}

// aggregateContexts pulls every chat-enabled app's /_chat/context for the
// user (ADR-0020) — parallel, identity forwarded, 16KB cap per app, misses
// skipped. The result feeds the dashboard's all-apps chat.
func aggregateContexts(ctx context.Context, login, name string, apps []manifest.App) string {
	host := cmp.Or(os.Getenv("BESPOKE_BIND_IP"), "127.0.0.1")

	type section struct {
		slug, text string
	}
	results := make([]section, len(apps))
	var wg sync.WaitGroup
	for i, app := range apps {
		wg.Add(1)
		go func(i int, app manifest.App) {
			defer wg.Done()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet,
				fmt.Sprintf("http://%s:%d/_chat/context", host, app.Port), nil)
			if err != nil {
				return
			}
			req.Header.Set("Tailscale-User-Login", login)
			req.Header.Set("Tailscale-User-Name", name)
			resp, err := contextClient.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return
			}
			body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
			if err != nil || len(body) == 0 {
				return
			}
			results[i] = section{app.Slug, string(body)}
		}(i, app)
	}
	wg.Wait()

	var b strings.Builder
	for _, s := range results {
		if s.text == "" {
			continue
		}
		fmt.Fprintf(&b, "## App: %s\n%s\n\n", s.slug, s.text)
	}
	if b.Len() == 0 {
		return "(no app data available right now)"
	}
	return b.String()
}

// fetchCards returns slug → card fragment HTML for apps that serve /_card.
// Missing/slow/broken cards are simply absent — the view falls back.
func fetchCards(ctx context.Context, login, name string, apps []manifest.App) map[string]string {
	host := cmp.Or(os.Getenv("BESPOKE_BIND_IP"), "127.0.0.1")

	var mu sync.Mutex
	cards := make(map[string]string, len(apps))
	var wg sync.WaitGroup
	for _, app := range apps {
		wg.Add(1)
		go func(app manifest.App) {
			defer wg.Done()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet,
				fmt.Sprintf("http://%s:%d/_card", host, app.Port), nil)
			if err != nil {
				return
			}
			// Forward the caller's identity: cards are per-user, and
			// platformd is a trusted platform component (ADR-0017).
			req.Header.Set("Tailscale-User-Login", login)
			req.Header.Set("Tailscale-User-Name", name)
			resp, err := cardClient.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return
			}
			body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<10))
			if err != nil || len(body) == 0 {
				return
			}
			mu.Lock()
			cards[app.Slug] = string(body)
			mu.Unlock()
		}(app)
	}
	wg.Wait()
	return cards
}
