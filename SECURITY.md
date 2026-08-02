# Security Policy

Bespoke's security model is unusual and worth stating plainly: there is no
login screen and never will be. All authentication happens at the tailnet
edge (Caddy + Tailscale), and every app trusts the identity headers the edge
injects. Anything that lets a request reach an app while bypassing or
forging that edge identity is a vulnerability of the highest order here.

Also in scope:

- SQL injection or cross-user data leaks (every query must be scoped to the
  authenticated login)
- The LLM gateway or MCP endpoint acting on behalf of the wrong user
- XSS through user- or LLM-authored markdown rendering

## Reporting

Please report vulnerabilities privately via
[GitHub Security Advisories](https://github.com/bketelsen/bespoke/security/advisories/new)
— not in public issues.

This is a personal project, not a company: no bounties, no SLA, but reports
get read and honest replies. Only the `main` branch is supported; there are
no versioned releases to backport to.
