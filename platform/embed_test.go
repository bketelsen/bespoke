package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeLemonade serves an OpenAI-compatible /embeddings endpoint returning a
// distinct vector per input, deliberately in reverse index order to prove
// the gateway reorders by the index field.
func fakeLemonade(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		type datum struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		}
		var data []datum
		for i := len(req.Input) - 1; i >= 0; i-- {
			data = append(data, datum{Index: i, Embedding: []float32{float32(i), 1}})
		}
		json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func embedRequest(t *testing.T, g *embedGateway, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	g.register(mux)
	req := httptest.NewRequest(http.MethodPost, "/llm/embed", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestEmbedGateway(t *testing.T) {
	srv := fakeLemonade(t)
	g := &embedGateway{base: srv.URL, model: "test-model", hc: &http.Client{Timeout: 5 * time.Second}}

	rec := embedRequest(t, g, `{"app":"notes","texts":["alpha","beta","gamma"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Model      string      `json:"model"`
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Model != "test-model" || len(out.Embeddings) != 3 {
		t.Fatalf("bad response: %+v", out)
	}
	for i, v := range out.Embeddings {
		if len(v) != 2 || v[0] != float32(i) {
			t.Errorf("embedding %d out of order or malformed: %v", i, v)
		}
	}
}

func TestEmbedGatewayUnavailable(t *testing.T) {
	g := &embedGateway{hc: http.DefaultClient}
	rec := embedRequest(t, g, `{"app":"notes","texts":["alpha"]}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503 without backend, got %d", rec.Code)
	}
}

func TestEmbedGatewayBounds(t *testing.T) {
	srv := fakeLemonade(t)
	g := &embedGateway{base: srv.URL, model: "m", hc: http.DefaultClient}

	if rec := embedRequest(t, g, `{"app":"notes","texts":[]}`); rec.Code != http.StatusBadRequest {
		t.Errorf("empty texts: want 400, got %d", rec.Code)
	}
	big, _ := json.Marshal(map[string]any{"app": "notes", "texts": []string{strings.Repeat("x", embedMaxTextSize+1)}})
	if rec := embedRequest(t, g, string(big)); rec.Code != http.StatusBadRequest {
		t.Errorf("oversized text: want 400, got %d", rec.Code)
	}
	many, _ := json.Marshal(map[string]any{"app": "notes", "texts": make([]string, embedMaxTexts+1)})
	if rec := embedRequest(t, g, string(many)); rec.Code != http.StatusBadRequest {
		t.Errorf("too many texts: want 400, got %d", rec.Code)
	}
}
