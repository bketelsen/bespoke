# Spec: Event notifications and automations

This contract governs how Bespoke apps publish user-scoped domain events, how
platformd turns them into durable notifications, and how deterministic rules
may invoke bounded AI transformations and app tools. It is consumed by apps
through `pkg/events`, by platformd's internal service and worker, and by the
AppShell notification UI.

## Scope

The first implementation covers durable events, an in-app notification inbox,
live toasts, deterministic rule evaluation, structured AI steps, and eligible
app-tool actions. It does not include web push, offline app data, recurring
time-based schedules, a persistent chat composer, arbitrary user code, or
free-form agents selecting tools. Those features may build on this contract
without changing the event record.

`web.Changed(login)` remains the live-fragment invalidation API. Publishing an
event neither replaces nor implicitly calls `web.Changed`.

## Interface

### App client

`pkg/events` exposes:

```go
type Event struct {
	ID         string
	Type       string
	SubjectID  string
	OccurredAt time.Time
	Data       map[string]any
	Notification *Notification
}

type Notification struct {
	Title, Body, AppSlug, Path, GroupKey string
}

func New(appSlug string) *Client
func (c *Client) Publish(ctx context.Context, user auth.User, event Event) error
```

`New` reads `BESPOKE_INTERNAL_URL`, defaulting to
`http://127.0.0.1:4001`. Deployment sets that platform-wide variable for
every app unit; event code does not reuse the legacy LLM-named
`BESPOKE_LLM_URL`.

`Publish` sends the app slug, authenticated user's login, and event to
platformd. The app calls it only after the app mutation commits. A publication
failure does not roll back that mutation; producers that require eventual
publication retain the event ID and retry it.

`Notification` is optional. When present, platformd creates the durable
notification in the same transaction that accepts the event. Its title, body,
and group key use the notification-record limits; `AppSlug` and `Path` use the
portable destination contract below. Automation `notify` steps use the same
record contract but are not part of event acceptance.

### Event record

| Field | Type | Required | Constraints |
| --- | --- | --- | --- |
| `id` | string | yes | Producer-generated UUID; immutable; unique within `source` |
| `source` | string | yes | Publishing app slug; 1–32 manifest-slug characters |
| `type` | string | yes | 1–100 characters; lowercase dot-separated tokens matching `[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+` |
| `user` | string | yes | Login supplied from `auth.User`; 1–320 UTF-8 bytes |
| `subject_id` | string | no | Source-app record identifier; at most 512 UTF-8 bytes |
| `occurred_at` | RFC 3339 timestamp | yes | UTC; describes the domain occurrence |
| `accepted_at` | RFC 3339 timestamp | server | UTC platformd receipt time |
| `data` | JSON object | yes | At most 64 KiB encoded; maximum nesting depth 8 |
| `causation_id` | string | no | Event ID that caused this event |
| `correlation_id` | string | server/default | First event ID in the chain, or `id` for a root event |
| `depth` | integer | server/default | Root is 0; caused events are parent depth + 1; maximum 8 |

`data` contains fields needed for matching and display, not an entire source
record by default. Credentials, session tokens, model-provider secrets, and
raw attachment bytes MUST NOT be published. A rule needing additional private
content obtains it through a user-scoped read tool.

Publishing the same `(source, id)` again returns success and the original
record without comparing the retry body. Producer-generated IDs are therefore
the idempotency contract; an app MUST NOT reuse an ID for a different domain
occurrence. The `data` limit is measured on its `encoding/json` representation
before storage. A `data` value over 64 KiB or a complete request over 80 KiB
returns `413 Payload Too Large`; other invalid fields return `400 Bad Request`.

### Internal event endpoint

Apps publish to `POST /events/publish` on platformd's port-4001 internal
listener. The event service also exposes `GET /events/healthz`; database or
worker unavailability returns `503`, appears in the dashboard warning strip,
and otherwise returns `200 ok`. Every internal request logs operation, source,
user, status, and duration, but never logs event `data` or notification text.
The JSON request is:

```json
{
  "source": "mail",
  "user": "alice@example.com",
  "event": {
    "id": "ae4bcf64-19e6-4e52-a282-6a1a90eb8e6f",
    "type": "mail.received",
    "subject_id": "message-123",
    "occurred_at": "2026-08-06T14:20:00Z",
    "data": {"account": "personal", "from": "person@example.com"},
    "notification": {
      "title": "New message",
      "body": "Dinner Friday?",
      "app_slug": "mail",
      "path": "/messages/message-123",
      "group_key": "mail:personal"
    }
  }
}
```

