# 0012 — Internal shared services: helpers first, services on demand

- **Status:** Accepted
- **Date:** 2026-08-01

## Context

The LLM gateway ([ADR-0009](0009-copilot-sdk-llm-gateway.md)) created an
internal listener on platformd (port 4001) — a network surface apps reach
that Caddy never routes. More shared capabilities will follow (classify,
summarize, files, notifications, search…). Two temptations exist: build a
service layer up front (violates the build-as-needed instinct and
[ADR-0002](0002-optimize-for-one-shot-agent-reliability.md)'s bias for
boring), or let each capability pick its own shape ad hoc (drift).

Observation: most wished-for "services" (e.g. `llm.Classify(text,
categories)`) are pure functions over existing primitives and need no new
runtime at all.

## Decision

Shared capabilities are added in two tiers, always on demand, never
speculatively:

- **Tier 1 (default): capability helpers** — plain functions in `pkg/*`
  composing existing primitives. `llm.Classify` is the canonical example: a
  method on `pkg/llm.Client` built on `CompleteJSON`, with the result
  validated against the caller's categories. No new process, port, or config.
- **Tier 2: internal services** — endpoints on platformd's internal listener,
  ONLY when the [ADR-0006](0006-library-first-shared-services.md) test is
  met: cross-app state or a heavy shared runtime. Each internal service gets
  a path prefix (`/llm/…`, `/files/…`), a `healthz`, per-call usage logging,
  and a `pkg/<name>` client — **apps never speak raw HTTP to the plane**.

Every capability, current or candidate, is cataloged in
[design/internal-services.md](../design/internal-services.md); adding one
starts with the decision tree there. Escalating a helper to a service later
is a client-compatible change (the `pkg/*` signature stays).

## Consequences

- Cheap capabilities stay cheap: a new helper is a function + a doc line.
- The internal plane stays uniform (one port, one convention for health,
  logging, and clients) instead of accreting bespoke shapes.
- The catalog gives agents one place to check "does this already exist?"
  before building — a one-shot-reliability win.
- Non-LLM services (files, notify, cron) inherit a ready-made home when they
  become real.

## Alternatives considered

- **Separate processes per internal service:** more units and ports for no
  isolation benefit at personal scale; platformd is already the shared-state
  home. Rejected until a service's blast radius demands it.
- **Build the service layer up front:** speculative surface area an agent can
  misuse; contradicts build-as-needed. Rejected.

## References

- Refines: [ADR-0006](0006-library-first-shared-services.md),
  [ADR-0009](0009-copilot-sdk-llm-gateway.md)
- Shapes: [design/internal-services.md](../design/internal-services.md),
  [design/llm-gateway.md](../design/llm-gateway.md)
