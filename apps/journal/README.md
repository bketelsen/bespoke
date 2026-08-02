# Journal

One stream for every journaling moment — scratch thoughts, work-log lines,
evening reflections — opened whenever, written in seconds.
(Spec produced by the design-app interview, 2026-08-01.)

## Records

- `entries` — id, login, body (freeform text), created_at (UTC). Append-only.
- `summaries` — id, login, range_start/range_end, body, created_at. One per
  generation; latest wins.

## Views

- `GET /` — tabbed capture card on top (**Random** free-form box · **Work
  Log**: project / task / time, all optional · **Evening Reflection**:
  general / work / family); entries below, newest first, grouped by day
  (Today / Yesterday / date). Link to /week.
- `POST /entries` — save a free-form entry, back to `/`.
- `POST /entries/work`, `POST /entries/reflection` — structured captures,
  formatted to markdown at save (`### Work log` / `### Evening reflection`
  with only the provided fields); an all-empty form saves nothing. The
  stream stays one undifferentiated list — structure lives in the entry
  text, not the schema (non-goal "no kinds" holds).
- `POST /entries/{id}/delete` — accident hatch only (append-only journal).
- `GET /week` — latest saved weekly summary + a "Summarize last 7 days"
  button.
- `POST /week/summarize` — generates via `llm.Complete` (~seconds, page
  action), saves, back to /week.
- `POST /entries/voice` — mic button (`ui.VoiceButton`) records in the
  browser; audio is transcribed via `audio.Transcribe` and saved as a normal
  entry.

## Platform surfaces

- `GET /_card` — dashboard card (ADR-0017): today's entry count, latest
  entry time and a 120-char snippet. Cheap queries only.
- `GET`/`POST /_intents/add-entry` — the declared `[[intents]]` (ADR-0018):
  any app can feed selected text into the journal; todo's "Journal it →"
  done-banner is the reference caller.
- `GET /_live` (ADR-0022) — every mutation (forms, voice, intent, tools)
  calls `web.Changed(login)`; the stream region patches on open pages.
- Chat (`web.EnableChat`, ADR-0015/0020/0021) — context is the last 30 days
  of entries plus the latest weekly summary; mic input and speak toggle
  come with the panel.
- Tools (`web.Tool`, ADR-0021) — `add_entry`, `list_entries`,
  `delete_entry`; exposed to in-app chat, dashboard chat, and MCP.
  **Deliberately no update tool** — the append-only non-goal binds every
  face of the app, not just the forms.

## Services

- `pkg/llm` for the weekly summary — on demand, cached in `summaries`, never
  automatic — and behind the chat panel (agentic over the tools above).
- `pkg/audio` for voice capture (first consumer, ADR-0014): WAV recorder →
  Whisper-Large-v3-Turbo on Lemonade, **validated live end to end
  2026-08-01**. Requires `BESPOKE_LEMONADE_URL` on platformd (set in dev
  too, e.g. `BESPOKE_LEMONADE_URL=http://<app-host>:13305/api/v1 just
  dev`); unset gives clearly-marked stub entries.

## Non-goals (confirmed)

- No tags/kinds — one undifferentiated stream.
- No editing — append-only; delete exists for misfires.
- No search, no export, no sharing in v1.

## Later

- Writing prompt on empty days (LLM, cached per day).
- Auto-mood tag via `llm.Classify` → mood glance view.
- Search, kind filters if the stream ever feels unmanageable.
