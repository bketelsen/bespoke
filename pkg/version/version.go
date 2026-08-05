// Package version reports which Bespoke release a process is running and
// whether a newer one has been published (ADR-0034). The running release comes
// from the build's module graph — an instance pins the platform in go.mod
// (ADR-0027), so nothing has to be stamped in at build time.
package version

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Module is the public platform module path (ADR-0027).
const Module = "github.com/bketelsen/bespoke"

// Dev is reported when the build carries no release pin: a working tree, a
// `go run`, or a replace/go.work override.
const Dev = "dev"

// releasesURL is GitHub's newest-release endpoint for the platform module.
const releasesURL = "https://api.github.com/repos/bketelsen/bespoke/releases/latest"

// Platform returns the pinned platform release ("v0.11.0"), or Dev.
func Platform() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return Dev
	}
	return platformFrom(info)
}

// platformFrom reads the platform's version out of a build's module graph:
// the main module in this repository, a dependency in an owner's instance.
func platformFrom(info *debug.BuildInfo) string {
	if info.Main.Path == Module {
		return release(info.Main.Version)
	}
	for _, dep := range info.Deps {
		if dep.Path != Module {
			continue
		}
		if dep.Replace != nil {
			return release(dep.Replace.Version)
		}
		return release(dep.Version)
	}
	return Dev
}

// release normalizes a module version to a release tag, or Dev when the build
// is not from a published one.
func release(v string) string {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "v") || v == "(devel)" {
		return Dev
	}
	return v
}

// Info is the version state a UI renders.
type Info struct {
	// Current is the running release, or Dev.
	Current string
	// Latest is the newest published release; empty until a check succeeds
	// and whenever checking is off.
	Latest string
	// URL is Latest's release page, empty alongside Latest.
	URL string
	// Outdated reports that Latest is a newer release than Current.
	Outdated bool
}

// Checker caches the newest published release. It is safe for concurrent use
// and never blocks a request: checks happen in the background and every
// failure is soft — an instance with no outbound network still renders its
// own version.
type Checker struct {
	current  string
	endpoint string
	client   *http.Client
	ttl      time.Duration
	// retry is how long to wait after a failed check; short enough that a
	// briefly offline host recovers, long enough not to hammer GitHub.
	retry    time.Duration
	disabled bool

	mu     sync.Mutex
	latest string
	url    string
	// next is the earliest time another check may run.
	next time.Time
	busy bool
}

// NewChecker returns a Checker for the running platform release. Dev builds
// have no release to compare against, and BESPOKE_UPDATE_CHECK=off drops the
// outbound call entirely (the footer then shows the running version alone).
func NewChecker() *Checker {
	current := Platform()
	return &Checker{
		current:  current,
		endpoint: cmp.Or(os.Getenv("BESPOKE_RELEASES_URL"), releasesURL),
		client:   &http.Client{Timeout: 10 * time.Second},
		ttl:      6 * time.Hour,
		retry:    15 * time.Minute,
		disabled: current == Dev || os.Getenv("BESPOKE_UPDATE_CHECK") == "off",
	}
}

// Info returns what the last successful check found, refreshing in the
// background when the cached answer has expired. It never blocks on the
// network, so the first dashboard render after a restart shows the running
// version and picks up the update notice on the next one.
func (c *Checker) Info() Info {
	if c.disabled {
		return Info{Current: c.current}
	}
	c.mu.Lock()
	info := Info{Current: c.current, Latest: c.latest, URL: c.url}
	stale := !c.busy && time.Now().After(c.next)
	if stale {
		c.busy = true
	}
	c.mu.Unlock()
	if stale {
		go c.refresh(context.Background())
	}
	info.Outdated = newer(info.Latest, info.Current)
	return info
}

func (c *Checker) refresh(ctx context.Context) {
	latest, url, err := c.fetch(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.busy = false
	if err != nil {
		c.next = time.Now().Add(c.retry)
		log.Printf("version: update check failed: %v", err)
		return
	}
	c.next = time.Now().Add(c.ttl)
	c.latest, c.url = latest, url
}

func (c *Checker) fetch(ctx context.Context) (tag, url string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("%s: %s", c.endpoint, resp.Status)
	}
	var body struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return "", "", err
	}
	tag = strings.TrimSpace(body.TagName)
	if !strings.HasPrefix(tag, "v") {
		return "", "", fmt.Errorf("unexpected release tag %q", body.TagName)
	}
	return tag, strings.TrimSpace(body.HTMLURL), nil
}

// newer reports whether release tag a is a later version than b. Anything it
// cannot parse compares as "not newer" — an unreadable tag must never nag.
func newer(a, b string) bool {
	an, apre, aok := parse(a)
	bn, bpre, bok := parse(b)
	if !aok || !bok {
		return false
	}
	for i := range an {
		if an[i] != bn[i] {
			return an[i] > bn[i]
		}
	}
	// Same numbers: a prerelease is older than the release it precedes.
	if apre == bpre {
		return false
	}
	return bpre != "" && (apre == "" || apre > bpre)
}

// parse splits a "vMAJOR.MINOR.PATCH[-prerelease][+build]" tag.
func parse(tag string) (nums [3]int, pre string, ok bool) {
	v, found := strings.CutPrefix(strings.TrimSpace(tag), "v")
	if !found {
		return nums, "", false
	}
	v, _, _ = strings.Cut(v, "+")
	v, pre, _ = strings.Cut(v, "-")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return nums, "", false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nums, "", false
		}
		nums[i] = n
	}
	return nums, pre, true
}
