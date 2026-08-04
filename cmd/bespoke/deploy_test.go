package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareBuildModfileIsIsolated(t *testing.T) {
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
	if err := os.WriteFile("go.sum", []byte("example.com/dependency v1.0.0 h1:test\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	modfile, cleanup, err := prepareBuildModfile()
	if err != nil {
		t.Fatal(err)
	}
	sumfile := filepath.Join(filepath.Dir(modfile), "go.sum")
	// Outside the instance: `bespoke dev` holds one open for a whole session,
	// where an in-repo copy would dirty git status and outlive a kill -9.
	if strings.HasPrefix(modfile, dir+string(filepath.Separator)) {
		t.Fatalf("modfile %q was created inside the instance %q", modfile, dir)
	}
	if got, err := os.ReadFile(modfile); err != nil || string(got) != "module example.com/instance\n" {
		t.Fatalf("modfile = %q, %v", got, err)
	}
	if got, err := os.ReadFile(sumfile); err != nil || string(got) != "example.com/dependency v1.0.0 h1:test\n" {
		t.Fatalf("sumfile = %q, %v", got, err)
	}

	cleanup()
	if _, err := os.Stat(modfile); !os.IsNotExist(err) {
		t.Fatalf("modfile remains after cleanup: %v", err)
	}
	if _, err := os.Stat(sumfile); !os.IsNotExist(err) {
		t.Fatalf("sumfile remains after cleanup: %v", err)
	}
}
