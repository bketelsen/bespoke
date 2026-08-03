# 0025 — Brave-backed web_search as a gateway tool

- **Status:** Accepted
- **Date:** 2026-08-02

## Context

[ADR-0024](0024-assistant-builtins-dashboard-chat.md) gave the dashboard
chat web access but had to route *search* through `web_fetch` against a
scrapeable results page, because the Copilot runtime's hosted `web_search`
tool is not exposed to SDK sessions. That ADR rejected a separate search
API on setup-and-billing grounds, with the caveat "reconsider only if SERP
scraping breaks."

The calculus changed: the owner already holds a Brave Search API key —
zero new signup — and SERP scraping is the weakest link in the assistant's
web access (fragile markup, no ranking metadata, bot-detection risk).

## Decision

The gateway implements `web_search` itself as a session-local custom tool
backed by the [Brave Search API](https://api.search.brave.com):

- **Same opt-in surface:** `web_search` joins the ADR-0024 builtin menu;
  callers still say `llm.WithBuiltins("web_search")`. The gateway swaps
  the name out of the runtime-builtin list and registers its own tool —
  callers cannot tell the difference, and if the SDK ever exposes hosted
  search the swap can be dropped without touching any caller.
- **The tool is read-only and in-process:** one GET per call with the
  subscription key, returning titles, URLs, and snippets for the top
  results. Failures return to the model as tool-level text (it can fall
  back to `web_fetch`), never as session errors.
- **Key is gateway config:** `BESPOKE_BRAVE_API_KEY` (or `BRAVE_API_KEY`)
  in platformd's environment. Without it `web_search` degrades to the
  ADR-0024 fetch-based search hint — never breaks.
- **Reading pages stays on `web_fetch`** and its public-internet-only
  permission guard; the system-prompt hint pairs the two tools when both
  are enabled.

## Consequences

- Search results become structured and reliable instead of scraped, and
  each search is one API call instead of a fetch-and-parse round-trip.
- New soft dependency: the Brave key in the env file on the platform host
  (degrades to fetch-based search when absent).
- Brave's API quota/billing now sits behind a chat surface; the gateway's
  `llm-tool web_search` log lines are the usage record.
- The gateway now owns a tool implementation of its own — a third kind of
  tool (app tools, runtime builtins, gateway tools) to keep in mind when
  reasoning about the surface.

## Alternatives considered

- **Keep SERP scraping (status quo):** works today but fragile and
  unranked; with a key already in hand the API is strictly better.
- **Brave's MCP server:** an extra Node process and MCP hop for what is a
  single HTTP GET; the direct call is simpler than the plumbing.
- **Hosted `web_search` via the Copilot runtime:** still not exposed to
  SDK sessions (verified v1.0.8); remains the preferred endgame — the
  menu name is chosen so it can take over transparently.

## References

- Shapes: [design/llm-gateway.md](../design/llm-gateway.md)
- Builds on: [ADR-0024](0024-assistant-builtins-dashboard-chat.md),
  [ADR-0009](0009-copilot-sdk-llm-gateway.md)
