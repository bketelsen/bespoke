// Package audio is the only way apps use speech capabilities
// (docs/adr/0014-audio-service-transcription.md): a thin client for
// platformd's audio service on the internal plane. Apps are blind to the
// backend — Lemonade when configured, a clearly-marked stub until then.
package audio

import (
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

// New returns the audio client for an app. The plane address comes from
// BESPOKE_AUDIO_URL, falling back to BESPOKE_LLM_URL (same listener), then
// the local default.
func New(app string) *Client {
	return &Client{
		app:  app,
		base: cmp.Or(os.Getenv("BESPOKE_AUDIO_URL"), os.Getenv("BESPOKE_LLM_URL"), "http://127.0.0.1:4001"),
		hc:   &http.Client{Timeout: 150 * time.Second},
	}
}

type Option func(*settings)

type settings struct{ mime string }

// WithMIME sets the audio content type (default audio/webm — what the
// pkg/ui recorder produces).
func WithMIME(m string) Option {
	return func(s *settings) { s.mime = m }
}

// Transcribe converts spoken audio to text.
func (c *Client) Transcribe(ctx context.Context, r io.Reader, opts ...Option) (string, error) {
	s := settings{mime: "audio/webm"}
	for _, o := range opts {
		o(&s)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/audio/transcribe", r)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", s.mime)
	req.Header.Set("X-Bespoke-App", c.app)
	resp, err := c.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("audio service: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("audio service: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Text, nil
}
