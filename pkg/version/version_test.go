package version

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime/debug"
	"testing"
	"time"
)

func TestPlatformFrom(t *testing.T) {
	cases := []struct {
		name string
		info *debug.BuildInfo
		want string
	}{
		{
			name: "instance pins the platform as a dependency",
			info: &debug.BuildInfo{
				Main: debug.Module{Path: "example.com/me/home", Version: "(devel)"},
				Deps: []*debug.Module{{Path: Module, Version: "v0.11.0"}},
			},
			want: "v0.11.0",
		},
		{
			name: "replaced dependency reports the replacement",
			info: &debug.BuildInfo{
				Main: debug.Module{Path: "example.com/me/home", Version: "(devel)"},
				Deps: []*debug.Module{{
					Path:    Module,
					Version: "v0.11.0",
					Replace: &debug.Module{Path: "../bespoke", Version: ""},
				}},
			},
			want: Dev,
		},
		{
			name: "platform repository built from source",
			info: &debug.BuildInfo{Main: debug.Module{Path: Module, Version: "(devel)"}},
			want: Dev,
		},
		{
			name: "released platform binary",
			info: &debug.BuildInfo{Main: debug.Module{Path: Module, Version: "v0.9.1"}},
			want: "v0.9.1",
		},
		{
			name: "platform missing from the module graph",
			info: &debug.BuildInfo{Main: debug.Module{Path: "example.com/me/home"}},
			want: Dev,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := platformFrom(tc.info); got != tc.want {
				t.Fatalf("platformFrom = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"v0.12.0", "v0.11.0", true},
		{"v0.11.1", "v0.11.0", true},
		{"v1.0.0", "v0.99.99", true},
		{"v0.11.0", "v0.11.0", false},
		{"v0.10.0", "v0.11.0", false},
		{"v0.11.0", "v0.11.0-rc.1", true},
		{"v0.11.0-rc.1", "v0.11.0", false},
		{"v0.11.0+meta", "v0.10.0", true},
		{"", "v0.11.0", false},
		{"v0.11.0", Dev, false},
		{"nonsense", "v0.11.0", false},
		{"v0.11", "v0.10.0", false},
	}
	for _, tc := range cases {
		if got := newer(tc.a, tc.b); got != tc.want {
			t.Errorf("newer(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func testChecker(t *testing.T, endpoint string) *Checker {
	t.Helper()
	return &Checker{
		current:  "v0.11.0",
		endpoint: endpoint,
		client:   &http.Client{Timeout: 5 * time.Second},
		ttl:      time.Hour,
		retry:    time.Minute,
	}
}

func TestCheckerReportsNewerRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"tag_name":"v0.12.0","html_url":"https://example.com/releases/v0.12.0"}`))
	}))
	defer srv.Close()

	c := testChecker(t, srv.URL)
	c.refresh(context.Background())

	info := c.Info()
	if !info.Outdated {
		t.Fatalf("Info = %+v, want Outdated", info)
	}
	if info.Current != "v0.11.0" || info.Latest != "v0.12.0" {
		t.Fatalf("Info = %+v, want current v0.11.0 and latest v0.12.0", info)
	}
	if info.URL != "https://example.com/releases/v0.12.0" {
		t.Fatalf("Info.URL = %q", info.URL)
	}
}

func TestCheckerCurrentIsLatest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"v0.11.0","html_url":"https://example.com/releases/v0.11.0"}`))
	}))
	defer srv.Close()

	c := testChecker(t, srv.URL)
	c.refresh(context.Background())

	if info := c.Info(); info.Outdated {
		t.Fatalf("Info = %+v, want up to date", info)
	}
}

func TestCheckerFailureIsSoft(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	defer srv.Close()

	c := testChecker(t, srv.URL)
	c.refresh(context.Background())

	info := c.Info()
	if info.Current != "v0.11.0" || info.Latest != "" || info.Outdated {
		t.Fatalf("Info = %+v, want the running version alone", info)
	}
	// A failed check retries sooner than the success TTL.
	c.mu.Lock()
	next := c.next
	c.mu.Unlock()
	if wait := time.Until(next); wait > c.retry {
		t.Fatalf("next check in %v, want at most %v", wait, c.retry)
	}
}

func TestCheckerDisabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("disabled checker called the network")
	}))
	defer srv.Close()

	c := testChecker(t, srv.URL)
	c.current, c.disabled = Dev, true

	info := c.Info()
	if info.Current != Dev || info.Latest != "" || info.Outdated {
		t.Fatalf("Info = %+v, want a bare dev version", info)
	}
}

func TestNewCheckerOptOut(t *testing.T) {
	t.Setenv("BESPOKE_UPDATE_CHECK", "off")
	if c := NewChecker(); !c.disabled {
		t.Fatal("BESPOKE_UPDATE_CHECK=off left the checker enabled")
	}
}
