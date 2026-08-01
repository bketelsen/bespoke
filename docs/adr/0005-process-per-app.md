# 0005 — One process per app

- **Status:** Accepted
- **Date:** 2026-08-01

## Context

Apps will be written and modified by LLM agents. A bad deploy of one app must
not take down the others. Deploys should be seconds, scoped to one app.

## Decision

Each app is a small Go binary running as its own systemd user unit, listening
on an assigned loopback port (see the [app manifest spec](../specs/app-manifest.md)).
Caddy routes each subdomain to its port. A thin `platformd` process serves the
dashboard/registry and the few genuinely shared services (ADR-0006, ADR-0009).

The routing contract is language-agnostic: anything that speaks HTTP on its
assigned loopback port and honors the auth header (ADR-0004) is a valid app.
Go + the framework packages is the default and the only path the agent skills
scaffold; other runtimes are deliberate exceptions.

## Consequences

- Crash isolation: an agent breaking app #5 leaves apps #1–4 running.
- Per-app deploy is `go build` + unit restart, ~1 second.
- Needs port allocation (sequential, recorded in the manifest) and generated
  systemd units — handled by the `bespoke` CLI.
- More processes than a monolith; each Go binary is small, so the overhead is
  negligible at personal scale.

## Alternatives considered

- **Single monolith routing by Host header:** simplest ops, but one bad deploy
  bricks everything and per-app rollback is awkward. Rejected.
- **Containers per app** (prior-art article uses Docker Compose): heavier than
  needed for single-binary Go apps on one host; systemd units give the same
  isolation-of-lifecycle with less machinery.
