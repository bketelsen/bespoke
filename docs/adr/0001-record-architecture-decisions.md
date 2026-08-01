# 0001 — Record architecture decisions

- **Status:** Accepted
- **Date:** 2026-08-01

## Context

Bespoke is a long-lived personal platform that will be built and maintained
largely by LLM agents across many sessions. Decisions and their rationale must
be discoverable by an agent (or a future me) with no conversational context.

## Decision

Record every significant architecture decision as a numbered ADR in
`docs/adr/`, using this format: Status, Date, Context, Decision, Consequences,
Alternatives considered. ADRs are immutable once accepted; a reversal is a new
ADR that marks the old one `Superseded by NNNN`.

## Consequences

- Agents can be pointed at `docs/adr/` to learn why things are the way they are
  before proposing changes.
- Slight writing overhead per decision; worth it for a project whose whole
  premise is agent-maintainability.
