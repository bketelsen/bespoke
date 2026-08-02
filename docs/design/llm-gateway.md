# LLM Gateway

Living document. Rationale:
[ADR-0009](../adr/0009-copilot-sdk-llm-gateway.md).

Inference rides the author's existing GitHub Copilot subscription via the
[Copilot SDK for Go](https://github.com/github/copilot-sdk)
(`github.com/github/copilot-sdk/go`, GA June 2026). The SDK drives the
**Copilot CLI** — a Node-based local agent runtime that must be installed on
the host and authenticated with GitHub.

## Design (implemented — platform/llm.go, pkg/llm)

```
app ──pkg/llm──► platformd :4001 /llm/* ──Copilot SDK──► copilot CLI ──► Copilot backend
```

- **Single client, internal listener.** platformd owns the one SDK client and
  serves `/llm/complete` + `/llm/healthz` on a **separate internal port
  (4001)** that Caddy never routes — reachable on-host and inside the
  ACL-blocked port range only ([ADR-0011](../adr/0011-split-host-deployment.md)).
  Apps find it via `BESPOKE_LLM_URL` (written to the env file at deploy;
  defaults to `http://127.0.0.1:4001` in dev). The same plane also carries
  the audio gateway (`/audio/*`,
  [ADR-0014](../adr/0014-audio-service-transcription.md)) and the `/notify`
  change fan-out ([ADR-0022](../adr/0022-live-updates.md)).
- **Session per request, locked by default:** every call creates a fresh SDK
  session with skills, config discovery, file hooks, git context, the
  session store, and embedding retrieval disabled, permission requests
  denied, and a scratch working directory so repo instructions never leak
  into app inference. The session is deleted after the response. Builtin
  tools stay off always; **app tools are the one opt-in**
  ([ADR-0021](../adr/0021-tools-agentic-chat-mcp.md)): a request carrying
  tool definitions gets exactly those tools (`AvailableTools` locks the
  list), the gateway executes calls by POSTing the app's `/_tools/<name>`
  with the requesting user's identity, and tool requests without
  `llm.WithUser` are rejected. Timeouts: 120s plain, 300s agentic.
- **`pkg/llm` interface (provider-neutral):** `llm.New(app)` →
  `Complete(ctx, prompt, ...opts)`, `CompleteJSON(ctx, prompt, &out, ...opts)`
  (JSON-only instruction + fence stripping), `Healthy(ctx)`, with options
  `WithSystem(s)`, `WithUser(login)`, and `WithTools([]llm.Tool{Name,
  Description, Schema, URL})`.
  Higher-level capability helpers (`Classify`, future `Summarize`/`Extract`)
  are tier-1 methods on the same client — see
  [internal-services.md](internal-services.md).
  Streaming is deferred until the first app needs it (the SDK supports
  message deltas; add a `/llm/stream` SSE endpoint then).
- **Model selection** is gateway config: `BESPOKE_LLM_MODEL` env on platformd
  (empty = CLI default). Apps never specify a model.
- **User brief injection ([ADR-0019](../adr/0019-user-brief.md)):** requests
  tagged with `llm.WithUser(login)` get that user's self-provided brief
  (edited at the dashboard's `/settings`, stored in `data/platformd.db`)
  prepended to the system prompt. Untagged calls are untouched.
- **Second backend (adopted for speech):** a
  [Lemonade](https://lemonade-server.ai/) server runs locally on selfie
  (OpenAI-compatible). The audio gateway uses it in production — Whisper
  transcription in, kokoro TTS out — behind the same plane
  ([ADR-0014](../adr/0014-audio-service-transcription.md)). Text completion
  on Lemonade remains a candidate: when adopted it slots in behind the same
  `pkg/llm` seam, with a future explicit privacy option (e.g. `WithLocal()`,
  not yet implemented) for prompts that must not leave the house. See the
  [internal services catalog](internal-services.md#backends).
- **MCP is the same wire, outward:** the tool surface apps register for
  chat is also exposed to external LLM clients at the apex `/mcp`
  (Streamable HTTP, tools namespaced `<slug>_<name>`, per-request identity
  from the edge headers) — [ADR-0021](../adr/0021-tools-agentic-chat-mcp.md).
- **Activity signal ([ADR-0023](../adr/0023-builder-plane-unprivileged-agent-spooled-deploys.md)):**
  `GET /llm/activity` on the same plane reports
  `{"inflight": n, "idle_seconds": s}` for in-flight completions. The
  deploy watcher polls it to quiesce before restarting units, so a deploy
  never kills a completion mid-reply. Agentic *coding* sessions are
  explicitly NOT a gateway feature — tool execution inherits the gateway's
  uid; they live in the builder runner instead
  ([builder-plane.md](builder-plane.md)).
- **Usage logging:** one line per call — app, prompt/output bytes, tool
  count, duration, error — plus one `llm-tool` line per tool invocation.
  The observability point for all LLM traffic.

## Operational notes

- Bootstrap dependency: `copilot` CLI on PATH for the platformd unit, logged
  in as the platform user (`copilot`, then sign in).
- platformd checks `GetAuthStatus` at startup and every 5 minutes; failures
  surface as a dashboard warning banner, and `/llm/healthz` returns 503.
  The gateway starting/down never blocks the dashboard.
- **Measured (2026-08-01, local):** simple completions ≈ **1.3–1.8s**
  end-to-end through the CLI runtime — fine for summaries/classification,
  not for keystroke-level interactivity. Agentic chat turns run longer
  (each tool round-trip adds a model pass; the gateway budgets 300s).
- Copilot terms: still worth a skim before high-frequency automated use
  (see [roadmap open questions](../plans/roadmap.md#open-questions)).
