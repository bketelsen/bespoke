package views

import (
	"fmt"

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

// AppBase is appURL exported for the search handler, which resolves each
// result's app-relative URL against its app's base.
func AppBase(dev bool, domain string, app manifest.App) templ.SafeURL {
	return appURL(dev, domain, app)
}
