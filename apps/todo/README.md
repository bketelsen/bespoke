# Todo

Tasks with optional due dates, priorities, and one level of subtasks.
(Spec provided directly by Brian, 2026-08-01 — one-shot log 2/3.)

## Records

- `tasks` — id, login, parent_id (nullable; parents must be top-level →
  exactly one level of nesting), description, due (nullable date),
  priority (`L`/`M`/`H`, default `L`), done, created_at, completed_at.

## Rules

- **Cascade up:** completing the last open subtask completes the parent;
  reopening any subtask reopens the parent.
- **Cascade down:** toggling a parent completes/reopens all its subtasks.
- Subtasks can't have their own subtasks (enforced server-side).
- Deleting a parent deletes its subtasks.

## Views

- `GET /` — add form (description, due date, priority) + all tasks, parents
  with nested subtasks. Open tasks sort: due date (nulls last), then
  priority H→M→L, then created. Completed shown by default;
  `?completed=hidden` toggle link hides them. Overdue dues render
  destructive; Today/Tomorrow humanized.
- `POST /tasks` — create top-level task. `POST /tasks/{id}/sub` — add
  subtask. `POST /tasks/{id}/toggle` — complete/reopen with cascades.
  `GET`/`POST /tasks/{id}/edit` — edit description/due/priority (pencil on
  each row). `POST /tasks/{id}/delete` — delete (and subtasks).
- Completing a task via the toggle redirects to `/?did=<description>` and
  shows a done banner offering **"Save as Note →"** — Notes' `add-note`
  intent discovered live from the registry (`ui.IntentsFrom`, ADR-0018).

## Platform surfaces

- `GET /_card` — dashboard card (ADR-0017): **Due Today** (includes
  overdue), **Due This Week** (next 7 days), **High Priority** — open
  *top-level* tasks only (subtasks never appear), deduplicated in that
  order, capped at 4 rows each with +N more counts.
- `GET`/`POST /_intents/create-task` — the declared `[[intents]]`
  (ADR-0018): highlight text anywhere, "Create Todo" appears.
- `GET /_live` (ADR-0022) — every mutation (forms, edit, toggle, tools,
  intent) calls `web.Changed(login)`; the task list patches on open pages.
- Tools (`web.Tool`, ADR-0021) — full CRUD: `list_tasks`, `create_task`,
  `update_task`, `set_task_done` (cascade-aware), `delete_task`; parent
  validation enforces the one-level nesting rule on every face. Exposed to
  in-app chat, dashboard chat, and MCP.

## Services

- `pkg/llm` chat (`/_chat`): all tasks (completed included, subtasks
  labeled) as context, and **agentic** over the tools above — "add milk to
  the shopping list" mutates, not just answers.

## Non-goals

- No recurring tasks, no reordering/drag, no tags/projects, no reminders.

## Later

- Voice capture of tasks (audio service exists — add when wanted).
- Natural-language due dates via `llm.CompleteJSON` on entry.
