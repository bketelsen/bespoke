# 0017 — App-provided dashboard cards

- **Status:** Accepted
- **Date:** 2026-08-01

## Context

The dashboard rendered static manifest cards (icon, name, description) —
redundant chrome once you know the apps, and it wastes the platform's best
real estate on descriptions instead of *state* ("3 entries today" beats
"one stream for everything").

## Decision

The app HTTP contract grows an optional endpoint: **`GET /_card`** returns
an HTML fragment of the app's live dashboard summary for the requesting
user, registered via `web.DashboardCard(mux, provider)` where
`provider(ctx, user)` returns a templ component. Rules:

- The dashboard (platformd) fetches every app's `/_card` server-side, in
  parallel, with the caller's `Tailscale-User-*` headers forwarded — cards
  are per-user, same trust model as the edge (platformd is a trusted
  platform component).
- Short timeout and a size cap; **any miss (no endpoint, error, slow)
  falls back to the manifest description** — a broken app can't break the
  dashboard, and apps without cards still look intentional.
- The dashboard owns the frame: icon + app name (the link to the app) on
  top, fragment below. Fragments are content only — no AppShell, no page
  chrome, no interactive forms (links are fine).
- `/_card` joins `/_chat` and `/healthz` in the platform's underscore
  namespace; the [manifest spec](../specs/app-manifest.md) now records the
  full app HTTP contract.

## Consequences

- The dashboard becomes a glanceable status board that improves app by app
  as cards are added — no platform change needed per app.
- Dashboard render cost = slowest card (bounded by the timeout); card
  providers must be cheap queries, never LLM calls.
- One more optional step in the new-app skill; hello deliberately has no
  card, keeping the fallback path exercised.

## Alternatives considered

- **Platformd queries app databases directly:** breaks per-app SQLite
  isolation (ADR-0007) and couples platformd to every schema. Rejected.
- **Apps push summaries to platformd:** stale data and a write path where
  a read suffices. Rejected.

## References

- Builds on: [ADR-0005](0005-process-per-app.md),
  [ADR-0015](0015-appshell-platform-chrome.md)
- Shapes: [specs/app-manifest.md](../specs/app-manifest.md),
  [design/architecture.md](../design/architecture.md)
