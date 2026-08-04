package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bketelsen/bespoke/internal/manifest"
)

func generateUnits(t *testing.T) map[string]string {
	t.Helper()
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.MkdirAll("dist/gen/units", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeUnits([]manifest.App{{Slug: "notes", Port: 4101}}); err != nil {
		t.Fatal(err)
	}
	units := map[string]string{}
	entries, err := os.ReadDir("dist/gen/units")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		body, err := os.ReadFile(filepath.Join("dist/gen/units", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		units[e.Name()] = string(body)
	}
	return units
}

// Every unit reads the shared env file, so an app secret left there is in every
// app's environment. The per-unit file is the fix and must be optional, or a
// host without one fails to start.
func TestUnitsReadAPerUnitEnvFile(t *testing.T) {
	units := generateUnits(t)
	for name, want := range map[string]string{
		"bespoke-notes.service":      "EnvironmentFile=-%h/bespoke/env.d/notes",
		"bespoke-platformd.service":  "EnvironmentFile=-%h/bespoke/env.d/platformd",
		"bespoke-litestream.service": "EnvironmentFile=-%h/bespoke/env.d/litestream",
	} {
		if !strings.Contains(units[name], want) {
			t.Errorf("%s missing %q\ngot:\n%s", name, want, units[name])
		}
	}
}

func TestUnitsAreSandboxed(t *testing.T) {
	units := generateUnits(t)
	// PrivatePIDs, not ProtectProc: hidepid filters by UID and every app here
	// runs as the same user, so only a PID namespace hides siblings.
	shared := []string{
		"NoNewPrivileges=yes",
		"PrivatePIDs=yes",
		"PrivateTmp=yes",
		"PrivateDevices=yes",
		"RestrictNamespaces=yes",
		"RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX",
		"RestrictSUIDSGID=yes",
		"LockPersonality=yes",
		"SystemCallArchitectures=native",
		"SystemCallFilter=@system-service",
		"UMask=0077",
	}
	for _, name := range []string{"bespoke-notes.service", "bespoke-platformd.service", "bespoke-litestream.service"} {
		for _, want := range shared {
			if !strings.Contains(units[name], want) {
				t.Errorf("%s missing %q", name, want)
			}
		}
		if strings.Contains(units[name], "ProtectProc=") {
			t.Errorf("%s sets ProtectProc, which filters by UID and cannot separate same-user apps", name)
		}
	}
}

// A memory cap on platformd takes the whole instance down under load; a cap on
// one app takes one app down. Only the second trade is worth making.
func TestOnlyAppUnitsAreMemoryCapped(t *testing.T) {
	units := generateUnits(t)
	app := units["bespoke-notes.service"]
	for _, want := range []string{"MemoryHigh=512M", "MemoryMax=1G", "TasksMax=128"} {
		if !strings.Contains(app, want) {
			t.Errorf("app unit missing %q", want)
		}
	}
	platform := units["bespoke-platformd.service"]
	if strings.Contains(platform, "MemoryMax=") || strings.Contains(platform, "MemoryHigh=") {
		t.Errorf("platformd should not be memory-capped:\n%s", platform)
	}
	if !strings.Contains(platform, "TasksMax=512") {
		t.Error("platformd missing TasksMax=512")
	}
}

// Verified against a live unit on systemd 261: inside the sandbox the data
// directory contains only the app's own subdirectory, so a sibling's database
// is absent rather than merely unreadable.
func TestAppUnitsGetTheirOwnFilesystemScope(t *testing.T) {
	units := generateUnits(t)
	app := units["bespoke-notes.service"]
	for _, want := range []string{
		"Environment=BESPOKE_DATA=%h/bespoke/data/notes",
		"ProtectSystem=strict",
		"ProtectHome=tmpfs",
		"BindReadOnlyPaths=%h/bespoke/bin/notes",
		"BindReadOnlyPaths=%h/bespoke/apps",   // web.Run + the app switcher
		"BindReadOnlyPaths=%h/bespoke/assets", // pkg/ui serves styles.css from disk
		"BindPaths=%h/bespoke/data/notes",
	} {
		if !strings.Contains(app, want) {
			t.Errorf("app unit missing %q\ngot:\n%s", want, app)
		}
	}
	if strings.Contains(app, "BindPaths=%h/bespoke/data\n") {
		t.Error("app unit mounts the whole data directory, defeating the scope")
	}

	// platformd reads every manifest and its own database, and execs copilot
	// from ~/.local/bin; litestream reads every app database by design.
	for _, name := range []string{"bespoke-platformd.service", "bespoke-litestream.service"} {
		if strings.Contains(units[name], "ProtectHome=tmpfs") {
			t.Errorf("%s should not be filesystem-scoped; it needs the whole tree", name)
		}
	}
}

// The sandbox must not cost platformd the things ADR-0009 needs to exec copilot.
func TestPlatformdKeepsItsEnvironment(t *testing.T) {
	units := generateUnits(t)
	platform := units["bespoke-platformd.service"]
	for _, want := range []string{
		"Environment=BESPOKE_ROOT=%h/bespoke",
		"Environment=PATH=%h/.local/bin:/usr/local/bin:/usr/bin:/bin",
		"-internal ${BESPOKE_BIND_IP}:4001",
	} {
		if !strings.Contains(platform, want) {
			t.Errorf("platformd unit missing %q", want)
		}
	}
	if !strings.Contains(units["bespoke-notes.service"], "[Install]") {
		t.Error("app unit lost its [Install] section")
	}
}

// Litestream paths must follow the data into per-app directories, while the
// replica URLs stay put so replication history survives the migration.
func TestLitestreamFollowsThePerAppLayout(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.MkdirAll("dist/gen", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeLitestream([]manifest.App{{Slug: "notes", Port: 4101}}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile("dist/gen/litestream.yml")
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	for _, want := range []string{
		"path: ${BESPOKE_DATA_DIR}/notes/notes.db",
		"url: ${BESPOKE_REPLICA_URL}/notes",
		"path: ${BESPOKE_DATA_DIR}/platformd.db", // not scoped, stays at the root
	} {
		if !strings.Contains(got, want) {
			t.Errorf("litestream.yml missing %q\ngot:\n%s", want, got)
		}
	}
}
