# 0020 — Dashboard chat over aggregated app contexts

- **Status:** Accepted
- **Date:** 2026-08-01

## Context

"What's due this week, and how was my day?" spans apps. Per-app chat
(ADR-0015) can't answer it, and platformd reading app databases directly is
explicitly rejected (ADR-0007 isolation; restated in ADR-0017). ADR-0015
anticipated a dashboard chat "later" — this is later.

## Decision

- Every `EnableChat` app also serves **`GET /_chat/context`**: its chat
  provider's output, raw, for the requesting user. Same auth as everything.
- The dashboard runs its own chat whose provider **aggregates all apps'
  contexts** over that endpoint — in parallel, identity forwarded (the
  ADR-0017 trusted-platform pattern), short timeout, per-app size cap,
  sections labeled per app, misses skipped silently.
- Chat-enabled apps join the dashboard chat with **zero new app code**;
  apps without chat simply aren't included. The user brief (ADR-0019) rides
  along via `llm.WithUser` as usual.

## Consequences

- Cross-app answers without cross-app database access — isolation holds.
- Dashboard chat cost = context fetches (cheap, bounded) + one completion;
  its context grows with the app count, so per-app caps matter.
- A future MCP/agentic upgrade (ADR-0018 convergence) can replace
  context-stuffing with tool calls behind the same UI.

## Alternatives considered

- **platformd reads app databases:** rejected again for the same reasons
  as in ADR-0017.
- **A separate "export for dashboard" provider per app:** duplicate of the
  chat provider in every app. The existing provider IS the export.

## References

- Builds on: [ADR-0015](0015-appshell-platform-chrome.md),
  [ADR-0017](0017-app-provided-dashboard-cards.md),
  [ADR-0019](0019-user-brief.md)
- Shapes: [specs/app-manifest.md](../specs/app-manifest.md),
  [design/internal-services.md](../design/internal-services.md)
