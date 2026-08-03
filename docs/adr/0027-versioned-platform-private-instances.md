# 0027 — Version the platform and keep owner instances private

- **Status:** Accepted
- **Date:** 2026-08-03

## Context

Bespoke's first implementation put the framework, owner-specific applications,
theme, and deployment identity in one public Go module. That made the initial
build loop simple, but a growing public audience made every new personal app
public by default. Forking also required rewriting the module and deleting the
original owner's applications rather than starting from a clean owner boundary.

The platform is still pre-1.0, so this boundary can change without supporting
both repository shapes indefinitely. Private apps must retain the same manifest
and HTTP contracts, and Tailwind must scan their templates without requiring the
UI toolchain on production hosts.

## Decision

- `github.com/bketelsen/bespoke` is the public, semantically versioned platform
  module. It contains the framework, CLI, platform processes, documentation,
  and synthetic Notes/Todo showcase.
- An owner operates one private instance module containing `apps/`, theme
  tokens, compiled instance CSS, deployment configuration, and owner-specific
  agent context. `bespoke init` creates it and pins the matching platform release.
- The instance invokes the pinned CLI through Go's `tool` directive. Released
  CLI archives bootstrap new instances; deployment builds platform processes
  from the pinned module source.
- The versioned platform owns base CSS and embedded fallback assets. The
  instance owns `design/theme.css` and committed `assets/styles.css`, compiled
  after scanning both platform and private templates and deployed with binaries.
- Releases are `v`-prefixed semantic tags selected by SVU and published by
  GoReleaser, which also updates the `bespoke` cask in `bketelsen/homebrew-tap`.
- Native Windows is unsupported; Windows development uses WSL2.

## Consequences

- Personal source is private by default and upgrades become explicit.
- Platform and instance CI are separate and need generated-instance compatibility tests.
- Deploy must ship the instance stylesheet; the embedded stylesheet remains a fallback.
- Release automation needs a token scoped to the separate Homebrew tap.
- Maintainers may use an uncommitted `go.work`; ordinary owners need one checkout.

## Alternatives considered

- **Private apps nested inside a public fork:** no clean version boundary or standalone CI.
- **One private fork containing everything:** upstream upgrades remain merges and source is easy to publish accidentally.
- **One repository per app:** needless version and operational overhead.

## References

- Shapes: [architecture](../design/architecture.md), [agent layer](../design/agent-layer.md),
  [CLI spec](../specs/bespoke-cli.md), [manifest spec](../specs/app-manifest.md),
  [roadmap](../plans/roadmap.md)
- Builds on: [ADR-0006](0006-library-first-shared-services.md),
  [ADR-0010](0010-templui-component-base.md),
  [ADR-0013](0013-agent-portable-instruction-surface.md),
  [ADR-0023](0023-builder-plane-unprivileged-agent-spooled-deploys.md)
