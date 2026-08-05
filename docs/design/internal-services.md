# Internal Services

Living catalog of shared capabilities apps can use. Rationale:
[ADR-0012](../adr/0012-internal-services-two-tier.md) (two tiers, built on
demand). **Check here before building a capability into an app** — and when
you add one, add its row.

## The plane

```text
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
| Embeddings | 2 | **live** | `llm.Embed(ctx, texts)` / `llm.EmbedQuery(ctx, q)` + `llm.Cosine` → gateway `/llm/embed` | ADR-0029; Lemonade-backed (nomic-embed-text-v2-moe, task prefixes applied by the gateway), no stub — `llm.ErrEmbedUnavailable` without a backend; vectors + model stored per-app as SQLite BLOBs |
| Global search | — | **live** | `web.Search(mux, provider)` → `GET /_search?q=`; dashboard box + `search` MCP/chat tool | ADR-0028; HTTP fan-out (like cards), grouped by app, never their databases; no central index |
| Image generation | 2 | candidate | future `llm.Image` | Backend: Lemonade on selfie |
| Private/local completion | 2 | candidate | future `llm` option (e.g. `WithLocal()`) | Route privacy-sensitive prompts to Lemonade instead of Copilot |
| In-app chat | 1 | **live** | `web.EnableChat(mux, slug, provider)` | ADR-0015; context-stuffed AND agentic — registered `web.Tool`s act automatically (ADR-0021) |
| Dashboard chat (all apps) | 2 | **live** | chat button on the apex dashboard | ADR-0020; aggregates every chat-enabled app's `/_chat/context`, never their databases |
| Markdown rendering | 1 | **live** | `ui.Markdown(text)` | GFM via goldmark, `prose`-styled, raw HTML omitted (tested) |
| App switcher | — | **live** | automatic (AppShell chrome) | ADR-0015; registry via request context, zero app code |
| Version + update check | 1 | **live** | dashboard footer; `version.Platform()`, `version.NewChecker().Info()` | [ADR-0034](../adr/0034-dashboard-version-footer-update-check.md); release read from the module graph, newest release cached 6h from GitHub, background refresh, fail-soft; `BESPOKE_UPDATE_CHECK=off` opts out |
| Transcription | 2 | **LIVE (real)** | `audio.New(slug).Transcribe` + `ui.VoiceButton` (WAV) | ADR-0014; whisper on Lemonade, validated end to end 2026-08-01; stub mode when `BESPOKE_LEMONADE_URL` unset |
| Speech synthesis | 2 | **LIVE** | `audio.New(slug).Speak`; chat panels get a persisted speak-toggle for free | First consumer: the chat speak toggle (ADR-0015 chrome); kokoro-v1 via `/audio/speak`, validated in prod 2026-08-01 |
| Cross-app intents | — | **live** | `[[intents]]` in app.toml + `web.Intent`; selection popover + `ui.IntentsFrom` | ADR-0018; journal↔todo are the reference pair |
| User brief | 2 | **live** | edit at apex `/settings`; injected by the gateway for any completion tagged `llm.WithUser(login)` | ADR-0019; chat + summaries get it automatically; mechanical calls (Classify) untouched |
| App tools + agentic chat | — | **LIVE** | `web.Tool(mux, def)`; chats act automatically (app-scoped and cross-app from the dashboard) | ADR-0021; full CRUD on todo, spec-bounded on journal; chat mic input via local whisper |
| Live updates | — | **LIVE** | `web.Changed(login)` after mutations + `web.Live(mux, fragment)` | ADR-0022; pages and dashboard cards patch over SSE from any face (forms, chat, MCP, intents) |
| MCP surface | 2 | **LIVE** | `claude mcp add --transport http bespoke https://<apex>/mcp` | ADR-0021; per-request identity scoping, tools namespaced `<slug>_<name>` |

## Audio (first-class service — transcription and speech both live)

Speech is a platform capability every app can assume, like auth and storage.
Transcription shipped 2026-08-01 with its first consumer (journal voice
capture) per [ADR-0014](../adr/0014-audio-service-transcription.md), and
runs REAL (Whisper via Lemonade) in prod — validated end to end the same
day:

- **Gateway:** `POST /audio/transcribe` + `GET /audio/healthz` on the 4001
  plane (`platform/audio.go`). Real mode whenever `BESPOKE_LEMONADE_URL` is
  set (the deploy-created env file sets it): OpenAI-compatible multipart to
  Lemonade, model from `BESPOKE_AUDIO_MODEL`. Without the env var the
  gateway falls back to **stub mode** — audio accepted, a clearly-marked
  placeholder returned — so local dev without Lemonade still works. Healthz
  reports the mode; in real mode an unreachable Lemonade becomes a
  dashboard warning.
- **App client:** `pkg/audio` — `audio.New(slug).Transcribe(ctx, r,
  audio.WithMIME(...))`. Same shape as `pkg/llm`; apps are mode-blind.
- **Browser:** `ui.VoiceButton(action)` renders the shared mic button +
  `recorder.js`/`wav.js` (Web Audio capture encoded to WAV client-side —
  Lemonade transcribes WAV only — POST → reload). Every app gets voice
  input the same way; journal's capture box is the reference use, and the
  chat panel's mic reuses the same `wav.js` path.
- **`Speak` / `POST /audio/speak` is live** (first consumer: the chat
  panel's speak toggle, 2026-08-01): JSON `{app, text}` → kokoro-v1 via
  Lemonade → mp3 stream. `pkg/audio.Speak(ctx, text)` returns the stream +
  content type. No stub mode — without a Lemonade backend it reports
  unavailable and chat stays silent. `BESPOKE_TTS_MODEL` overrides the
  model. Every `EnableChat` app auto-mounts `POST /_chat/speak`; the shared
  panel adds the toggle (localStorage-persisted) and autoplays replies.

## Backends

The gateway pattern (ADR-0009/0012) means backends are invisible to apps —
`pkg/llm` is the seam. Two are available:

- **GitHub Copilot** (live): frontier models via the Copilot CLI.
  Cloud inference; ~1.5s/call ([llm-gateway.md](llm-gateway.md)).
- **[Lemonade](https://lemonade-server.ai/)** — local, OpenAI-compatible
  server ON selfie at `http://<app-host>:13305/api/v1` (localhost from
  platformd in prod; the deploy-created env file sets
  `BESPOKE_LEMONADE_URL`). Ideal for embeddings, image generation,
  lighter-weight inference, and **privacy-sensitive prompts that never
  leave the house**; zero marginal cost. Verified 2026-08-01:
  **transcription works** (Whisper-Large-v3-Turbo; the first call after
  idle loads the model and is slow — budget for it), **TTS works**
  (kokoro-v1 generated real audio), and `nomic-embed-text-v2-moe-GGUF` is
  downloaded and serving `llm.Embed`
  ([ADR-0029](../adr/0029-embeddings-via-llm-gateway.md)). Transcription
  accepts **WAV only** —
  the pkg/ui recorder encodes WAV client-side for exactly this reason.

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
  [ADR-0006](../adr/0006-library-first-shared-services.md),
  [ADR-0034](../adr/0034-dashboard-version-footer-update-check.md)
- Contracts: gateway wire format in [llm-gateway.md](llm-gateway.md)
- Built in: [roadmap — Later/ideas](../plans/roadmap.md)
