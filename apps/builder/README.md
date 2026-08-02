# Builder

You type "build me a…" on the dashboard and watch it become a deployed app:
interview → spec → build → test → live. The app owns the interview, the
spec gate, and run state; everything past the gate runs outside this
process behind the spool
([ADR-0023](../../docs/adr/0023-builder-plane-unprivileged-agent-spooled-deploys.md),
[builder-plane.md](../../docs/design/builder-plane.md)).

## Records

- `runs` — id (`r<unix-ms>`, shared with the spool), idea, slug, spec
  markdown, status (`interviewing → ready → building → deploying → live |
  failed`), failure detail.
- `messages` — the interview transcript (user/assistant).
- `run_events` — mirror of the spool's `events.jsonl` (seq = line number,
  so tailing is idempotent).

## Views

- `GET /` — idea input + run list (live region on the standard `/_live` hub).
- `GET /runs/{id}` — interview chat, spec + approve button, build log. Live
  via a per-run SSE loop (`GET /runs/{id}/live`) that re-renders every 2s
  and patches only on change — the `/_live` hub carries no per-run context.
- `POST /runs`, `POST /runs/{id}/say`, `POST /runs/{id}/approve`.

## The pipeline

1. **Interview** — design-app skill as a gateway conversation
   (`llm.WithUser`); protocol: the final turn starts `SPEC_READY
   slug=<slug>` followed by the spec (interview.go). Bad/taken slugs bounce
   back one interview turn.
2. **Spec gate** — the approve button; the ONLY human gate (by owner
   decision, 2026-08-02).
3. **Build/test/deploy** — the app writes `build/<run>.request.json`; the
   runner (as `builder`) builds in a clone and sandbox-tests on
   `127.0.0.1:42101`; the monitor goroutine tails events, then writes the
   deploy request; the deploy watcher (as the platform user) re-checks,
   pushes main, quiesces, deploys `--edge`.

## Platform surfaces

- Dashboard card: active run status, else shipped count.
- Chat over run state; tools `list_runs`, `get_run`, `start_build` (marked
  explicit-request-only).
- Intent `build-app` (accepts text): selected text anywhere becomes an app
  idea.

## Services

`pkg/llm` for the interview (~2s/turn; the say handler is synchronous and
redirects — no streaming needed). No audio.

## Non-goals

- No modifying existing apps — v1 builds new apps only (guardrails for
  migrations on live data are undesigned).
- No deploy gate and no PR flow — spec approval is deliberately the last
  click; commits go straight to main (owner decision, 2026-08-02).
- One run at a time end-to-end: the runner and watcher drain serially; the
  app doesn't enforce it but the plane does.
- In dev (`just dev`) requests queue in `data/spool` with nothing
  consuming them — runs sit honestly at "building". The plane exists only
  on the app host.

## Later

- Modify-existing-apps with migration/backup guardrails.
- Voice input for the interview (`ui.VoiceButton` — natural fit, deferred).
- Stall detection: a run stuck in building for hours should self-fail.
