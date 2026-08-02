# Spec: App Manifest (`app.toml`)

Every app directory `apps/<slug>/` MUST contain an `app.toml`. The manifest is
the **single source of truth**: platformd's registry, the dashboard, generated
Caddy routes, systemd units, and Litestream config are all derived from
scanning `apps/*/app.toml`. There is no separate registration step or database.

## Fields

| Field | Type | Required | Constraints |
| --- | --- | --- | --- |
| `name` | string | yes | Display name for the dashboard |
| `slug` | string | yes | `[a-z0-9-]{1,32}`; equals the directory name, the subdomain, the binary name, and the SQLite filename |
| `port` | integer | yes | Loopback port; unique across apps; assigned sequentially from 4101 by `bespoke new`; never hand-picked |
| `icon` | string | yes | Iconify icon name, or a path to a PNG inside the app directory |
| `description` | string | yes | One line, shown on the dashboard |

## Example

```toml
name        = "Journal"
slug        = "journal"
port        = 4101
icon        = "notebook"
description = "Daily notes with LLM weekly summaries"
```

## Derived artifacts

| Artifact | Derivation |
| --- | --- |
| Route | `<slug>.bespoke.example.com` → `localhost:<port>` (generated Caddy import) |
| Unit | `bespoke-<slug>.service` (systemd user unit, generated) |
| Database | `data/<slug>.db` via `pkg/db.Open` |
| Backup | Litestream entry for `data/<slug>.db` (generated) |
| Dashboard card | `name`, `icon`, `description`, link to route |

## Rules

- `slug` is immutable after creation (it names the subdomain, DB, and unit).
  Renaming an app is: new app, migrate data, retire old.
- A directory under `apps/` without a valid `app.toml` is ignored by platformd
  and flagged as a warning on the dashboard.
- Reserved: port `4000` and the apex subdomain (platformd), and port `4001`
  (platformd's internal LLM gateway listener — never routed by Caddy).
- The manifest deliberately does not record language/runtime: the contract is
  only "HTTP on `port`, honor the auth header"
  ([ADR-0005](../adr/0005-process-per-app.md)).

## App HTTP contract

Beyond the manifest, an app MUST/MAY serve these endpoints (all provided or
mounted by `pkg/web` for Go apps):

| Endpoint | Requirement | Purpose |
| --- | --- | --- |
| `GET /healthz` | MUST | Deploy gates and monitoring; 200 "ok" |
| `GET /_bespoke/*` | MUST (automatic) | Design-system assets, embedded |
| `GET /_card` | MAY | Per-user dashboard card fragment ([ADR-0017](../adr/0017-app-provided-dashboard-cards.md)); content-only HTML, cheap queries, no LLM calls; dashboard falls back to `description` when absent |
| `POST /_chat`, `/_chat/speak` | MAY (via `web.EnableChat`) | In-app chat + TTS ([ADR-0015](../adr/0015-appshell-platform-chrome.md)) |
| `GET`+`POST /_intents/<name>` | MUST for each declared `[[intents]]` (via `web.Intent`) | Cross-app intent confirm + execute ([ADR-0018](../adr/0018-cross-app-intents.md)) |

The `_`-prefixed path namespace is reserved for the platform.

## Intents (`[[intents]]`)

Optional repeated table declaring cross-app actions
([ADR-0018](../adr/0018-cross-app-intents.md)):

| Field | Required | Constraints |
| --- | --- | --- |
| `name` | yes | `[a-z0-9-]{1,32}`, unique within the app; the `/_intents/<name>` path |
| `title` | yes | Button label other apps show ("Create Todo") |
| `accepts` | no | Payload type; v1 supports only `text` (default) |

Every declared intent MUST be mounted with `web.Intent` — the declaration
is the promise other apps' chrome relies on.
