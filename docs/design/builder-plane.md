# Builder Plane

Living document. Rationale:
[ADR-0023](../adr/0023-builder-plane-unprivileged-agent-spooled-deploys.md).
Contracts: spool file formats below (graduate to a spec when a second
producer appears).

## Overview

The machinery that lets the **builder app** turn an approved spec into a
deployed app with no human past the spec gate. Three trust domains on the
app host, connected only by files in shared spool directories:

```
builder app (bjk, ordinary Bespoke app)
   │  build request (JSON)                     ▲ events (JSONL), git bundle
   ▼                                           │
build spool  /var/lib/bespoke/spool/build   ───┘
   │
   ▼
runner (builder user, own user-systemd + own Copilot CLI/auth)
   agentic session per run, WorkingDirectory = fresh clone,
   sandbox test on 127.0.0.1:42xxx (BESPOKE_LISTEN + BESPOKE_DATA)
   │
   ▼  deploy request (JSON)
deploy spool /var/lib/bespoke/spool/deploy
   │
   ▼
deploy watcher (bjk, systemd --user path unit + oneshot, generated)
   fetch bundle → just check → push main → quiesce → bespoke deploy <slug> --edge
```

## Design

- **Builder app** (`apps/builder/`): ordinary app; owns the interview chat,
  the spec gate, run state (`runs`, `run_events` in its SQLite), and the
  live progress UI. It writes requests into the spools and tails result
  files; it never executes builds or deploys itself.
- **Runner** (`cmd/builder-runner`, deployed to `/home/builder/bin`): a
  path-unit-triggered service in the `builder` user's systemd. Per build:
  fresh clone of the repo (read-only remote), one agentic Copilot SDK
  session (`WorkingDirectory` = clone, permissions auto-approved, repo
  AGENTS.md + skills loaded from the working directory), streams session
  events to `<run>.events.jsonl`, runs the sandbox verification, commits,
  and writes `<run>.bundle` (git bundle of the new commits) + a final
  status line. The runner has no push credentials and no access to prod
  state.
- **Deploy watcher** (`bespoke gen`-generated `bespoke-deploywatch.path` +
  `.service` under `bjk`): on a deploy request, fetches the bundle into
  the canonical on-host clone, runs `just check` (nothing agent-produced
  is trusted before this), pushes `main`, polls `GET /llm/activity` until
  the gateway is idle (bounded wait + grace), then `bespoke deploy <slug>
  --edge`, and writes `<run>.result.json`.
- **Gateway activity endpoint** (`platform/llm.go`): `GET /llm/activity`
  on the 4001 plane → `{"inflight": n, "idle_seconds": s}` counting
  in-flight `/llm/complete` calls. See
  [llm-gateway.md](llm-gateway.md).
- **Ports:** manifest port assigned by `bespoke new` (4101–4999) as usual;
  sandbox tests bind `127.0.0.1:42<port-suffix>` via `BESPOKE_LISTEN`,
  which overrides the manifest value without touching the manifest.

### Spool contracts (v1)

Rooted at `/var/lib/bespoke/spool` (`BESPOKE_SPOOL` overrides; app-side dev
fallback `./data/spool`). All dirs are setgid group `bespoke` (members: the
platform user and `builder`), mode 2770. Run IDs look like `r<unix-ms>`.

- `build/<run>.request.json` — `{"run", "slug", "idea", "spec_markdown"}`;
  the path units watch `build/` and `deploy/` with `DirectoryNotEmpty=`,
  so watched dirs hold ONLY pending requests. Processors archive the
  request to `archive/` *before* working (at-most-once; a crash never
  retriggers forever — the outcome files are the durable record).
- `runs/<run>/events.jsonl` — appended by runner and watcher both;
  `{"ts", "kind", "text"}`, kinds `status`, `agent`, `tool`, `error`,
  `deploy`. The builder app tails this into its own `run_events`.
- `runs/<run>/repo.bundle` — git bundle of the run's new commits on `main`.
- `runs/<run>/status.json` — runner outcome `{"run", "ok", "detail"}`.
- `deploy/<run>.request.json` — `{"run", "slug"}` (bundle path is implied).
- `runs/<run>/deploy.json` — watcher outcome
  `{"run", "ok", "detail", "deployed_at"}`.

Outcome files are written `.tmp`-then-rename so readers never see partial
JSON. Shared binaries (`bespoke`, `builder-runner`, `copilot`) live in
`/var/lib/bespoke/bin` (2775), delivered by deploy, executed by both users.

## Operational notes

- **One-time root bootstrap** (`deploy/bootstrap-builder.sh`, run with
  sudo): create `builder` user + `bespoke` group, add both users, create
  spool dirs, `loginctl enable-linger builder`. Everything after is
  unprivileged.
- **Per-user Copilot auth:** the CLI must be logged in for `builder`
  separately (`sudo -iu builder copilot` once, interactive). If auth
  expires, runs fail at session start — the runner writes an `error`
  event; the builder app surfaces it.
- **Toolchain on the app host:** Go, just, golangci-lint via Homebrew
  (`/home/linuxbrew`, world-readable, shared by both users). Refines
  ADR-0011's "no toolchain on the app host".
- **Self-deploys:** the watcher runs outside every app's cgroup, so
  deploying the builder app itself (or a future `--all`) cannot kill the
  deploy mid-flight. Every deploy still restarts platformd (sub-second;
  dashboard SSE reconnects).
- **Quiesce race:** a completion starting between the idle poll and the
  restart is lost; the wait bounds are config in the watcher script, not
  platform API.
- **Failure modes:** runner dead → requests sit in the spool untouched
  (builder app shows "queued" with age; `systemctl --user status` under
  `builder`). Watcher dead → same, deploy side, under `bjk`. Bundle that
  fails `just check` → result file `ok:false` with the check output;
  nothing was pushed or deployed.
- **One run at a time:** the runner processes requests serially; the
  builder app enforces a single active run in its own state.

## References

- Rationale:
  [ADR-0023](../adr/0023-builder-plane-unprivileged-agent-spooled-deploys.md),
  [ADR-0011](../adr/0011-split-host-deployment.md),
  [ADR-0009](../adr/0009-copilot-sdk-llm-gateway.md)
- Contracts: spool formats above; gateway wire format in
  [llm-gateway.md](llm-gateway.md);
  [specs/bespoke-cli.md](../specs/bespoke-cli.md) (deploy semantics)
- Built in: [roadmap — Later/ideas → Builder app](../plans/roadmap.md#later--ideas)