Success returns `202 Accepted` for a new event and `200 OK` for an idempotent
duplicate. The response is `application/json` containing `{"id":"…"}`. This
endpoint is never Caddy-routed. Under the current same-UID/on-host trust model,
both `source` and `user` are caller assertions: any installed app process that
can reach port 4001 can publish as another app or user. The same limitation
applies to internal notification reads. Browser routes still enforce user
isolation, but this contract does not claim isolation from a hostile installed
app; changing that boundary requires per-app service credentials or UIDs.

If `Publish` runs inside a tool call caused by an automation, it obtains the
causing event ID from context and includes it. Platformd rejects a missing,
unknown, or different-user causing event with `400`. It derives correlation
and depth from the stored parent rather than accepting either from the caller.
A child of a depth-8 event would have depth 9 and is rejected with `409`.

### Notification record

| Field | Type | Required | Constraints |
| --- | --- | --- | --- |
| `id` | string | yes | Server-generated UUID |
| `user` | string | yes | Copied from the event |
| `event_id` | string | yes | Originating stored event |
| `source` | string | yes | Originating app slug |
| `title` | string | yes | 1–120 UTF-8 bytes |
| `body` | string | no | At most 500 UTF-8 bytes |
| `url` | string | server/no | Resolved from `app_slug` plus `path`; absent for retired sources |
| `group_key` | string | no | At most 200 UTF-8 bytes |
| `dedup_key` | UUID | no | Step-action UUID; unique when present |
| `created_at` | timestamp | yes | UTC |
| `read_at` | timestamp/null | yes | UTC when marked read |
| `dismissed_at` | timestamp/null | yes | UTC when dismissed |

Notification rendering escapes title and body as text. Producers and rules
specify a destination as `{app_slug, path}`, where `path` begins with `/` and
is at most 1,024 bytes. Platformd resolves it through the registered app and
current dev/production domain configuration. An empty slug means the apex.
Callers cannot store an arbitrary URL.

The internal plane owns the data APIs:

| Route | Contract |
| --- | --- |
| `GET /events/notifications?user=<login>&after=<cursor>&limit=<n>` | JSON page; default 30, maximum 100 |
| `GET /events/notifications/unread?user=<login>` | JSON `{"count":n}`, capped for display at 999 |
| `GET /events/notifications/live?user=<login>` | SSE records for notifications created after connection; 45-second heartbeat |
| `POST /events/notifications/{id}/read` | JSON `{"user":"…"}`; set `read_at` once |
| `POST /events/notifications/{id}/dismiss` | JSON `{"user":"…"}`; set `dismissed_at` once |
| `POST /events/notifications/read-all` | JSON `{"user":"…"}`; mark that user's current notifications read |

List responses use `application/json` with
`{"notifications":[<notification>],"next":"<opaque-or-empty>"}`. Mutation
success is `204`; an invalid cursor or body is `400`; a notification absent for
the asserted user is `404`. The plane SSE event is named `notification` and
its JSON data is one notification record.

Authenticated same-origin routes mounted automatically by `pkg/web` are thin
proxies to those internal APIs:

| Route | Contract |
| --- | --- |
| `GET /_notifications?after=<cursor>&limit=<n>` | Proxies the JSON page for the authenticated user |
| `GET /_notifications/live` | Relays the plane SSE for the authenticated user as Datastar patches |
| `POST /_notifications/{id}/read` | Sets `read_at` once; `204` if already read |
| `POST /_notifications/{id}/dismiss` | Sets `dismissed_at` once; `204` if already dismissed |
| `POST /_notifications/read-all` | Marks all current-user notifications read; returns `204` |

The browser never supplies `user`; the proxy derives it from
`auth.FromContext`. An ID owned by another user is indistinguishable from a
missing ID and returns `404`. Cursors are opaque; an invalid cursor returns
`400`.

The relay patches id-stable AppShell elements `#bespoke-notification-count`,
`#bespoke-notification-list`, and `#bespoke-notification-toasts`. The list and
count are re-fetched from the plane before each patch; the toast is rendered
from the streamed record. `/_notifications/live` is intentionally a second
SSE stream beside app-specific `/_live`: notification data is owned by
platformd and must update even when the app has no live fragment. At personal
scale the simpler ownership and reconnect behavior outweigh one additional
connection and goroutine per open tab.

