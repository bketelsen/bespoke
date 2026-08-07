# Automations

Living document. Rationale: [ADR-0035](../adr/0035-durable-events-notifications-automations.md).
Contracts: [specs/event-notifications-automations.md](../specs/event-notifications-automations.md).

## Overview

Automations turn app-published domain events into user-defined reactions:
deterministic rules that notify, run a bounded AI transformation, and invoke
eligible app tools. There is no rule-builder UI yet — rules are created and
operated through authenticated JSON routes on the apex domain, from the
browser console, curl on the tailnet, or an agent. This document is the
practical companion to the spec: how to author a rule today, what the
template syntax is, and how to read a run.

```
app event ──► platformd (match rules) ──► run: notify / ai_json / tool ──► app
                    │                                          (idempotent)
                    └──► notification inbox / toast
```

## Creating a rule

The apex JSON routes (all authenticated, current-user scoped):

| Route | Purpose |
| --- | --- |
| `POST /_automations/rules` | Create; rules start **disabled** |
| `POST /_automations/rules/{id}/enable` | Validate against app/tool registries, then enable |
| `POST /_automations/rules/{id}/disable` | Stop new runs; history is kept |
| `PUT /_automations/rules/{id}` | Full replacement; must send the current `revision` (`409` on staleness) |
| `POST /_automations/rules/{id}/dry-run` | Body `{"event_id": …}`; evaluates without side effects |
| `GET /_automations/runs?rule_id=…` | Newest-first run summaries |
| `GET /_automations/runs/{id}` | Run plus per-step records |
| `POST /_automations/runs/{id}/retry` | Resume a failed run at its first non-succeeded step |

A rule body names an exact `source` (app slug) and `event_type` (no
wildcards), 0–12 AND-ed `conditions`, and 1–8 ordered `steps`:

```json
{
  "name": "Work log to task",
  "source": "journal",
  "event_type": "journal.entry_created",
  "conditions": [
    {"path": "data.source", "operator": "equals", "value": "work"}
  ],
  "steps": [
    {"name": "extract", "type": "ai_json",
     "instruction": "Respond with ONLY a JSON object of exactly this shape: {\"action\": \"<short imperative task>\"} …",
     "input": {"entry": "{{event.data.preview}}"},
     "schema": {"type": "object", "properties": {"action": {"type": "string"}}, "required": ["action"]}},
    {"name": "task", "type": "tool",
     "tool_app": "todo", "tool_name": "create_task",
     "args": {"description": "$steps.extract.action", "priority": "M"}}
  ]
}
```

Condition operators: `equals`, `not_equals`, `contains`, `starts_with`,
`exists`, `greater_than`, `less_than`, over `subject_id` or dot-paths under
`data`. A missing path fails every operator except `exists`.

### Template and reference syntax

- `{{event.data.x}}`, `{{event.subject_id}}`, `{{event.id}}` interpolate into
  any step string (notify titles/bodies, `ai_json` input values, tool args).
- A tool-arg string starting with `$` passes the referenced value **raw**,
  preserving JSON types: `"$event.data.id"` stays a number,
  `"$steps.extract.items"` stays an array.
- `{{steps.<name>.<field>}}` / `"$steps.<name>.<field>"` read a completed
  `ai_json` step's schema-validated output. Steps can only reference the
  event and steps listed before them.

### Step types

- **`notify`** — creates a durable notification (title ≤120 bytes, body ≤500,
  destination `{app_slug, path}`). Over-limit expansion fails the step; it is
  never truncated.
- **`ai_json`** — one LLM call under a fixed JSON-only system instruction;
  output is validated against the step's JSON Schema. Invalid or oversized
  output fails the step and is never passed onward — model prose is never
  treated as an action. Small local models usually need the instruction to
  spell out the exact object shape with an example; "extract X" alone tends to
  come back as a bare string, which the validator rejects.
- **`tool`** — invokes one automation-eligible app tool (`tool_app` +
  `tool_name`) with mapped `args`. Only tools whose `web.ToolDef` declares
  `read_only` or `idempotent` validate; everything else is rejected at
  save/enable time. Idempotent tools receive the step's stable action UUID as
  `Idempotency-Key`, so worker retries re-address the same effect. `GET
  /_tools` lists each tool's mode.

## Operating rules

- **Order of operations**: create → dry-run → enable → trigger. Rules only see
  events accepted after `enabled_at` — there is no historical replay, so
  enable first, then cause the event.
- **Dry-run** needs a stored current-user event ID. The easiest source is
  `GET /_notifications` (each record carries its `event_id`); for silent
  events, take `event_id` from a previous run record.
- **Runs are durable**: one run per `(rule, event)`, leased workers, three
  attempts per step with 5 s / 30 s backoff, per-step `input_json` /
  `output_json` / `error` records. A failed step stops the run; `retry`
  resumes from the first non-succeeded step without repeating completed ones.
  `needs_attention` marks a tool transport failure that may have reached the
  tool — the platform will not guess, a person decides.
- **Edits are revisioned**: each run stores the rule revision it evaluated, so
  editing a rule never changes what history means. A failed run retried after
  an edit still runs its original revision — trigger a fresh event to exercise
  the new one.
- **Event dedup is the producer's ID**: publishing the same `(source, id)`
  again is a no-op. Apps exploit this with deterministic UUIDs (e.g.
  `uuid.NewSHA1` over login + subject + milestone) to make recomputed
  milestones single-shot.

## Operational notes

- The internal plane (`POST /events/publish`, `GET /events/healthz`) listens
  on `BESPOKE_INTERNAL_URL` — the Tailscale bind IP, port 4001, **not**
  loopback. A `curl 127.0.0.1:4001/events/healthz` connection-refused is a
  probe error, not an outage; read the address from `~/bespoke/env`.
- Plane degradation (database/worker unavailable) returns `503` from healthz
  and appears in the dashboard warning strip. Apps log-and-swallow publish
  errors, so a down plane silently drops events — check healthz first when
  notifications stop.
- Events, notifications, and runs are retained 180 days; cleanup runs in
  platformd at startup and daily.
- Not yet expressible: time-based triggers (no schedules — events fire on
  domain occurrences only), wildcard event types, regex conditions, web push.

## References

- Rationale: [ADR-0035](../adr/0035-durable-events-notifications-automations.md)
- Contracts: [specs/event-notifications-automations.md](../specs/event-notifications-automations.md)
- Built in: [roadmap — Phase 8](../plans/roadmap.md)
