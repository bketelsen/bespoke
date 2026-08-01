# 0011 — Split-host deployment: edge, app host, dev machine

- **Status:** Accepted
- **Date:** 2026-08-01

## Context

The original sketch assumed Caddy and the apps share one host, so apps could
listen on loopback and header forgery was impossible by construction
([ADR-0004](0004-tailscale-identity-via-caddy.md)). In reality:

- Caddy already runs on its own server (the **edge host**), serving other things.
- Apps deploy to a separate standalone server (the **app host**).
- Development happens on a third machine (the **dev machine**).

All three are on the tailnet. DNS is on Cloudflare.

## Decision

Three roles, connected only over the tailnet:

- **Edge host** — existing Caddy, rebuilt with the `caddy-tailscale` and
  `caddy-dns/cloudflare` modules. Terminates TLS (wildcard via Cloudflare DNS
  challenge), authenticates via tailscaled, strips + sets identity headers,
  and reverse-proxies each subdomain to `<app-host-tailscale-ip>:<port>`.
- **App host** — runs platformd and all app processes as systemd user units.
  Processes bind to the app host's **tailscale interface** (never a public
  interface, never 0.0.0.0).
- **Dev machine** — builds and deploys: cross-compile (`GOOS=linux`), rsync
  binaries + units over ssh, restart units; push the generated Caddy route
  file to the edge host and reload.

**Header-forgery protection moves from loopback to a Tailscale ACL:** only the
edge host may reach app-host ports 4000–4999. Any other tailnet device could
otherwise hit an app port directly and forge `Tailscale-User-*` headers. The
ACL is part of the platform's security invariants, same standing as header
stripping.

## Consequences

- The app host needs no Go toolchain — it only ever receives binaries.
- Deploys require ssh (tailnet) to both servers; the `bespoke` CLI's deploy
  spec gains remote steps.
- [ADR-0004](0004-tailscale-identity-via-caddy.md)'s "loopback only" rule is
  refined: bind to the tailscale interface + ACL instead.
- Local dev mode binds loopback and fakes the identity header.
- Caddy→app traffic crosses the tailnet (WireGuard): latency negligible on a
  LAN, and encrypted by default.

## Alternatives considered

- **Colocate Caddy on the app host:** contradicts the existing setup; the edge
  host serves other sites. Rejected.
- **mTLS between Caddy and apps:** stronger than an ACL but heavy machinery
  for a personal tailnet. Rejected while the ACL suffices.
- **`tailscale serve` per app:** no wildcard story, and identity would need
  re-plumbing per app. Rejected.

## References

- Refines: [ADR-0004](0004-tailscale-identity-via-caddy.md),
  [ADR-0005](0005-process-per-app.md)
- Shapes: [design/architecture.md](../design/architecture.md),
  [specs/bespoke-cli.md](../specs/bespoke-cli.md),
  [plans/roadmap.md — Phase 1](../plans/roadmap.md)
