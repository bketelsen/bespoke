// Package ui is the Bespoke design system (docs/adr/0010-templui-component-base.md):
// templUI components vendored under components/ (never hand-edited — see
// CLAUDE.md), the compiled theme + JS under assets/ (embedded), and Bespoke
// wrapper components in shell.templ.
//
// Regenerate after changing any .templ file or design/input.css:
//
//	scripts/build-ui.sh
package ui

import (
	"bytes"
	"cmp"
	"context"
	"embed"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"

	"github.com/a-h/templ"
	"github.com/bketelsen/bespoke/pkg/ui/components/icon"
)

//go:embed assets
var assetsFS embed.FS

// Handler serves the design system's compiled CSS and JS. pkg/web mounts it
// at /_bespoke/ on every app, outside auth (static assets, tailnet-only).
func Handler() http.Handler {
	sub, _ := fs.Sub(assetsFS, "assets")
	embedded := http.StripPrefix("/_bespoke/", http.FileServerFS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The instance owns the compiled stylesheet because Tailwind must scan
		// its private templates. Only this exact file is served from disk; JS
		// and every other asset remain versioned and embedded in the platform.
		if r.URL.Path == "/_bespoke/styles.css" {
			root := cmp.Or(os.Getenv("BESPOKE_ROOT"), ".")
			path := filepath.Join(root, "assets", "styles.css")
			if _, err := os.Stat(path); err == nil {
				http.ServeFile(w, r, path)
				return
			}
		}
		embedded.ServeHTTP(w, r)
	})
}

// AppIcon renders a Lucide icon by name, falling back to a generic icon for
// names the vendored set doesn't know (manifest icons are free-text).
func AppIcon(name, class string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		var buf bytes.Buffer
		if err := icon.Icon(name)(icon.Props{Class: class}).Render(ctx, &buf); err != nil {
			return icon.Icon("package")(icon.Props{Class: class}).Render(ctx, w)
		}
		_, err := w.Write(buf.Bytes())
		return err
	})
}

// homeURL is the dashboard address, shown as the brand link in every AppShell.
// In dev mode (BESPOKE_DEV_USER set, no edge proxy) it points at platformd's
// local port instead of the production domain.
func homeURL() templ.SafeURL {
	if os.Getenv("BESPOKE_DEV_USER") != "" {
		return templ.SafeURL("http://localhost:4000/")
	}
	return templ.SafeURL("https://" + cmp.Or(os.Getenv("BESPOKE_DOMAIN"), "bespoke.example.com") + "/")
}
