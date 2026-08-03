// Package manifest loads app manifests (app.toml) per docs/specs/app-manifest.md.
// The manifests are the registry: platformd, the deploy tooling, and (later)
// the bespoke CLI all derive their view of the world from LoadAll.
package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/BurntSushi/toml"
)

type App struct {
	Name        string   `toml:"name"`
	Slug        string   `toml:"slug"`
	Port        int      `toml:"port"`
	Icon        string   `toml:"icon"`
	Description string   `toml:"description"`
	Package     string   `toml:"package"`
	Intents     []Intent `toml:"intents"`
}

// Intent is a declared cross-app action (docs/adr/0018-cross-app-intents.md):
// the app promises to serve GET/POST /_intents/<name>.
type Intent struct {
	Name    string `toml:"name"`    // slug-like, unique within the app
	Title   string `toml:"title"`   // shown on buttons: "Create Todo"
	Accepts string `toml:"accepts"` // payload type; v1: "text" (default)
}

var slugRe = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

func (a App) validate(dir string) error {
	switch {
	case a.Name == "":
		return fmt.Errorf("name is required")
	case !slugRe.MatchString(a.Slug):
		return fmt.Errorf("slug %q must match %s", a.Slug, slugRe)
	case a.Slug != filepath.Base(dir):
		return fmt.Errorf("slug %q must equal directory name %q", a.Slug, filepath.Base(dir))
	case a.Port < 4101 || a.Port > 4999:
		return fmt.Errorf("port %d outside app range 4101-4999", a.Port)
	case a.Icon == "":
		return fmt.Errorf("icon is required")
	case a.Description == "":
		return fmt.Errorf("description is required")
	}
	seen := map[string]bool{}
	for i, in := range a.Intents {
		switch {
		case !slugRe.MatchString(in.Name):
			return fmt.Errorf("intents[%d]: name %q must match %s", i, in.Name, slugRe)
		case in.Title == "":
			return fmt.Errorf("intents[%d] (%s): title is required", i, in.Name)
		case in.Accepts != "" && in.Accepts != "text":
			return fmt.Errorf("intents[%d] (%s): accepts %q unsupported (v1: text)", i, in.Name, in.Accepts)
		case seen[in.Name]:
			return fmt.Errorf("intents[%d]: duplicate name %q", i, in.Name)
		}
		seen[in.Name] = true
	}
	return nil
}

// Load reads and validates a single app's manifest from root/apps/<slug>/app.toml.
func Load(root, slug string) (App, error) {
	dir := filepath.Join(root, "apps", slug)
	var app App
	if _, err := toml.DecodeFile(filepath.Join(dir, "app.toml"), &app); err != nil {
		return App{}, err
	}
	if err := app.validate(dir); err != nil {
		return App{}, fmt.Errorf("%s: %w", dir, err)
	}
	return app, nil
}

// LoadAll scans root/apps/*/app.toml. Directories without a manifest are
// ignored; invalid manifests become warnings so the dashboard can surface them.
func LoadAll(root string) (apps []App, warnings []string, err error) {
	dirs, err := filepath.Glob(filepath.Join(root, "apps", "*"))
	if err != nil {
		return nil, nil, err
	}
	ports := map[int]string{}
	for _, dir := range dirs {
		fi, statErr := os.Stat(dir)
		if statErr != nil || !fi.IsDir() {
			continue
		}
		path := filepath.Join(dir, "app.toml")
		if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
			continue // packaged app source may live under apps without being enabled
		}
		var app App
		if _, decErr := toml.DecodeFile(path, &app); decErr != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", dir, decErr))
			continue
		}
		if vErr := app.validate(dir); vErr != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", dir, vErr))
			continue
		}
		if other, dup := ports[app.Port]; dup {
			warnings = append(warnings, fmt.Sprintf("%s: port %d already used by %s", dir, app.Port, other))
			continue
		}
		ports[app.Port] = app.Slug
		apps = append(apps, app)
	}
	sort.Slice(apps, func(i, j int) bool { return apps[i].Name < apps[j].Name })
	return apps, warnings, nil
}
