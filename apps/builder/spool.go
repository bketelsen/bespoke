package main

import (
	"cmp"
	"encoding/json"
	"os"
	"path/filepath"
)

// Spool layout per docs/design/builder-plane.md. On the app host
// BESPOKE_SPOOL is set by the unit; the dev fallback keeps `just dev`
// working (requests queue up with nothing consuming them — the UI shows
// "queued" honestly).
func spoolRoot() string {
	return cmp.Or(os.Getenv("BESPOKE_SPOOL"), "data/spool")
}

func runDir(runID string) string {
	return filepath.Join(spoolRoot(), "runs", runID)
}

// writeRequest drops a request JSON into a watched spool dir, tmp+rename so
// the path unit never sees a partial file.
func writeRequest(dir, name string, v any) error {
	if err := os.MkdirAll(dir, 0o770); err != nil {
		return err
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, name+".tmp")
	if err := os.WriteFile(tmp, data, 0o660); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, name))
}

func writeBuildRequest(runID, slug, idea, spec string) error {
	return writeRequest(filepath.Join(spoolRoot(), "build"), runID+".request.json", map[string]string{
		"run": runID, "slug": slug, "idea": idea, "spec_markdown": spec,
	})
}

func writeDeployRequest(runID, slug string) error {
	return writeRequest(filepath.Join(spoolRoot(), "deploy"), runID+".request.json", map[string]string{
		"run": runID, "slug": slug,
	})
}

// readOutcome decodes runs/<run>/<name> if present; ok=false means not yet.
func readOutcome(runID, name string, v any) bool {
	data, err := os.ReadFile(filepath.Join(runDir(runID), name))
	if err != nil {
		return false
	}
	return json.Unmarshal(data, v) == nil
}
