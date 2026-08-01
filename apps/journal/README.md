# Journal

One stream for every journaling moment — scratch thoughts, work-log lines,
evening reflections — opened whenever, written in seconds.
(Spec produced by the design-app interview, 2026-08-01.)

## Records

- `entries` — id, login, body (freeform text), created_at (UTC). Append-only.
- `summaries` — id, login, range_start/range_end, body, created_at. One per
  generation; latest wins.

## Views

- `GET /` — capture box (textarea) on top; entries below, newest first,
  grouped by day (Today / Yesterday / date). Link to /week.
- `POST /entries` — save an entry, back to `/`.
- `POST /entries/{id}/delete` — accident hatch only (append-only journal).
- `GET /week` — latest saved weekly summary + a "Summarize last 7 days"
  button.
- `POST /week/summarize` — generates via `llm.Complete` (~seconds, page
  action), saves, back to /week.
- `POST /entries/voice` — mic button (`ui.VoiceButton`) records in the
  browser; audio is transcribed via `audio.Transcribe` and saved as a normal
  entry.

## Services

- `pkg/llm` for the weekly summary — on demand, cached in `summaries`, never
  automatic.
- `pkg/audio` for voice capture (first consumer, ADR-0014). **Stub-backed
  until the Lemonade backlog clears** — entries arrive clearly marked as
  stubs; flipping `BESPOKE_LEMONADE_URL` on platformd makes them real with
  no app change.

## Non-goals (confirmed)

- No tags/kinds — one undifferentiated stream.
- No editing — append-only; delete exists for misfires.
- No search, no export, no sharing in v1.

## Later

- Writing prompt on empty days (LLM, cached per day).
- Auto-mood tag via `llm.Classify` → mood glance view.
- Search, kind filters if the stream ever feels unmanageable.
