# 0031 — Install apps from third-party modules

- **Status:** Accepted
- **Date:** 2026-08-04

## Context

[ADR-0027](0027-versioned-platform-private-instances.md) made every owner's
apps private by default. That is the right default, but it left no way to give
one app to someone else: the app's source lives in a private instance module,
and the only other supported source was the platform module itself.

The manifest already carries an optional `package` field — a Go build-source
override that lets `apps/<slug>/app.toml` name a package in another module,
with no Go source in the instance at all. `bespoke dev` and `bespoke deploy`
both honor it, and the platform's own Builder app is installed this way. The
field was specified as a first-party mechanism, but nothing in the build path
depends on the module being the platform's.

Go modules already provide what an app registry would otherwise have to build:
immutable versions, integrity via the checksum database, private modules via
`GOPRIVATE`, and version negotiation with the platform through MVS.

One thing does depend on the module boundary. Tailwind compiles the instance
stylesheet by scanning template sources, and `bespoke ui` only pointed it at
the instance's `apps/` tree (the platform's own templates are scanned by
`design/input.css`). Classes it never sees are pruned, so an app from any
other module compiled and ran but rendered unstyled — a silent failure.

## Decision

- `package` in `app.toml` may name a package in **any** module, not only the
  platform's. An instance installs a shared app by pinning its module with
  `go get -tool` and writing an `app.toml` naming it; no source is vendored
  into the instance. The `tool` directive rather than a plain requirement:
  nothing in the instance imports a `main` package, so `go mod tidy` prunes
  anything weaker, and `tool` already pins the CLI the same way.
- `bespoke ui` derives its Tailwind scan roots from the registry: the
  instance's `apps/` tree plus the module directory of every app with a
  `package`. An unresolvable `package` fails the command rather than producing
  a stylesheet that is quietly missing rules.
- A publishable app is an ordinary Go module that commits its generated
  `*_templ.go` output. Instances never run `templ generate` over the module
  cache, which is read-only.
- `slug` remains instance-owned but is effectively chosen by the publisher: an
  app's source names its own database and process via `db.Open`/`web.Run`, and
  manifest validation requires `slug` to equal the directory name. A published
  app documents the slug it must be installed under.
- `bespoke add` installs an app and `bespoke search` lists an index of them.
  The index is one TOML file in a public repository — short name to module
  path, added by pull request, checked only for "does this resolve". It is a
  phone book, not a registry: no hosting, no artifacts, no review, and
  `BESPOKE_INDEX` points the commands at any other list. A module path always
  works without an index at all.
- A published app declares its own identity in an `app.toml.example` at its
  module root. The installing owner supplies the port and nothing else.
- Installing an app is running its author's code as the instance owner. Apps
  share a UID, a data directory, and the internal services plane; the platform
  makes no isolation claim beyond process-per-app
  ([ADR-0005](0005-process-per-app.md)), which exists for crash containment,
  not for containing malice. Distribution stays deliberately unofficial: the
  platform provides the mechanism and vouches for nothing installed with it.

## Consequences

- Apps become shareable without weakening the private-by-default boundary, and
  the mechanism is the same one first-party apps already use, so it stays
  exercised.
- Bespoke needs no registry, no artifact hosting, and no signing story; the Go
  proxy and checksum database supply integrity and immutability.
- `bespoke ui` now depends on the registry and on module resolution, so it
  fails on an instance whose modules have not been fetched.
- Installing an app can raise the instance's platform version through MVS. An
  owner can be upgraded by an app they installed.
- Publishers take on obligations instances cannot check: commit generated
  templ output, keep the slug stable, and treat the slug like the immutable
  identifier it is.
- The same-UID trust model is now load-bearing in a way it was not when every
  app was the owner's own. Per-app systemd hardening, and eventually per-app
  UIDs, become worth their cost; neither is decided here.
- `bespoke add` makes installing easy enough that people will do it without
  reading the source. The commands say what they are handing over, which is the
  most a phone book can honestly do.
- The index is a single file one person merges to. It is a bottleneck and a
  point of trust for *discovery*, which is why nothing depends on it: the
  module path is always the real address.

## Alternatives considered

- **Prebuilt binaries or OCI artifacts:** the instance compiles precisely so
  one Tailwind pass covers the platform, the owner's theme, and every app;
  shipping binaries ships unthemed apps.
- **A hosted registry with accounts and review:** infrastructure and a
  gatekeeper in exchange for a guarantee this deliberately does not offer.
- **No index at all, only module paths:** correct but undiscoverable; a phone
  book costs one text file.
- **A vendoring `bespoke install` that copies source into `apps/`:** upgrades
  become merges and the instance's own apps stop being distinguishable from
  someone else's.
- **Keeping `package` first-party and adding a parallel third-party path:** two
  build paths for one behavior, and the first-party one would rot.

## References

- Shapes: [manifest spec](../specs/app-manifest.md),
  [CLI spec](../specs/bespoke-cli.md),
  [architecture](../design/architecture.md)
- Builds on: [ADR-0005](0005-process-per-app.md),
  [ADR-0010](0010-templui-component-base.md),
  [ADR-0027](0027-versioned-platform-private-instances.md)
