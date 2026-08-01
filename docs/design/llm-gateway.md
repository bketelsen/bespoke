# LLM Gateway

Living document. Rationale:
[ADR-0009](../adr/0009-copilot-sdk-llm-gateway.md).

Inference rides the author's existing GitHub Copilot subscription via the
[Copilot SDK for Go](https://github.com/github/copilot-sdk)
(`github.com/github/copilot-sdk/go`, GA June 2026). The SDK drives the
**Copilot CLI** — a Node-based local agent runtime that must be installed on
the host and authenticated with GitHub.

## Design

```
app ──pkg/llm──► platformd /llm (loopback) ──Copilot SDK──► copilot CLI ──► Copilot backend
```

- **Single client.** platformd owns the one SDK client / CLI session pool.
  Apps never talk to the SDK directly; GitHub auth lives in one place.
- **`pkg/llm` interface (provider-neutral):**
  - `Complete(ctx, prompt, opts) (string, error)`
  - `CompleteJSON(ctx, prompt, schema, out) error` — validated structured output
  - `Stream(ctx, prompt, opts) (iter.Seq[string], error)`
- **Inference mode by default:** sessions run with tool invocation and file
  access disabled — plain completions only. An opt-in agentic session type can
  be added later if an app genuinely needs one.
- **Model selection** is gateway config (Copilot fronts multiple models,
  including Claude models). Apps never specify a model.
- **Usage logging:** the gateway logs app, prompt size, latency per call —
  the observability point for all LLM traffic.

## Operational notes

- Bootstrap dependency: `copilot` CLI on PATH for the platformd unit, logged in
  as the platform user.
- platformd health-checks the CLI on a timer and surfaces a dashboard warning
  when auth expires — otherwise every app's LLM features fail at once, silently.
- **Open:** measure end-to-end latency through the CLI runtime before building
  interactive-latency features; skim Copilot terms before high-frequency
  automated use (see [roadmap open questions](../plans/roadmap.md#open-questions)).
