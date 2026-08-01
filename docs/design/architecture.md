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
   ├── bespoke.example.com       ──► selfie:4000  platformd (dashboard/registry/LLM gateway)
   ├── hello.bespoke.example.com ──► selfie:4101
   └── <slug>.bespoke.example.com ──► selfie:<port>

APP HOST (`selfie`) — platformd + one process per app, systemd user units,
   each bound to selfie's tailscale interface (never 0.0.0.0, never public)

DEV MACHINE — builds (GOOS=linux), deploys via rsync/ssh to selfie,
   pushes generated Caddy routes to the edge host
```

- One Go process per app ([ADR-0005](../adr/0005-process-per-app.md));
  subdomain per app, wildcard cert via Cloudflare DNS challenge
  ([ADR-0003](../adr/0003-subdomain-per-app-routing.md)).
- platformd is deliberately thin: dashboard, registry (scans manifests),
  generated Caddy route config, LLM gateway ([llm-gateway.md](llm-gateway.md)).
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
├── CLAUDE.md            # conventions the agent treats as law
├── .claude/skills/      # new-app, new-component, ...
├── bin/bespoke          # CLI (see specs/bespoke-cli.md)
├── deploy/              # caddy snippet, systemd units, runbook (ADR-0011)
├── scripts/             # deploy.sh (until the bespoke CLI lands)
├── platform/            # platformd: dashboard + registry + LLM gateway
├── pkg/                 # auth, db, web, ui, llm, notify (ADR-0006)
├── design/              # input.css theme (ADR-0010) + design system docs
├── apps/
│   └── <slug>/
│       ├── app.toml     # manifest (specs/app-manifest.md)
│       ├── main.go
│       ├── app.go       # routes + handlers
│       ├── views/       # templ files
│       └── migrations/  # embedded .sql, run by pkg/db
└── data/                # SQLite + blobs (gitignored, Litestream-replicated)
```

Single Go module monorepo — one dependency graph
([ADR-0006](../adr/0006-library-first-shared-services.md)).

## Shared framework (`pkg/*`)

| Package | Provides |
| --- | --- |
| `pkg/auth` | Tailscale header middleware, `User(ctx)` |
| `pkg/db` | `Open(app)`: opens `data/<slug>.db`, runs embedded migrations, registers with Litestream |
| `pkg/web` | Server scaffold: routing, logging, graceful shutdown, loopback-only listener, Datastar SSE helpers |
| `pkg/ui` | Vendored templUI components + Bespoke wrappers implementing the design system |
| `pkg/llm` | Provider-neutral client for the platformd LLM gateway |
| `pkg/notify` | Push/ntfy/email from any app |

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
  (`ui.AppShell`, `ui.Page`, …) composing them into house idioms.
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

## Deploy loop

([ADR-0011](../adr/0011-split-host-deployment.md))

`bespoke deploy <slug>` from the dev machine → cross-compile
(`GOOS=linux`) → rsync binary + manifest + unit to selfie → restart unit,
await healthz → regenerate Caddy route import from manifests → push to the
edge host → `caddy reload`. Selfie needs no Go toolchain; it only receives
binaries. Details in the [CLI spec](../specs/bespoke-cli.md); Phase 1 uses
`scripts/deploy.sh` until the CLI exists.
