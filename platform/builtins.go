// Curated runtime builtins for assistant surfaces (ADR-0024): gateway
// sessions keep the Copilot CLI's shell, filesystem, and session tools
// locked out unconditionally; this file defines the small read-only set a
// request may re-enable (llm.WithBuiltins) and the permission policy that
// guards it.
package main

import (
	"fmt"
	"net"
	"net/url"
	"slices"
	"sort"
	"strings"

	copilot "github.com/github/copilot-sdk/go"
	"github.com/github/copilot-sdk/go/rpc"
)

// assistantBuiltins is the full menu: guarded URL fetch, real web search
// (gateway-implemented against the Brave API, ADR-0025 — the runtime's
// hosted web_search tool is not exposed to SDK sessions, verified against
// v1.0.8), and the read-only GitHub MCP subset the Copilot CLI enables by
// default. Never add shell, filesystem, or session-store tools here — they
// would execute as platformd's uid on the platform host; agentic coding
// stays in the unprivileged builder runner (ADR-0023).
var assistantBuiltins = map[string]bool{
	"web_fetch":                             true,
	"web_search":                            true,
	"github-mcp-server-search_code":         true,
	"github-mcp-server-get_file_contents":   true,
	"github-mcp-server-search_users":        true,
	"github-mcp-server-get_copilot_space":   true,
	"github-mcp-server-list_copilot_spaces": true,
}

// assistantBuiltinNames is the menu in stable order — platformd's own
// dashboard chat opts into all of it.
func assistantBuiltinNames() []string {
	names := make([]string, 0, len(assistantBuiltins))
	for n := range assistantBuiltins {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// checkBuiltins rejects any requested name outside the menu.
func checkBuiltins(names []string) error {
	for _, n := range names {
		if !assistantBuiltins[n] {
			return fmt.Errorf("builtin %q is not enabled on this gateway", n)
		}
	}
	return nil
}

const githubMCPPrefix = "github-mcp-server-"

// githubMCPConfig returns the session's MCP server block when the request
// enables GitHub tools. The hosted GitHub MCP endpoint needs a bearer token
// — without one the server contributes no tools and the rest of the session
// works normally (verified live). The server key must stay
// "github-mcp-server" so wire names match the allowlist entries.
func githubMCPConfig(builtins []string, token string) map[string]copilot.MCPServerConfig {
	if token == "" {
		return nil
	}
	var tools []string
	for _, b := range builtins {
		if name, ok := strings.CutPrefix(b, githubMCPPrefix); ok {
			tools = append(tools, name)
		}
	}
	if len(tools) == 0 {
		return nil
	}
	return map[string]copilot.MCPServerConfig{
		"github-mcp-server": copilot.MCPHTTPServerConfig{
			URL:     "https://api.githubcopilot.com/mcp/",
			Tools:   tools,
			Headers: map[string]string{"Authorization": "Bearer " + token},
		},
	}
}

// webFetchHint is appended to the system prompt when web_fetch is enabled.
// With a real search tool in the session it just pairs the two; without one
// it points the model at a results page it can fetch — otherwise some
// models just declare they cannot search.
func webFetchHint(builtins []string, hasSearch bool) string {
	if !slices.Contains(builtins, "web_fetch") {
		return ""
	}
	if hasSearch {
		return "\n\nWeb access: use web_search to search the public web, then web_fetch " +
			"to read a promising result in full."
	}
	return "\n\nWeb access: the web_fetch tool fetches public URLs. To search the web, " +
		"fetch https://html.duckduckgo.com/html/?q=<query> and read the results, then " +
		"fetch the promising links."
}

// builtinPermissions approves exactly what the enabled builtins need —
// web_fetch's URL prompts (guarded) and read-only GitHub MCP calls — and
// denies everything else, same as denyAll.
func builtinPermissions(enabled []string) copilot.PermissionHandlerFunc {
	on := make(map[string]bool, len(enabled))
	for _, n := range enabled {
		on[n] = true
	}
	return func(req copilot.PermissionRequest, _ copilot.PermissionInvocation) (rpc.PermissionDecision, error) {
		switch r := req.(type) {
		case *rpc.PermissionRequestURL:
			if on["web_fetch"] && fetchAllowed(r) {
				return &rpc.PermissionDecisionApproveOnce{}, nil
			}
		case *rpc.PermissionRequestMCP:
			// The runtime reports the tool's internal name; wire names are
			// server-prefixed, so accept either spelling.
			if r.ReadOnly && (on[r.ToolName] || on[r.ServerName+"-"+r.ToolName]) {
				return &rpc.PermissionDecisionApproveOnce{}, nil
			}
		}
		return &rpc.PermissionDecisionDeniedInteractivelyByUser{}, nil
	}
}

// fetchAllowed keeps web_fetch on the public internet: no sandbox bypass,
// http(s) only, and no way to name the platform's own planes — loopback,
// private, and link-local literals, tailnet-ish suffixes, and dotless hosts
// are all refused. DNS rebinding is accepted risk: the tailnet perimeter
// (ADR-0004) rejects unauthenticated requests to every app host anyway.
func fetchAllowed(r *rpc.PermissionRequestURL) bool {
	if r.RequestSandboxBypass != nil && *r.RequestSandboxBypass {
		return false
	}
	u, err := url.Parse(r.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || !strings.Contains(host, ".") ||
		strings.HasSuffix(host, ".ts.net") || strings.HasSuffix(host, ".local") ||
		strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".lan") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil && (!ip.IsGlobalUnicast() || ip.IsPrivate()) {
		return false
	}
	return true
}
