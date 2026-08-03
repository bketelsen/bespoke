package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/bketelsen/bespoke/internal/manifest"
	"github.com/bketelsen/bespoke/pkg/web"
)

func TestSearchToolFormatting(t *testing.T) {
	t.Setenv("BESPOKE_BIND_IP", "127.0.0.1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"results": []web.SearchResult{
			{Title: "buy milk", URL: "/#task-3"},
		}})
	}))
	defer srv.Close()
	port, _ := strconv.Atoi(srv.URL[strings.LastIndex(srv.URL, ":")+1:])
	apps := []manifest.App{{Slug: "todo", Name: "Todo", Port: port}}

	out := formatSearchGroups(aggregateSearch(context.Background(), "me@x", "Me", "milk", true, "example.test", apps))
	wantURL := fmt.Sprintf("http://localhost:%d/#task-3", port)
	if !strings.Contains(out, "Todo") || !strings.Contains(out, "buy milk") || !strings.Contains(out, wantURL) {
		t.Fatalf("bad tool output: %q", out)
	}

	if got := formatSearchGroups(nil); !strings.Contains(got, "no results") {
		t.Errorf("empty output = %q", got)
	}
}
