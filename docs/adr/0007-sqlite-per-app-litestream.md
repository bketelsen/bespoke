# 0007 — SQLite per app + Litestream

- **Status:** Accepted
- **Date:** 2026-08-01

## Context

Personal-scale apps on a single host. Storage should require zero
administration, isolate apps from each other, and be backed up continuously
without per-app effort.

## Decision

Each app gets one SQLite database file at `data/<slug>.db`, WAL mode, opened
via `pkg/db.Open(app)`, which also runs the app's embedded migrations
(`migrations/*.sql`). Litestream replicates `data/*.db` to object storage
(R2/B2); its config is generated from the app manifests so every new app is
backed up automatically.

## Consequences

- No database server to run; backups need no per-app thought.
- An agent-authored bad migration corrupts at most one app's data, and
  point-in-time restore is per-app.
- Cross-app queries are impossible by construction — data shared between apps
  must go through platformd (ADR-0006). This is a feature.

## Alternatives considered

- **Shared Postgres:** one more daemon, and one blast radius. Overkill at this
  scale.
- **One shared SQLite file:** couples app schemas and makes restore
  all-or-nothing. Rejected.
