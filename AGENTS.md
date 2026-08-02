# Bespoke

A personal platform for one-off, just-for-me apps, designed to be built and
maintained by LLM agents. Start at [docs/README.md](docs/README.md); the
architecture lives in [docs/design/architecture.md](docs/design/architecture.md).

This file (`AGENTS.md`) is the CANONICAL agent instructions — `CLAUDE.md`,
`GEMINI.md`, and `.github/copilot-instructions.md` are symlinks to it, and
`.claude/skills` symlinks to `.agents/skills/`
([ADR-0013](docs/adr/0013-agent-portable-instruction-surface.md)). Edit only
the canonical paths; keep content tool-agnostic.

Platform invariants are specified in
[docs/design/agent-layer.md](docs/design/agent-layer.md) and graduate into
this file as the code enforcing them lands.

## Skills (follow these for common tasks)

Step-by-step procedures live in [.agents/skills/](.agents/skills/); follow
them rather than improvising, whichever agent you are:

- **Turning a thin app idea into a spec** (run this first for one-line
  requests) → [.agents/skills/design-app/SKILL.md](.agents/skills/design-app/SKILL.md)
- **Building a new app** → [.agents/skills/new-app/SKILL.md](.agents/skills/new-app/SKILL.md)
- **Adding/adjusting UI components or the theme** → [.agents/skills/new-component/SKILL.md](.agents/skills/new-component/SKILL.md)
- **Adapting a fresh fork for a new owner** → [MAKE-IT-YOUR-OWN.md](MAKE-IT-YOUR-OWN.md)

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
- Render user- or LLM-authored markdown with `ui.Markdown(text)` (GFM,
  `prose`-styled, raw HTML omitted by goldmark — never enable unsafe HTML);
  never store HTML or roll your own renderer.
- **Mobile-first is enforced
  ([ADR-0016](docs/adr/0016-mobile-first-ui-standard.md)):** every view must
  work at 375px with a coarse pointer. No hover-only affordances (add
  `pointer-coarse:opacity-100` + `focus-within:`); icon controls add
  `pointer-coarse:size-8`; overlays cap height in `dvh` and width with
  `max-w-[calc(100vw-2rem)]`; wide content scrolls in its own
  `overflow-x-auto` container, never the page; grids collapse to one column
  by default. The 16px-input rule in `design/input.css` is load-bearing —
  never remove or override it. A view that needs a mouse is a failed build.
- LLM inference only via `llm.New(slug)` → `Complete`/`CompleteJSON`/
  `Classify` ([design](docs/design/llm-gateway.md)); never call a model
  provider or the Copilot SDK from an app. Expect ~1.5s per call — design
  features accordingly. Local dev needs platformd running (`just dev`) and
  the `copilot` CLI authenticated. Tag user-facing completions with
  `llm.WithUser(user.Login)` so the gateway injects the user's brief
  (ADR-0019) — chat does this automatically; omit it for mechanical calls.
- The AppShell provides platform chrome automatically (ADR-0015): the app
  switcher needs nothing from you; in-app LLM chat is one call —
  `web.EnableChat(mux, slug, provider)` where provider returns the user's
  recent app data as text. Add chat to any app whose data invites questions.
  Chat panels include a speak toggle (local TTS, persisted, autoplay) for
  free — never build your own TTS path; `audio.New(slug).Speak` exists for
  non-chat uses.
- Give every app a live dashboard card: `web.DashboardCard(mux, provider)`
  serving `GET /_card` (ADR-0017) — a content-only fragment with the user's
  current state ("3 entries today"), cheap queries only, never LLM calls.
  Without one the dashboard falls back to the manifest description.
- **Cross-app intents (ADR-0018):** declare an `[[intents]]` in `app.toml`
  for anything other apps might feed this one (text → entry/task/etc.) and
  mount it with `web.Intent(mux, appTitle, def)`. The selection popover
  offers them everywhere automatically; for event-driven follow-ups use
  `ui.IntentsFrom(ctx)` (see todo's "Journal it?" banner).
- **Integration review is part of adding an app:** when a new app lands,
  reconsider the EXISTING apps — should any of them offer this app's
  intents after an event, or declare a new intent the new app would call?
  Wire the natural ones; note rejected ones in the app README's Non-goals.
- Voice input: `ui.VoiceButton("/your/endpoint")` in the view +
  `audio.New(slug).Transcribe` in the handler ([ADR-0014](docs/adr/0014-audio-service-transcription.md));
  never touch MediaRecorder or a speech backend directly. Currently
  stub-backed — transcriptions are placeholders until Lemonade is wired.
- Timestamps are stored UTC (`datetime('now')`) but ALWAYS convert to local
  time before showing users or feeding LLM contexts/prompts (see journal's
  `localStamp`) — raw UTC makes chat narrate evening entries as "after
  midnight". The platform chat prompt states the owner's local time.
- Before building a shared capability into an app, check the
  [internal services catalog](docs/design/internal-services.md) and follow
  its decision tree: `pkg/*` helper first, internal service on platformd's
  4001 plane only for cross-app state or heavy runtimes (ADR-0012). Update
  the catalog when you add one.
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
