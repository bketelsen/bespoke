# 0030 — Give AppShell explicit content widths

- **Status:** Accepted
- **Date:** 2026-08-03

## Context

`ui.AppShell` constrains every app to a 48rem reading column. That is a useful
default for forms and prose, but it prevents applications such as mail clients,
content-management systems, data explorers, and media libraries from presenting
workspace-style views. Applications still need the shared header, identity,
app switcher, chat, intents, and mobile-first guarantees; replacing AppShell is
therefore not an acceptable way to gain horizontal space.

Width alone does not define responsive master/detail or multi-pane behavior.
Those compositions need their own navigation, visibility, and focus decisions
and should not be implicit side effects of selecting a wider page.

## Decision

`ui.ShellProps` exposes a typed `Width` option with three values:

- `ShellWidthReading` uses the existing 48rem maximum and remains the zero-value
  default.
- `ShellWidthWide` allows a 80rem maximum for dashboards and multi-column views.
- `ShellWidthFull` uses all available viewport width, retaining AppShell's
  horizontal padding.

The header and main region always use the same width. Width selection changes
available space only; it does not provide or imply a pane layout. Responsive
workspace compositions remain Bespoke-specific `pkg/ui` wrappers and must meet
the mobile-first standard independently.

## Consequences

- Existing applications retain their current rendering without code changes.
- Larger applications keep all platform chrome and can opt into a suitable
  canvas per page.
- Apps must deliberately choose wide or full width; reading-oriented pages do
  not expand merely because another page in the app needs a workspace.
- A future master/detail or three-pane wrapper can be designed and tested
  independently on top of the wider shell.

## Alternatives considered

- **Make every app wide:** rejected because long lines degrade reading and most
  existing views were designed for the current measure.
- **Accept an arbitrary CSS class:** rejected because it weakens the shared
  layout contract and makes generated applications less predictable.
- **Ship a three-pane shell at the same time:** rejected because pane collapse,
  mobile navigation, URL state, and focus restoration require a separate design
  rather than a width toggle.

## References

- Shapes: [architecture](../design/architecture.md),
  [roadmap phase 3](../plans/roadmap.md#phase-3--design-system)
- Builds on: [ADR-0010](0010-templui-component-base.md),
  [ADR-0015](0015-appshell-platform-chrome.md),
  [ADR-0016](0016-mobile-first-ui-standard.md)
