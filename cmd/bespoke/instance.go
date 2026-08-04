package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"text/template"

	"github.com/bketelsen/bespoke/internal/manifest"
)

const platformModule = "github.com/bketelsen/bespoke"

func currentModule() (string, error) {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Path}}").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("read instance module: %w: %s", err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

func frameworkPackage(local string) string {
	module, err := currentModule()
	if err == nil && module == platformModule {
		return local
	}
	return platformModule + "/" + strings.TrimPrefix(local, "./")
}

func cmdInit(args []string) error {
	var root, module, platformVersion string
	var withBuilder bool
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--module", "--platform-version":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", args[i])
			}
			i++
			if args[i-1] == "--module" {
				module = args[i]
			} else {
				platformVersion = args[i]
			}
		case "--with-builder":
			withBuilder = true
		default:
			if strings.HasPrefix(args[i], "-") || root != "" {
				return fmt.Errorf("unexpected argument %q", args[i])
			}
			root = args[i]
		}
	}
	if root == "" || module == "" {
		return fmt.Errorf("usage: bespoke init <dir> --module <module-path> [--platform-version vX.Y.Z] [--with-builder]")
	}
	v := strings.TrimSpace(platformVersion)
	if v == "" {
		v = buildVersion()
	}
	if v == "dev" {
		return fmt.Errorf("development CLI has no release pin; pass --platform-version vX.Y.Z")
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	if entries, err := os.ReadDir(root); err == nil && len(entries) != 0 {
		return fmt.Errorf("%s is not empty", root)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	data := instanceTemplateData{Module: module, Version: v}
	files := map[string]string{
		"go.mod": instanceGoMod, "Justfile": instanceJustfile,
		".gitignore": instanceGitignore, "README.md": instanceReadme,
		".golangci.yml": instanceGolangCI,
		"AGENTS.md":     instanceAgents, "design/theme.css": instanceTheme,
		"scripts/setup-tools.sh":             instanceSetupTools,
		"deploy/deploy.env.example":          instanceDeployExample,
		".agents/skills/design-app/SKILL.md": instanceDesignSkill,
		".agents/skills/new-app/SKILL.md":    instanceNewAppSkill,
	}
	for path, body := range files {
		if err := renderInstanceFile(root, path, body, data); err != nil {
			return err
		}
	}
	if err := createInstanceLinks(root); err != nil {
		return err
	}
	for _, slug := range []string{"notes", "todo"} {
		if err := writeStarterApp(root, slug, data); err != nil {
			return err
		}
	}
	format := exec.Command("gofmt", "-w",
		filepath.Join(root, "apps", "notes", "main.go"),
		filepath.Join(root, "apps", "todo", "main.go"))
	if out, err := format.CombinedOutput(); err != nil {
		return fmt.Errorf("format starter apps: %w: %s", err, out)
	}
	if withBuilder {
		if err := renderInstanceFile(root, "apps/builder/app.toml", instanceBuilderManifest, data); err != nil {
			return err
		}
	}
	fmt.Printf("created Bespoke instance %s pinned to %s\nnext: cd %s && go mod tidy && go tool bespoke ui && just check\n", module, v, root)
	return nil
}

func cmdUpgrade(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: bespoke upgrade <version|latest>")
	}
	target := args[0]
	if target != "latest" && !strings.HasPrefix(target, "v") {
		target = "v" + target
	}
	cmd := exec.Command("go", "get", platformModule+"@"+target)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	cmd = exec.Command("go", "mod", "tidy")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	module, err := currentModule()
	if err != nil {
		return err
	}
	resolved, err := exec.Command("go", "list", "-m", "-f", "{{.Version}}", platformModule).Output()
	if err != nil {
		return err
	}
	data := instanceTemplateData{Module: module, Version: strings.TrimSpace(string(resolved))}
	managed := map[string]string{
		"AGENTS.md":                          instanceAgents,
		"scripts/setup-tools.sh":             instanceSetupTools,
		".agents/skills/design-app/SKILL.md": instanceDesignSkill,
		".agents/skills/new-app/SKILL.md":    instanceNewAppSkill,
	}
	for path, body := range managed {
		if err := renderInstanceFile(".", path, body, data); err != nil {
			return err
		}
	}
	return nil
}

