# 0008 — Go + templ + Datastar frontend, enforced design system

- **Status:** Accepted — refined by [ADR-0010](0010-templui-component-base.md)
  (pkg/ui is built on vendored templUI components rather than hand-rolled)
- **Date:** 2026-08-01

## Context

The author dislikes writing React; with an LLM doing the writing, language
familiarity matters less than convention-tightness (ADR-0002). A pile of
one-off apps should still feel like one coherent product.

## Decision

Apps are server-rendered Go using [templ](https://templ.guide) for views and
[Datastar](https://data-star.dev) for interactivity (SSE-driven updates), via
helpers in `pkg/web`. No Node toolchain, no per-app JS build.

The design system is:

- `design/tokens.css` — colors, type scale, spacing, dark mode; authored once
  (with Claude), referenced everywhere.
- `pkg/ui` — templ component library (layout shell, nav, cards, forms, tables).

**Apps may not write ad-hoc CSS** or bypass `pkg/ui`; they compose components.
Changing the look means changing tokens/components once — every app updates.

## Consequences

- One deployable per app, no node_modules, one rendering paradigm for agents
  to learn.
- Datastar's SSE model covers live-updating UI that plain htmx struggles with.
- A future app that truly needs SPA-grade interactivity is an ADR-0005
  exception (any-HTTP-process contract), not a framework change.

## Alternatives considered

- **React + Tailwind + shadcn/ui** (prior-art article's choice): the strongest
  argument is shadcn's enormous LLM training footprint. Rejected on stack
  weight (Node builds per app) and author preference; conventions-rigidity is
  the counter-bet.
- **htmx:** fine, but Datastar subsumes its patterns and handles streaming
  updates better.
- **Svelte:** nicer than React, still a JS build per app. Escape hatch only.
