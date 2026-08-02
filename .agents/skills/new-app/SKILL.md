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
   design the UX for it; requires platformd running). Live updates:
   `web.NewSSE` (Datastar is already loaded by AppShell). If the app's data
   invites questions ("is X trending?"), add in-app chat:
   `web.EnableChat(mux, slug, provider)` with a provider returning the
   user's recent data as text (see apps/journal for the reference). The app
   switcher is automatic — never build navigation between apps.
7. **Regenerate.** After any `.templ` change: `just ui`, and commit the
   generated `*_templ.go` + `pkg/ui/assets/styles.css` alongside your
   sources.
8. **Verify — all of these, not a subset:**
   - `just check` passes (vet, tests, CGO-free linux cross-compile).
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
