package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/bketelsen/bespoke/internal/manifest"
)

// The app index (ADR-0031) is a plain TOML file in a public repository, not a
// service: it maps short names to module paths and vouches for nothing. Point
// BESPOKE_INDEX at your own to use a different list.
const defaultIndexURL = "https://raw.githubusercontent.com/bketelsen/bespoke-apps/main/apps.toml"

// indexEntry is one row of the index. Everything an instance actually needs to
// install the app comes from the module itself; these fields exist so a person
// can decide whether they want it.
type indexEntry struct {
	Module      string `toml:"module"`
	Name        string `toml:"name"`
	Description string `toml:"description"`
	Author      string `toml:"author"`
	Source      string `toml:"source"`
}

// shortName is what a person types: the module's last element with the
// conventional bespoke-app- prefix removed.
func (e indexEntry) shortName() string {
	return strings.TrimPrefix(path.Base(e.Module), "bespoke-app-")
}

func indexURL() string {
	if u := strings.TrimSpace(os.Getenv("BESPOKE_INDEX")); u != "" {
		return u
	}
	return defaultIndexURL
}

func fetchIndex() ([]indexEntry, error) {
	url := indexURL()
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch index %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch index %s: %s", url, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read index %s: %w", url, err)
	}
	var doc struct {
		App []indexEntry `toml:"app"`
	}
	if err := toml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse index %s: %w", url, err)
	}
	sort.Slice(doc.App, func(i, j int) bool { return doc.App[i].shortName() < doc.App[j].shortName() })
	return doc.App, nil
}

// cmdSearch lists index entries matching every query term (name, module, or
// description, case-insensitively). No query lists everything.
func cmdSearch(args []string) error {
	entries, err := fetchIndex()
	if err != nil {
		return err
	}
	var matched []indexEntry
	for _, e := range entries {
		hay := strings.ToLower(e.Name + " " + e.Module + " " + e.Description + " " + e.Author)
		keep := true
		for _, term := range args {
			if !strings.Contains(hay, strings.ToLower(term)) {
				keep = false
				break
			}
		}
		if keep {
			matched = append(matched, e)
		}
	}
	if len(matched) == 0 {
		fmt.Printf("no apps in %s match %s\n", indexURL(), strings.Join(args, " "))
		return nil
	}
	for _, e := range matched {
		fmt.Printf("%-16s %s\n%-16s %s\n", e.shortName(), e.Description, "", e.Module)
	}
	fmt.Printf("\n%d app(s). Install with `bespoke add <name>`. The index vouches for nothing:\n", len(matched))
	fmt.Println("an installed app runs as you, beside every other app's data — read it first.")
	return nil
}

