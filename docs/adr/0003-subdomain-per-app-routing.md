# 0003 — Subdomain per app

- **Status:** Accepted
- **Date:** 2026-08-01

## Context

The original idea ([vision](../design/vision.md)) proposed path-based routing
(`home.example.com/app1`). Path-prefix hosting is a perennial tax: base hrefs,
asset paths, redirects, and cookie scoping all fight the prefix, in every
framework. Plenty of spare domains are available.

## Decision

Each app gets a subdomain: `<slug>.bespoke.example.com`. Caddy obtains a
wildcard certificate via the ACME DNS challenge. DNS resolves only on the
tailnet, so the wildcard leaks nothing publicly.

## Consequences

- Apps are written as if they own the origin — no prefix awareness anywhere,
  which also simplifies agent-generated code (ADR-0002).
- Requires a DNS provider API for the ACME DNS challenge.
- The dashboard lives at the apex (`bespoke.example.com`).

## Alternatives considered

- **Path prefixes under one host:** rejected for the tax above.
- **Public DNS + public certs per subdomain:** unnecessary; everything is
  tailnet-only.
