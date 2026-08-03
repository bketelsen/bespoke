package views

import (
	"fmt"
	"strings"

	"github.com/a-h/templ"
	"github.com/bketelsen/bespoke/internal/manifest"
)

// appURL builds an app's dashboard link: subdomain in production, direct
// localhost port in dev mode (no edge proxy to route subdomains locally).
func appURL(dev bool, domain string, app manifest.App) templ.SafeURL {
	if dev {
		return templ.SafeURL(fmt.Sprintf("http://localhost:%d/", app.Port))
	}
	return templ.SafeURL(fmt.Sprintf("https://%s.%s/", app.Slug, domain))
}

// AppBase is appURL exported for the search fan-out, which resolves each
// result's app-relative URL against its app's base.
func AppBase(dev bool, domain string, app manifest.App) templ.SafeURL {
	return appURL(dev, domain, app)
}

// ResultHref joins an app's base URL with a search result's app-relative
// URL; an empty rel means the app's home (spec: docs/specs/app-search.md).
func ResultHref(base, rel string) string {
	if rel == "" {
		rel = "/"
	}
	return strings.TrimSuffix(base, "/") + rel
}