func cmdUI(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: bespoke ui")
	}
	if _, err := os.Stat("tools/templ"); err != nil {
		return fmt.Errorf("tools/templ missing; run the instance tools setup first")
	}
	if _, err := os.Stat("tools/tailwindcss"); err != nil {
		return fmt.Errorf("tools/tailwindcss missing; run the instance tools setup first")
	}
	cmd := exec.Command(filepath.FromSlash("tools/templ"), "generate")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	moduleDir, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", platformModule).Output()
	if err != nil {
		return fmt.Errorf("locate Bespoke module: %w", err)
	}
	platformDir := strings.TrimSpace(string(moduleDir))
	base := filepath.Join(platformDir, "design", "base.css")
	roots, err := uiSourceRoots(platformDir)
	if err != nil {
		return err
	}
	input := filepath.Join("dist", "ui-input.css")
	if err := os.MkdirAll("dist", 0o755); err != nil {
		return err
	}
	css := uiInputCSS(base, mustAbs("design/theme.css"), roots)
	if err := os.WriteFile(input, []byte(css), 0o644); err != nil {
		return err
	}
	if err := os.MkdirAll("assets", 0o755); err != nil {
		return err
	}
	cmd = exec.Command(filepath.FromSlash("tools/tailwindcss"), "-i", input, "-o", filepath.FromSlash("assets/styles.css"), "--minify")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

// uiInputCSS is the generated Tailwind entrypoint: the platform's base layer,
// then the instance's theme tokens (order matters — tokens override), then one
// scan glob pair per source root.
func uiInputCSS(base, theme string, roots []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "@import %q;\n@import %q;\n", base, theme)
	for _, root := range roots {
		fmt.Fprintf(&b, "@source %q;\n@source %q;\n",
			filepath.Join(root, "**", "*.templ"), filepath.Join(root, "**", "*_templ.go"))
	}
	return b.String()
}

// uiSourceRoots lists the trees Tailwind must scan for utility classes: the
// instance's own apps, plus the source module of every app installed from
// another module (`package` in app.toml). Classes Tailwind never sees are
// pruned from assets/styles.css, so an unscanned app renders unstyled — the
// failure is silent, which is why a package that cannot be resolved is a hard
// error here. The platform's own templates are already scanned by
// design/input.css, so its module is skipped.
func uiSourceRoots(platformDir string) ([]string, error) {
	apps, warnings, err := manifest.LoadAll(".")
	if err != nil {
		return nil, err
	}
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	roots := []string{mustAbs("apps")}
	skip := []string{platformDir, mustAbs(".")}
	for _, a := range apps {
		if a.Package == "" {
			continue
		}
		dir, err := packageModuleDir(a.Package)
		if err != nil {
			return nil, fmt.Errorf("app %s: %w", a.Slug, err)
		}
		if dir == "" || slices.Contains(skip, dir) || slices.Contains(roots, dir) {
			continue
		}
		roots = append(roots, dir)
	}
	return roots, nil
}

// packageModuleDir resolves the on-disk module directory providing a package.
// The module directory rather than the package directory: a shared app's
// templates may sit above its main package, and scanning a little extra costs
// nothing while missing a template costs the app its styling.
func packageModuleDir(pkg string) (string, error) {
	cmd := exec.Command("go", "list", "-f", "{{with .Module}}{{.Dir}}{{else}}{{.Dir}}{{end}}", pkg)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("locate package %s (is it required? try `go get %s`): %w: %s",
			pkg, pkg, err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

func mustAbs(path string) string {
	p, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return p
}

type instanceTemplateData struct{ Module, Version string }

func createInstanceLinks(root string) error {
	links := map[string]string{
		"CLAUDE.md": "AGENTS.md", "GEMINI.md": "AGENTS.md",
		".github/copilot-instructions.md": "../AGENTS.md",
		".claude/skills":                  "../.agents/skills",
	}
	for path, target := range links {
		name := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			return err
		}
		if _, err := os.Lstat(name); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := os.Symlink(target, name); err != nil {
			return err
		}
	}
	return nil
}

func renderInstanceFile(root, path, body string, data instanceTemplateData) error {
	t, err := template.New(path).Parse(body)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return err
	}
	target := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if strings.HasSuffix(path, ".sh") {
		mode = 0o755
	}
	return os.WriteFile(target, buf.Bytes(), mode)
}

const instanceGoMod = `module {{.Module}}

go 1.26.5

require github.com/bketelsen/bespoke {{.Version}}

tool github.com/bketelsen/bespoke/cmd/bespoke
`

const instanceJustfile = `_default:
    @just --list --unsorted

dev:
    go tool bespoke dev

new slug:
    go tool bespoke new {{"{{"}} slug {{"}}"}}

ui:
    go tool bespoke ui

tools:
    scripts/setup-tools.sh

check:
    go vet ./...
    go test ./...
    golangci-lint run
    go mod tidy -diff
    CGO_ENABLED=0 GOOS=linux go build ./...

deploy *args:
    go tool bespoke deploy --all {{"{{"}} args {{"}}"}}
`

