# 0002 — Optimize for one-shot agent reliability

- **Status:** Accepted
- **Date:** 2026-08-01

## Context

The point of Bespoke is not any single app — it is making the *next* app nearly
free: a one-line prompt ("build me an app that X") should reliably produce a
working, deployed, on-brand app. Framework design choices often trade
flexibility against predictability.

## Decision

Every framework decision is scored against one fitness function: **does it make
the one-shot "build me an app that X" prompt more reliable?** A choice that
adds flexibility but hurts one-shot reliability is rejected.

Concrete consequences of this rule appear throughout the other ADRs: rigid
conventions, one blessed stack, no per-app deviation in auth, styling, storage,
or deployment.

## Consequences

- Conventions read as strict, even authoritarian ("apps may not write ad-hoc
  CSS"). That is intentional: every degree of freedom removed from an app is a
  failure mode removed from the one-shot prompt.
- Escape hatches exist but must be deliberate, documented exceptions (see
  ADR-0005's language-agnostic process contract), never the default path.

## Alternatives considered

- **Flexibility-first framework** (apps pick their own stack/storage/styling):
  matches how humans like to work, but each degree of freedom multiplies the
  ways an agent-generated app can be subtly wrong. Rejected.
