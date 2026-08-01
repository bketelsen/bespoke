# 0006 — Library-first shared services

- **Status:** Accepted
- **Date:** 2026-08-01

## Context

The original idea proposed "a single backend with shared services exposed as an
API." But every app is a Go server rendering its own HTML (ADR-0008): the
consumer of shared services is Go code, not a browser. Network APIs would add
versioning, serialization, and failure modes with no consumer that needs them.

## Decision

Shared services are **Go packages** (`pkg/*`) imported by apps, not network
services:

- `pkg/auth` — Tailscale header middleware (ADR-0004)
- `pkg/db` — per-app SQLite open + migrations + Litestream registration (ADR-0007)
- `pkg/web` — server scaffold: routing, logging, graceful shutdown, Datastar SSE
- `pkg/ui` — templ component library implementing the design system (ADR-0008)
- `pkg/llm` — thin client for the platformd LLM gateway (ADR-0009)
- `pkg/notify` — push/ntfy/email

platformd hosts a network service only where state (or a heavy runtime) is
genuinely shared across apps: the registry, the LLM gateway, and — if two apps
ever need shared uploads — a blob store.

The repo is a **single Go module monorepo**: one dependency graph, one `pkg/`
version, no per-app `go.mod`. An agent reasons about exactly one codebase.

## Consequences

- No API versioning tax; refactoring `pkg/*` updates all apps at compile time.
- Cross-cutting upgrades (e.g. a new design token) are one commit touching one
  place.
- Non-Go apps (the ADR-0005 escape hatch) don't get `pkg/*` and must call
  platformd's endpoints for anything shared — acceptable friction for a
  deliberate exception.

## Alternatives considered

- **go-micro:** effectively dormant, and solves microservice problems this
  system doesn't have. Rejected.
- **Embedded PocketBase** as the shared backend: buys admin UI, realtime, and
  file storage on day one, but adds a second paradigm alongside templ/Datastar
  and weakens control over conventions (ADR-0002). Benched — revisit if `pkg/`
  starts reinventing too much of it.
