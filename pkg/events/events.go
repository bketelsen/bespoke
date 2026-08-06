// Package events publishes durable domain events and accesses the platformd
// notification service (ADR-0035).
package events

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/bketelsen/bespoke/pkg/auth"
)

type Notification struct {
	ID          string  `json:"id,omitempty"`
	EventID     string  `json:"event_id,omitempty"`
	Source      string  `json:"source,omitempty"`
	Title       string  `json:"title"`
	Body        string  `json:"body,omitempty"`
	AppSlug     string  `json:"app_slug,omitempty"`
	Path        string  `json:"path,omitempty"`
	URL         string  `json:"url,omitempty"`
	GroupKey    string  `json:"group_key,omitempty"`
	DedupKey    string  `json:"dedup_key,omitempty"`
	CreatedAt   string  `json:"created_at,omitempty"`
	ReadAt      *string `json:"read_at"`
	DismissedAt *string `json:"dismissed_at"`
	Retired     bool    `json:"source_retired,omitempty"`
}

type Event struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	SubjectID    string         `json:"subject_id,omitempty"`
	OccurredAt   time.Time      `json:"occurred_at"`
	Data         map[string]any `json:"data"`
	Notification *Notification  `json:"notification,omitempty"`
}

type Page struct {
	Notifications []Notification `json:"notifications"`
	Next          string         `json:"next"`
}

type Client struct {
	app  string
	base string
	hc   *http.Client
}

func New(app string) *Client {
	base := os.Getenv("BESPOKE_INTERNAL_URL")
	if base == "" {
		if bind := os.Getenv("BESPOKE_BIND_IP"); bind != "" {
			base = "http://" + bind + ":4001"
		} else {
			base = "http://127.0.0.1:4001"
		}
	}
	return &Client{app: app, base: base, hc: &http.Client{Timeout: 5 * time.Second}}
}

type causeKey struct{}

func WithCausation(ctx context.Context, eventID string) context.Context {
	return context.WithValue(ctx, causeKey{}, eventID)
}

func Causation(ctx context.Context) string {
	v, _ := ctx.Value(causeKey{}).(string)
	return v
}

func (c *Client) Publish(ctx context.Context, user auth.User, event Event) error {
	body, err := json.Marshal(map[string]any{"source": c.app, "user": user.Login, "causation_id": Causation(ctx), "event": event})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/events/publish", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("event service: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("event service: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	return nil
}

func (c *Client) List(ctx context.Context, login, after string, limit int) (Page, error) {
	q := url.Values{"user": {login}, "limit": {fmt.Sprint(limit)}}
	if after != "" {
		q.Set("after", after)
	}
	var out Page
	err := c.getJSON(ctx, "/events/notifications?"+q.Encode(), &out)
	return out, err
}

func (c *Client) Unread(ctx context.Context, login string) (int, error) {
	var out struct {
		Count int `json:"count"`
	}
	err := c.getJSON(ctx, "/events/notifications/unread?user="+url.QueryEscape(login), &out)
	return out.Count, err
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return statusError(resp)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) Mutate(ctx context.Context, login, id, action string) (int, error) {
	path := "/events/notifications/" + id + "/" + action
	if id == "" {
		path = "/events/notifications/" + action
	}
	b, _ := json.Marshal(map[string]string{"user": login})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return resp.StatusCode, statusError(resp)
	}
	return resp.StatusCode, nil
}

func statusError(resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("event service: %s: %s", resp.Status, strings.TrimSpace(string(b)))
}

// Stream opens the plane SSE feed and calls fn for every notification record.
func (c *Client) Stream(ctx context.Context, login string, fn func(Notification) error) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/events/notifications/live?user="+url.QueryEscape(login), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return statusError(resp)
	}
	s := bufio.NewScanner(resp.Body)
	s.Buffer(make([]byte, 4096), 128<<10)
	for s.Scan() {
		line := s.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var n Notification
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &n); err != nil {
			continue
		}
		if n.ID != "" {
			if err := fn(n); err != nil {
				return err
			}
		}
	}
	return s.Err()
}
