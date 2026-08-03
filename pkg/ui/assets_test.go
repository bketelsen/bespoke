package ui

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandlerPrefersInstanceStylesheet(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "styles.css"), []byte("instance-theme"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BESPOKE_ROOT", root)
	req := httptest.NewRequest("GET", "/_bespoke/styles.css", nil)
	w := httptest.NewRecorder()
	Handler().ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "instance-theme") {
		t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
	}
}

func TestHandlerFallsBackToEmbeddedStylesheet(t *testing.T) {
	t.Setenv("BESPOKE_ROOT", t.TempDir())
	req := httptest.NewRequest("GET", "/_bespoke/styles.css", nil)
	w := httptest.NewRecorder()
	Handler().ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "--color-primary") {
		t.Fatalf("status=%d missing embedded stylesheet", w.Code)
	}
}
