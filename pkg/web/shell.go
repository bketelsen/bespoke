package web

import (
	"cmp"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/bketelsen/bespoke/internal/manifest"
	"github.com/bketelsen/bespoke/pkg/ui"
)

// withShellData feeds the AppShell's platform chrome (ADR-0015): the app
// switcher from the registry and the chat-enabled flag, via request context.
// Apps never touch any of this.
func withShellData(slug string, next http.Handler) http.Handler {
	var (
		mu      sync.Mutex
		cached  []ui.AppLink
		fetched time.Time
	)
	dev := os.Getenv("BESPOKE_DEV_USER") != ""
	domain := cmp.Or(os.Getenv("BESPOKE_DOMAIN"), "bespoke.example.com")
	root := cmp.Or(os.Getenv("BESPOKE_ROOT"), ".")

	home := "https://" + domain + "/"
	if dev {
		home = "http://localhost:4000/"
	}

	var cachedIntents []ui.IntentLink
	links := func() ([]ui.AppLink, []ui.IntentLink) {
		mu.Lock()
		defer mu.Unlock()
		if time.Since(fetched) < 10*time.Second {
			return cached, cachedIntents
		}
		apps, _, err := manifest.LoadAll(root)
		if err != nil {
			return cached, cachedIntents // stale beats broken chrome
		}
		cached, cachedIntents = cached[:0], cachedIntents[:0]
		for _, a := range apps {
			url := fmt.Sprintf("https://%s.%s/", a.Slug, domain)
			if dev {
				url = fmt.Sprintf("http://localhost:%d/", a.Port)
			}
			cached = append(cached, ui.AppLink{Name: a.Name, Slug: a.Slug, Icon: a.Icon, URL: url})
			if a.Slug == slug {
				continue // never offer an app its own intents
			}
			for _, in := range a.Intents {
				cachedIntents = append(cachedIntents, ui.IntentLink{
					App: a.Slug, Name: in.Name, Title: in.Title,
					URL: url + "_intents/" + in.Name,
				})
			}
		}
		fetched = time.Now()
		return cached, cachedIntents
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apps, intents := links()
		ctx := ui.WithShellData(r.Context(), ui.ShellData{
			Apps:        apps,
			Current:     slug,
			HomeURL:     home,
			ChatEnabled: chatEnabled.Load(),
			Intents:     intents,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
