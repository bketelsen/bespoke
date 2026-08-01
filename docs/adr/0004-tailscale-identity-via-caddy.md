# 0004 — Tailscale identity via Caddy headers

- **Status:** Accepted — refined by [ADR-0011](0011-split-host-deployment.md)
  (Caddy and apps run on separate hosts: the loopback-only rule becomes
  bind-to-tailscale-interface + ACL restricting app ports to the edge host)
- **Date:** 2026-08-01

## Context

All access is over the tailnet; Tailscale already authenticates every peer.
Apps need to know *who* is calling but should never implement auth themselves
(ADR-0002).

## Decision

Caddy authenticates connections against the local tailscaled using the
[tailscale/caddy-tailscale](https://github.com/tailscale/caddy-tailscale)
plugin and sets `Tailscale-User-Login` / `Tailscale-User-Name` headers on
proxied requests. Caddy **strips any inbound copies of these headers first** —
this is the non-negotiable security rule of the platform.

Apps consume identity exclusively through `pkg/auth` middleware: reject if the
header is absent, otherwise expose `auth.User(ctx)`.

## Consequences

- Zero auth code in apps; adding a login system to an app is impossible by
  construction, which is the point.
- Apps are unreachable off-tailnet by design.
- App processes must only listen on loopback — anything that can reach an app
  port directly can forge the header. Enforced by `pkg/web`'s listener setup.

## Alternatives considered

- **Cloudflare Access / Tunnel** (as in the prior-art article): buys access
  without the Tailscale client at the cost of a public dependency. Tailscale is
  already deployed here.
- **forward_auth to `tailscale.nginx-auth`:** works, but the Caddy plugin is
  first-party and simpler.
