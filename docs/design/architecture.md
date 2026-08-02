# Architecture

Living document — kept current as the system evolves. Rationale lives in the
[ADRs](../adr/); contracts live in the [specs](../specs/).

## Topology

Three hosts, connected only over the tailnet
([ADR-0011](../adr/0011-split-host-deployment.md)):

```
browser (tailnet)
   │
   ▼
EDGE HOST — existing Caddy + caddy-tailscale + caddy-dns/cloudflare
   │  wildcard *.bespoke.example.com (Cloudflare DNS ACME challenge)
   │  tailscale auth → strips inbound + sets Tailscale-User-* headers
   │
   │  tailnet; ACL: only edge may reach selfie:4000-4999
   ├── bespoke.example.com       ──► selfie:4000  platformd (apex)
   ├── hello.bespoke.example.com ──► selfie:4101
   └── <slug>.bespoke.example.com ──► selfie:<port>

APP HOST (`selfie`) — platformd + one process per app, systemd user units,
   each bound to selfie's tailscale interface (never 0.0.0.0, never public);
   platformd also owns the internal services plane on 127.0.0.1:4001
   and talks to a local Lemonade server for speech (never routed)

DEV MACHINE — builds (GOOS=linux), deploys via rsync/ssh to selfie,
   pushes generated Caddy routes to the edge host
```

(`bespoke.example.com` is the fork-friendly placeholder throughout the
docs; the reference deployment lives at `bespoke.ketelsen.cloud`.)

- One Go process per app ([ADR-0005](../adr/0005-process-per-app.md));
  subdomain per app, wildcard cert via Cloudflare DNS challenge
  ([ADR-0003](../adr/0003-subdomain-per-app-routing.md)).
- platformd wears two hats. On the apex (4000): dashboard with live
  per-user cards fetched from each app's `/_card`
  ([ADR-0017](../adr/0017-app-provided-dashboard-cards.md),
  [ADR-0022](../adr/0022-live-updates.md)), cross-app agentic chat over
  every app's context and tools
  ([ADR-0020](../adr/0020-dashboard-chat-aggregated-context.md),
  [ADR-0021](../adr/0021-tools-agentic-chat-mcp.md)), `/settings` for the
  per-user brief ([ADR-0019](../adr/0019-user-brief.md)), and the MCP
  endpoint `/mcp` exposing every app's tools to external LLM clients. On
  the internal plane (4001, never Caddy-routed,
  [ADR-0012](../adr/0012-internal-services-two-tier.md)): the LLM gateway
  ([llm-gateway.md](llm-gateway.md)), the audio gateway
  (transcribe/speak via Lemonade,
  [ADR-0014](../adr/0014-audio-service-transcription.md)), and `/notify`
  fan-out for live updates. The registry is just the manifests, rescanned.
- The Tailscale ACL restricting app ports to the edge host is a security
  invariant, same standing as header stripping.

## Auth flow

([ADR-0004](../adr/0004-tailscale-identity-via-caddy.md))

1. Caddy terminates TLS; connection arrives over the tailnet.
2. The caddy-tailscale plugin authenticates against tailscaled and sets
   `Tailscale-User-Login` / `Tailscale-User-Name`.
3. Caddy strips inbound copies of those headers first — **non-negotiable**.
4. Apps use `pkg/auth` middleware only: reject if absent, else `auth.User(ctx)`.
5. App processes bind to selfie's tailscale interface; the ACL ensures only
   the edge host can reach those ports
   ([ADR-0011](../adr/0011-split-host-deployment.md)). Local dev binds
   loopback and fakes the header.

## Repo layout

```
bespoke/
├── AGENTS.md            # conventions as law (ADR-0013); CLAUDE.md/GEMINI.md/
│                        #   .github/copilot-instructions.md are symlinks to it
├── MAKE-IT-YOUR-OWN.md  # the forking skill (also symlinked into skills)
├── .agents/skills/      # design-app, new-app, new-component, make-it-your-own
│                        #   (.claude/skills is a symlink here)
├── .github/             # CI (runs `just check`), issue/PR templates, dependabot
├── docs/                # adr/ design/ specs/ plans/ — see docs/README.md
├── cmd/bespoke/         # the platform CLI (specs/bespoke-cli.md)
├── internal/manifest/   # app.toml parsing + validation (the registry)
├── deploy/              # deploy.env + runbook (ADR-0011); units/routes are
│                        #   GENERATED into dist/gen/, never hand-written
├── scripts/             # UI toolchain + edge-Caddy build helpers
├── tools/               # vendored build tools (templ, templui, tailwind, xcaddy)
├── platform/            # platformd: apex (dashboard/chat/MCP/settings)
│                        #   + internal plane (LLM/audio/notify)
├── pkg/                 # auth, db, web, ui, llm, audio (ADR-0006)
├── design/              # input.css theme (ADR-0010)
├── apps/
│   └── <slug>/
│       ├── app.toml     # manifest, incl. [[intents]] (specs/app-manifest.md)
│       ├── main.go      # web.Run(slug, register): routes + handlers
│       ├── tools.go     # web.Tool definitions (ADR-0021), if any
│       ├── README.md    # the app's spec + non-goals
│       ├── views/       # templ files
│       └── migrations/  # embedded .sql, run by pkg/db
├── dist/                # build output + generated artifacts (gitignored)
└── data/                # SQLite + blobs (gitignored, Litestream-replicated)
```

