package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An app installed from another module keeps its templates outside the
// instance tree; if they are not scanned, Tailwind prunes every class the app
// uses and it renders unstyled.
func TestUIInputCSSScansEverySourceRoot(t *testing.T) {
	css := uiInputCSS("/plat/design/base.css", "/home/design/theme.css",
		[]string{"/home/apps", "/cache/example.com/friend/cool-app@v1.0.0"})
	for _, want := range []string{
		`@import "/plat/design/base.css";`,
		`@import "/home/design/theme.css";`,
		`@source "/home/apps/**/*.templ";`,
		`@source "/home/apps/**/*_templ.go";`,
		`@source "/cache/example.com/friend/cool-app@v1.0.0/**/*.templ";`,
		`@source "/cache/example.com/friend/cool-app@v1.0.0/**/*_templ.go";`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("generated CSS missing %s\ngot:\n%s", want, css)
		}
	}
	if i, j := strings.Index(css, "base.css"), strings.Index(css, "theme.css"); i > j {
		t.Error("theme tokens must be imported after the platform base layer")
	}
}

func TestPackageModuleDirResolvesToModuleRoot(t *testing.T) {
	dir, err := packageModuleDir(platformModule + "/apps/builder")
	if err != nil {
		t.Fatal(err)
	}
	// The module root, not the package directory: a shared app's templates
	// may live above its main package.
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		t.Fatalf("packageModuleDir returned %q, which has no go.mod: %v", dir, err)
	}
}

func TestPackageModuleDirRejectsUnknownPackage(t *testing.T) {
	if _, err := packageModuleDir("example.invalid/not/a/package"); err == nil {
		t.Fatal("resolving an unrequired package succeeded")
	} else if !strings.Contains(err.Error(), "go get") {
		t.Errorf("error should tell the owner how to fix it, got: %v", err)
	}
}

func TestInitCreatesPinnedInstance(t *testing.T) {
	root := filepath.Join(t.TempDir(), "home")
	if err := cmdInit([]string{root, "--module", "example.com/me/home", "--platform-version", "v0.1.0", "--with-builder"}); err != nil {
		t.Fatal(err)
	}
	checks := map[string]string{
		"go.mod":                 "tool github.com/bketelsen/bespoke/cmd/bespoke",
		"apps/notes/app.toml":    `slug = "notes"`,
		"apps/todo/app.toml":     `slug = "todo"`,
		"apps/builder/app.toml":  `package = "github.com/bketelsen/bespoke/apps/builder"`,
		"design/theme.css":       "--primary",
		"scripts/setup-tools.sh": "tailwindcss-macos-arm64",
	}
	for path, want := range checks {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if !strings.Contains(string(data), want) {
			t.Errorf("%s does not contain %q", path, want)
		}
	}
	if err := cmdInit([]string{root, "--module", "example.com/me/home", "--platform-version", "v0.1.0"}); err == nil {
		t.Fatal("second init of non-empty directory succeeded")
	}
}

func TestInitDevBuildRequiresVersion(t *testing.T) {
	old := version
	version = "dev"
	t.Cleanup(func() { version = old })
	err := cmdInit([]string{filepath.Join(t.TempDir(), "home"), "--module", "example.com/me/home"})
	if err == nil || !strings.Contains(err.Error(), "--platform-version") {
		t.Fatalf("got %v", err)
	}
}
