package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/bketelsen/bespoke/pkg/auth"
)

var testSkillsFS = fstest.MapFS{
	"gardening/SKILL.md": {Data: []byte(
		"---\nname: gardening\ndescription: Tidy the wiki.\n---\n\n# Gardening\n\nFind stubs and fill them.\n")},
	"authoring/SKILL.md": {Data: []byte(
		"---\nname: authoring\ndescription: Write good pages.\n---\n\n# Authoring\n\nLink liberally.\n")},
	"bare/SKILL.md": {Data: []byte("# No frontmatter\n\nStill a skill.\n")},
}

func TestParseSkills(t *testing.T) {
	skills, err := parseSkills(testSkillsFS)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 3 {
		t.Fatalf("got %d skills, want 3", len(skills))
	}
	// Sorted by name; "bare" falls back to its directory name.
	if skills[0].Name != "authoring" || skills[1].Name != "bare" || skills[2].Name != "gardening" {
		t.Errorf("order/names wrong: %+v", skills)
	}
	if skills[2].Description != "Tidy the wiki." {
		t.Errorf("description = %q", skills[2].Description)
	}
	if !strings.Contains(skills[0].Body, "Link liberally") || strings.Contains(skills[0].Body, "---") {
		t.Errorf("body not cleanly separated from frontmatter: %q", skills[0].Body)
	}
	if !strings.Contains(skills[1].Body, "Still a skill") {
		t.Errorf("frontmatterless body lost: %q", skills[1].Body)
	}
}

func TestSkillsEndpointsAndTool(t *testing.T) {
	inner := http.NewServeMux()
	if err := Skills(inner, testSkillsFS); err != nil {
		t.Fatal(err)
	}
	// Tool handlers read identity from context, same as in production.
	mux := auth.Middleware(inner)
	identified := func(method, target string, body string) *http.Request {
		var r *http.Request
		if body == "" {
			r = httptest.NewRequest(method, target, nil)
		} else {
			r = httptest.NewRequest(method, target, strings.NewReader(body))
		}
		r.Header.Set("Tailscale-User-Login", "test@example")
		return r
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, identified(http.MethodGet, "/_skills", ""))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "gardening") {
		t.Errorf("/_skills: %d %q", rec.Code, rec.Body.String())
	}

	// The loader tool serves the body and rejects unknown names.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, identified(http.MethodPost, "/_tools/load_skill", `{"name":"gardening"}`))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Find stubs") {
		t.Errorf("load skill: %d %q", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, identified(http.MethodPost, "/_tools/load_skill", `{"name":"nope"}`))
	if rec.Code == http.StatusOK {
		t.Error("unknown skill should error")
	}

	// The tool's advertised description carries the index.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, identified(http.MethodGet, "/_tools", ""))
	if !strings.Contains(rec.Body.String(), "gardening — Tidy the wiki.") {
		t.Errorf("tool listing missing skill index: %q", rec.Body.String())
	}
}

func TestSkillsEmptyFS(t *testing.T) {
	if err := Skills(http.NewServeMux(), fstest.MapFS{}); err == nil {
		t.Error("empty skills FS should error")
	}
}
