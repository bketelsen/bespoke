package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
