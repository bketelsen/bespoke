# Agent Layer

The differentiator. Everything else in this system exists to make the one-shot
"build me an app that X" prompt reliable
([ADR-0002](../adr/0002-optimize-for-one-shot-agent-reliability.md)).

## CLAUDE.md — conventions as law

The repo-root `CLAUDE.md` states the invariants an agent may never route
around:

- Use `pkg/*` for auth, db, web scaffold, UI, LLM, notifications — never
  hand-roll any of them.
- Never write ad-hoc CSS; compose `pkg/ui` components. Tailwind utilities for
  layout only, theme tokens only
  ([ADR-0008](../adr/0008-go-templ-datastar-frontend.md),
  [ADR-0010](../adr/0010-templui-component-base.md)).
- Never hand-edit vendored templUI files in `pkg/ui` — customization goes in
  the theme (`design/input.css`) or wrapper components.
- Apps listen on loopback only, on the port assigned in their manifest.
- Deploy exclusively via the `bespoke` CLI
  ([spec](../specs/bespoke-cli.md)); never hand-edit Caddy config or systemd
  units.
- The manifest schema is the [app-manifest spec](../specs/app-manifest.md).
- Security rules from [ADR-0004](../adr/0004-tailscale-identity-via-caddy.md)
  are non-negotiable.
- New significant decisions get an ADR
  ([ADR-0001](../adr/0001-record-architecture-decisions.md)).

## Skills

`.claude/skills/`:

- **new-app** — from a one-line description: run `bespoke new <slug>`, model
  the schema, write migrations + handlers + views composing `pkg/ui`, deploy
  via `bespoke deploy`, verify with a request through the local port, confirm
  it appears on the dashboard.
- **new-component** — vendor an additional templUI component (`templui add`)
  or add a Bespoke wrapper in `pkg/ui`; adjust the theme if needed; update the
  design-system docs; never fork styling into an app.

Planned: a resident maintenance agent on a schedule (dependency bumps, backup
verification, log triage) — stolen from the
[prior-art article](vision.md#prior-art).

## The feedback loop

The framework is done when the loop is boring:

1. Prompt the new-app skill with one line.
2. If anything required manual intervention, that's a framework bug — fix the
   convention, the scaffold, or the skill, not the app.
3. Repeat.
