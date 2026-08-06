package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/bketelsen/bespoke/pkg/auth"
	"github.com/bketelsen/bespoke/pkg/events"
)

func TestToolAdvertisesAutomationAndPropagatesContext(t *testing.T) {
	toolsMu.Lock()
	toolDefs = nil
	toolsMu.Unlock()
	toolsOnce = sync.Once{}
	mux := http.NewServeMux()
	Tool(mux, ToolDef{Name: "create", Automation: AutomationPolicy{Mode: AutomationIdempotent}, Schema: map[string]any{"type": "object"}, Handler: func(ctx context.Context, u auth.User, args json.RawMessage) (string, error) {
		key, ok := IdempotencyKey(ctx)
		if !ok || key != "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" {
			t.Errorf("key=%q ok=%v", key, ok)
		}
		if got := events.Causation(ctx); got != "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb" {
			t.Errorf("cause=%q", got)
		}
		return "ok", nil
	}})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/_tools", nil))
	if !strings.Contains(w.Body.String(), `"automation":"idempotent"`) {
		t.Fatalf("listing=%s", w.Body.String())
	}
	r := httptest.NewRequest("POST", "/_tools/create", strings.NewReader("{}"))
	r.Header.Set("Tailscale-User-Login", "a")
	r.Header.Set("Idempotency-Key", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	r.Header.Set("Bespoke-Causation-ID", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
	w = httptest.NewRecorder()
	auth.Middleware(mux).ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("status=%d %s", w.Code, w.Body.String())
	}
}
