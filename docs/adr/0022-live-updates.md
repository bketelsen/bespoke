# 0022 — Live updates: Datastar SSE over a change hub

- **Status:** Accepted
- **Date:** 2026-08-01

## Context

Datastar shipped in the AppShell since Phase 3, unused. Meanwhile CRUD grew
faces that bypass the page — chat tools, MCP, intents, other tabs — and
pages sat stale. The acute case: chat panel open on the todo page creates
a task, says "done", and the list behind it doesn't move. A page reload
would fix staleness but destroy the DOM-held chat history.

## Decision

- **Per-process change hub in pkg/web:** every mutation calls
  `web.Changed(login)` — handlers, tools, and intents alike. It wakes local
  `/_live` subscribers and fire-and-forgets a nudge to platformd's plane
  (`POST /notify`), which wakes dashboard subscribers via `web.Notify`.
- **`web.Live(mux, fragment)`** mounts `GET /_live`: a Datastar SSE stream
  that re-renders the app's live region on each change and patches it in
  place (`PatchElementTempl`). Pages subscribe from a stable wrapper
  (`data-on-load="@get('/_live')"`) around an id-stable fragment — only
  the region morphs, so chat panels, forms, and focus survive.
- Apps expose their dynamic region as a named fragment (journal
  `StreamLive`, todo `TasksLive`); the dashboard's `CardGrid` re-fetches
  all cards on any app's change. 45s heartbeats keep intermediaries from
  reaping idle streams.

## Consequences

- Any mutation from any face updates every open page for that user within
  a beat — chat says "added" and the list already shows it.
- Fragments render the default view: a todo user in `?completed=hidden`
  sees completed reappear after a live patch (documented trade-off; fix
  belongs in signals if it ever matters).
- The logging middleware had to pass `Flush`/`Unwrap` through — found the
  hard way; SSE through wrapping middleware silently buffers otherwise.
- Change fan-out is per-login, so multi-user stays isolated: her edits
  never patch his pages.

## Alternatives considered

- **Full page reload on change:** kills chat history and focus. Rejected.
- **Polling:** works but wastes the SSE machinery already shipped, and
  the hub was ~60 lines. Rejected.
- **A message bus (NATS etc.):** a dependency for what one HTTP POST to
  the plane does at personal scale. Rejected.

## References

- Builds on: [ADR-0008](0008-go-templ-datastar-frontend.md),
  [ADR-0015](0015-appshell-platform-chrome.md), [ADR-0021](0021-tools-agentic-chat-mcp.md)
- Shapes: [specs/app-manifest.md](../specs/app-manifest.md),
  [design/internal-services.md](../design/internal-services.md)