AppShell displays a bell with the unread count on every page, an inbox ordered
newest first, and an in-page toast for newly streamed notifications. A toast
is transient presentation only: dismissing the toast does not dismiss or mark
the notification read. Multiple unread records sharing `(source, group_key)`
may be summarized in the UI, but remain independently addressable records.

### Automation rule

| Field | Type | Required | Constraints |
| --- | --- | --- | --- |
| `id` | string | yes | Server-generated UUID |
| `user` | string | yes | Owner; never accepted from browser form data |
| `name` | string | yes | 1–100 UTF-8 bytes |
| `enabled` | boolean | yes | New rules default false until validation succeeds |
| `source` | string | yes | Exact registered app slug |
| `event_type` | string | yes | Exact event type; no wildcard in v1 |
| `conditions` | array | yes | 0–12 deterministic conditions, all of which must match |
| `steps` | array | yes | 1–8 ordered steps |
| `revision` | integer | yes | Starts at 1 and increments on every successful update |
| `enabled_at` | timestamp/null | yes | Set to current UTC time on every disabled-to-enabled transition |
| `created_at` / `updated_at` | timestamps | yes | UTC |

A condition has `path`, `operator`, and `value`. `path` is `subject_id` or a
dot-separated path below `data`; array indexing is not supported. Supported
operators are `equals`, `not_equals`, `contains`, `starts_with`, `exists`,
`greater_than`, and `less_than`. String comparisons are Unicode case-sensitive.
Ordering operators accept only JSON numbers. A missing path makes every
operator except `exists` false. There are no regular expressions or executable
expressions in v1.

Event types are validated syntactically, not against a manifest registry.
Rule forms offer types observed for the selected source and user, while still
allowing a valid type that has not yet occurred. Create and update reject an
unknown source, syntactically invalid event type, invalid condition, unknown
step reference, ineligible tool, or limit violation with `400`; they do not
partially save. Enabling a rule requires successful validation against the
current app and tool registries. One user may store at most 100 non-deleted
rules; creation beyond the cap returns `409`.

Authenticated apex JSON routes are:

| Route | Contract |
| --- | --- |
| `GET /_automations/rules` | `{"rules":[…]}` for the current user |
| `POST /_automations/rules` | Create from the rule fields excluding server fields; returns `201` with the stored rule |
| `GET /_automations/rules/{id}` | One current-user rule and its validation status |
| `PUT /_automations/rules/{id}` | Full replacement; requires the current `revision`; stale revision returns `409` |
| `DELETE /_automations/rules/{id}` | Soft-delete and disable; returns `204` |
| `POST /_automations/rules/{id}/enable` | Validate, set `enabled=true`, reset `enabled_at`, increment revision |
| `POST /_automations/rules/{id}/disable` | Set `enabled=false`, clear `enabled_at`, increment revision |
| `POST /_automations/rules/{id}/dry-run` | Body `{"event_id":"…"}`; return match and resolved mappings without effects |
| `GET /_automations/runs?rule_id=<id>&after=<cursor>&limit=<n>` | Newest-first run summaries; default 30, maximum 100 |
| `GET /_automations/runs/{id}` | Run plus step records for the current user |
| `POST /_automations/runs/{id}/retry` | Resume an eligible failed run at its first non-succeeded step |

All bodies and successful responses except `204` are `application/json`. A
foreign-user ID returns `404`. Dry-run requires a stored current-user event,
evaluates conditions and reference/template expansion, and validates tool
eligibility and schemas. It does not call an LLM, create a notification, invoke
a tool, or create a run; an `ai_json` step is reported as `not_executed` and
later references to its output as `unresolved`.

### Automation steps

Steps execute in listed order and have a stable name unique within the rule.
The supported step types are:

1. `notify`: creates a notification using bounded templates over event fields
   and prior step outputs.
2. `ai_json`: calls `llm.Complete` with a fixed JSON-only system instruction,
   parses the response, and validates it in the worker against a JSON Schema
   Draft 2020-12 document using `github.com/google/jsonschema-go/jsonschema`.
3. `tool`: invokes one named app tool with JSON arguments mapped from literals,
   event fields, or prior structured outputs.

Template and mapping references can read only the current event and completed
step outputs. A rendered notification title/body and tool argument document
must satisfy their destination limits. Expansion beyond a limit fails the
step; it is never silently truncated.

