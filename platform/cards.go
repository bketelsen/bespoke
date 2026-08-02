// Dashboard card fetching (ADR-0017): pull each app's /_card fragment in
// parallel with the caller's identity forwarded; any miss falls back to the
// manifest description.
package main

import (
	"cmp"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/bketelsen/bespoke/internal/manifest"
)

var cardClient = &http.Client{Timeout: 900 * time.Millisecond}

// fetchCards returns slug → card fragment HTML for apps that serve /_card.
// Missing/slow/broken cards are simply absent — the view falls back.
func fetchCards(r *http.Request, apps []manifest.App) map[string]string {
	host := cmp.Or(os.Getenv("BESPOKE_BIND_IP"), "127.0.0.1")
	login := r.Header.Get("Tailscale-User-Login")
	name := r.Header.Get("Tailscale-User-Name")

	var mu sync.Mutex
	cards := make(map[string]string, len(apps))
	var wg sync.WaitGroup
	for _, app := range apps {
		wg.Add(1)
		go func(app manifest.App) {
			defer wg.Done()
			req, err := http.NewRequestWithContext(r.Context(), http.MethodGet,
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
