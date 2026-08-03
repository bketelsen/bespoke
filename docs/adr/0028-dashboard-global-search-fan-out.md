# 0028 — Dashboard global search via app fan-out

- **Status:** Accepted
- **Date:** 2026-08-03

## Context

Data lives in one SQLite database per app, isolated by construction:
platformd never reads an app's database, and every cross-app surface so far
(dashboard cards, aggregated chat context, tools/MCP) works by forwarding the
caller's identity to an app-contract HTTP endpoint while the app owns its own
row-level user scoping (ADR-0007, ADR-0017, ADR-0020). There is no way, today,
to search across apps from one place — the internal-services catalog lists
"Search" only as a future Tier-2 candidate. A central search index would need
an indexing protocol (the live-update notify carries only the login — no slug,
id, content, or delete), invalidation, delete semantics, and a new store.

## Decision

The app HTTP contract grows an optional endpoint: **`GET /_search?q=`**
returns the requesting user's matching results as JSON, registered via
`web.Search(mux, provider)`. The dashboard fans out live to every app and
groups the results. Rules:

- platformd fetches every app's `/_search?q=` server-side, in parallel, with
  the caller's identity headers forwarded (same trust model as `/_card`), a
  short per-app timeout and a size cap; any miss (no endpoint, error, slow)
  drops that app from the results — a broken app cannot break search.
- The app owns matching and scoping: it queries its own database
  `WHERE login=?` with a matching strategy of its choice (substring for v1;
  FTS5 later, contract unchanged) and returns `{ "results": [ {title,
  snippet?, url?, timestamp?} ] }`.
- Results are **grouped by app**, never merged into one ranked list — fan-out
  produces no cross-app relevance score, so grouping is the honest
  presentation.
- `url` is app-relative and resolved by platformd to the app's base URL.
  **Deep links are preferred/best-effort**: an app SHOULD return a specific
  per-result URL; a bare home URL is an acceptable fallback.
- The same fan-out aggregator is exposed as a platform-owned MCP/chat tool
  `search`, so `/mcp` and the dashboard chat can search across apps and cite
  deep links.

## Consequences

- Cross-app search ships without any central index, indexing protocol, or new
  store, and improves app by app as apps mount `/_search` — no platform change
  per app.
- Search cost = slowest app (bounded by the timeout); providers must be cheap
  queries, never LLM calls.
- No cross-app ranking is possible; the UI groups by app rather than
  pretending to a unified relevance order.
- One more optional endpoint in the app contract and the new-app skill; apps
  without it are silently absent, keeping the fallback path exercised.

## Alternatives considered

- **Central Tier-2 search index:** better ranking and a single fast query, but
  requires an indexing/delete protocol the platform does not have and a shared
  store that erodes per-app isolation (ADR-0007). Rejected for v1; the
  internal-services catalog records fan-out as the resolution.
- **Merged, cross-app-ranked result list:** fan-out yields no global score, so
  any merged order would be arbitrary and misleading. Rejected in favour of
  per-app grouping.
- **Platformd queries app databases directly:** breaks per-app SQLite
  isolation (ADR-0007). Rejected, as for cards.

## References

- Builds on: [ADR-0012](0012-internal-services-two-tier.md),
  [ADR-0017](0017-app-provided-dashboard-cards.md),
  [ADR-0020](0020-dashboard-chat-aggregated-context.md),
  [ADR-0021](0021-tools-agentic-chat-mcp.md)
- Shapes: [specs/app-search.md](../specs/app-search.md),
  [design/internal-services.md](../design/internal-services.md)