An `ai_json` instruction is at most 4 KiB, mapped input is at most 32 KiB, its
schema is at most 16 KiB, and its validated output is at most 32 KiB. A step
times out after 90 seconds, below the gateway's existing 120-second
non-agentic deadline. Invalid or oversized output fails the step and is not
exposed to later steps. Model prose that is not valid against the declared
schema is never treated as an action. The call omits `llm.WithUser`: automation
is a mechanical transformation whose stored instruction and inputs must not
silently change meaning when the user edits their brief.

### Automation-eligible tools

Existing `web.Tool` registrations remain ineligible by default. A tool may opt
in with automation metadata:

```go
Automation: web.AutomationPolicy{
	Mode: web.AutomationIdempotent,
}
```

The only v1 modes are `forbidden` (the zero value), `read_only`, and
`idempotent`. Destructive tools MUST remain `forbidden`. An `idempotent` tool
receives an `Idempotency-Key` equal to the action-run UUID and MUST persist the
key and completed result in the same database transaction as its mutation.
Repeating the key returns the stored result without repeating the mutation.
The stored result is the successful handler's opaque UTF-8 string; replay
returns that string with the normal `200 text/plain` response. Errors are not
cached. The tool retains idempotency records for at least 180 days.

`GET /_tools` adds an `automation` string with one of those three modes to
each advertised tool. `pkg/web` copies `Idempotency-Key` into the handler
context; handlers read it with `web.IdempotencyKey(ctx) (string, bool)`.
Missing or malformed keys on direct chat/MCP calls remain valid for tools whose
normal behavior permits them; automation invokes an `idempotent` tool only
with a valid UUID key.

The automation worker invokes tools through their existing user-scoped HTTP
endpoint with the rule owner's forwarded identity. It also sends
`Bespoke-Causation-ID` containing the originating event ID. `pkg/web` validates
that header's UUID syntax and places it in context using `pkg/events`; a later
`events.Publish` automatically propagates it. Platformd looks up the parent to
derive correlation and depth. A tool removed from the registry or changed to
`forbidden` makes future action attempts fail without calling it; it does not
silently disable or rewrite the rule.

### Run and action records

One run exists for each `(rule_id, event_id)` and that pair is unique.

| Run field | Type | Contract |
| --- | --- | --- |
| `id` | UUID | Stable run ID |
| `user`, `rule_id`, `event_id` | strings | Ownership and unique input pair |
| `rule_revision` | integer | Immutable snapshot revision used by the run |
| `status` | enum | `pending`, `running`, `succeeded`, `failed`, or `needs_attention` |
| `next_attempt_at` | timestamp/null | Earliest claim time for a retry |
| `lease_owner` | string/null | Worker-instance UUID while claimed |
| `lease_expires_at` | timestamp/null | Claim expiry, 150 seconds after claim or renewal |
| `created_at`, `started_at`, `finished_at` | timestamps/null | UTC lifecycle times |

| Step field | Type | Contract |
| --- | --- | --- |
| `id` | UUID | Stable action/idempotency key assigned before the first attempt |
| `run_id`, `name`, `position`, `type` | values | Owning run and rule-step identity |
| `status` | enum | `pending`, `running`, `succeeded`, `failed`, or `needs_attention` |
| `attempt` | integer | Starts at 0; incremented atomically when claimed |
| `next_attempt_at` | timestamp/null | Retry eligibility |
| `input_json`, `output_json` | JSON/null | Each at most 32 KiB after redaction |
| `error` | string/null | At most 4 KiB |
| `started_at`, `finished_at` | timestamps/null | UTC lifecycle times |

Secrets returned by tools MUST be redacted before persistence.

Each step action has a stable UUID assigned before its first attempt. A
`notify` retry uses that UUID as its notification deduplication key, and a
`tool` retry uses it as `Idempotency-Key`; retries therefore address the same
effect rather than creating a new one.

The worker atomically claims an eligible run and current step by setting its
lease. It renews the lease every 30 seconds during work. On startup and once
per minute, a worker returns expired `running` leases to `pending` without
incrementing the attempt; the next claim performs that increment. A completed
step is never reclaimed.

Failed `notify` and `ai_json` steps
receive at most three attempts with delays of 5 seconds and 30 seconds.
`read_only` and `idempotent` tools receive at most three attempts with the same
delays. A tool transport failure after the request may have reached a tool
that violated its idempotency contract is marked `needs_attention`; subsequent
steps do not execute automatically.

