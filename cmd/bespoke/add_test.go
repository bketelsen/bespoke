package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

const testIndex = `
[[app]]
module      = "github.com/someone/bespoke-app-recipes"
name        = "Recipes"
description = "Dinner, tracked"
author      = "someone"
source      = "https://github.com/someone/bespoke-app-recipes"

[[app]]
module      = "codeberg.org/other/bespoke-app-birds"
name        = "Birds"
description = "Yard sightings"
author      = "other"
source      = "https://codeberg.org/other/bespoke-app-birds"
`

func serveIndex(t *testing.T, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("BESPOKE_INDEX", srv.URL)
}

func TestFetchIndexSortsByShortName(t *testing.T) {
	serveIndex(t, testIndex)
	entries, err := fetchIndex()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	// birds sorts before recipes even though it is second in the file.
	if got := entries[0].shortName(); got != "birds" {
		t.Errorf("first entry short name = %q, want birds", got)
	}
	if got := entries[1].shortName(); got != "recipes" {
		t.Errorf("second entry short name = %q, want recipes", got)
	}
}

// The short name is what someone types, so the bespoke-app- convention must
// come off — and a module not following it still has to resolve to something.
func TestShortName(t *testing.T) {
	for module, want := range map[string]string{
		"github.com/bketelsen/bespoke-app-gh-tracker": "gh-tracker",
		"github.com/someone/recipes":                  "recipes",
		"example.com/deep/path/bespoke-app-x":         "x",
	} {
		if got := (indexEntry{Module: module}).shortName(); got != want {
			t.Errorf("shortName(%q) = %q, want %q", module, got, want)
		}
	}
}

func TestIndexURLPrefersEnv(t *testing.T) {
	t.Setenv("BESPOKE_INDEX", "")
	if indexURL() != defaultIndexURL {
		t.Errorf("empty BESPOKE_INDEX should fall back to the default, got %q", indexURL())
	}
	t.Setenv("BESPOKE_INDEX", "https://example.com/mine.toml")
	if indexURL() != "https://example.com/mine.toml" {
		t.Errorf("BESPOKE_INDEX ignored, got %q", indexURL())
	}
}

func TestAddRejectsBadInputBeforeTouchingGoMod(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.WriteFile("go.mod", []byte("module example.com/instance\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string][]string{
		"bad slug":       {"example.com/x", "--slug", "Not A Slug"},
		"low port":       {"example.com/x", "--port", "80"},
		"high port":      {"example.com/x", "--port", "5000"},
		"no target":      {},
		"unknown flag":   {"example.com/x", "--nope"},
		"missing value":  {"example.com/x", "--slug"},
		"port not a num": {"example.com/x", "--port", "abc"},
	}
	for name, args := range cases {
		if err := cmdAdd(args); err == nil {
			t.Errorf("%s: cmdAdd(%q) succeeded, want error", name, args)
		}
	}
	after, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("go.mod was modified by a rejected add:\n%s", after)
	}
}

func TestAddUnknownIndexNameSuggestsModulePath(t *testing.T) {
	serveIndex(t, testIndex)
	err := cmdAdd([]string{"nosuchapp"})
	if err == nil {
		t.Fatal("adding an unindexed short name succeeded")
	}
	if !strings.Contains(err.Error(), "full module path") {
		t.Errorf("error should point at the escape hatch, got: %v", err)
	}
}

func TestRenderInstalledManifestRoundTrips(t *testing.T) {
	m := installedManifest{
		Name: "Recipes", Slug: "recipes", Icon: "chef-hat",
		Description: `Dinner, "tracked"`, Package: "github.com/someone/bespoke-app-recipes",
		Port: 4107,
	}
	got := renderInstalledManifest(m)
	for _, want := range []string{
		`name        = "Recipes"`,
		`slug        = "recipes"`,
		`port        = 4107`,
		`package     = "github.com/someone/bespoke-app-recipes"`,
		`description = "Dinner, \"tracked\""`, // quoting, not string concatenation
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered manifest missing %s\ngot:\n%s", want, got)
		}
	}
}