Single Go module monorepo — one dependency graph
([ADR-0006](../adr/0006-library-first-shared-services.md)).

## Shared framework (`pkg/*`)

| Package | Provides |
| --- | --- |
| `pkg/auth` | Tailscale header middleware, `FromContext(ctx)` |
| `pkg/db` | `Open(slug, migrations)`: opens `data/<slug>.db`, WAL, embedded migrations |
| `pkg/web` | Server scaffold (routing, logging, graceful shutdown, healthz, asset serving) **plus the whole app contract**: `EnableChat`/`EnableChatWithTools` (ADR-0015/0021), `Tool` (ADR-0021), `Intent` (ADR-0018), `DashboardCard` (ADR-0017), `Live`/`Changed` (ADR-0022), Datastar SSE helpers. Binds loopback in dev, selfie's tailscale IP in prod (`BESPOKE_BIND_IP`) |
| `pkg/ui` | Vendored templUI components + Bespoke wrappers (AppShell chrome, VoiceButton, Markdown, intents popover) implementing the design system |
| `pkg/llm` | Provider-neutral gateway client: `Complete`/`CompleteJSON`/`Classify`, `WithSystem`/`WithUser`/`WithTools` — the last making chats agentic |
| `pkg/audio` | `Transcribe` + `Speak` via the audio gateway (ADR-0014) — speech in and out for any app |

## Data & backups

([ADR-0007](../adr/0007-sqlite-per-app-litestream.md))

- One SQLite file per app in `data/`, WAL mode, via `pkg/db`.
- Litestream replicates `data/*.db` to object storage; config generated from
  manifests, so new apps are backed up automatically.
- Cross-app data goes through platformd or not at all.

## Design system

([ADR-0008](../adr/0008-go-templ-datastar-frontend.md),
[ADR-0010](../adr/0010-templui-component-base.md))

- `pkg/ui`: [templUI](https://templui.io) components vendored via the
  `templui` CLI (never hand-edited), plus Bespoke wrapper components
  (`ui.AppShell`, `ui.VoiceButton`, `ui.Markdown`, `ui.IntentConfirm`, …)
  composing them into house idioms.
- `design/input.css`: the bespoke theme — CSS variables (oklch colors, radius,
  typography), light/dark. This file is the visual identity; change it and
  every app restyles.
- CSS is compiled by the Tailwind v4 **standalone binary**
  (`scripts/build-ui.sh`); the compiled stylesheet and generated templ code
  are committed and embedded via go:embed, served by every app at
  `/_bespoke/` — builds and deploys need no UI toolchain, and there is still
  no Node anywhere.
- Apps compose `pkg/ui`; Tailwind utilities for layout only, theme tokens
  only, no custom CSS files.
- The AppShell is platform chrome
  ([ADR-0015](../adr/0015-appshell-platform-chrome.md)): app switcher fed
  from the registry via request context (zero app code), an in-app LLM
  chat panel when the app opts in with `web.EnableChat` — agentic over the
  app's registered tools ([ADR-0021](../adr/0021-tools-agentic-chat-mcp.md)),
  with mic input and a persisted speak toggle — and the selection popover
  offering other apps' intents
  ([ADR-0018](../adr/0018-cross-app-intents.md)).
- Mobile-first is a standing invariant
  ([ADR-0016](../adr/0016-mobile-first-ui-standard.md)): 375px/coarse-pointer
  usability, enforced partly by the design system (16px input rule,
  dvh-capped chat panel, breaking markdown, truncating header) and partly by
  per-view rules the skills gate on.

## Deploy loop

([ADR-0011](../adr/0011-split-host-deployment.md))

`just deploy` (→ `bespoke deploy --all`) from the dev machine → regenerate
units/routes/litestream from manifests → cross-compile (`CGO_ENABLED=0
GOOS=linux`) → rsync to selfie (binaries staged, old kept as `.prev`) →
restart units, await healthz, roll back on failure → `--edge` pushes the
generated Caddy routes and reloads. Selfie needs no Go toolchain; it only
receives binaries. Details in the [CLI spec](../specs/bespoke-cli.md).

CI (`.github/workflows/ci.yml`) runs `just check` — vet, tests,
golangci-lint, `go mod tidy -diff`, and the same CGO-free cross-compile the
deploy uses — so the local gate and the merge gate are one recipe.