const instanceGitignore = `/data/
/dist/
/tools/
/deploy/deploy.env
`

const instanceGolangCI = `version: "2"
linters:
  default: standard
  enable: [misspell, unconvert, nolintlint]
  settings:
    errcheck:
      exclude-functions:
        - (net/http.ResponseWriter).Write
        - (*encoding/json.Encoder).Encode
        - io.Copy
        - (io.Closer).Close
        - (*os.File).Close
        - (*database/sql.DB).Close
        - (*database/sql.Rows).Close
        - (*database/sql.Tx).Rollback
        - (github.com/a-h/templ.Component).Render
  exclusions:
    generated: lax
formatters:
  enable: [gofmt]
`

const instanceDeployExample = `# Copy to deploy/deploy.env and fill in owner values.
DOMAIN=bespoke.example.com
SELFIE_SSH=you@your-app-host
SELFIE_TS_IP=100.0.0.1
EDGE_SSH=you@your-edge-host
EDGE_CADDY_FILE=/etc/caddy/bespoke.caddy
GOARCH=amd64
`

const instanceReadme = `# Bespoke instance

Private apps, theme, and deployment configuration for this Bespoke installation.
The platform is pinned in go.mod; use ` + "`go tool bespoke upgrade <version>`" + ` to upgrade it.
Deployment follows https://github.com/bketelsen/bespoke/blob/main/deploy/README.md.
`

const instanceAgents = `# Bespoke instance

This repository contains private Bespoke apps and owner configuration. Follow the
platform conventions documented at https://github.com/bketelsen/bespoke/blob/main/AGENTS.md.
Use ` + "`go tool bespoke new`" + ` for apps, ` + "`go tool bespoke ui`" + ` after templ or theme changes,
and ` + "`just check`" + ` before declaring work complete. Never commit data, secrets, or deploy.env.
`

const instanceDesignSkill = `---
name: design-app
description: Turn a thin private app idea into a compact implementation-ready spec.
---

# Design a Bespoke app

Before building a one-line idea, ask only decisions that change the result:
the usage moment, record shape, essential views/actions, useful LLM or audio
features, cross-app intents, and explicit non-goals. Write the agreed result to
` + "`apps/<slug>/README.md`" + `, then use the new-app skill.
`

const instanceNewAppSkill = `---
name: new-app
description: Build and verify an app inside a private Bespoke instance.
---

# Build a Bespoke app

Run ` + "`just new <slug>`" + `, implement the approved README using only the pinned
Bespoke packages, and expose every meaningful mutation as a tool. Add a cheap
dashboard card, live fragment plus ` + "`web.Changed`" + `, chat when the data invites
questions, and intents for natural cross-app inputs. Run ` + "`just ui`" + ` after templ
changes and finish with ` + "`just check`" + `. Review existing apps for reciprocal intents.
`

const instanceTheme = `/* Owner-controlled theme tokens. Platform structure and mobile invariants live in Bespoke's design/base.css. */
:root { --primary: oklch(0.46 0.085 205); --accent: oklch(0.72 0.13 55); --radius: 0.5rem; }
.dark { --primary: oklch(0.72 0.1 200); --accent: oklch(0.75 0.12 60); }
`

const instanceBuilderManifest = `name = "Builder"
slug = "builder"
port = 4105
icon = "hammer"
description = "Type an idea, watch it become a deployed app"
package = "github.com/bketelsen/bespoke/apps/builder"

[[intents]]
name = "build-app"
title = "Build an App"
accepts = "text"
`

const instanceSetupTools = `#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p tools
GOBIN="$PWD/tools" go install github.com/a-h/templ/cmd/templ@v0.3.1020
os=$(uname -s)
arch=$(uname -m)
case "$os/$arch" in
  Linux/x86_64) asset=tailwindcss-linux-x64 ;;
  Linux/aarch64) asset=tailwindcss-linux-arm64 ;;
  Darwin/x86_64) asset=tailwindcss-macos-x64 ;;
  Darwin/arm64) asset=tailwindcss-macos-arm64 ;;
  *) echo "unsupported platform $os/$arch" >&2; exit 1 ;;
esac
curl -fsSL -o tools/tailwindcss "https://github.com/tailwindlabs/tailwindcss/releases/download/v4.3.3/$asset"
chmod +x tools/tailwindcss
echo "UI tools installed in ./tools"
`

