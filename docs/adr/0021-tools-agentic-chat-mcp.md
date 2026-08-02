# 0021 — App tools: agentic chat and the MCP surface

- **Status:** Accepted
- **Date:** 2026-08-01

## Context

Chat could read (contexts, ADR-0020) but not act — "add a todo" answered
with instructions instead of a todo. The action registry was already
designed twice: intents (ADR-0018, human face) and the parked MCP idea
(LLM face). Multi-user made it urgent: the second user's natural interface
is telling the chat what to do.

## Decision

Apps register **tools** — user-scoped, LLM-callable actions:

- **Registry:** `web.Tool(mux, def)` — name, description, JSON schema,
  handler(ctx, user, args). Served as `GET /_tools` (definitions) and
  `POST /_tools/<name>` (execute; auth'd like everything). Handlers scope
  by user, same law as HTTP handlers.
- **Agentic chat:** `/llm/complete` accepts tools; the gateway builds a
  Copilot session whose tool handlers POST back to the owning app with the
  requesting user's identity (the ADR-0017 forwarding pattern). Lockdown
  *tightens*: `AvailableTools` = exactly our tool names; builtins stay
  denied. Tools require a tagged user — no user, no tools. App chats get
  their own tools automatically via `EnableChat`; the dashboard chat
  aggregates every app's, namespaced `<slug>_<name>`, so cross-app actions
  work from the apex ("journal this, then count my tasks").
- **MCP endpoint:** `/mcp` at the apex (official Go SDK, Streamable HTTP,
  behind auth like any page). A server is built per request scoped to the
  caller's tailnet identity; tools execute as them. External clients:
  `claude mcp add --transport http bespoke https://<apex>/mcp` from any
  tailnet device.
- **Chat voice input:** the panel's mic records WAV (shared `wav.js`),
  `POST /_chat/transcribe` (local whisper), and the transcript lands in the
  textarea **editable before sending** — speech is input, commitment stays
  explicit.
- **Full CRUD shipped, spec-bounded:** todo exposes list/create/update/
  set_done (with cascades)/delete; journal exposes add/list/delete only —
  append-only is a spec non-goal, so no update tool exists. Destructive
  tools carry only-on-explicit-request instructions in both the tool
  description and the chat style prompt.

## Consequences

- The wife test passes: "add milk to my list" in chat does it — per-app or
  from the dashboard, typed or spoken.
- Every future `web.Tool` is simultaneously chat-usable and MCP-exposed;
  intents (ADR-0018) remain the human-confirm face of the same actions.
- Agentic chat calls take longer (tool loop, 300s gateway ceiling) and are
  usage-logged per tool call.
- Trust: tools run with forwarded identity from trusted platform
  components. The plane ACL (roadmap) remains the boundary that makes
  header-forging a non-issue.

## Alternatives considered

- **Gateway-side tool execution against app DBs:** violates isolation
  (ADR-0007/0017). HTTP-back-to-the-app keeps apps sovereign.
- **MCP only, chat stays read-only:** leaves the primary interface (chat)
  crippled for the primary user.
- **Auto-deriving tools from intents:** intents are confirm-page-shaped
  (one text field); tools need schemas. They share the philosophy, not the
  plumbing.

## References

- Builds on: [ADR-0009](0009-copilot-sdk-llm-gateway.md),
  [ADR-0018](0018-cross-app-intents.md), [ADR-0020](0020-dashboard-chat-aggregated-context.md)
- Shapes: [specs/app-manifest.md](../specs/app-manifest.md),
  [design/llm-gateway.md](../design/llm-gateway.md),
  [design/internal-services.md](../design/internal-services.md)
