// Package llm is the only way apps run LLM inference
// (docs/adr/0009-copilot-sdk-llm-gateway.md): a thin, provider-neutral client
// for platformd's gateway. Apps never talk to a model provider directly.
package llm

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type Client struct {
	app  string
	base string
	hc   *http.Client
}

// New returns the gateway client for an app. The gateway address comes from
// BESPOKE_LLM_URL (set in the production env file); the default matches
// platformd's local internal listener.
func New(app string) *Client {
	return &Client{
		app:  app,
		base: cmp.Or(os.Getenv("BESPOKE_LLM_URL"), "http://127.0.0.1:4001"),
		hc:   &http.Client{Timeout: 150 * time.Second},
	}
}

type Option func(*request)

type request struct {
	App    string `json:"app"`
	System string `json:"system,omitempty"`
	Prompt string `json:"prompt"`
	Login  string `json:"login,omitempty"`
}

// WithSystem adds system instructions to a completion.
func WithSystem(s string) Option {
	return func(r *request) { r.System = s }
}

// WithUser tags the completion with the requesting user's login; the
// gateway prepends that user's self-provided brief (ADR-0019). Use for
// anything user-facing (chat, summaries); omit for mechanical calls.
func WithUser(login string) Option {
	return func(r *request) { r.Login = login }
}

// Complete returns the model's text response for prompt.
func (c *Client) Complete(ctx context.Context, prompt string, opts ...Option) (string, error) {
	req := request{App: c.app, Prompt: prompt}
	for _, o := range opts {
		o(&req)
	}
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/llm/complete", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("llm gateway: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("llm gateway: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Text, nil
}

// CompleteJSON asks for JSON only and unmarshals the response into out
// (pass a pointer). Markdown fences are stripped if the model adds them.
func (c *Client) CompleteJSON(ctx context.Context, prompt string, out any, opts ...Option) error {
	opts = append(opts, WithSystem(
		"Respond with a single valid JSON value and nothing else: no prose, no markdown fences."))
	text, err := c.Complete(ctx, prompt, opts...)
	if err != nil {
		return err
	}
	cleaned := strings.TrimSpace(text)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	if err := json.Unmarshal([]byte(strings.TrimSpace(cleaned)), out); err != nil {
		return fmt.Errorf("llm returned non-JSON (%w): %.200s", err, text)
	}
	return nil
}

// Healthy reports whether the gateway is up and authenticated.
func (c *Client) Healthy(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/llm/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("%s", strings.TrimSpace(string(msg)))
	}
	return nil
}