func writeStarterApp(root, slug string, data instanceTemplateData) error {
	port := 4101
	name, icon, description := "Notes", "notebook-pen", "A simple stream of notes"
	intentName, intentTitle := "add-note", "Save as Note"
	if slug == "todo" {
		port, name, icon, description, intentName, intentTitle = 4102, "Todo", "list-checks", "Tasks worth doing", "add-task", "Create Todo"
	}
	d := struct {
		instanceTemplateData
		Slug, Name, Icon, Description, IntentName, IntentTitle string
		Port                                                   int
	}{data, slug, name, icon, description, intentName, intentTitle, port}
	files := map[string]string{
		"app.toml": starterManifest, "main.go": starterMain,
		"migrations/0001_init.sql": starterMigration, "views/home.templ": starterView,
	}
	for path, body := range files {
		t, err := template.New(path).Parse(body)
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		if err := t.Execute(&buf, d); err != nil {
			return err
		}
		target := filepath.Join(root, "apps", slug, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, buf.Bytes(), 0o644); err != nil {
			return err
		}
	}
	return nil
}

const starterManifest = `name = "{{.Name}}"
slug = "{{.Slug}}"
port = {{.Port}}
icon = "{{.Icon}}"
description = "{{.Description}}"

[[intents]]
name = "{{.IntentName}}"
title = "{{.IntentTitle}}"
accepts = "text"
`
const starterMigration = `CREATE TABLE items (id INTEGER PRIMARY KEY, login TEXT NOT NULL, body TEXT NOT NULL, created_at TEXT NOT NULL DEFAULT (datetime('now')));
CREATE INDEX items_login_created ON items(login, created_at DESC);
`
const starterMain = `package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"{{.Module}}/apps/{{.Slug}}/views"
	"github.com/bketelsen/bespoke/pkg/auth"
	"github.com/bketelsen/bespoke/pkg/db"
	"github.com/bketelsen/bespoke/pkg/web"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

func main() {
	migrations, _ := fs.Sub(migrationFS, "migrations")
	sqldb, err := db.Open("{{.Slug}}", migrations); if err != nil { log.Fatal(err) }
	web.Run("{{.Slug}}", func(mux *http.ServeMux) {
		mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			rows, _ := sqldb.QueryContext(r.Context(), "SELECT body FROM items WHERE login=? ORDER BY id DESC LIMIT 50", user.Login)
			var items []string
			if rows != nil { defer rows.Close(); for rows.Next() { var body string; _ = rows.Scan(&body); items = append(items, body) } }
			views.Home(user, items).Render(r.Context(), w)
		})
		mux.HandleFunc("POST /items", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			text := strings.TrimSpace(r.FormValue("text")); if text == "" { http.Error(w, "text is required", http.StatusBadRequest); return }
			if _, err := sqldb.ExecContext(r.Context(), "INSERT INTO items(login,body) VALUES(?,?)", user.Login, text); err != nil { http.Error(w, err.Error(), http.StatusInternalServerError); return }
			web.Changed(user.Login); http.Redirect(w, r, "/", http.StatusSeeOther)
		})
		web.Intent(mux, "{{.Name}}", web.IntentDef{Name: "{{.IntentName}}", Title: "{{.IntentTitle}}", Prompt: "Review the text before saving.", Handler: func(ctx context.Context, user auth.User, text string) (string, error) {
			_, err := sqldb.ExecContext(ctx, "INSERT INTO items(login,body) VALUES(?,?)", user.Login, text); if err == nil { web.Changed(user.Login) }; return "/", err
		}})
	})
}
`
const starterView = `package views

import (
	"github.com/bketelsen/bespoke/pkg/auth"
	"github.com/bketelsen/bespoke/pkg/ui"
	"github.com/bketelsen/bespoke/pkg/ui/components/button"
	"github.com/bketelsen/bespoke/pkg/ui/components/card"
	"github.com/bketelsen/bespoke/pkg/ui/components/textarea"
)

templ Home(user auth.User, items []string) {
	@ui.AppShell(ui.ShellProps{Title: "{{.Name}}", User: user}) {
		@card.Card() {
			@card.Content() {
				<form method="post" action="/items" class="space-y-3 pt-6">
					@textarea.Textarea(textarea.Props{Name: "text", Placeholder: "Add to {{.Name}}…"})
					@button.Button(button.Props{Type: button.TypeSubmit}) { Add }
				</form>
			}
		}
		<div class="space-y-3">
			for _, item := range items {
				@card.Card() {
					@card.Content() {
						<p class="select-text pt-6">{ item }</p>
					}
				}
			}
		</div>
	}
}
`
