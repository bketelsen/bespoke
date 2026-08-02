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
  gating, dashboard-from-manifests, hello). Remote progress, evening:
  ✅ edge host live in deploy.env (`you@your-caddy-host`, ts <edge-tailscale-ip>); ✅ scoped
  sudoers installed and validated; ✅ **custom Caddy v2.11.4
  (tailscale + cloudflare-dns) built on the dev machine, pushed, and
  running on the edge** (`just caddy-push`; rollback kept as caddy.bak).
  ✅ **selfie deployed and validated** (same evening): `just deploy` shipped
  platformd/hello/journal, units active, all healthz green over the
  tailnet, audio gateway in real mode (Lemonade via localhost), dashboard
  rendering with the switcher. LLM gateway degraded as designed — copilot
  CLI not installed on selfie yet (install + `copilot` sign-in there to
  light up chat/summaries in prod). ✅ Domain chosen and DNS validated:
  `bespoke.ketelsen.cloud` + wildcard → <edge-tailscale-ip> (apex, hello,
  journal, and a random name all resolve); selfie env + deploy.env
  updated, apps redeployed, routes pushed to the edge with the real
  domain. ✅ **DONE-WHEN CLOSED (2026-08-01):** import line + token drop-in
  landed, caddy restarted, wildcard cert issued
  (`*.bespoke.ketelsen.cloud`, expires 2026-10-31), and
  `https://hello.bespoke.ketelsen.cloud` renders "Hello, Brian Ketelsen —
  Authenticated as bketelsen" through the full real path (tailscale_auth
  on the edge → selfie). Dashboard and journal live over HTTPS. ⚠ One
  security item before calling it production: the tailnet ACL below.
  Superseded checklist (kept for history): add
  `import /etc/caddy/bespoke.caddy` to the main Caddyfile, add the
  `CLOUDFLARE_API_TOKEN` drop-in, reload; and the tailnet ACL for
  selfie:4000-4999 (verified needed: a fake identity header from another
  tailnet device reaches apps today). Then the done-when closes at
  `https://hello.bespoke.ketelsen.cloud`.

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
  gracefully when the host is unreachable. **Remote deploy VALIDATED
  2026-08-01**: `just deploy` shipped three apps to selfie end to end —
  staged binary swap, unit restarts, healthz gates all green, repeated for
  the domain change and the `--edge` route push. Still unexercised:
  rollback-on-failed-healthz (needs a bad deploy to prove it), remote
  `logs`/`rm`, and the litestream restore drill (litestream not yet
  installed on selfie) — the done-when stays open on the restore drill.
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
  - **One-shot log (1/3): journal** (2026-08-01, "build me a journal") —
    design-app interview (4 questions) → spec → built and verified live,
    including an LLM weekly summary through the gateway. Interventions: none
    outside the skills' own paths (textarea vendored via the new-component
    route). Journal is also the designated first consumer for the audio
    service (voice capture) once the Lemonade backlog clears.

## Later / ideas

- Internal services, built on demand per the
  [catalog](../design/internal-services.md) (ADR-0012): more LLM helpers
  (Summarize/Extract), files/blobs, notifications, search — never up front.
- Lemonade backend (local inference on selfie): embeddings, image
  generation, private/local completions behind the existing `pkg/llm` seam —
  ADR when first wired ([catalog](../design/internal-services.md#backends)).
  **Status 2026-08-01: RESOLVED — Lemonade fully operational** at
  `http://<app-host-ip>:13305/api/v1`. Verified live: transcription
  (Whisper-Large-v3-Turbo, first load was just slow), TTS (kokoro-v1),
  embeddings model downloaded (nomic-embed-text-v2-moe). A real voice
  entry went through journal → gateway → whisper end to end. Remaining
  candidates (embeddings/image/private-completion helpers) build on
  demand per the catalog.
- **Audio:** transcription is LIVE stub-first with journal voice capture as
  first consumer ([ADR-0014](../adr/0014-audio-service-transcription.md)) —
  flipping to real transcription = clear the Lemonade backlog above, then
  set `BESPOKE_LEMONADE_URL` (+ optionally `BESPOKE_AUDIO_MODEL`) in
  platformd's env and validate the OpenAI-compatible request shape against
  the live server. `Speak` stays planned until its first consumer.
- **Builder app (idea, 2026-08-01): the app that builds new apps.** A
  Bespoke app on the dashboard where you type "build me a…" and watch it
  ship: the [design-app](../../.agents/skills/design-app/SKILL.md) interview
  as a chat UI (Datastar SSE), then an agentic session executing the
  new-app skill ON selfie against a repo clone, `bespoke deploy` from there,
  live progress streamed, new app appears on the dashboard. What it touches:
  builds move dev-machine → selfie (refines
  [ADR-0011](../adr/0011-split-host-deployment.md); selfie gains a Go
  toolchain), the LLM gateway grows the opt-in *agentic* session type
  ADR-0009 deferred (tools/file access enabled — needs guardrails: dedicated
  workdir, branch-only commits, deploy gated on `just check`), and the
  Copilot runtime already living on selfie does the heavy lifting. Big; park
  until the one-shot log hits 3/3 and the skills are proven boring.
- **MCP surface (idea, 2026-08-01): every app's actions available to any
  LLM.** One aggregate MCP server on platformd (official Go SDK, Streamable
  HTTP) at the apex `/mcp`, routed by Caddy like any page. Apps opt
  functions in with a one-line `web.Tool("get_entries", desc, schema, fn)`
  next to their routes; pkg/web serves a tool manifest at a well-known
  path; platformd aggregates via the registry and namespaces as
  `<slug>_get_entries` (MCP names forbid dots). Identity flows through
  free: tailnet client → Caddy stamps Tailscale headers → platformd proxies
  tool calls to apps as that user — same auth model as the browser, no
  tokens. Add the server once per MCP client; every future app's tools
  appear automatically (and the new-app skill gains an "expose your core
  actions" step). The LLM gateway's future agentic sessions can consume the
  same surface (Copilot SDK takes MCP configs) — apps calling apps through
  the front door. Guardrail to decide at build time: write-capable tools
  opt-in/flagged, read-only the default. Builds on ADR-0012's plane; gets
  its ADR when adopted.
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
