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
	Name        string `toml:"name"`
	Slug        string `toml:"slug"`
	Port        int    `toml:"port"`
	Icon        string `toml:"icon"`
	Description string `toml:"description"`
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
	return nil
}

// LoadAll scans root/apps/*/app.toml. Invalid or missing manifests never fail
// the scan; they are reported as warnings so the dashboard can surface them.
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
