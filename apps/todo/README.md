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
  `POST /tasks/{id}/delete` — delete (and subtasks).
- `GET /_card` — dashboard card: **Due Today** (includes overdue), **Due
  This Week** (next 7 days), **High Priority** — open tasks only,
  deduplicated in that order, capped at 4 rows each with +N more counts.

## Services

- `pkg/llm` chat (`/_chat`): open tasks with dues/priorities as context —
  "what should I do today?"

## Non-goals

- No recurring tasks, no reordering/drag, no tags/projects, no reminders.

## Later

- Voice capture of tasks (audio service exists — add when wanted).
- Natural-language due dates via `llm.CompleteJSON` on entry.