A failed step stops the run. Re-running a failed run is an explicit user
operation and continues from the first non-succeeded step. Disabling a rule
prevents new runs and pending retries but does not erase history.

Events at causation depth 8 are stored and may create notifications, but MUST
NOT start automation runs. This bounds cross-app action loops.

## Rules

- Event, notification, rule, and run queries are always scoped to the
  authenticated user's login.
- Event acceptance, notification projection, and creation of matching pending
  runs occur in one platformd database transaction.
- Rules see events accepted after the rule was enabled. Saving or enabling a
  rule does not replay historical events in v1. Re-enabling resets
  `enabled_at`; the service compares it to event `accepted_at`.
- Conditions are evaluated against the immutable stored event, not by fetching
  mutable source records.
- A rule is evaluated at most once per event; the unique run key enforces this
  across worker restarts.
- Rule edits affect only runs created after the edit commits. Each run stores
  the rule revision it evaluated.
- Notification creation and automation execution are independent: one may be
  configured without the other.
- Event, run, and notification records are retained for 180 days based on
  `accepted_at` or `created_at`, never producer-controlled `occurred_at`.
  platformd's in-process worker performs cleanup once at startup and every 24
  hours; no systemd timer or second service is generated.
- Failures to display a toast or maintain an SSE connection never change
  durable notification or run state.
- Times are stored in UTC and converted to the user's local time for display
  and LLM input.
- The service stores data in platformd's existing database and migrations.
  Existing generated Litestream configuration already replicates
  `platformd.db`; this feature MUST NOT add a second replica entry.
- When an app disappears from the manifest registry, platformd disables its
  enabled rules on the next registry scan. Historical events, notifications,
  and runs remain until retention cleanup and are marked `source_retired` in
  API responses; notification destinations for that slug render without a
  clickable link. Restoring the app does not automatically re-enable rules.

## Acceptance criteria

1. Publishing a valid event stores one immutable event and returns `202`;
   publishing the same `(source, id)` with either identical or changed retry
   content returns `200` without another notification or automation run.
2. A 65-KiB `data` object and an over-80-KiB request envelope each return `413`
   without storing any part of the request.
3. A notification created while no browser is connected appears unread on the
   next inbox request. When connected, the same record also produces a toast
   without being marked read.
4. Through authenticated browser routes, one user's notification and rule IDs
   return `404` to another user, and no list or SSE response crosses that
   boundary. A separate test documents that the trusted internal API accepts
   its caller-asserted user under the current on-host trust model.
5. Exact event type plus all deterministic conditions selects a rule; a
   missing path and a type mismatch each produce no run.
6. Two workers racing the same `(rule, event)` create or claim only one run.
7. Invalid AI JSON, schema-invalid AI JSON, and AI output over 32 KiB each fail
   the step without invoking a following tool.
8. A tool without automation metadata cannot be saved in an enabled rule. An
   idempotent tool retried with the same action key performs one mutation and
   returns the first stored result.
9. A caused event at depth 8 is stored and can notify, but creates no
   automation run.
10. Restarting platformd during a running step preserves the run; after the
    150-second lease expires another worker resumes it under the retry rules,
    and completed steps are not repeated.
11. `web.Changed` continues to patch live app fragments whether or not the app
    publishes an event, and event publication alone does not patch them.
12. An unknown or different-user causation ID returns `400`, a child of a
    depth-8 event returns `409`, and a depth-8 event creates no run.
13. Rule CRUD enforces revision conflicts, the 100-rule cap, foreign-ID `404`s,
    side-effect-free dry runs, and observed-but-not-required event type choices.
14. Internal notification list/mutation endpoints and their same-origin thin
    proxies return the specified JSON/status shapes; the relayed SSE patches
    all three named AppShell element IDs.
15. `just check` passes with unit tests for validation and matching, database
    tests for uniqueness and claiming, and HTTP tests for user isolation and
    size limits.

## Derived artifacts

| Artifact | Derivation |
| --- | --- |
| AppShell notification inbox and bell | Current user's durable notification records |
| Live toast | Newly streamed notification record |
| Automation run history | Stored run and per-step action records |
| Future web push payload | Notification title, body, source, and validated deep link |

## References

- Rationale: [ADR-0035](../adr/0035-durable-events-notifications-automations.md)
- Context: [architecture](../design/architecture.md), [internal services](../design/internal-services.md)
- Delivery plan: [roadmap — Phase 8](../plans/roadmap.md#phase-8--events-notifications-and-automations)
