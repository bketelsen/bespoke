# Print Projects

A compact queue and history for personal 3D-print projects.

## Records

- `projects` — user-scoped name, source URL, freeform notes, and creation time.
- `print_history` — user-scoped project, print date, notes, and optional image.
  A project with no history means “want to print.” Images are stored in this
  app's SQLite database, limited to 8 MiB and JPEG, PNG, WebP, or GIF.

## Views and actions

- `GET /` — create projects and browse project cards, with unprinted projects
  first. Each card shows its source, notes, and newest-first print history.
- `POST /projects` — create a project.
- `POST /projects/{id}/prints` — add history from a dialog; the date defaults
  to today and an image is optional.
- `POST /projects/{id}/edit` — correct a project's name, URL, or notes.
- `GET /prints/{id}/image` — serve a user-scoped history image.
- `POST /projects/{id}/delete` — delete a project and its history.

## Platform surfaces

- Dashboard card: count waiting-to-print projects, total projects, and prints
  in the last 30 days.
- Live region: every form, intent, and tool mutation refreshes open pages.
- Chat and tools: list/create/delete projects and add/list print history.
- `create-project` intent: selected text becomes a project name.

## Non-goals

- No slicer settings, filament inventory, ratings, search, sharing, or file
  hosting in v1. The URL points to STL/STEP pages or files elsewhere.
- Existing apps have no natural event that should automatically propose a
  print-project intent, so no event banners were added.
- Images are app-local because shared blob storage is only a candidate in the
  internal-services catalog; graduate it when a second app needs uploads.
