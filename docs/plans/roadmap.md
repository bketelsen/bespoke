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
- **Status (2026-08-01):** code complete and smoke-tested locally (auth
  gating, dashboard-from-manifests, hello). ⚠ **Remote steps UNVALIDATED** —
  edge Caddy rebuild, Cloudflare DNS, Tailscale ACL, and the first real
  deploy have not been executed; the runbook is written but untested. The
  done-when remains open until then.

## Phase 2 — Framework v0

- `pkg/auth`, `pkg/db` (migrations + WAL), `pkg/web` (scaffold, healthz,
  identity enforcement, logging, graceful shutdown).
- Port hello-world onto `pkg/*`.
- **Done when:** hello-world contains zero infrastructure code.
- **Status (2026-08-01):** ✅ done. hello is routes + one migration; platformd
  runs on `web.Serve`. SQLite driver is modernc.org/sqlite so deploys stay
  CGO-free. Local dev bypass: `BESPOKE_DEV_USER=me@github`. Datastar SSE
  helpers moved to Phase 3, where the first UI that needs them lands.

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
- Datastar SSE helpers in `pkg/web` (moved from Phase 2 — built alongside the
  first UI that uses them).
- Dashboard rebuilt on `pkg/ui`.
- **Done when:** dashboard and hello-world are visually indistinguishable in style.
- **Status (2026-08-01):** ✅ done. Nine components vendored (+icon with 1700
  Lucide icons); theme is "tailor's chalk" (warm paper, teal primary, copper
  accent) in `design/input.css`; `tw-animate-css` and `datastar.js` (v1.0.2)
  vendored; `web.NewSSE` helper wired. Compiled CSS and generated
  `*_templ.go` are COMMITTED and embedded, so builds/deploys need no UI
  toolchain — `scripts/setup-tools.sh` + `scripts/build-ui.sh` are needed
  only to change the design system. Dashboard and hello both render through
  `ui.AppShell` with the same embedded stylesheet, verified locally.
  In-browser visual pass still pending (headless environment).

## Phase 4 — Operations

- `bespoke` CLI per [spec](../specs/bespoke-cli.md): new/deploy/list/logs/rm,
  generated systemd units, Caddy route generation, rollback on failed healthz.
- Litestream config generation + restore drill.
- **Done when:** `bespoke new` → `bespoke deploy` works end to end and a
  restored DB matches.
- **Status (2026-08-01):** CLI implemented (`cmd/bespoke`; Justfile wraps it)
  with `new`/`dev`/`gen`/`deploy`/`list`/`logs`/`rm`. Validated locally:
  scaffold compiles immediately, `dev` runs everything from the manifests,
  generated units/routes/litestream carry GENERATED headers, `rm` degrades
  gracefully when the host is unreachable. ⚠ Remote paths (deploy with
  rollback, logs, rm cleanup, litestream restore drill) are **UNVALIDATED**
  until the Phase 1 runbook is executed — the done-when stays open.
  scripts/deploy.sh and the static units/caddy files are retired.

## Phase 5 — LLM gateway

- platformd `/llm` endpoint on the Copilot SDK per
  [design](../design/llm-gateway.md); `pkg/llm` client; CLI auth health check
  on the dashboard.
- Latency measurement (see open questions).
- **Done when:** an app calls `llm.Complete` with no Copilot-specific code.
- **Status (2026-08-01):** ✅ done, validated LIVE: `llm.Complete` and
  `llm.CompleteJSON` returned real completions through pkg/llm → gateway
  (internal port 4001) → Copilot CLI, ~1.3–1.8s per call, usage-logged.
  Sessions are inference-locked (no tools/skills/discovery, permissions
  denied, scratch working dir, deleted after use). Auth health checked every
  5 min with a dashboard warning banner. Streaming deferred until the first
  app needs it.

## Phase 6 — The agent layer (the point)

- `AGENTS.md` invariants + `.agents/skills/new-app` and `new-component`,
  portable across agents ([ADR-0013](../adr/0013-agent-portable-instruction-surface.md)).
- Build the next real app by prompt only; every manual intervention is filed
  as a framework bug and fixed.
- **Done when:** three consecutive apps ship as one-shot prompts.
- **Status (2026-08-01):** skills + cross-agent surface implemented —
  AGENTS.md canonical with CLAUDE.md/GEMINI.md/copilot-instructions symlinks,
  skills in `.agents/skills/` (symlinked for Claude). The done-when now
  depends on real usage: build the next three apps by one-shot prompt (any
  agent) and log every manual intervention here as a framework bug.
  Suggested first: the voice-notes app, which also wires the audio service.

## Later / ideas

- Internal services, built on demand per the
  [catalog](../design/internal-services.md) (ADR-0012): more LLM helpers
  (Summarize/Extract), files/blobs, notifications, search — never up front.
- Lemonade backend (local inference on selfie): embeddings, image
  generation, private/local completions behind the existing `pkg/llm` seam —
  ADR when first wired ([catalog](../design/internal-services.md#backends)).
  Ops prerequisites before anything can adopt it (as of 2026-08-01):
  - Re-download the Lemonade models on selfie — not reinstalled since the
    server rebuild.
  - Change Lemonade's listen address from `127.0.0.1` so the gateway can
    reach it during local dev (platformd on the dev machine → selfie over
    the tailnet). Prefer binding selfie's **tailscale interface** over
    `0.0.0.0` (ADR-0011: nothing binds all interfaces), and note its port
    sits outside the edge-only 4000-4999 ACL — add a tailnet ACL entry for
    dev-machine → selfie:<lemonade port> rather than leaving it open.
- **Audio (first-class, planned):** `pkg/audio` Transcribe/Speak via gateway
  `/audio/*` routes, Lemonade-backed — contract pinned in the
  [catalog](../design/internal-services.md#audio-planned-first-class-service);
  build with its first consumer. Suggested pairing: make the Phase 6 one-shot
  test app a voice-notes app so the first consumer and the skill test are the
  same exercise.
- Resident maintenance agent on a schedule (dep bumps, backup verify, log triage).
- Generated app icons.
- Blob store in platformd when two apps first need shared files.
- Opt-in agentic LLM sessions for apps.

## Open questions

- **Domain:** which one? Cosmetic; decide by Phase 1.
- **Copilot terms:** skim before building anything high-frequency on the
  subscription (e.g. a 5-minute cron summarizer).
- ~~**Copilot latency**~~ resolved 2026-08-01: ≈1.3–1.8s per simple
  completion locally — fine for summaries/classification, not keystroke-level
  interactivity (see [llm-gateway.md](../design/llm-gateway.md)).
