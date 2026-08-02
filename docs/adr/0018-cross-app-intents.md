# 0018 — Cross-app intents

- **Status:** Accepted
- **Date:** 2026-08-01

## Context

Apps were islands: text in a journal entry that should become a todo, a
completed todo worth journaling. Mobile OSes solved this with intents/share
targets — declared actions any app can invoke on a payload. The platform
already has the pieces: manifests as registry (flowing to every app via the
AppShell context), the `_` platform namespace, and dev-aware URL building.

## Decision

Apps declare **intents** — actions other surfaces can offer:

- **Declaration is manifest data**: `[[intents]]` in `app.toml` (`name`,
  `title`, `accepts` — v1 accepts only `text`). The registry already
  reaches every app, so discovery costs nothing and needs no new plumbing.
- **Runtime is an app-contract endpoint** mounted by
  `web.Intent(mux, slug, def)`: `GET /_intents/<name>?text=…` renders a
  platform-provided **confirm page** (prefilled, editable — the human
  tweaks the payload before committing); `POST` executes the handler and
  redirects into the target app. Navigation, not fetch: no CORS, works on
  mobile, and the confirm step doubles as the edit step.
- **Two idioms** ship with the primitive:
  1. *Selection → intent*: platform chrome (`intents.js`) floats a small
     popover over selected text listing other apps' text intents.
  2. *Event → intent*: apps compose follow-ups themselves — e.g. todo's
     completion banner links to journal's `add-entry` intent with
     "Completed: …" prefilled. `ui.IntentsFrom(ctx)` exposes the registry
     to app views for this.
- **Integration is a standing design duty** (AGENTS.md): a new app should
  declare intents for anything other apps might feed it, and adding an app
  triggers a review of existing apps for natural integrations.

## Consequences

- Intents converge with the parked MCP surface idea: one action registry,
  two faces — confirm pages for humans, MCP tools for LLMs. Building MCP
  later starts from `[[intents]]`.
- Manifest changes require target-app redeploys to appear in other apps'
  chrome (10s registry cache) — fine at personal scale.
- The selection popover is a desktop-first affordance (touch selection
  fights native menus); mobile still gets event-idiom banners and could
  gain a share-sheet-style entry later.
- Cross-app references are by `<app>/<intent>` name (e.g. todo's banner
  targets `journal/add-entry`) — app-level coupling, documented in the
  READMEs of both apps; the platform stays generic.

## Alternatives considered

- **fetch()-based invocation from foreign origins:** CORS machinery, silent
  execution without an edit step, and worse mobile behavior. Rejected.
- **platformd as intent broker:** an extra hop with no gain — the registry
  already reaches every app, and apps can render their own confirm pages.
- **Free-form per-app integration code:** what this replaces; N×N bespoke
  glue instead of one declared surface. Rejected.

## References

- Builds on: [ADR-0015](0015-appshell-platform-chrome.md),
  [ADR-0017](0017-app-provided-dashboard-cards.md)
- Shapes: [specs/app-manifest.md](../specs/app-manifest.md),
  [design/internal-services.md](../design/internal-services.md) (MCP
  convergence), `AGENTS.md`
