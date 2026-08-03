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

// Embedded is the result of an embedding call: one vector per input text,
// in input order, plus the backend model that produced them. Store Model
// alongside vectors — a model change invalidates them (ADR-0029).
type Embedded struct {
	Model   string
	Vectors [][]float32
}

// Embed embeds documents for storage and later ranking with llm.Cosine
// (ADR-0029). Mechanical like Classify — no options, never
// user-brief-tagged. At most 64 texts of 8KB each per call; embedding is
// best-effort by design, so a failed Embed should never fail the write
// that triggered it. Returns ErrEmbedUnavailable (wrapped) when the
// gateway has no backend.
func (c *Client) Embed(ctx context.Context, texts []string) (*Embedded, error) {
	return c.embed(ctx, "document", texts)
}

// EmbedQuery embeds a search query for ranking against stored document
// vectors (retrieval models treat queries and documents asymmetrically;
// the gateway applies the model's task prefixes). One text in, one vector
// out at Vectors[0].
func (c *Client) EmbedQuery(ctx context.Context, q string) (*Embedded, error) {
	return c.embed(ctx, "query", []string{q})
}

func (c *Client) embed(ctx context.Context, kind string, texts []string) (*Embedded, error) {
	if len(texts) == 0 {
		return &Embedded{}, nil
	}
	body, _ := json.Marshal(map[string]any{"app": c.app, "kind": kind, "texts": texts})
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
		Model      string      `json:"model"`
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Embeddings) != len(texts) {
		return nil, fmt.Errorf("llm gateway: got %d embeddings for %d texts", len(out.Embeddings), len(texts))
	}
	return &Embedded{Model: out.Model, Vectors: out.Embeddings}, nil
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
