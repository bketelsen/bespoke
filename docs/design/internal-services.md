# Internal Services

Living catalog of shared capabilities apps can use. Rationale:
[ADR-0012](../adr/0012-internal-services-two-tier.md) (two tiers, built on
demand). **Check here before building a capability into an app** — and when
you add one, add its row.

## The plane

```
app ──pkg/<name> client──► platformd internal listener (:4001, never Caddy-routed)
app ──pkg/* helper────────► (no network at all — composes existing primitives)
```

- Tier 1 **helpers**: plain `pkg/*` functions. Default choice.
- Tier 2 **services**: endpoints on the internal listener — only for
  cross-app state or heavy shared runtimes. Path prefix + healthz + usage
  logging + `pkg/<name>` client, always.

## Catalog

| Capability | Tier | Status | Use from apps | Notes |
| --- | --- | --- | --- | --- |
| Completion | 2 (gateway) | **live** | `llm.New(slug).Complete/CompleteJSON` | [llm-gateway.md](llm-gateway.md); ~1.5s/call |
| Classify | 1 | **live** | `llm.New(slug).Classify(ctx, text, categories)` | Validates the answer is one of the given categories |
| Summarize / Extract | 1 | as needed | future `llm.Client` methods | Same pattern as Classify |
| Streaming completions | 2 | deferred | future `web.NewSSE` + `/llm/stream` | When the first app needs live tokens |
| Files/blobs | 2 | candidate | future `pkg/files` | When two apps first share uploads (ADR-0006) |
| Notifications | 1 or 2 | candidate | future `pkg/notify` | Tier depends on delivery mechanism |
| Scheduled jobs | — | candidate | systemd timers per app first | Escalate only if cross-app coordination appears |
| Embeddings | 2 | candidate | future `llm.Embed` | Backend: Lemonade on selfie (see Backends) |
| Search | 2 | candidate | future `pkg/search` | Shared index; pairs with embeddings |
| Image generation | 2 | candidate | future `llm.Image` | Backend: Lemonade on selfie |
| Private/local completion | 2 | candidate | future `llm` option (e.g. `WithLocal()`) | Route privacy-sensitive prompts to Lemonade instead of Copilot |
| Speech / transcription | 2 | candidate | future `llm` helpers | Lemonade exposes both |

## Backends

The gateway pattern (ADR-0009/0012) means backends are invisible to apps —
`pkg/llm` is the seam. Two are available:

- **GitHub Copilot** (live): frontier models via the Copilot CLI.
  Cloud inference; ~1.5s/call ([llm-gateway.md](llm-gateway.md)).
- **[Lemonade](https://lemonade-server.ai/)** (available on selfie, not yet
  wired): local, OpenAI-compatible server — chat, vision, image, speech,
  transcription, embeddings. Not frontier-strength for text, but ideal for
  embeddings, image generation, lighter-weight inference, and
  **privacy-sensitive prompts that should never leave the house**. Zero
  marginal cost. Wiring it in = new gateway routes on the 4001 plane backed
  by Lemonade's endpoint + `pkg/llm` helpers; record an ADR when the first
  capability adopts it.

## Adding a capability (decision tree)

1. **Can it be a function over existing primitives?** → Tier 1: add a method
   to the relevant `pkg/*`, validate outputs (see `llm.Classify`), done.
2. **Does it need cross-app state or a shared runtime?** → Tier 2: add
   handlers under a new path prefix on the internal listener
   (`platform/llm.go` is the template: healthz, usage log line, timeouts),
   plus a `pkg/<name>` client. Reserve nothing new — the plane is port 4001.
3. **Neither?** It belongs inside the app, not here.
4. Either way: add the catalog row above, and cross-link per
   [CLAUDE.md](../../CLAUDE.md) rules.

## References

- Rationale: [ADR-0012](../adr/0012-internal-services-two-tier.md),
  [ADR-0006](../adr/0006-library-first-shared-services.md)
- Contracts: gateway wire format in [llm-gateway.md](llm-gateway.md)
- Built in: [roadmap — Later/ideas](../plans/roadmap.md)
