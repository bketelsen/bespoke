## What & why

<!-- One or two sentences. Link the issue if there is one. -->

## Checklist

- [ ] `just check` passes locally (CI runs exactly the same recipe)
- [ ] If any `.templ` file or `design/input.css` changed: ran
      `scripts/build-ui.sh` and committed the generated `*_templ.go` and
      `pkg/ui/assets/styles.css`
- [ ] If this changes a contract or adds a capability: docs updated per
      [AGENTS.md](../AGENTS.md) (ADR first for new decisions; cross-links in
      both directions)
- [ ] No hand-edits to `pkg/ui/components/` (vendored) or `dist/gen/`
      (generated)
