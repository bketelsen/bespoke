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
  defaults to `http://127.0.0.1:4001` in dev).
- **Session per request, inference-locked:** every call creates a fresh SDK
  session with tools, skills, config discovery, file hooks, git context, the
  session store, and embedding retrieval all disabled, permission requests
  denied, and a scratch working directory so repo instructions never leak
  into app inference. The session is deleted after the response.
- **`pkg/llm` interface (provider-neutral):** `llm.New(app)` →
  `Complete(ctx, prompt, ...opts)`, `CompleteJSON(ctx, prompt, &out, ...opts)`
  (JSON-only instruction + fence stripping), `Healthy(ctx)`, `WithSystem(s)`.
  Streaming is deferred until the first app needs it (the SDK supports
  message deltas; add a `/llm/stream` SSE endpoint then).
- **Model selection** is gateway config: `BESPOKE_LLM_MODEL` env on platformd
  (empty = CLI default). Apps never specify a model.
- **Usage logging:** one line per call — app, prompt/output bytes, duration,
  error — the observability point for all LLM traffic.

## Operational notes

- Bootstrap dependency: `copilot` CLI on PATH for the platformd unit, logged
  in as the platform user (`copilot`, then sign in).
- platformd checks `GetAuthStatus` at startup and every 5 minutes; failures
  surface as a dashboard warning banner, and `/llm/healthz` returns 503.
  The gateway starting/down never blocks the dashboard.
- **Measured (2026-08-01, local):** simple completions ≈ **1.3–1.8s**
  end-to-end through the CLI runtime — fine for summaries/classification,
  not for keystroke-level interactivity.
- Copilot terms: still worth a skim before high-frequency automated use
  (see [roadmap open questions](../plans/roadmap.md#open-questions)).
