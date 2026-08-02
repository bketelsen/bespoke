# 0016 — Mobile-first UI standard

- **Status:** Accepted
- **Date:** 2026-08-01

## Context

These apps are used from a phone at least as often as from a desk — capture
happens in pockets. A 2026-08-01 audit found desktop-only assumptions had
already crept in (hover-only delete buttons, iOS input auto-zoom, a chat
panel taller than the visual viewport, tab labels overflowing 375px).
Without a standing rule, every agent-built view will regress the same ways.

## Decision

Every view must be fully usable on an iPhone-class viewport (**375px wide,
coarse pointer, on-screen keyboard**). The enforceable rules:

1. **No hover-only affordances.** Anything revealed on hover must also be
   reachable on touch: `pointer-coarse:opacity-100` (plus
   `focus-within:` for keyboards) on hover-revealed controls.
2. **No iOS zoom traps.** Form controls are ≥16px on coarse pointers —
   enforced globally by a deliberately **unlayered** rule in
   `design/input.css` that outranks utility classes. Never remove, layer,
   or override it; never "fix" zooming with `maximum-scale`.
3. **Overlays fit the visual viewport:** height caps in `dvh` (not `vh`),
   width caps like `max-w-[calc(100vw-2rem)]`, bottom offsets respecting
   `env(safe-area-inset-bottom)`. The chat panel is the reference.
4. **Nothing forces horizontal page scroll:** long text gets
   `break-words`/`truncate` (`ui.Markdown` does this; header names
   truncate); genuinely wide content (tables) scrolls inside its own
   `overflow-x-auto` container, never the page.
5. **Touch targets grow on touch:** icon-sized controls add
   `pointer-coarse:size-8` (or larger).
6. **Layouts collapse:** grids are single-column by default,
   `sm:grid-cols-*` up; labels shrink responsively
   (`<span class="hidden sm:inline">`) rather than overflowing.
7. `viewport-fit=cover` stays in the AppShell meta tag (notched devices).

The AppShell and design system own rules 2, 3, 4 (markdown), and 7 — apps
inherit them. Apps are responsible for 1, 5, 6 in their own views, and the
new-app/new-component skills gate on a 375px check.

## Consequences

- The one-shot verification step now includes a mobile pass; a view that
  needs a mouse is a failed build, not a nitpick.
- Coarse-pointer styling relies on Tailwind v4.1+ `pointer-coarse:`
  variants — available in the pinned toolchain.
- A real-device visual pass still catches what static rules can't; the
  rules make regressions rare, not impossible.

## Alternatives considered

- **Adaptive/separate mobile views:** double the surface for an
  agent-maintained platform; responsive single views are the one-shot-safe
  choice.
- **`maximum-scale=1` to stop iOS zoom:** breaks pinch-zoom accessibility.
  Rejected; the 16px rule fixes the cause.

## References

- Builds on: [ADR-0008](0008-go-templ-datastar-frontend.md),
  [ADR-0010](0010-templui-component-base.md),
  [ADR-0015](0015-appshell-platform-chrome.md)
- Shapes: [design/architecture.md](../design/architecture.md),
  `design/input.css`, [.agents/skills/new-app](../../.agents/skills/new-app/SKILL.md),
  [.agents/skills/new-component](../../.agents/skills/new-component/SKILL.md)
