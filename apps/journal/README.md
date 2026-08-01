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

## Services

- `pkg/llm` for the weekly summary — on demand, cached in `summaries`, never
  automatic.

## Non-goals (confirmed)

- No tags/kinds — one undifferentiated stream.
- No editing — append-only; delete exists for misfires.
- No search, no export, no sharing in v1.

## Later

- **Voice capture — designated first consumer for the platform audio
  service** (`audio.Transcribe`, [contract](../../docs/design/internal-services.md#audio-planned-first-class-service)):
  mic button on the capture box → transcribed entry. Gated on the Lemonade
  ops prerequisites in the roadmap backlog.
- Writing prompt on empty days (LLM, cached per day).
- Auto-mood tag via `llm.Classify` → mood glance view.
- Search, kind filters if the stream ever feels unmanageable.
