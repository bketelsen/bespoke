# Global Search for Bespoke — Design

**Date:** 2026-08-03
**Status:** Approved, ready for implementation plan

## Overview

A search box on the platformd dashboard that queries all registered apps in
parallel and shows user-scoped results grouped by app. The same capability is
exposed as a platform-owned MCP/chat tool. There is **no central index and no
cross-app database access** — it reuses the existing HTTP fan-out pattern that
already powers dashboard cards (`/_card`), aggregated chat context
(`/_chat/context`), and tools (`/_tools`).

## Decisions

| Question | Decision |
| --- | --- |
| Where it lives | Dashboard search box **and** MCP/chat `search` tool, both in v1 |
| Architecture | Live HTTP fan-out to per-app `GET /_search?q=` — no central index |
| Text matching | App's choice; substring (`LIKE`) for v1, FTS5-upgradable behind an unchanged contract |
| Presentation | Grouped by app (no cross-app ranking); plain server-rendered GET |
| Deep links | Preferred / best-effort in the contract; **both** Notes and Todo ship real deep links |

## Why fan-out, not a central index

The platform's core isolation rule is that platformd never reads app SQLite
files; cross-app features work by forwarding the caller's identity to
app-contract HTTP endpoints, and each app owns its own row-level user scoping
(ADR-0007, ADR-0017, ADR-0020). A central Tier-2 search index would need an
indexing protocol (the current `web.Changed(login)` notify carries only the
login — no slug, id, content, or delete), index invalidation, delete
semantics, and a new store. Fan-out is the proven pattern here and keeps app
sovereignty. It cannot rank across apps, which is why results are grouped by
app rather than merged into one ranked list — an honest presentation of what
fan-out can actually deliver.

## The `/_search` contract

A new helper mounts the endpoint:

```go
web.Search(mux, provider) // → GET /_search?q=<query>
```

- Route is wrapped in `auth.Middleware`; `provider(user auth.User, q string)`
  returns the user's matching results.
- The app runs its own user-scoped query (`WHERE login=? AND <text> LIKE
  '%q%'`), choosing its own matching strategy. Substring for v1; an app may
  switch to FTS5 internally without changing the contract.
- JSON response:

  ```json
  { "results": [
    { "title": "...", "snippet": "...", "url": "/task/123", "timestamp": "..." }
  ] }
  ```

  - `title` required; `snippet`, `url`, `timestamp` optional.
  - `url` is app-relative; platformd resolves it to the app's dev/prod base
    URL using the same logic as the app switcher.
  - Deep links are **preferred / best-effort**: an app SHOULD return a specific
    deep URL per result; a bare home URL (`/`) is an acceptable fallback.
- An app that does not mount `/_search` is silently absent from results
  (same fallback behaviour as dashboard cards).

## platformd fan-out

New `platform/search.go`, modeled on `platform/cards.go`:

- Loads manifests (existing 10s cache), concurrently GETs each app's
  `/_search?q=`, forwards the caller's identity headers, applies a per-app
  timeout (~900ms) and a response cap (~32KB), and silently drops failures and
  absent endpoints.
- Tags each result group with the app's manifest name + slug. No cross-app
  ranking.
- The aggregator is a shared function reused by **both** the dashboard results
  view and the MCP/chat tool.

## Dashboard UI

- A `type="search"` input in the dashboard chrome
  (`platform/views/dashboard.templ`), built from `pkg/ui` components.
- Submitting `q` renders (plain server-rendered GET) a results view: one
  section per app ("Notes (3)", "Todo (1)"), each result a link to the resolved
  deep URL; `ui.Markdown` used for snippets where useful.
- Clear empty-query and no-results states.
- Mobile-first at 375px with a coarse pointer, no hover-only affordances
  (ADR-0016).

## MCP / chat search tool

- A **platform-owned** tool `search` registered on the dashboard's tool set,
  reusing the same fan-out aggregator.
- Returns the same grouped-by-app structure (app → results with
  title/snippet/deep url) so the agent can cite deep links.
- Available via `/mcp` and dashboard chat. Because the dashboard's tool set is
  currently built only from `allAppTools`, a small seam adds this
  platform-owned tool alongside the aggregated app tools.

## Showcase app participation (deep-link exemplars)

- **Notes**: search `body` `WHERE login=?` ordered `created_at DESC`; add
  `id="note-<id>"` anchors to the list; result `url = "/#note-<id>"`; title =
  first line, snippet = matched context.
- **Todo**: search `description` `WHERE login=?`; add a task deep-link target
  (a `/task/<id>` route or a root `#task-<id>` anchor); result `url` = that
  deep link; title = description, snippet = due/priority/done metadata.
- Builder is unregistered (no `app.toml`) and is skipped.

## Documentation obligations (AGENTS.md-mandated)

- **New ADR 0028** — "Dashboard global search via app fan-out": records the
  fan-out-over-central-index decision and the MCP-tool exposure; builds on
  ADR-0012, ADR-0017, ADR-0020.
- **New spec** `docs/specs/app-search.md` — the `/_search` request/JSON
  contract, user-scoping, deep-links-preferred/best-effort, absent-endpoint
  fallback.
- Update `docs/design/internal-services.md` — resolve the "Search" catalog
  candidate as a **fan-out contract**, not a central Tier-2 service.
- Update **AGENTS.md** — a bullet: apps expose `web.Search(mux, provider)`;
  results are user-scoped; deep links are **preferred/best-effort** (home URL
  an acceptable fallback); the app joins dashboard search and the MCP `search`
  tool automatically.
- Add both new docs to `docs/README.md` index; cross-link ADR↔spec↔design in
  both directions with relative paths.

## Testing

- `pkg/web`: `web.Search` mounts the route, enforces auth, shapes JSON.
- `platform`: the aggregator merges multiple apps, applies timeout/cap, drops
  failures, forwards identity, groups by app; the MCP tool returns the grouped
  structure.
- Apps: Notes/Todo return only the caller's matching rows; deep-link URLs are
  well-formed.
- `just check` (vet + tests + golangci-lint + `go mod tidy -diff` + CGO-free
  linux cross-compile) passes; after `.templ` changes run `just ui` and commit
  generated `*_templ.go` plus the compiled stylesheet.

## Non-goals (v1)

Central index; cross-app relevance ranking; fuzzy/typo/search-as-you-type;
in-app search UIs; indexing or live-update of search results.
