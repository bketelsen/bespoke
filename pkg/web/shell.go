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

	links := func() []ui.AppLink {
		mu.Lock()
		defer mu.Unlock()
		if time.Since(fetched) < 10*time.Second {
			return cached
		}
		apps, _, err := manifest.LoadAll(root)
		if err != nil {
			return cached // stale beats broken chrome
		}
		cached = cached[:0]
		for _, a := range apps {
			url := fmt.Sprintf("https://%s.%s/", a.Slug, domain)
			if dev {
				url = fmt.Sprintf("http://localhost:%d/", a.Port)
			}
			cached = append(cached, ui.AppLink{Name: a.Name, Slug: a.Slug, Icon: a.Icon, URL: url})
		}
		fetched = time.Now()
		return cached
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := ui.WithShellData(r.Context(), ui.ShellData{
			Apps:        links(),
			Current:     slug,
			HomeURL:     home,
			ChatEnabled: chatEnabled.Load(),
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
