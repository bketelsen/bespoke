package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	copilot "github.com/github/copilot-sdk/go"
)

func fakeBrave(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	old := braveSearchURL
	braveSearchURL = srv.URL
	t.Cleanup(func() { braveSearchURL = old })
}

func TestBraveSearch(t *testing.T) {
	fakeBrave(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Subscription-Token") != "tok" {
			t.Errorf("missing subscription token, headers: %v", r.Header)
		}
		if q := r.URL.Query().Get("q"); q != "go releases" {
			t.Errorf("query = %q", q)
		}
		w.Write([]byte(`{"web":{"results":[
			{"title":"Go 1.26","url":"https://go.dev/dl/","description":"Downloads"},
			{"title":"Release notes","url":"https://go.dev/doc/","description":"Docs"}]}}`))
	})
	out, err := braveSearch("tok", "go releases")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Go 1.26", "https://go.dev/dl/", "2. Release notes"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestBraveSearchErrors(t *testing.T) {
	fakeBrave(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"quota"}`, http.StatusTooManyRequests)
	})
	if _, err := braveSearch("tok", "x"); err == nil {
		t.Error("non-200 should error")
	}

	fakeBrave(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"web":{"results":[]}}`))
	})
	out, err := braveSearch("tok", "obscure")
	if err != nil || !strings.Contains(out, "No results") {
		t.Errorf("empty results: out=%q err=%v", out, err)
	}
}

func TestBraveSearchToolHandler(t *testing.T) {
	fakeBrave(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"web":{"results":[{"title":"T","url":"https://example.com","description":"D"}]}}`))
	})
	tool := braveSearchTool("tok")
	if tool.Name != "web_search" {
		t.Fatalf("tool name = %q", tool.Name)
	}

	res, err := tool.Handler(copilot.ToolInvocation{Arguments: map[string]any{"query": "anything"}})
	if err != nil || res.ResultType != "success" || !strings.Contains(res.TextResultForLLM, "example.com") {
		t.Errorf("success case: %+v err=%v", res, err)
	}

	res, err = tool.Handler(copilot.ToolInvocation{Arguments: map[string]any{"query": "  "}})
	if err != nil || res.ResultType != "failure" {
		t.Errorf("empty query should be a tool-level failure: %+v err=%v", res, err)
	}
}
