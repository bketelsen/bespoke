# 0019 — User brief: per-person context for every LLM feature

- **Status:** Accepted
- **Date:** 2026-08-01

## Context

Every LLM feature (chat, summaries) knew the app's data but nothing about
the *person* — name ("bketelsen" instead of Brian), standing preferences
("never suggest bedtime"), the people and places entries refer to. Each app
teaching the model the same facts is repeated prompt boilerplate; a
platform capability should say it once.

## Decision

- **Storage:** platformd gets its own database (`data/platformd.db`, same
  `pkg/db` path as apps, added to Litestream generation). Table `briefs`
  keyed by Tailscale login — per-person, ready for a second tailnet user.
- **Editing:** `GET/POST /settings` on the dashboard (apex): two fields —
  "call me" and a freeform markdown brief. Deliberately schema-less beyond
  the name: prose about rules, people, places, and taste beats a form the
  model has to reassemble anyway; the placeholder suggests what's useful.
- **Injection at the gateway, not in apps:** `/llm/complete` gains a
  `login` field (`llm.WithUser(login)`); when present and a brief exists,
  the gateway prepends an "About the user (self-provided)" section to the
  system prompt (capped length). Chat and summaries pass the user
  automatically via pkg/web and the app code that already has one;
  `Classify` and other user-less calls are untouched — no brief skewing
  structured outputs.

## Consequences

- Personalization is platform-wide with near-zero app code (chat needed no
  app changes at all; journal's summary added one option).
- The brief is a prompt-injection surface by design — it is the user
  instructing their own model, same trust as the rest of the platform.
- platformd is now stateful: its database joins the backup set.

## Alternatives considered

- **Apps fetch the brief and assemble prompts:** N copies of the same
  boilerplate; the gateway is the single choke point all completions
  already pass through. Rejected.
- **Inject into every completion unconditionally:** would skew
  classification and other mechanical calls; opt-in-by-user-tag keeps
  intent explicit. Rejected.
- **Structured preference fields (units, tone, pronouns…):** premature
  schema for prose the model consumes as prose. Revisit if anything
  non-LLM ever needs to read a specific field.

## References

- Builds on: [ADR-0009](0009-copilot-sdk-llm-gateway.md),
  [ADR-0012](0012-internal-services-two-tier.md),
  [ADR-0007](0007-sqlite-per-app-litestream.md)
- Shapes: [design/llm-gateway.md](../design/llm-gateway.md),
  [design/internal-services.md](../design/internal-services.md)
