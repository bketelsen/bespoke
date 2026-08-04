# Spec: App Manifest (`app.toml`)

Every app directory `<instance>/apps/<slug>/` MUST contain an `app.toml`. The manifest is
the **single source of truth**: platformd's registry, the dashboard, generated
Caddy routes, systemd units, and Litestream config are all derived from
scanning `apps/*/app.toml`. There is no separate registration step or database.
The instance boundary is defined by
[ADR-0027](../adr/0027-versioned-platform-private-instances.md); the manifest
wire contract itself is unchanged.

## Fields

| Field | Type | Required | Constraints |
| --- | --- | --- | --- |
| `name` | string | yes | Display name for the dashboard |
| `slug` | string | yes | `[a-z0-9-]{1,32}`; equals the directory name, the subdomain, the binary name, and the SQLite filename |
| `port` | integer | yes | Loopback port in `4101–4999` (validation rejects outside the range); unique across apps — a duplicate is warned and the later app dropped from the registry; assigned sequentially by `bespoke new`; never hand-picked |
| `icon` | string | yes | Iconify icon name, or a path to a PNG inside the app directory |
| `description` | string | yes | One line, shown on the dashboard |
| `package` | string | no | Go package providing the app's source, in any module — the platform's own opt-in apps or a third party's ([ADR-0031](../adr/0031-third-party-app-packages.md)); omitted for ordinary instance-local apps |
| `[[intents]]` | table array | no | Cross-app actions this app accepts — see [Intents](#intents-intents) below |

## Example

A local Notes app manifest:

```toml
name        = "Notes"
slug        = "notes"
port        = 4102
icon        = "notebook-pen"
description = "A searchable stream of short notes"

[[intents]]
name    = "add-note"
title   = "Add to Notes"
accepts = "text"
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
- A directory under `apps/` without `app.toml` is ignored; this permits the
  platform module to ship opt-in app packages. A present but invalid manifest
  is ignored and flagged as a warning on the dashboard.
- Reserved: port `4000` and the apex subdomain (platformd), and port `4001`
  (platformd's internal LLM gateway listener — never routed by Caddy).
- The runtime contract is only "HTTP on `port`, honor the auth header." The
  optional `package` is a Go build-source override; it does not change that
  contract ([ADR-0005](../adr/0005-process-per-app.md)).

## Installed apps (`package`)

An app whose manifest sets `package` has no Go source in the instance: the
directory holds `app.toml` and nothing else, and `dev`/`deploy` build the named
package instead of `./apps/<slug>`. This serves both the platform's opt-in apps
(Builder) and apps published by third parties
([ADR-0031](../adr/0031-third-party-app-packages.md)).

Requirements on the **instance**:

- The providing module must be pinned in the instance's `go.mod` with
  `go get -tool <module>`, so builds and `bespoke ui` can resolve it. A plain
  `go get` is not enough: no instance source imports an installed app (it is a
  `main` package, which Go forbids importing), so `go mod tidy` prunes the
  requirement. The `tool` directive is what survives — the same mechanism that
  pins the `bespoke` CLI itself.
- The directory name must be the slug the published app expects — its source
  names its own database and process.
- The port is the installing owner's to choose, subject to the usual range and
  uniqueness rules.

Requirements on the **published app**:

- Commit generated `*_templ.go`. Instances never run `templ generate` over the
  read-only module cache.
- Use Iconify `icon` names. Only `app.toml` is shipped to the app host, so a
  PNG icon inside a module cache directory would never arrive.
- Bundle chat skills with `go:embed` + `web.Skills` (already required by
  [ADR-0026](../adr/0026-app-bundled-chat-skills.md)) — they travel in the
  binary.
- Declare the minimum platform version in the module's own `go.mod`. MVS may
  raise the installing instance's platform version.

Installing an app runs its author's code as the instance owner, alongside every
other app's data. The platform vouches for nothing installed this way.

## App HTTP contract

Beyond the manifest, an app MUST/MAY serve these endpoints (all provided or
mounted by `pkg/web` for Go apps):

| Endpoint | Requirement | Purpose |
| --- | --- | --- |
| `GET /healthz` | MUST | Deploy gates and monitoring; 200 "ok" |
| `GET /_bespoke/*` | MUST (automatic) | Design-system assets, embedded |
| `GET /_card` | MAY | Per-user dashboard card fragment ([ADR-0017](../adr/0017-app-provided-dashboard-cards.md)); content-only HTML, cheap queries, no LLM calls; dashboard falls back to `description` when absent |
| `POST /_chat`, `/_chat/speak`, `/_chat/transcribe`, `GET /_chat/context` | MAY (via `web.EnableChat`, all four together) | In-app chat + TTS + mic input ([ADR-0015](../adr/0015-appshell-platform-chrome.md), [ADR-0021](../adr/0021-tools-agentic-chat-mcp.md)); context feeds the dashboard's all-apps chat ([ADR-0020](../adr/0020-dashboard-chat-aggregated-context.md)) |
| `GET`+`POST /_intents/<name>` | MUST for each declared `[[intents]]` (via `web.Intent`) | Cross-app intent confirm + execute ([ADR-0018](../adr/0018-cross-app-intents.md)) |
| `GET /_tools`, `POST /_tools/<name>` | MAY (via `web.Tool`) | User-scoped LLM actions: agentic chat + the platform MCP surface ([ADR-0021](../adr/0021-tools-agentic-chat-mcp.md)) |
| `GET /_search?q=` | MAY (via `web.Search`) | User-scoped search results feeding the dashboard search box + platform `search` tool ([ADR-0028](../adr/0028-dashboard-global-search-fan-out.md), [app-search.md](app-search.md)); cheap DB queries, no LLM calls |
| `GET /_live` | SHOULD (via `web.Live`) | Datastar SSE patching the app's live region on `web.Changed(login)` ([ADR-0022](../adr/0022-live-updates.md)); mutations MUST call `web.Changed` |

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

## References

- Rationale: [ADR-0005](../adr/0005-process-per-app.md),
  [ADR-0018](../adr/0018-cross-app-intents.md),
  [ADR-0027](../adr/0027-versioned-platform-private-instances.md)
- Context: [architecture](../design/architecture.md)
