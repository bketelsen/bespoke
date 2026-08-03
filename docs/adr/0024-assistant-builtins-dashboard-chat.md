# 0024 — Curated runtime builtins for the dashboard chat

- **Status:** Accepted
- **Date:** 2026-08-02

## Context

The LLM gateway locks every session down to inference plus explicitly
provided app tools ([ADR-0009](0009-copilot-sdk-llm-gateway.md),
[ADR-0021](0021-tools-agentic-chat-mcp.md)): `AvailableTools` whitelists
exactly the app tool names and every runtime permission request is denied.
That lockdown exists because the Copilot runtime's builtin toolset includes
shell and filesystem tools that would execute as platformd's uid on the
platform host.

But the same runtime also carries genuinely useful, low-risk assistant
capabilities the lockdown throws away: URL fetch and the hosted GitHub MCP
server's read-only tools. The dashboard chat
([ADR-0020](0020-dashboard-chat-aggregated-context.md)) is the owner's
personal assistant surface; "look this up on the web" and "search my GitHub"
are natural asks it could not serve.

Empirical constraints (verified against SDK v1.0.8, 2026-08-02):

- `web_fetch` is a normal builtin; enabling it works, and each fetch raises
  a URL permission request the host decides.
- The hosted `web_search` server tool exists only in the Copilot CLI's own
  host layer — SDK sessions cannot enable it at all.
- The GitHub MCP tools are likewise absent from SDK sessions; they appear
  only when the session explicitly configures the hosted GitHub MCP endpoint
  (`https://api.githubcopilot.com/mcp/`) with a bearer token.

## Decision

The gateway keeps its deny-by-default posture but adds a **curated builtin
allowlist** a request may opt into via `llm.WithBuiltins(names...)`:

- **Menu (gateway-owned, hardcoded):** `web_fetch` plus the read-only GitHub
  MCP subset (`github-mcp-server-search_code`, `-get_file_contents`,
  `-search_users`, `-get_copilot_space`, `-list_copilot_spaces`). Requests
  naming anything else are rejected. Shell, filesystem, and session-store
  tools are never eligible; agentic coding stays in the unprivileged builder
  runner ([ADR-0023](0023-builder-plane-unprivileged-agent-spooled-deploys.md)).
- **Permission policy replaces deny-all only for enabled builtins:**
  `web_fetch` URLs are approved only when they point at the public internet
  — no sandbox bypass, http(s) only, no loopback/private/link-local
  literals, no tailnet-ish or dotless hosts. GitHub MCP calls are approved
  only when the runtime marks them read-only. Everything else stays denied.
- **Web search rides `web_fetch`:** when `web_fetch` is enabled the gateway
  appends a system-prompt hint pointing the model at a fetchable search
  results page. When the SDK exposes the hosted `web_search` tool, it joins
  the menu and the hint goes away.
- **GitHub auth is gateway config:** `BESPOKE_GITHUB_TOKEN` or
  `GITHUB_TOKEN`, falling back to the host's `gh auth token`. With no token
  the GitHub entries quietly contribute nothing and the session still works.
- **Only the dashboard chat opts in** (all of the menu); per-app chats and
  mechanical completions are unchanged. Like app tools, builtins require
  `llm.WithUser`.

## Consequences

- The dashboard chat can answer from the live web and the owner's GitHub,
  and combine that with app tools in one agentic turn.
- The inference-only invariant of ADR-0009 is now "inference plus an
  explicit, gateway-curated opt-in" — the gateway remains the single choke
  point deciding what any surface may touch.
- `web_fetch` executes on the platform host: prompt-injected content could
  ask it to probe internal endpoints. The URL guard blocks direct attempts;
  DNS rebinding and redirects are accepted risk because the tailnet
  perimeter ([ADR-0004](0004-tailscale-identity-via-caddy.md)) rejects
  unauthenticated requests to every app host anyway.
- New soft dependency: a GitHub token on the platform host for the GitHub
  tools (absent token degrades, never breaks).
- Search quality depends on a scrapeable results page until the SDK exposes
  hosted search — revisit on SDK upgrades.

## Alternatives considered

- **Enable the CLI's full default toolset for the dashboard:** hands shell
  and filesystem on the platform host to a prompt-injectable surface;
  rejected outright.
- **Per-app opt-in to builtins:** no app needs them yet, and every extra
  surface multiplies the injection audit area. The wire supports it
  (`llm.WithBuiltins` is an option like any other); policy says dashboard
  only until a concrete need appears.
- **A separate search API (SerpApi etc.):** new account, new key, new
  billing for something the existing runtime does adequately via
  `web_fetch`; reconsider only if SERP scraping breaks.

## References

- Shapes: [design/llm-gateway.md](../design/llm-gateway.md)
- Builds on: [ADR-0009](0009-copilot-sdk-llm-gateway.md),
  [ADR-0020](0020-dashboard-chat-aggregated-context.md),
  [ADR-0021](0021-tools-agentic-chat-mcp.md),
  [ADR-0023](0023-builder-plane-unprivileged-agent-spooled-deploys.md)
