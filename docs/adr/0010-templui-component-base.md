# 0010 — Build pkg/ui on vendored templUI components

- **Status:** Accepted
- **Date:** 2026-08-01

## Context

[ADR-0008](0008-go-templ-datastar-frontend.md) committed to a design system in
`pkg/ui` but implied hand-building the component library. Hand-rolling 40
accessible components (dialogs, date pickers, toasts…) is months of work and
the least differentiated part of the system.

[templUI](https://templui.io) is a shadcn-style library for exactly our stack:
40+ accessible templ components, a CLI that **vendors component source into
your own repo**, theming via CSS custom properties (oklch, light/dark via
`.dark`), per-component vanilla JS loaded explicitly via `@component.Script()`.
It requires Tailwind CSS v4.1+, available as a **standalone binary** — no Node
toolchain, preserving ADR-0008's constraint.

## Decision

`pkg/ui` is built on templUI components vendored via the `templui` CLI, under
a customized theme:

- **Vendored, not imported.** Component source lives in-repo (`pkg/ui/`), so
  agents can read exactly what renders. The Go-module import mode is rejected.
- **Vendored files are never hand-edited** — `templui` updates overwrite them.
  Customization happens in exactly two places: the theme, and wrapper
  components in `pkg/ui` (e.g. `ui.AppShell`, `ui.Page`) that compose vendored
  parts into Bespoke idioms.
- **The bespoke theme** is CSS variables in `design/input.css` (colors as
  oklch, radius, typography), authored with Claude. This file *is* the visual
  identity; changing it restyles every app.
- **Styling rules for apps:** compose `pkg/ui`; Tailwind utilities are allowed
  for layout only and must use theme tokens — no arbitrary colors/values, no
  custom CSS files (unchanged spirit of ADR-0008).
- **JS coexistence:** templUI's component scripts handle widget behavior;
  Datastar handles app interactivity and SSE. Both are loaded by the
  `pkg/ui` layout shell.

## Consequences

- Phase 3 shrinks from "build six components" to "vendor + theme + wrap".
  Accessibility comes largely solved.
- New bootstrap dependencies: `templui` CLI and the Tailwind v4 standalone
  binary; CSS compilation joins the `bespoke deploy` build step.
- Component updates are a deliberate `templui` run + diff review, never a
  silent dependency bump.

## Alternatives considered

- **Hand-rolled components** (ADR-0008's implicit plan): full control, but
  slow, and agent-generated a11y is exactly where subtle bugs hide. Rejected.
- **Import templUI as a Go module:** simpler updates, but components become a
  black box to agents and theme-level customization hits limits. Rejected.
- **daisyUI/Franken UI + hand-written templ:** not templ-native; every
  component still needs writing. Rejected.

## References

- Refines: [ADR-0008](0008-go-templ-datastar-frontend.md)
- Shapes: [design/architecture.md](../design/architecture.md),
  [design/agent-layer.md](../design/agent-layer.md),
  [plans/roadmap.md — Phase 3](../plans/roadmap.md)
