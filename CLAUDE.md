# Bespoke

A personal platform for one-off, just-for-me apps, designed to be built and
maintained by LLM agents. Start at [docs/README.md](docs/README.md); the
architecture lives in [docs/design/architecture.md](docs/design/architecture.md).

Platform invariants are specified in
[docs/design/agent-layer.md](docs/design/agent-layer.md) and graduate into
this file as the code enforcing them lands.

## Code conventions (live — the code exists)

- An app is `apps/<slug>/`: `app.toml` manifest, `main.go` calling
  `web.Run(slug, register)`, handlers, `migrations/*.sql`. Nothing else. See
  [apps/hello](apps/hello/main.go) for the canonical shape.
- Identity only via `auth.FromContext` (handlers are already behind
  `auth.Middleware`); never read `Tailscale-User-*` headers directly, never
  add auth of any kind.
- Storage only via `db.Open(slug, migrations)` — SQLite, embedded migrations
  named `NNNN_description.sql`, applied in order. Driver stays
  modernc.org/sqlite; deploys cross-compile with `CGO_ENABLED=0`, so no cgo
  dependencies anywhere.
- Ports come from manifests ([spec](docs/specs/app-manifest.md)); never
  hardcode a listen address — `bespoke new` assigns them.
- Create apps with `just new <slug>`; run everything locally with `just dev`
  (platformd + every manifest app on `127.0.0.1`, fake identity via
  `BESPOKE_DEV_USER`, dashboard links point at localhost ports). New apps
  join `dev` automatically — the manifests are the registry.
- Deploy/operate only via the `bespoke` CLI ([spec](docs/specs/bespoke-cli.md);
  Justfile wraps it). Units, Caddy routes, and Litestream config are
  GENERATED into `dist/gen/` — never write them by hand.
- UI: pages are templ views composing `pkg/ui` — `ui.AppShell` wraps every
  page; vendored components live in `pkg/ui/components` and are NEVER
  hand-edited (`./tools/templui add <name>` to vendor more; run
  `scripts/setup-tools.sh` once first). The look lives only in
  `design/input.css`. After changing any `.templ` file or the theme, run
  `scripts/build-ui.sh` and COMMIT the generated `*_templ.go` and
  `pkg/ui/assets/styles.css` — builds and deploys must not need the UI
  toolchain.
- Live UI updates use `web.NewSSE` (Datastar); the AppShell already loads
  `datastar.js`.
- LLM inference only via `llm.New(slug)` → `Complete`/`CompleteJSON`
  ([design](docs/design/llm-gateway.md)); never call a model provider or the
  Copilot SDK from an app. Expect ~1.5s per call — design features
  accordingly. Local dev needs platformd running (`just dev`) and the
  `copilot` CLI authenticated.
- Run `just check` (vet + tests + the CGO-free linux cross-compile) before
  calling any change done.

## Documentation rules (enforced)

Docs live in `docs/` in four categories. **Every new doc starts from its
category's `TEMPLATE.md`** and follows its structure:

- `docs/adr/` — why we decided. Immutable once Accepted; reversals are new
  ADRs that mark the old one Superseded.
- `docs/design/` — how it fits together. Living; updated in place to match
  reality.
- `docs/specs/` — exact contracts. Change only alongside implementing code.
- `docs/plans/` — order of work. Phases with "Done when" outcomes.

### Cross-linking is mandatory

A doc without its required links is incomplete — do not finish a docs change
until they exist, in both directions:

- **ADR** → links every design doc/spec it shapes, and prior ADRs it builds on.
- **Design doc** → links the ADR(s) providing its rationale, the spec(s)
  pinning its contracts, and the roadmap phase that builds it.
- **Spec** → links its motivating ADR(s) and the design doc showing where it
  fits.
- **Plan** → every phase links the design docs/specs it implements; resolved
  open questions become ADRs.

When you touch a doc, verify its links still hold (targets exist, section
anchors valid) and add the back-links on the targets. Use relative paths.

### Housekeeping

- New doc ⇒ add a line to the index in [docs/README.md](docs/README.md).
- New significant decision ⇒ new ADR *first*, then update the affected design
  docs/specs in the same change.
- Convert relative dates ("next weekend") to absolute dates in all docs.
