package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/bketelsen/bespoke/internal/manifest"
	"github.com/bketelsen/bespoke/pkg/web"
)

// startSearchApp spins a fake app server returning the given results for any
// query, and returns a manifest.App pointed at it.
func startSearchApp(t *testing.T, slug, name string, results []web.SearchResult) manifest.App {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_search" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Tailscale-User-Login") == "" {
			t.Error("identity not forwarded")
		}
		json.NewEncoder(w).Encode(map[string]any{"results": results})
	}))
	t.Cleanup(srv.Close)
	// httptest servers listen on 127.0.0.1:<port>; extract the port.
	port, _ := strconv.Atoi(srv.URL[strings.LastIndex(srv.URL, ":")+1:])
	return manifest.App{Slug: slug, Name: name, Port: port}
}

func TestAggregateSearch(t *testing.T) {
	t.Setenv("BESPOKE_BIND_IP", "127.0.0.1")
	notes := startSearchApp(t, "notes", "Notes", []web.SearchResult{{Title: "milk note", URL: "/#note-3"}})
	todo := startSearchApp(t, "todo", "Todo", []web.SearchResult{{Title: "buy milk", URL: "/task/7"}})
	empty := startSearchApp(t, "empty", "Empty", nil)

	groups := aggregateSearch(context.Background(), "me@x", "Me", "milk",
		[]manifest.App{notes, todo, empty})

	if len(groups) != 2 {
		t.Fatalf("want 2 groups, got %d: %+v", len(groups), groups)
	}
	if groups[0].Slug != "notes" || groups[1].Slug != "todo" {
		t.Fatalf("group order/slug wrong: %+v", groups)
	}
	if groups[0].Results[0].Title != "milk note" {
		t.Fatalf("notes result wrong: %+v", groups[0].Results)
	}
}
