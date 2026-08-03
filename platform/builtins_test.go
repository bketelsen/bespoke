package main

import (
	"strings"
	"testing"

	copilot "github.com/github/copilot-sdk/go"
	"github.com/github/copilot-sdk/go/rpc"
)

func TestCheckBuiltins(t *testing.T) {
	if err := checkBuiltins(assistantBuiltinNames()); err != nil {
		t.Fatalf("full menu rejected: %v", err)
	}
	if err := checkBuiltins(nil); err != nil {
		t.Fatalf("empty list rejected: %v", err)
	}
	for _, bad := range []string{"bash", "apply_patch", "view", "sql", "rg", ""} {
		if err := checkBuiltins([]string{"web_fetch", bad}); err == nil {
			t.Errorf("checkBuiltins allowed %q", bad)
		}
	}
}

func TestGithubMCPConfig(t *testing.T) {
	if githubMCPConfig(assistantBuiltinNames(), "") != nil {
		t.Error("MCP server configured without a token")
	}
	if githubMCPConfig([]string{"web_fetch"}, "tok") != nil {
		t.Error("MCP server configured without GitHub builtins")
	}
	cfg := githubMCPConfig([]string{"web_fetch", "github-mcp-server-search_code"}, "tok")
	if cfg == nil {
		t.Fatal("no MCP config for GitHub builtins with token")
	}
	if _, ok := cfg["github-mcp-server"]; !ok {
		t.Fatalf("server key must be github-mcp-server, got %v", cfg)
	}
}

func TestWebFetchHint(t *testing.T) {
	if webFetchHint([]string{"github-mcp-server-search_code"}, false) != "" {
		t.Error("hint added without web_fetch")
	}
	noSearch := webFetchHint([]string{"web_fetch"}, false)
	if !strings.Contains(noSearch, "duckduckgo") {
		t.Errorf("keyless hint should carry fetch-based search, got %q", noSearch)
	}
	withSearch := webFetchHint([]string{"web_fetch"}, true)
	if withSearch == "" || strings.Contains(withSearch, "duckduckgo") {
		t.Errorf("with web_search the hint should pair the tools, got %q", withSearch)
	}
}

func TestFetchAllowed(t *testing.T) {
	yes := true
	cases := []struct {
		req  rpc.PermissionRequestURL
		want bool
	}{
		{rpc.PermissionRequestURL{URL: "https://example.com/page"}, true},
		{rpc.PermissionRequestURL{URL: "http://go.dev/doc"}, true},
		{rpc.PermissionRequestURL{URL: "https://example.com/x", RequestSandboxBypass: &yes}, false},
		{rpc.PermissionRequestURL{URL: "ftp://example.com/f"}, false},
		{rpc.PermissionRequestURL{URL: "file:///etc/passwd"}, false},
		{rpc.PermissionRequestURL{URL: "http://localhost:4001/llm/complete"}, false},
		{rpc.PermissionRequestURL{URL: "http://127.0.0.1:4001/notify"}, false},
		{rpc.PermissionRequestURL{URL: "http://10.0.0.8/"}, false},
		{rpc.PermissionRequestURL{URL: "http://192.168.1.1/"}, false},
		{rpc.PermissionRequestURL{URL: "http://169.254.169.254/latest/meta-data"}, false},
		{rpc.PermissionRequestURL{URL: "http://[::1]:4001/"}, false},
		{rpc.PermissionRequestURL{URL: "http://[::ffff:127.0.0.1]/"}, false},
		{rpc.PermissionRequestURL{URL: "https://selfie.tail1234.ts.net/"}, false},
		{rpc.PermissionRequestURL{URL: "https://nas.local/"}, false},
		{rpc.PermissionRequestURL{URL: "https://platformd/"}, false},
		{rpc.PermissionRequestURL{URL: "notaurl"}, false},
	}
	for _, c := range cases {
		if got := fetchAllowed(&c.req); got != c.want {
			t.Errorf("fetchAllowed(%q) = %v, want %v", c.req.URL, got, c.want)
		}
	}
}

func TestBuiltinPermissions(t *testing.T) {
	perms := builtinPermissions(assistantBuiltinNames())

	decide := func(req rpc.PermissionRequest) rpc.PermissionDecision {
		t.Helper()
		d, err := perms(req, copilot.PermissionInvocation{})
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		return d
	}
	approved := func(d rpc.PermissionDecision) bool {
		_, ok := d.(*rpc.PermissionDecisionApproveOnce)
		return ok
	}

	if !approved(decide(&rpc.PermissionRequestURL{URL: "https://example.com/"})) {
		t.Error("public web_fetch URL denied")
	}
	if approved(decide(&rpc.PermissionRequestURL{URL: "http://127.0.0.1:4001/"})) {
		t.Error("loopback web_fetch URL approved")
	}
	if !approved(decide(&rpc.PermissionRequestMCP{ServerName: "github-mcp-server", ToolName: "search_code", ReadOnly: true})) {
		t.Error("read-only enabled MCP tool denied")
	}
	if approved(decide(&rpc.PermissionRequestMCP{ServerName: "github-mcp-server", ToolName: "create_issue", ReadOnly: false})) {
		t.Error("writable MCP tool approved")
	}
	// Shell must stay denied no matter what the request claims.
	if approved(decide(&rpc.PermissionRequestShell{})) {
		t.Error("shell request approved")
	}

	// Without web_fetch enabled, even public URLs are denied.
	noFetch := builtinPermissions([]string{"github-mcp-server-search_code"})
	d, err := noFetch(&rpc.PermissionRequestURL{URL: "https://example.com/"}, copilot.PermissionInvocation{})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if approved(d) {
		t.Error("URL approved without web_fetch enabled")
	}
}
