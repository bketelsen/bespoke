package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"text/template"

	"github.com/bketelsen/bespoke/internal/manifest"
)

var slugRe = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// cmdNew scaffolds apps/<slug> per the canonical shape (CLAUDE.md) and
// assigns the next free port. Exits without side effects on any validation
// failure (docs/specs/bespoke-cli.md).
func cmdNew(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: bespoke new <slug>")
	}
	slug := args[0]
	if !slugRe.MatchString(slug) {
		return fmt.Errorf("slug %q must match %s", slug, slugRe)
	}
	if slug == "platformd" {
		return fmt.Errorf("slug %q is reserved", slug)
	}
	dir := filepath.Join("apps", slug)
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("%s already exists", dir)
	}

	apps, warnings, err := manifest.LoadAll(".")
	if err != nil {
		return err
	}
	if len(warnings) > 0 {
		return fmt.Errorf("fix manifest warnings before adding apps: %v", warnings)
	}
	port := 4101
	for _, a := range apps {
		if a.Port >= port {
			port = a.Port + 1
		}
	}
	if port > 4999 {
		return fmt.Errorf("port range 4101-4999 exhausted")
	}

	data := map[string]any{"Slug": slug, "Port": port}
	files := map[string]*template.Template{
		filepath.Join(dir, "app.toml"):                    scaffoldManifest,
		filepath.Join(dir, "main.go"):                     scaffoldMain,
		filepath.Join(dir, "views", "home.templ"):         scaffoldView,
		filepath.Join(dir, "migrations", "0001_init.sql"): scaffoldMigration,
	}
	for path, tmpl := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		if err := tmpl.Execute(f, data); err != nil {
			f.Close()
			return err
		}
		f.Close()
		fmt.Println("created", path)
	}

	// Generate templ output now if the toolchain is installed, so the app
	// compiles immediately; otherwise tell the user what to run.
	if _, err := os.Stat("tools/templ"); err == nil {
		out, err := exec.Command("./tools/templ", "generate", "-f", filepath.Join(dir, "views", "home.templ")).CombinedOutput()
		if err != nil {
			return fmt.Errorf("templ generate: %v\n%s", err, out)
		}
		fmt.Println("generated templ output")
	} else {
		fmt.Println("NOTE: run `just tools` then `just ui` before building (templ output not generated)")
	}

	fmt.Printf("\n%s scaffolded on port %d — next:\n  1. edit %s (name, icon, description)\n  2. just dev   → http://localhost:%d\n", slug, port, filepath.Join(dir, "app.toml"), port)
	return nil
}

var scaffoldManifest = template.Must(template.New("m").Parse(`name        = "{{.Slug}}"
slug        = "{{.Slug}}"
port        = {{.Port}}
icon        = "package"
description = "TODO: one line for the dashboard"
`))

var scaffoldMain = template.Must(template.New("g").Parse(`// {{.Slug}}: a Bespoke app — manifest, routes, views, migrations, nothing
// else (CLAUDE.md).
package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"

	"github.com/bketelsen/bespoke/apps/{{.Slug}}/views"
	"github.com/bketelsen/bespoke/pkg/auth"
	"github.com/bketelsen/bespoke/pkg/db"
	"github.com/bketelsen/bespoke/pkg/web"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

func main() {
	migrations, _ := fs.Sub(migrationFS, "migrations")
	sqldb, err := db.Open("{{.Slug}}", migrations)
	if err != nil {
		log.Fatal(err)
	}
	_ = sqldb // remove once the app has queries

	web.Run("{{.Slug}}", func(mux *http.ServeMux) {
		mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			views.Home(auth.FromContext(r.Context())).Render(r.Context(), w)
		})
	})
}
`))

var scaffoldView = template.Must(template.New("v").Parse(`package views

import (
	"github.com/bketelsen/bespoke/pkg/auth"
	"github.com/bketelsen/bespoke/pkg/ui"
	"github.com/bketelsen/bespoke/pkg/ui/components/card"
)

templ Home(user auth.User) {
	@ui.AppShell(ui.ShellProps{Title: "{{.Slug}}", User: user}) {
		@card.Card() {
			@card.Header() {
				@card.Title() {
					{{.Slug}}
				}
				@card.Description() {
					Scaffolded and ready — replace this card with the app.
				}
			}
		}
	}
}
`))

var scaffoldMigration = template.Must(template.New("s").Parse(`-- {{.Slug}} schema, applied by pkg/db in lexical order.
-- Add tables here; never edit an applied migration — add 0002_*.sql instead.
`))
