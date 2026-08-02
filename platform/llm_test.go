package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestActivityEndpoint(t *testing.T) {
	g := newLLMGateway()
	mux := http.NewServeMux()
	g.register(mux)

	get := func() (inflight, idle int) {
		t.Helper()
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/llm/activity", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("activity status = %d", rec.Code)
		}
		var body struct {
			Inflight    int `json:"inflight"`
			IdleSeconds int `json:"idle_seconds"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return body.Inflight, body.IdleSeconds
	}

	if inflight, _ := get(); inflight != 0 {
		t.Fatalf("idle gateway reports inflight=%d, want 0", inflight)
	}

	g.begin()
	inflight, idle := get()
	if inflight != 1 {
		t.Fatalf("during completion inflight=%d, want 1", inflight)
	}
	if idle != 0 {
		t.Fatalf("during completion idle_seconds=%d, want 0", idle)
	}

	g.end()
	g.mu.Lock()
	g.lastDone = time.Now().Add(-42 * time.Second)
	g.mu.Unlock()
	inflight, idle = get()
	if inflight != 0 || idle < 42 {
		t.Fatalf("after completion inflight=%d idle=%d, want 0 and >=42", inflight, idle)
	}
}
