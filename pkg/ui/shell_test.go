package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/bketelsen/bespoke/pkg/auth"
)

func TestAppShellWidths(t *testing.T) {
	tests := []struct {
		name  string
		width ShellWidth
		class string
	}{
		{name: "zero value is reading width", class: "max-w-3xl"},
		{name: "explicit reading width", width: ShellWidthReading, class: "max-w-3xl"},
		{name: "wide", width: ShellWidthWide, class: "max-w-7xl"},
		{name: "full", width: ShellWidthFull, class: "max-w-none"},
		{name: "unknown values are safe", width: ShellWidth("unknown"), class: "max-w-3xl"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder
			err := AppShell(ShellProps{
				Title: "Test",
				User:  auth.User{Name: "Test User"},
				Width: tt.width,
			}).Render(context.Background(), &out)
			if err != nil {
				t.Fatal(err)
			}
			// Header and main use the same width so their edges remain aligned.
			html := out.String()
			for _, prefix := range []string{
				`class="mx-auto flex items-center justify-between px-4 py-3 `,
				`class="mx-auto px-4 py-8 `,
			} {
				if !strings.Contains(html, prefix+tt.class+`"`) {
					t.Fatalf("expected class list %q", prefix+tt.class)
				}
			}
		})
	}
}
