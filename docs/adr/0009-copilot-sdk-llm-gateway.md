# 0009 — LLM inference via Copilot SDK gateway

- **Status:** Accepted
- **Date:** 2026-08-01

## Context

Apps will use LLM features routinely (summaries, classification, generation).
The author already has GitHub Copilot access, making it the zero-setup
choice over signing up for a separate per-token API. The
[Copilot SDK for Go](https://github.com/github/copilot-sdk)
(`github.com/github/copilot-sdk/go`, GA June 2026) provides programmatic
access, but it works by driving the **Copilot CLI** — a Node-based local agent
runtime that must be installed and GitHub-authenticated on the host.

## Decision

platformd hosts a single **LLM gateway** owning one Copilot SDK client/CLI
session pool, exposed on loopback. Apps import `pkg/llm`, a thin
provider-neutral client (`Complete`, `CompleteJSON`, streaming).

- **Inference mode by default:** the SDK's native shape is agentic (tools, file
  edits); the gateway disables all of that and serves plain completions.
- **Model choice is gateway config** (Copilot fronts multiple models, including
  Claude); apps never pick models.
- **Health:** platformd health-checks the CLI and surfaces a dashboard warning
  when GitHub auth expires — otherwise every app's LLM features fail at once,
  silently.

## Consequences

- One process pays the Node runtime cost, not N; GitHub auth lives in one place.
- `pkg/llm`'s interface is provider-neutral: swapping the gateway to the direct
  Claude API later touches zero app code.
- Host bootstrap dependency: `copilot` CLI on PATH for the platformd unit.
- Latency goes through the CLI runtime, not a raw HTTP API — fine for
  summaries; measure before building interactive-latency features on it.
- Copilot terms should be skimmed before building anything high-frequency
  (e.g. a 5-minute cron summarizer) through a subscription service.

## Alternatives considered

- **Direct Claude API via `pkg/llm`:** cleanest technically; per-token cost.
  Remains the designated fallback behind the same interface.
- **SDK client embedded in every app** (pure library-first, per ADR-0006):
  would spawn a Node CLI per app process. The "heavy shared runtime" clause in
  ADR-0006 exists for exactly this case.
