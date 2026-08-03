# 0026 — App-bundled chat skills via a loader tool

- **Status:** Accepted
- **Date:** 2026-08-02

## Context

An app contributes two things to chat surfaces: its context
([ADR-0015](0015-appshell-platform-chrome.md),
[ADR-0020](0020-dashboard-chat-aggregated-context.md)) and its tools
([ADR-0021](0021-tools-agentic-chat-mcp.md)). Procedural knowledge —
"how to write a good page in this wiki", "how to run a review" — has no
home: it is too long to stuff into every turn's context and too
app-specific to hardcode in prompts.

The requirement that shapes everything: skills must follow the *tools*.
The dashboard chat and external MCP clients hold an app's tools without
ever touching the app's own chat panel, so any skill mechanism bolted to
one surface misses the others. The Copilot runtime's native skill support
(`SkillDirectories`) wants directories on the session host's disk —
nowhere near the apps — and is deliberately disabled by the gateway
([ADR-0009](0009-copilot-sdk-llm-gateway.md)).

## Decision

Skills ride the existing tool plumbing as **one loader tool per app**:

- An app bundles `skills/<name>/SKILL.md` files — the same frontmatter
  format (`name:`, `description:`) as the repo's agent skills in
  `.agents/skills/`, one convention everywhere — and registers them with
  `web.Skills(mux, fs)` inside `web.Run`.
- `web.Skills` registers a `load_skill` tool whose **description carries the
  index** (every skill's name and one-line description), with the skill
  names as a schema enum. Calling it returns the skill's full markdown
  body. It also mounts `GET /_skills` (the parsed set) for introspection.
- Because it is an ordinary tool, every surface gets it with zero new
  plumbing: the app's own chat, the dashboard chat (as `<slug>_load_skill`),
  and the platform MCP endpoint — external LLM clients can load app
  skills too.
- Skills are **on-demand only**: the model sees the index in the tool
  description and pays one tool round-trip (~2s) when it loads one. No
  always-injected skills; short rules that must shape every answer belong
  in the app's chat context instead.
- Skills must stay honest to the app's actual tool surface (a skill must
  not instruct the model to call tools the app doesn't register).

## Consequences

- Chat quality knowledge becomes a versioned, reviewable artifact in the
  app directory rather than folklore in prompts.
- The tool description grows with the skill count; the index format (name
  + one line) keeps it bounded. An app with many skills should split them
  or trim descriptions — the enum keeps invalid loads impossible either
  way.
- MCP clients see `<slug>_load_skill` like any tool; no gateway or platformd
  changes were needed, and none of the ADR-0009 lockdown moved.
- First consumer: personal-wiki (`page-authoring`, `wiki-gardening`).

## Alternatives considered

- **Gateway-side native skills:** materialize app skills into session
  temp dirs and enable the runtime's `SkillDirectories`. Native UX, but
  new app→gateway aggregation, per-session file plumbing, re-opens a
  locked-down runtime feature, and MCP surfaces get nothing.
- **Stuff skill bodies into chat context:** simplest, but pays every
  skill's full token cost on every turn; collapses beyond a couple of
  skills.
- **One tool per skill:** `<slug>_skill_<name>` clutters the tool list
  and adds nothing over an enum on one loader.

## References

- Shapes: [design/architecture.md](../design/architecture.md) (pkg/web
  app contract)
- Builds on: [ADR-0015](0015-appshell-platform-chrome.md),
  [ADR-0020](0020-dashboard-chat-aggregated-context.md),
  [ADR-0021](0021-tools-agentic-chat-mcp.md)
