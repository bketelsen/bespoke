# 0033 — Give each app its own data directory and filesystem scope

- **Status:** Accepted
- **Date:** 2026-08-04

## Context

[ADR-0032](0032-app-unit-sandboxing.md) bounded what one app process can reach
but left the exposure that matters: every app's SQLite file sat in one
`~/bespoke/data`, owned by one UID, so any app could read any other app's
database. With apps now installable from other people's modules
([ADR-0031](0031-third-party-app-packages.md)), that is the gap.

`pkg/db.Open` already reads `BESPOKE_DATA` before falling back to
`$BESPOKE_ROOT/data`, and the mail app resolves its attachment store the same
way. Nothing in the platform requires the flat layout.

Three runtime reads constrain how far an app can be confined. `web.Run` loads
`$BESPOKE_ROOT/apps/<slug>/app.toml` for its port; the app shell builds its
switcher from *every* manifest; and `pkg/ui` serves `$BESPOKE_ROOT/assets/styles.css`
from disk with the embedded copy as fallback.

## Decision

- Each app's unit sets `BESPOKE_DATA=%h/bespoke/data/<slug>`, so its database
  and any sibling files it keeps live under a directory of its own.
- App units replace the home directory with an empty tmpfs (`ProtectHome=tmpfs`,
  `ProtectSystem=strict`) and mount back exactly four things: the app's own
  binary, `apps/` (read-only, for the manifests), `assets/` (read-only), and its
  own data directory (writable).
- The result is absence, not merely denial: inside the sandbox `data/` contains
  one subdirectory. Verified on systemd 261 with a probe exercising the real
  workload — manifest read, switcher read, stylesheet read, database write,
  attachment mkdir, listen, outbound HTTPS — while a sibling's database was
  reported as `no such file or directory`.
- Deploy creates every app's data directory before units start. A missing
  `BindPaths` source fails the unit rather than letting it write unscoped.
- platformd is deliberately not scoped: it reads every manifest and execs
  `copilot`. Litestream is deliberately not scoped: reading every app database
  is its purpose. Both keep ADR-0032's process-level sandbox.
- Litestream paths follow the data to `<slug>/<slug>.db`; replica URLs do not
  change, so replication history survives the move.

## Consequences

- An app can no longer read another app's data, which is what made installing
  someone else's app a bounded decision rather than a total one.
- Existing hosts need a migration: databases, their `-wal`/`-shm` files, and
  mail's attachment directory move into per-app directories while units are
  stopped. It is not automatic, and doing it wrong loses data.
- An app that writes beside its database keeps working only because it derives
  the path from `BESPOKE_DATA`. One that hardcodes `~/bespoke/data` breaks —
  loudly, at the first write.
- Retired apps' preserved databases stay at the old flat path, unreferenced.
- The remaining exposure is egress: a scoped app still reaches the network and
  can send its own data anywhere. That needs per-app UIDs and firewall rules.

## Alternatives considered

- **`InaccessiblePaths=` per sibling:** every app's unit would have to be
  regenerated whenever any other app is added, and it denies rather than hides.
- **A UID per app:** the complete answer, and the only route to egress rules,
  but it rewrites deploy, data ownership, and the services plane. Still open.
- **Keeping the flat layout and binding individual files:** `BindPaths` sources
  must exist, and SQLite creates `-wal`/`-shm` on demand.

## References

- Shapes: [CLI spec](../specs/bespoke-cli.md),
  [deploy runbook](../../deploy/README.md)
- Builds on: [ADR-0007](0007-sqlite-per-app-litestream.md),
  [ADR-0031](0031-third-party-app-packages.md),
  [ADR-0032](0032-app-unit-sandboxing.md)
