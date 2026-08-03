# Agent Layer

The differentiator. Everything else in this system exists to make the one-shot
"build me an app that X" prompt reliable
([ADR-0002](../adr/0002-optimize-for-one-shot-agent-reliability.md)).

## Cross-agent surface (implemented)

Per [ADR-0013](../adr/0013-agent-portable-instruction-surface.md), the
canonical files are `AGENTS.md` and `.agents/skills/<name>/SKILL.md`;
`CLAUDE.md`, `GEMINI.md`, `.github/copilot-instructions.md`, and
`.claude/skills` are symlinks. Any agent — Claude Code, Copilot CLI, Codex,
Gemini — reads the same law and the same skills, which matters exactly when
quota forces a mid-project switch.

## AGENTS.md — conventions as law

The repo-root `AGENTS.md` (which `CLAUDE.md` symlinks) states the
invariants an agent may never route around:

- Use `pkg/*` for auth, db, web scaffold, UI, LLM, audio (speech in and
  out) — never hand-roll any of them.
- Never write ad-hoc CSS; compose `pkg/ui` components. Tailwind utilities for
  layout only, theme tokens only
  ([ADR-0008](../adr/0008-go-templ-datastar-frontend.md),
  [ADR-0010](../adr/0010-templui-component-base.md)).
- Never hand-edit vendored templUI files in `pkg/ui` — customization goes in
  the instance theme (`design/theme.css`) or wrapper components.
- Apps listen on loopback only, on the port assigned in their manifest.
- Deploy exclusively via the `bespoke` CLI
  ([spec](../specs/bespoke-cli.md)); never hand-edit Caddy config, systemd
  units, or anything in `dist/gen/` — they are generated.
- Wire the app contract, not just pages: `web.Tool` for every meaningful
  action ([ADR-0021](../adr/0021-tools-agentic-chat-mcp.md)),
  `web.DashboardCard` ([ADR-0017](../adr/0017-app-provided-dashboard-cards.md)),
  intents where other apps might feed this one
  ([ADR-0018](../adr/0018-cross-app-intents.md)), and `web.Changed` after
  every mutation with a `web.Live` fragment
  ([ADR-0022](../adr/0022-live-updates.md)).
- `just check` before calling anything done; CI runs the identical recipe,
  so a local pass is the merge gate.
- The manifest schema is the [app-manifest spec](../specs/app-manifest.md).
- Security rules from [ADR-0004](../adr/0004-tailscale-identity-via-caddy.md)
  are non-negotiable.
- New significant decisions get an ADR
  ([ADR-0001](../adr/0001-record-architecture-decisions.md)).

## Skills (implemented)

`.agents/skills/` (symlinked as `.claude/skills/`):

- **[design-app](../../.agents/skills/design-app/SKILL.md)** — the front
  half of the one-shot: a 5–8 question interview (one question at a time,
  options over essays) that turns a bare "build me a journal" into a
  half-page spec at `apps/<slug>/README.md` — usage moment, record shape,
  views, service leverage, confirmed non-goals, parked ideas.
- **[new-app](../../.agents/skills/new-app/SKILL.md)** — one-line description
  → `just new`, schema, handlers, views on `pkg/ui`, shared-capability check
  against the [internal services catalog](internal-services.md), full local
  verification (check + dev + curl + dashboard).
- **[new-component](../../.agents/skills/new-component/SKILL.md)** — the
  three cases: vendor from templUI, Bespoke wrapper in `pkg/ui`, or theme
  tokens; always `build-ui` + commit generated output; never fork styling
  into an app.
- **[make-it-your-own](../../.agents/skills/make-it-your-own/SKILL.md)**
  (canonical file: root `MAKE-IT-YOUR-OWN.md`) — create a private instance,
  choose deployment and inference settings, and pin a platform release
  ([ADR-0027](../adr/0027-versioned-platform-private-instances.md)).

Planned: a resident maintenance agent on a schedule (dependency bumps, backup
verification, log triage) — stolen from the
[prior-art article](vision.md#prior-art).

## The feedback loop

The framework is done when the loop is boring:

1. Prompt the new-app skill with one line.
2. If anything required manual intervention, that's a framework bug — fix the
   convention, the scaffold, or the skill, not the app.
3. Repeat.
