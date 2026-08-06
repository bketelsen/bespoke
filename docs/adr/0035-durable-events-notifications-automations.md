# 0035 — Build notifications and automations on durable domain events

- **Status:** Accepted
- **Date:** 2026-08-06

## Context

Apps currently announce mutations with `web.Changed(login)`. That signal is
deliberately payload-free, lossy, and process-local except for a nudge to
platformd; it is sufficient to re-render live fragments but cannot support an
inbox, offline delivery, retries, or an audit trail.

Several desired capabilities need the same stronger input: durable in-app
notifications, optional web push, and user-authored rules such as “when a new
message from this sender arrives, extract a task and add it to Todo.” The
existing `web.Tool` registry already supplies user-scoped app actions, while
the LLM gateway supplies structured inference. Neither defines when an action
should run or records why it ran.

## Decision

Platformd will own a durable, user-scoped event and automation service on the
internal plane. Apps publish immutable domain events after committing the
corresponding app mutation. The service stores each event, projects matching
events into durable notifications, and evaluates enabled automation rules.

Rule triggers and conditions are deterministic. Rules may contain bounded,
schema-validated AI transformation steps, but AI output is never itself an
unrestricted instruction stream. Mutations execute only through app-registered
tools explicitly marked safe for unattended automation. Every evaluation and
action records its event, rule, user, inputs, outcome, and causation chain.

`web.Changed` remains the independent mechanism for refreshing open pages.
Notifications are rendered from their durable store as an AppShell inbox and
as transient toasts. Web push, when implemented, is an optional delivery
channel for the same notification records; it is not the system of record.

The apex dashboard is the installable PWA and owns its push subscription.
Notification clicks may deep-link to app subdomains. Service workers remain
origin-scoped; this decision does not attempt to make the wildcard domain one
browser origin.

## Consequences

- Mail, calendar, and future apps share one event contract without sharing
  databases.
- A notification remains inspectable when no browser was open, and automation
  runs have enough provenance to explain their effects.
- Platformd gains cross-app durable state and a background worker lifecycle.
- Under the current on-host trust model, installed apps can assert another
  source or user to internal event and notification APIs. Browser isolation
  remains enforced; app-to-app isolation needs a later credential or UID
  boundary.
- Event producers must publish only after their local transaction commits and
  must use stable event IDs when retrying publication.
- App tools intended for unattended use must implement the automation safety
  and idempotency contract; existing chat/MCP tools do not become automation
  actions implicitly.
- PWA installation improves notification delivery but does not make app data
  available offline.

## Alternatives considered

- **Extend `web.Changed` with payloads:** rejected because its fire-and-forget,
  coalescing behavior is useful for UI invalidation and incompatible with a
  durable work queue.
- **Let each app own its notifications and rules:** rejected because cross-app
  actions, one AppShell inbox, and centralized push subscriptions would then
  require every app to rediscover and coordinate every other app.
- **Run a free-form resident agent over all changes:** rejected because trigger
  matching, permissions, retry behavior, and causation would not be
  deterministic or readily auditable.
- **Treat web push as the notification store:** rejected because permission can
  be denied, subscriptions expire, and delivery is not a durable user inbox.

## References

- Shapes: [event notifications and automations spec](../specs/event-notifications-automations.md), [CLI spec](../specs/bespoke-cli.md), [architecture](../design/architecture.md), [internal services](../design/internal-services.md), [deploy runbook](../../deploy/README.md), [roadmap — Phase 8](../plans/roadmap.md#phase-8--events-notifications-and-automations)
- Builds on: [ADR-0012](0012-internal-services-two-tier.md), [ADR-0021](0021-tools-agentic-chat-mcp.md), [ADR-0022](0022-live-updates.md), [ADR-0032](0032-app-unit-sandboxing.md)