// cmdAdd installs an app published as its own module (ADR-0031): pin the
// module, read the manifest template it ships, assign a free port, and write
// apps/<slug>/app.toml. No Go source enters the instance.
func cmdAdd(args []string) error {
	var target, slugOverride string
	port := 0
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--slug", "--port":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", args[i])
			}
			i++
			if args[i-1] == "--slug" {
				slugOverride = args[i]
			} else {
				p, err := strconv.Atoi(args[i])
				if err != nil {
					return fmt.Errorf("--port %q is not a number", args[i])
				}
				port = p
			}
		default:
			if strings.HasPrefix(args[i], "-") || target != "" {
				return fmt.Errorf("unexpected argument %q", args[i])
			}
			target = args[i]
		}
	}
	if target == "" {
		return fmt.Errorf("usage: bespoke add <module|name>[@version] [--slug <slug>] [--port <port>]")
	}

	module, version, _ := strings.Cut(target, "@")
	// A bare name (no dots, no slashes) is an index lookup; anything else is
	// a module path, so an instance can install apps that were never indexed.
	if !strings.ContainsAny(module, "./") {
		entries, err := fetchIndex()
		if err != nil {
			return err
		}
		i := slices.IndexFunc(entries, func(e indexEntry) bool { return e.shortName() == module })
		if i < 0 {
			return fmt.Errorf("no app named %q in %s (pass a full module path to install an unindexed app)", module, indexURL())
		}
		module = entries[i].Module
	}

	// Everything checkable without the module is checked before fetching, so
	// an obvious mistake leaves go.mod alone (`new` has the same guarantee).
	if slugOverride != "" {
		if !slugRe.MatchString(slugOverride) {
			return fmt.Errorf("slug %q must match %s", slugOverride, slugRe)
		}
		if _, err := os.Stat(filepath.Join("apps", slugOverride)); err == nil {
			return fmt.Errorf("apps/%s already exists", slugOverride)
		}
	}
	if port != 0 && (port < 4101 || port > 4999) {
		return fmt.Errorf("port %d outside app range 4101-4999", port)
	}
	apps, warnings, err := manifest.LoadAll(".")
	if err != nil {
		return err
	}
	if len(warnings) > 0 {
		return fmt.Errorf("fix manifest warnings before adding apps: %v", warnings)
	}

	pin := module
	if version != "" {
		pin += "@" + version
	}
	fmt.Printf("==> pinning %s\n", pin)
	// -tool, not a plain require: no instance source imports a main package,
	// so `go mod tidy` would drop anything weaker (ADR-0031).
	get := exec.Command("go", "get", "-tool", pin)
	get.Stdout, get.Stderr = os.Stdout, os.Stderr
	if err := get.Run(); err != nil {
		return fmt.Errorf("go get -tool %s: %w", pin, err)
	}
	// Past this point go.mod has been written. Anything that fails now leaves
	// a pin behind, so say how to take it back out rather than guessing —
	// the module may have been pinned before this command ran.
	pinned := func(err error) error {
		return fmt.Errorf("%w\n%s is still pinned; `go get -tool %s@none` removes it", err, module, module)
	}

	dir, err := packageModuleDir(module)
	if err != nil {
		return pinned(err)
	}
	tmpl, err := loadManifestTemplate(dir, module)
	if err != nil {
		return pinned(err)
	}
	if slugOverride != "" {
		tmpl.Slug = slugOverride
	}
	if _, err := os.Stat(filepath.Join("apps", tmpl.Slug)); err == nil {
		return pinned(fmt.Errorf("apps/%s already exists", tmpl.Slug))
	}
	if port == 0 {
		port = 4101
		for _, a := range apps {
			if a.Port >= port {
				port = a.Port + 1
			}
		}
	}
	if port > 4999 {
		return pinned(fmt.Errorf("port range 4101-4999 exhausted"))
	}
	tmpl.Port = port

	manifestPath := filepath.Join("apps", tmpl.Slug, "app.toml")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		return pinned(err)
	}
	if err := os.WriteFile(manifestPath, []byte(renderInstalledManifest(tmpl)), 0o644); err != nil {
		return pinned(err)
	}
	// Validate what was just written the same way every other command reads
	// it, so a malformed template fails here and not mid-deploy.
	if _, err := manifest.Load(".", tmpl.Slug); err != nil {
		return pinned(fmt.Errorf("%s: %w", manifestPath, err))
	}
	fmt.Println("created", manifestPath)

	if _, err := os.Stat("tools/tailwindcss"); err == nil {
		fmt.Println("==> recompiling instance CSS (the app's templates are new scan roots)")
		if err := cmdUI(nil); err != nil {
			return err
		}
	} else {
		fmt.Println("NOTE: run `just tools` then `just ui` — until you do, this app renders unstyled")
	}

	fmt.Printf(`
%s installed on port %d from %s

  It runs as you, beside every other app's data. Nothing vetted it.
  Source: %s

next:
  1. just check
  2. just dev            → http://localhost:%d
  3. just deploy --edge  (new subdomain needs the route pushed)
`, tmpl.Slug, port, module, dir, port)
	return nil
}

// installedManifest is the subset of a manifest a published app decides for
// itself. Port belongs to the installing owner.
type installedManifest struct {
	Name        string            `toml:"name"`
	Slug        string            `toml:"slug"`
	Icon        string            `toml:"icon"`
	Description string            `toml:"description"`
	Package     string            `toml:"package"`
	Intents     []manifest.Intent `toml:"intents"`
	Port        int               `toml:"-"`
}

// loadManifestTemplate reads the app.toml.example a published app ships at its
// module root. It is the app's own statement of its slug, icon, name, and
// intents — the instance only chooses the port.
func loadManifestTemplate(dir, module string) (installedManifest, error) {
	var m installedManifest
	example := filepath.Join(dir, "app.toml.example")
	if _, err := toml.DecodeFile(example, &m); err != nil {
		if os.IsNotExist(err) {
			return m, fmt.Errorf("%s ships no app.toml.example, so it does not declare a slug or icon; "+
				"write apps/<slug>/app.toml by hand with package = %q", module, module)
		}
		return m, fmt.Errorf("read %s: %w", example, err)
	}
	if m.Package == "" {
		m.Package = module
	}
	return m, nil
}

func renderInstalledManifest(m installedManifest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Installed app — source lives in %s.\n", m.Package)
	fmt.Fprintf(&b, "# Port is this instance's choice; everything else comes from the app.\n")
	fmt.Fprintf(&b, "name        = %q\n", m.Name)
	fmt.Fprintf(&b, "slug        = %q\n", m.Slug)
	fmt.Fprintf(&b, "port        = %d\n", m.Port)
	fmt.Fprintf(&b, "icon        = %q\n", m.Icon)
	fmt.Fprintf(&b, "description = %q\n", m.Description)
	fmt.Fprintf(&b, "package     = %q\n", m.Package)
	for _, in := range m.Intents {
		accepts := in.Accepts
		if accepts == "" {
			accepts = "text"
		}
		fmt.Fprintf(&b, "\n[[intents]]\nname    = %q\ntitle   = %q\naccepts = %q\n", in.Name, in.Title, accepts)
	}
	return b.String()
}
