---
name: new-app
description: Build a new Bespoke app from a one-line description — scaffold with the bespoke CLI, model the schema, write handlers and templ views on pkg/ui, verify locally end to end. Use whenever asked to create/build a new app for this platform.
---

# Build a new Bespoke app

The goal is a one-shot build: description in, working styled app out, with
zero infrastructure code. Everything infrastructural already exists — if you
find yourself writing auth, CSS, port numbers, SQL plumbing, or deploy
config, stop: you're off the path (see AGENTS.md).

## Steps

0. **Thin prompt? Design first.** If the request is a bare noun or one
   sentence ("build me a journal") and the user hasn't said "just build it",
   run [design-app](../design-app/SKILL.md) first and build from its spec.
   If a spec exists, put it at `apps/<slug>/README.md` and treat it as the
   contract.
   Also plan the dashboard card (step 6a): what one glance should show.
1. **Scaffold.** `just new <slug>` (slug: `[a-z0-9-]{1,32}`). This assigns
   the port, writes the manifest, and generates a compiling app. Never pick
   ports or create `apps/<slug>/` by hand.
2. **Manifest.** Edit `apps/<slug>/app.toml`: display `name`, one-line
   `description` (shows on the dashboard), `icon` = a
   [Lucide](https://lucide.dev/icons) name (unknown names fall back to a
   generic icon — pick a real one).
3. **Schema.** Replace the comment in `migrations/0001_init.sql` with real
   tables. Applied migrations are immutable — later changes are
   `0002_*.sql` onward. Keep `login TEXT` columns for per-user data
   (identity is `auth.FromContext(ctx).Login`).
4. **Handlers.** Extend `main.go`'s `web.Run` registration: standard
   `net/http` mux patterns (`GET /{$}`, `POST /notes`, …). Identity ONLY via
   `auth.FromContext`; storage ONLY via the `db.Open` handle from the
   scaffold.
5. **Views.** templ files in `views/`, every page wrapped in
   `ui.AppShell(ui.ShellProps{Title, User})`, composing components from
   `pkg/ui/components/*` (card, table, badge, input, button, …). NO custom
   CSS, no style attributes; Tailwind utilities for layout only, theme
   tokens only. Missing component? Use the `new-component` skill — never
   improvise one inline.
6. **Shared capabilities.** Check `docs/design/internal-services.md` BEFORE
   building any capability into the app. LLM: `llm.New(slug)` →
   `Complete`/`CompleteJSON`/`Classify` (~1.5s per call — background it or
   design the UX for it; requires platformd running); render LLM or user
   markdown only with `ui.Markdown(text)`. Voice: `ui.VoiceButton(action)`
   in the view + `audio.New(slug).Transcribe` in the handler — real Whisper
   transcription, one line each (journal `/entries/voice` is the
   reference); `audio.Speak` exists for TTS beyond the chat panel's
   built-in toggle. If the app's data invites questions ("is X
   trending?"), add in-app chat: `web.EnableChat(mux, slug, provider)`
   with a provider returning the user's recent data as text (see
   apps/journal for the reference). The app switcher is automatic — never
   build navigation between apps. (`web.NewSSE` is the low-level escape
   hatch for custom streams; for keeping pages fresh use 6-live below.)
   6-tools. **Tools (ADR-0021)**: expose every meaningful action as
   `web.Tool(mux, def)` in a `tools.go` — user-scoped handler, JSON
   schema, honest description (destructive ones say "only on an explicit
   user request"). Register tools BEFORE `EnableChat` so chat sees them;
   they also join dashboard chat and MCP automatically. Respect the spec:
   an append-only app gets no update tool.
   6-skills. **Skills (ADR-0026, optional)**: when chat needs procedural
   knowledge ("how to write a good entry here"), bundle
   `skills/<name>/SKILL.md` files (same frontmatter as this file) and
   register `web.Skills(mux, fs)` before `EnableChat` — chat surfaces get
   a `load_skill` tool. Skills may only reference tools the app
   registers (apps/personal-wiki is the reference).
   6-live. **Live region (ADR-0022)**: render the dynamic part of the page
   as an id-stable fragment, mount `web.Live(mux, fragment)`, wrap with
   `data-init="@get('/_live')"`, and call `web.Changed(user.Login)`
   after EVERY mutation — handlers, tools, and intents (journal/todo are
   references). A page that goes stale after a chat action is a bug.
   6a. **Dashboard card**: `web.DashboardCard(mux, provider)` returning a
   small templ fragment of the user's live state (apps/journal `DashCard`
   is the reference) — cheap queries only, no LLM calls, no AppShell.
   6b. **Intents (ADR-0018)**: declare `[[intents]]` for anything other
   apps might feed this one + `web.Intent(mux, ...)`; then REVIEW EXISTING
   APPS for natural integrations both ways (event banners via
   `ui.IntentsFrom` — todo's "Journal it?" is the reference) and wire them
   in the same change.
7. **Regenerate.** After any `.templ` change: `just ui`, and commit the
   generated `*_templ.go` + `pkg/ui/assets/styles.css` alongside your
   sources.
8. **Verify — all of these, not a subset:**
   - `just check` passes (vet, tests, golangci-lint, `go mod tidy -diff`,
     CGO-free linux cross-compile — CI runs the identical recipe).
   - `just dev`, then: app responds at `http://localhost:<port>` and renders
     through AppShell; the dashboard at `http://localhost:4000` lists it;
     `curl http://localhost:<port>/healthz` says ok. Exercise each route you
     added (create/read at minimum) with curl and confirm the data persists.
   - **Mobile pass (ADR-0016):** walk every view mentally (or in devtools) at
     375px/coarse pointer — no hover-only controls (`pointer-coarse:`
     present on any hover-revealed element), no fixed widths that force page
     scroll, wide tables wrapped in `overflow-x-auto`, overlays capped with
     `dvh`. A view that needs a mouse is a failed build.
9. **Commit** sources + generated files together, message
   `<slug>: <what the app does>`.

## If something needed manual intervention

That's a framework bug, not an app problem. Note what blocked you in
`docs/plans/roadmap.md` (Phase 6 status) so the convention, scaffold, or
this skill gets fixed — then continue.
