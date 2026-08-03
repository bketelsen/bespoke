# GH Tracker

Brian opens this to quickly scan open PRs and issues across his configured
GitHub projects. (Spec approved directly by Brian, 2026-08-02.)

## Records

- `projects` — id, login, owner_repo (text, e.g. "octocat/hello-world"),
  position (int, display order), last_fetched_at (datetime, nullable),
  cached_items (JSON blob: list of `{type: pr|issue, title, url, number,
  updated_at}`), created_at. Max 7 rows per login (enforced server-side).
- `settings` — login (primary key), github_token (text, nullable).

## Views

- `GET /` — list of project groups (repo name as header, linking to the
  GitHub repo), each showing its combined open PR/issue list: a badge
  (PR/Issue icon), item title linking out to GitHub, "updated X ago" per
  item. A group whose cache is older than 15 minutes is refreshed from the
  GitHub API before rendering. Empty state points at `/settings` when no
  projects are configured.
- `GET /settings` — manage the list of `owner/repo` entries (add, remove;
  max 7) and the GitHub token field (used for API auth — raises the
  unauthenticated rate limit and is required for private repos).
  `POST /settings/projects` add, `POST /settings/projects/{id}/delete`
  remove, `POST /settings/token` save the token.

## Rules

- Fetching uses the GitHub REST API (`/search/issues` with a
  `repo:owner/name is:open` query, PRs and issues distinguished by the
  `pull_request` field on the result) — one call per stale project, so a
  page load with several stale repos does one call each, not one huge
  query.
- A failed refresh (bad repo name, rate limit, network error) keeps
  serving the last good cache with an inline error note on that group,
  never blanks the list.
- `last_fetched_at` and per-item `updated_at` are stored UTC and rendered
  as local relative times ("updated 12 min ago" / "synced 3 min ago").

## Platform surfaces

- Dashboard card: total open PR/issue count across all configured
  projects (from cache, no live fetch), tap to open `/`.
- No chat tools/intents (read-only tracker, per spec).

## Non-goals

- No creating/commenting/closing PRs or issues.
- No support for more than 7 projects.
- No notifications/push alerts on new items.

## Later

- Filtering by author or label.
- Background periodic refresh independent of app open.
