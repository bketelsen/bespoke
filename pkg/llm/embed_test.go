package llm

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func embedClient(base string) *Client {
	return &Client{app: "test", base: base, hc: &http.Client{Timeout: 5 * time.Second}}
}

func TestEmbed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/llm/embed" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			App   string   `json:"app"`
			Kind  string   `json:"kind"`
			Texts []string `json:"texts"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.App != "test" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.Kind != "document" && req.Kind != "query" {
			http.Error(w, "bad kind "+req.Kind, http.StatusBadRequest)
			return
		}
		vecs := make([][]float32, len(req.Texts))
		for i := range vecs {
			vecs[i] = []float32{float32(i), 1}
		}
		json.NewEncoder(w).Encode(map[string]any{"model": "m", "embeddings": vecs})
	}))
	defer srv.Close()

	got, err := embedClient(srv.URL).Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != "m" || len(got.Vectors) != 2 || got.Vectors[1][0] != 1 {
		t.Fatalf("unexpected embeddings: %+v", got)
	}

	qv, err := embedClient(srv.URL).EmbedQuery(context.Background(), "find me")
	if err != nil {
		t.Fatal(err)
	}
	if len(qv.Vectors) != 1 {
		t.Fatalf("query: want 1 vector, got %+v", qv)
	}

	if got, err := embedClient(srv.URL).Embed(context.Background(), nil); err != nil || len(got.Vectors) != 0 {
		t.Errorf("empty input: want empty result, got %+v, %v", got, err)
	}
}

func TestEmbedUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "embeddings unavailable: BESPOKE_LEMONADE_URL unset", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := embedClient(srv.URL).Embed(context.Background(), []string{"a"})
	if !errors.Is(err, ErrEmbedUnavailable) {
		t.Fatalf("want ErrEmbedUnavailable, got %v", err)
	}
}

func TestCosine(t *testing.T) {
	if got := Cosine([]float32{1, 0}, []float32{1, 0}); math.Abs(got-1) > 1e-9 {
		t.Errorf("identical vectors: want 1, got %v", got)
	}
	if got := Cosine([]float32{1, 0}, []float32{0, 1}); math.Abs(got) > 1e-9 {
		t.Errorf("orthogonal vectors: want 0, got %v", got)
	}
	if got := Cosine([]float32{1, 0}, []float32{-1, 0}); math.Abs(got+1) > 1e-9 {
		t.Errorf("opposite vectors: want -1, got %v", got)
	}
	if got := Cosine([]float32{1}, []float32{1, 2}); got != 0 {
		t.Errorf("mismatched lengths: want 0, got %v", got)
	}
	if got := Cosine([]float32{0, 0}, []float32{1, 0}); got != 0 {
		t.Errorf("zero vector: want 0, got %v", got)
	}
}
