# 0034 — Show the running release and check for a newer one

- **Status:** Accepted
- **Date:** 2026-08-04

## Context

An instance pins the platform in its `go.mod`
([ADR-0027](0027-versioned-platform-private-instances.md)) and upgrades only
when the owner runs `bespoke upgrade`. Nothing in a running instance said which
release it was serving, so answering "what am I running, and is it current?"
meant reading `go.mod` on the dev machine — the one place an owner is not when
they are using their apps from a phone. Releases publish frequently and carry
platform fixes an owner otherwise has no signal to pick up.

Deployment builds platform processes from the pinned module with plain
`go build`, without linker flags, so a version cannot be stamped in the way the
CLI stamps its own. GitHub publishes the newest release over an unauthenticated
endpoint rate-limited per source IP, and an instance may be running with no
outbound internet at all.

## Decision

- `pkg/version` resolves the running release from the build's module graph:
  the platform's dependency version in an instance, the main module's version
  in a released platform binary, `dev` for any working-tree or replaced build.
  No build flags, no generated constants.
- `version.Checker` caches the newest published release from GitHub's
  latest-release endpoint for six hours, refreshes in the background, and
  retries fifteen minutes after a failure. `Info()` never blocks a request and
  never fails: an instance with no network renders its own version alone.
- The dashboard renders a footer with the running release, plus a link to the
  newer release's page when the check found one. It reports; it does not
  upgrade, and it does not interrupt.
- A `dev` build performs no check — there is no release to compare against.
  `BESPOKE_UPDATE_CHECK=off` drops the outbound call for any instance;
  `BESPOKE_RELEASES_URL` overrides the endpoint.

## Consequences

- Every owner can see which release their instance runs from any device, and
  learns about a newer one where they already are.
- platformd makes an outbound call to GitHub — the first platform process
  behavior that talks to the public internet without the owner asking. The
  request carries no identity, no instance data, and no query parameters, and
  the opt-out is one environment variable.
- A version resolved from the module graph is `dev` whenever a `go.work` or
  `replace` is in play, so a maintainer's checkout shows `dev` rather than a
  misleading tag.
- The footer's accuracy depends on GitHub's release feed; a rate-limited or
  failing check silently shows the running version alone rather than a stale
  or wrong claim.

## Alternatives considered

- **Stamp the version with `-ldflags` at deploy time:** would need every build
  path (deploy, builder plane, local `go run`) to agree on flags, and the
  module graph already carries the truthful answer.
- **Check for updates in the CLI only:** the dev machine already has `go.mod`;
  the surface that lacks the answer is the running instance.
- **Auto-upgrade when a newer release appears:** upgrades recompile apps and
  restart units, which is the owner's decision, not the dashboard's.
- **Show the footer on every page via AppShell:** repeats platform trivia in
  apps that are about the owner's data; the dashboard is where instance-level
  facts belong.

## References

- Shapes: [architecture](../design/architecture.md),
  [internal services](../design/internal-services.md),
  [CLI spec](../specs/bespoke-cli.md)
- Builds on: [ADR-0027](0027-versioned-platform-private-instances.md),
  [ADR-0006](0006-library-first-shared-services.md),
  [ADR-0015](0015-appshell-platform-chrome.md)
