# 0015 — AppShell platform chrome: app switcher and in-app chat

- **Status:** Accepted
- **Date:** 2026-08-01

## Context

With several apps live, two platform-level UI needs emerged: switching
between apps without going back to the dashboard (the Office-365 waffle
pattern), and asking an LLM questions *about the current app's data* from
inside the app ("is my mood trending up?"). Both must cost apps nothing —
every feature an app must wire by hand is a one-shot failure mode
([ADR-0002](0002-optimize-for-one-shot-agent-reliability.md)).

## Decision

The AppShell becomes platform chrome, fed through the request context so
apps change zero code:

- **App switcher (automatic):** `pkg/web`'s middleware scans the manifests
  (the registry, 10s cache) on each request and stashes `ui.ShellData` —
  app links (name, Lucide icon, dev-aware URL) and the current slug — in the
  request context. `ui.AppShell` renders a waffle menu (native
  `<details>`, no JS) in the header's top-left: dashboard first, then every
  app, current one highlighted. New apps appear in every app's menu on
  their next request.
- **In-app chat (one-line opt-in):** an app calls
  `web.EnableChat(mux, slug, provider)` where `provider(ctx, user)` returns
  a text bundle of the user's relevant app data. pkg/web mounts
  `POST /_chat` (behind auth like everything else): prompt = app-scoped
  system message + provider context + client-held history + question, sent
  through `pkg/llm` to the gateway. The AppShell shows the chat button and
  slide-over panel only when chat is enabled (context flag).
- **v1 is context-stuffing, deliberately.** The provider dumps recent data
  into the prompt — simple, ~1.5s answers. When the MCP surface idea lands,
  chat upgrades to agentic sessions calling the app's own tools; the
  `EnableChat` seam stays.

## Consequences

- Zero app changes for the switcher; one line + one provider function for
  chat. The new-app skill can offer chat as a standard optional step.
- Apps (not just platformd) now read the manifests, so all units get
  `BESPOKE_ROOT` via the shared env file.
- Context size is the provider's responsibility (recent data, not full
  dumps); the gateway logs every chat call like any LLM use.
- Chat answers are bounded by what the provider includes — stated in the
  panel's empty state so expectations stay honest.

## Alternatives considered

- **Apps receive the registry as config/flags:** restart-coupled and another
  thing to wire; the context middleware is invisible and always fresh.
- **One global chat on the dashboard instead of per-app:** loses the "this
  app's data" scoping that makes answers useful; nothing stops a dashboard
  chat later.
- **Tools/agentic chat now:** blocked on the MCP surface idea; context
  stuffing ships today and the seam survives the upgrade.

## References

- Builds on: [ADR-0009](0009-copilot-sdk-llm-gateway.md),
  [ADR-0010](0010-templui-component-base.md),
  [ADR-0012](0012-internal-services-two-tier.md)
- Shapes: [design/architecture.md](../design/architecture.md),
  [design/internal-services.md](../design/internal-services.md)
