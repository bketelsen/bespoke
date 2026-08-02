# Bespoke Documentation

Docs are split by the question they answer:

| Directory | Question | Contents |
| --- | --- | --- |
| [adr/](adr/) | **Why** did we choose this? | Architecture Decision Records — immutable once accepted; superseded, never edited |
| [design/](design/) | **How** does it fit together? | Living documents describing the current architecture |
| [specs/](specs/) | **What exactly** is the contract? | Precise, testable interface definitions |
| [plans/](plans/) | **When/in what order** do we build? | Roadmaps and phase plans; updated as work lands |

## Index

### Decisions (ADRs)

- [0001 — Record architecture decisions](adr/0001-record-architecture-decisions.md)
- [0002 — Optimize for one-shot agent reliability](adr/0002-optimize-for-one-shot-agent-reliability.md)
- [0003 — Subdomain per app](adr/0003-subdomain-per-app-routing.md)
- [0004 — Tailscale identity via Caddy headers](adr/0004-tailscale-identity-via-caddy.md)
- [0005 — One process per app](adr/0005-process-per-app.md)
- [0006 — Library-first shared services](adr/0006-library-first-shared-services.md)
- [0007 — SQLite per app + Litestream](adr/0007-sqlite-per-app-litestream.md)
- [0008 — Go + templ + Datastar frontend](adr/0008-go-templ-datastar-frontend.md)
- [0009 — LLM inference via Copilot SDK gateway](adr/0009-copilot-sdk-llm-gateway.md)
- [0010 — Build pkg/ui on vendored templUI components](adr/0010-templui-component-base.md)
- [0011 — Split-host deployment: edge, app host, dev machine](adr/0011-split-host-deployment.md)
- [0012 — Internal shared services: helpers first, services on demand](adr/0012-internal-services-two-tier.md)
- [0013 — Agent-portable instruction surface](adr/0013-agent-portable-instruction-surface.md)
- [0014 — Audio service: transcription, stub-first](adr/0014-audio-service-transcription.md)
- [0015 — AppShell platform chrome: app switcher and in-app chat](adr/0015-appshell-platform-chrome.md)
- [0016 — Mobile-first UI standard](adr/0016-mobile-first-ui-standard.md)
- [0017 — App-provided dashboard cards](adr/0017-app-provided-dashboard-cards.md)
- [0018 — Cross-app intents](adr/0018-cross-app-intents.md)
- [0019 — User brief: per-person context for every LLM feature](adr/0019-user-brief.md)
- [0020 — Dashboard chat over aggregated app contexts](adr/0020-dashboard-chat-aggregated-context.md)
- [0021 — App tools: agentic chat and the MCP surface](adr/0021-tools-agentic-chat-mcp.md)

### Design

- [Vision](design/vision.md) — the premise and prior art
- [Architecture](design/architecture.md) — topology, auth flow, repo layout, data
- [LLM gateway](design/llm-gateway.md) — Copilot SDK integration in platformd
- [Internal services](design/internal-services.md) — catalog of shared capabilities + how to add one
- [Agent layer](design/agent-layer.md) — conventions, skills, CLAUDE.md as law

### Specs

- [App manifest (`app.toml`)](specs/app-manifest.md)
- [`bespoke` CLI](specs/bespoke-cli.md)

### Plans

- [Roadmap](plans/roadmap.md) — build order and open questions

## Conventions

- **New docs start from their category's `TEMPLATE.md`** (in each directory).
- New decision → new ADR with the next number; if it reverses an old one, mark
  the old one `Superseded by NNNN` rather than editing it.
- Design docs are updated in place to always reflect reality.
- Specs change only alongside the code that implements them.
- Cross-links between categories are mandatory in both directions — see the
  documentation rules in [AGENTS.md](../AGENTS.md) (CLAUDE.md/GEMINI.md are
  symlinks to it, ADR-0013).
- Adding a doc means adding it to the index above.
