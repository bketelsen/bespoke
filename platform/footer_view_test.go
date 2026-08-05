package main

import (
	"context"
	"strings"
	"testing"

	"github.com/bketelsen/bespoke/pkg/version"
	"github.com/bketelsen/bespoke/platform/views"
)

func renderFooter(t *testing.T, info version.Info) string {
	t.Helper()
	var sb strings.Builder
	if err := views.Footer(info).Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	return sb.String()
}

func TestFooterShowsRunningVersion(t *testing.T) {
	out := renderFooter(t, version.Info{Current: "v0.11.0"})
	if !strings.Contains(out, "Bespoke v0.11.0") {
		t.Fatalf("footer omits the running version:\n%s", out)
	}
	if strings.Contains(out, "available") {
		t.Fatalf("footer nags an up-to-date instance:\n%s", out)
	}
}

func TestFooterLinksNewerRelease(t *testing.T) {
	out := renderFooter(t, version.Info{
		Current:  "v0.11.0",
		Latest:   "v0.12.0",
		URL:      "https://github.com/bketelsen/bespoke/releases/tag/v0.12.0",
		Outdated: true,
	})
	if !strings.Contains(out, "v0.12.0 available") {
		t.Fatalf("footer omits the update notice:\n%s", out)
	}
	if !strings.Contains(out, "https://github.com/bketelsen/bespoke/releases/tag/v0.12.0") {
		t.Fatalf("footer omits the release link:\n%s", out)
	}
}

func TestFooterDevBuild(t *testing.T) {
	out := renderFooter(t, version.Info{Current: version.Dev})
	if !strings.Contains(out, "Bespoke dev") {
		t.Fatalf("footer omits the dev version:\n%s", out)
	}
}
