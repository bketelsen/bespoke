package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
)

// ErrEmbedUnavailable reports that the gateway has no embeddings backend
// (BESPOKE_LEMONADE_URL unset on platformd). Callers MUST degrade — switch
// semantic features off and keep lexical paths working — rather than fail
// the surrounding feature (ADR-0029).
var ErrEmbedUnavailable = errors.New("embeddings unavailable")

// Embed returns one vector per text, in input order, via the gateway's
// Lemonade-backed embeddings endpoint (ADR-0029). Rank with llm.Cosine.
// Mechanical like Classify — no options, never user-brief-tagged. At most
// 64 texts of 8KB each per call; embedding is best-effort by design, so a
// failed Embed should never fail the write that triggered it.
func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	body, _ := json.Marshal(map[string]any{"app": c.app, "texts": texts})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/llm/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm gateway: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusServiceUnavailable {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("%w: %s", ErrEmbedUnavailable, strings.TrimSpace(string(msg)))
	}
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("llm gateway: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	var out struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Embeddings) != len(texts) {
		return nil, fmt.Errorf("llm gateway: got %d embeddings for %d texts", len(out.Embeddings), len(texts))
	}
	return out.Embeddings, nil
}

// Cosine returns the cosine similarity of two vectors in [-1, 1], or 0 for
// mismatched or zero-magnitude inputs. The one ranking function every app
// uses, so scores stay comparable across the platform (ADR-0029).
func Cosine(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
