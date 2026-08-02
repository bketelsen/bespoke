# 0014 — Audio service: transcription, stub-first

- **Status:** Accepted — fully validated live later the same day: WAV-only
  input discovered (the pkg/ui recorder now encodes WAV client-side) and a
  real voice entry transcribed end to end via Whisper-Large-v3-Turbo on
  Lemonade. Stub mode remains the no-config fallback.
- **Date:** 2026-08-01

## Context

The audio contract was pinned in the
[internal services catalog](../design/internal-services.md#audio-planned-first-class-service)
with a build-with-first-consumer rule (ADR-0012). The journal app now wants
voice capture — the designated first consumer — but the Lemonade backend on
selfie isn't ready (models not re-downloaded, loopback-only; see the roadmap
backlog). Waiting on ops would leave the whole client/UI path unbuilt and
unvalidated.

## Decision

Build the transcription path now, backend-flexible:

- **Gateway:** `POST /audio/transcribe` + `GET /audio/healthz` on platformd's
  internal listener (4001 plane), per the pinned contract. The Lemonade call
  is implemented (OpenAI-compatible `audio/transcriptions` multipart,
  `BESPOKE_LEMONADE_URL` + `BESPOKE_AUDIO_MODEL` config) but **stub mode is
  active whenever `BESPOKE_LEMONADE_URL` is unset**: the gateway accepts the
  audio and returns a clearly-marked placeholder transcription. Healthz
  reports the mode; in real mode an unreachable Lemonade surfaces as a
  dashboard warning like the LLM gateway's.
- **Client:** `pkg/audio` — `audio.New(app).Transcribe(ctx, r, opts…)`,
  mirroring `pkg/llm`'s shape. Apps are mode-blind.
- **Browser:** `ui.VoiceButton(action)` + `recorder.js` in pkg/ui assets:
  MediaRecorder toggle, POSTs the blob to the app's endpoint — the shared
  voice-input path every app reuses.
- **Speech synthesis (`/audio/speak`) stays unbuilt** — no consumer yet.

## Consequences

- The entire app-facing surface ships and is testable today; flipping to
  real transcription is setting one env var on selfie after the Lemonade
  backlog clears — no app or client changes.
- Stub transcriptions are visibly placeholders, so no one mistakes them for
  real data.
- The OpenAI-compatible request shape is written against Lemonade docs but
  unvalidated against a live server — verify at flip-on (noted in healthz
  and the catalog).

## Alternatives considered

- **Wait for Lemonade ops:** blocks the consumer and validates nothing.
- **Cloud transcription interim (e.g. via Copilot):** adds a second real
  backend to rip out later; voice is exactly the privacy-sensitive data the
  local backend exists for. Rejected.

## References

- Builds on: [ADR-0012](0012-internal-services-two-tier.md),
  [ADR-0009](0009-copilot-sdk-llm-gateway.md)
- Shapes: [design/internal-services.md](../design/internal-services.md),
  [apps/journal/README.md](../../apps/journal/README.md)
