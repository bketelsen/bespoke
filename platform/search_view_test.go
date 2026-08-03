package main

import (
	"context"
	"strings"
	"testing"

	"github.com/bketelsen/bespoke/pkg/auth"
	"github.com/bketelsen/bespoke/platform/views"
)

func TestSearchResultsView(t *testing.T) {
	var sb strings.Builder
	g := views.Group{Name: "Notes", Base: "http://localhost:4102/", Results: []views.Result{{Title: "milk", URL: "/#note-1"}}}
	err := views.SearchResults(auth.User{Login: "me@x"}, true, "x.example", "milk", []views.Group{g}).Render(context.Background(), &sb)
	if err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	if !strings.Contains(out, "Notes (1)") {
		t.Errorf("missing group heading: %s", out)
	}
	if !strings.Contains(out, "http://localhost:4102/#note-1") {
		t.Errorf("missing resolved deep link: %s", out)
	}
}
