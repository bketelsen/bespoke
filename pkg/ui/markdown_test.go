package ui

import (
	"context"
	"strings"
	"testing"
)

func render(t *testing.T, src string) string {
	t.Helper()
	var b strings.Builder
	if err := Markdown(src).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestMarkdownRenders(t *testing.T) {
	out := render(t, "### Work log\n- **Project:** bespoke")
	for _, want := range []string{"<h3>Work log</h3>", "<strong>Project:</strong>", `class="prose`} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestMarkdownNeverEmitsRawHTML(t *testing.T) {
	// Block-level raw HTML: the whole block is omitted by goldmark's default.
	out := render(t, `<script>alert(1)</script>`)
	if strings.Contains(out, "<script") {
		t.Fatalf("raw HTML block leaked:\n%s", out)
	}

	// Inline raw HTML mid-sentence: tags omitted, surrounding markdown renders.
	out = render(t, `hello <img src=x onerror=alert(2)> and **bold** text`)
	for _, banned := range []string{"<img", "onerror"} {
		if strings.Contains(out, banned) {
			t.Fatalf("inline raw HTML leaked (%q):\n%s", banned, out)
		}
	}
	if !strings.Contains(out, "<strong>bold</strong>") {
		t.Errorf("markdown around inline raw HTML should still render:\n%s", out)
	}
}
