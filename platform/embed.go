// The embeddings gateway (ADR-0029): POST /llm/embed on the internal 4001
// plane, backed by Lemonade's OpenAI-compatible /embeddings endpoint when
// BESPOKE_LEMONADE_URL is set. No stub mode — fake vectors would silently
// poison stored indexes, so without a backend the endpoint returns 503 and
// pkg/llm surfaces ErrEmbedUnavailable for callers to degrade on.
package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	embedMaxTexts    = 64
	embedMaxTextSize = 8 << 10
)

type embedGateway struct {
	base  string // Lemonade OpenAI-compatible base URL; empty = unavailable
	model string
	hc    *http.Client
}

func newEmbedGateway() *embedGateway {
	return &embedGateway{
		base:  strings.TrimSuffix(os.Getenv("BESPOKE_LEMONADE_URL"), "/"),
		model: cmp.Or(os.Getenv("BESPOKE_EMBED_MODEL"), "nomic-embed-text-v2-moe"),
		hc:    &http.Client{Timeout: 60 * time.Second},
	}
}

func (g *embedGateway) register(mux *http.ServeMux) {
	mux.HandleFunc("POST /llm/embed", func(w http.ResponseWriter, r *http.Request) {
		if g.base == "" {
			http.Error(w, "embeddings unavailable: BESPOKE_LEMONADE_URL unset", http.StatusServiceUnavailable)
			return
		}
		var req struct {
			App   string   `json:"app"`
			Texts []string `json:"texts"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil || len(req.Texts) == 0 {
			http.Error(w, "bad request: need {app, texts}", http.StatusBadRequest)
			return
		}
		if len(req.Texts) > embedMaxTexts {
			http.Error(w, fmt.Sprintf("too many texts (max %d per call)", embedMaxTexts), http.StatusBadRequest)
			return
		}
		for i, t := range req.Texts {
			if len(t) > embedMaxTextSize {
				http.Error(w, fmt.Sprintf("text %d too large (max %dB)", i, embedMaxTextSize), http.StatusBadRequest)
				return
			}
		}
		start := time.Now()
		vecs, err := g.embed(r.Context(), req.Texts)
		log.Printf("embed app=%s texts=%d dur=%s err=%v",
			req.App, len(req.Texts), time.Since(start).Round(time.Millisecond), err)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"model": g.model, "embeddings": vecs})
	})
}

// embed calls Lemonade's OpenAI-compatible embeddings endpoint and returns
// one vector per input, in input order (the response's index field is
// authoritative, not its array position).
func (g *embedGateway) embed(ctx context.Context, texts []string) ([][]float32, error) {
	body, _ := json.Marshal(map[string]any{"model": g.model, "input": texts})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.base+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lemonade: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("lemonade: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	var out struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("lemonade: %w", err)
	}
	if len(out.Data) != len(texts) {
		return nil, fmt.Errorf("lemonade: got %d embeddings for %d texts", len(out.Data), len(texts))
	}
	vecs := make([][]float32, len(texts))
	for _, d := range out.Data {
		if d.Index < 0 || d.Index >= len(vecs) || vecs[d.Index] != nil {
			return nil, fmt.Errorf("lemonade: bad embedding index %d", d.Index)
		}
		if len(d.Embedding) == 0 {
			return nil, fmt.Errorf("lemonade: empty embedding at index %d", d.Index)
		}
		vecs[d.Index] = d.Embedding
	}
	return vecs, nil
}
