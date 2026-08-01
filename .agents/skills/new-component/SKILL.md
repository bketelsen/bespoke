---
name: new-component
description: Extend the Bespoke design system — vendor a templUI component or add a Bespoke wrapper/theme change in pkg/ui. Use when an app needs a UI component that pkg/ui doesn't have yet, or when the look needs adjusting.
---

# Extend the design system

The look lives in exactly two places: `design/input.css` (theme tokens) and
`pkg/ui` (components). Apps never carry styling. Decide which of the three
cases you're in:

## 1. templUI has the component → vendor it

```sh
ls tools/templui || scripts/setup-tools.sh   # one-time toolchain install
./tools/templui list                          # see what's available
./tools/templui add <name>                    # vendors into pkg/ui/components/<name>
```

Vendored files (`pkg/ui/components/`, `pkg/ui/utils/`) are NEVER hand-edited
— updates overwrite them. If the component ships JS, it lands in
`pkg/ui/assets/js/` and its `@<name>.Script()` must be rendered by any page
using it.

## 2. Bespoke-specific composition → wrapper in pkg/ui

Add a `.templ` file at the `pkg/ui` root (like `shell.templ`) composing
vendored components into the house idiom. This is where repeated app
patterns graduate to (e.g. a standard list-with-empty-state). Export it as
`ui.<Name>`.

## 3. The look itself → theme tokens

Edit `design/input.css` only: oklch color variables (light + `.dark` dark
blocks), radius, typography. Never add per-component CSS there; never touch
`pkg/ui/assets/styles.css` (generated).

## Always finish with

```sh
scripts/build-ui.sh   # templ generate + Tailwind compile (or: just ui)
just check
```

Commit generated output (`*_templ.go`, `pkg/ui/assets/styles.css`) with your
change, and add a row to the design-system notes in
`docs/design/architecture.md` only if you added a wrapper with non-obvious
usage.
