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
