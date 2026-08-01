# Roadmap

Updated as work lands. Phases are sequential; each ends with something
demonstrably working.

## Phase 1 — Prove the loop (weekend-sized)

([ADR-0011](../adr/0011-split-host-deployment.md): edge host runs Caddy,
apps deploy to `selfie`, builds happen on the dev machine)

- Rebuild edge Caddy with `caddy-tailscale` + `caddy-dns/cloudflare`
  (xcaddy); Cloudflare API token on the edge host; wildcard DNS record.
- Tailscale ACL: only the edge host reaches `selfie:4000-4999`.
- platformd v0: dashboard reading `apps/*/app.toml` manifests.
- Hello-world app behind the full route/auth path.
- `scripts/deploy.sh`: cross-compile → rsync to selfie → restart units →
  push Caddy routes to edge. Runbook in `deploy/README.md`.
- **Done when:** `hello.bespoke.example.com` renders my Tailscale login name.

## Phase 2 — Framework v0

- `pkg/auth`, `pkg/db` (migrations + WAL), `pkg/web` (scaffold, healthz,
  loopback-only, Datastar helpers).
- Port hello-world onto `pkg/*`.
- **Done when:** hello-world contains zero infrastructure code.

## Phase 3 — Design system

([ADR-0010](../adr/0010-templui-component-base.md))

- Tooling: `templui` CLI + Tailwind v4 standalone binary; wire CSS compile
  into the build.
- Vendor the starter component set (button, card, form/input, table, dialog,
  toast) via `templui add`.
- `design/input.css` bespoke theme (oklch variables, light/dark) — authored
  with Claude.
- Bespoke wrappers in `pkg/ui`: `AppShell` (nav, auth display, Datastar +
  component scripts), `Page`.
- Dashboard rebuilt on `pkg/ui`.
- **Done when:** dashboard and hello-world are visually indistinguishable in style.

## Phase 4 — Operations

- `bespoke` CLI per [spec](../specs/bespoke-cli.md): new/deploy/list/logs/rm,
  generated systemd units, Caddy route generation, rollback on failed healthz.
- Litestream config generation + restore drill.
- **Done when:** `bespoke new` → `bespoke deploy` works end to end and a
  restored DB matches.

## Phase 5 — LLM gateway

- platformd `/llm` endpoint on the Copilot SDK per
  [design](../design/llm-gateway.md); `pkg/llm` client; CLI auth health check
  on the dashboard.
- Latency measurement (see open questions).
- **Done when:** an app calls `llm.Complete` with no Copilot-specific code.

## Phase 6 — The agent layer (the point)

- `CLAUDE.md` invariants + `.claude/skills/new-app` and `new-component`.
- Build the next real app by prompt only; every manual intervention is filed
  as a framework bug and fixed.
- **Done when:** three consecutive apps ship as one-shot prompts.

## Later / ideas

- Resident maintenance agent on a schedule (dep bumps, backup verify, log triage).
- Generated app icons.
- Blob store in platformd when two apps first need shared files.
- Opt-in agentic LLM sessions for apps.

## Open questions

- **Domain:** which one? Cosmetic; decide by Phase 1.
- **Copilot terms:** skim before building anything high-frequency on the
  subscription (e.g. a 5-minute cron summarizer).
- **Copilot latency:** measure through the CLI runtime in Phase 5 before
  betting interactive features on it.
