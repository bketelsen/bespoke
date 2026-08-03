package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bketelsen/bespoke/pkg/auth"
)

func TestSearchEndpoint(t *testing.T) {
	inner := http.NewServeMux()
	Search(inner, func(ctx context.Context, user auth.User, q string) ([]SearchResult, error) {
		if user.Login != "test@example" {
			t.Errorf("provider got login %q", user.Login)
		}
		if q != "milk" {
			t.Errorf("provider got q %q", q)
		}
		return []SearchResult{{Title: "Buy milk", URL: "/task/1"}}, nil
	})
	mux := auth.Middleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/_search?q=milk", nil)
	req.Header.Set("Tailscale-User-Login", "test@example")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var got struct {
		Results []SearchResult `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("bad json: %v (%s)", err, rec.Body.String())
	}
	if len(got.Results) != 1 || got.Results[0].Title != "Buy milk" || got.Results[0].URL != "/task/1" {
		t.Fatalf("unexpected results: %+v", got.Results)
	}
}

func TestSearchEndpointEmpty(t *testing.T) {
	inner := http.NewServeMux()
	Search(inner, func(ctx context.Context, user auth.User, q string) ([]SearchResult, error) {
		return nil, nil
	})
	mux := auth.Middleware(inner)
	req := httptest.NewRequest(http.MethodGet, "/_search?q=x", nil)
	req.Header.Set("Tailscale-User-Login", "test@example")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if body := rec.Body.String(); body != "{\"results\":[]}\n" {
		t.Fatalf("empty search body = %q", body)
	}
}
