---
name: make-it-your-own
description: Create and configure a private Bespoke instance from a tagged platform release.
---

# Make it your own

Bespoke's platform is public; your apps, theme, deployment identity, and agent
context belong in one private instance repository
([ADR-0027](docs/adr/0027-versioned-platform-private-instances.md)). This skill
creates that boundary rather than rewriting a public fork.

Agent: work in order, interview where marked, commit each phase, and run
`just check` between phases. Never guess a domain, host, repository path, or
inference provider.

## 1. Install and initialize

- macOS: `brew install --cask bketelsen/tap/bespoke`.
- Linux and Windows/WSL2: install the release archive or
  `go install github.com/bketelsen/bespoke/cmd/bespoke@latest`.
- **ASK:** private module path and destination directory.
- Run `bespoke init <dir> --module <path>`. Add `--with-builder` only when the
  owner wants the unattended builder plane and accepts its host prerequisites.
- Create a private Git remote before adding any personal material. Confirm its
  visibility through the hosting provider; do not infer privacy from its name.

The generated `go.mod` pins the platform and CLI together. Use `go tool bespoke`
inside the instance. A platform maintainer may create an uncommitted `go.work`
joining a local platform checkout and this instance.

## 2. Identity and deployment

- **ASK:** domain, edge host and Tailscale IPv4, app host and Tailscale IPv4,
  architecture, and SSH username.
- Copy the generated deployment example to ignored `deploy/deploy.env` and fill
  it in. Walk the public deployment runbook for Caddy, wildcard DNS, ACLs,
  systemd linger, and optional Lemonade.
- Keep secrets and databases ignored. Generated units, Caddy routes, and
  Litestream configuration remain under `dist/gen/`.

## 3. Inference provider

- **ASK:** what the owner already pays for or runs. Keep Copilot, or change the
  public platform gateway through an ADR and upstream contribution/fork.
- Apps continue to use only `pkg/llm`; private apps never call providers directly.

## 4. Apps and integrations

- Notes and Todo are synthetic examples demonstrating bidirectional intents.
  Keep them until the first owner app proves the loop; then remove either normally.
- Build the first app with the `design-app` and `new-app` skills copied into the
  instance agent surface.
- Review cross-app intents whenever an app is added. Record rejected integrations
  as non-goals in that app's README.

## 5. Theme

- **ASK:** reference colors, warm/cool direction, radius, and density.
- Edit only `design/theme.css`, run `just tools` once and `just ui`, then commit
  `assets/styles.css` and generated `*_templ.go` files.
- Platform base CSS owns structural and mobile invariants; never copy or override
  those rules into an app.

## 6. Verify and operate

- `just check` passes and private CI is green.
- `just dev` shows the owner's identity, theme, and apps.
- Deploy and verify the dashboard plus every app health check.
- Upgrade deliberately with `go tool bespoke upgrade <version>`, inspect the
  diff, regenerate UI when instructed, and rerun `just check` before deployment.
