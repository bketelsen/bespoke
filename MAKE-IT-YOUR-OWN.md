---
name: make-it-your-own
description: Turn a fork of Brian's Bespoke into YOUR Bespoke — swap identity, apps, theme, and deployment for your own while keeping the platform. Point your coding agent at this file right after forking and it will interview you and do the work.
---

# Make it your own

You forked a *personal* platform — most of what's here is the platform
(keep it) wearing Brian's specifics (replace them). This skill walks an
agent through the swap. Agent: work top to bottom, one phase per commit,
`just check` between phases. Interview the new owner where marked — never
guess their domain, hosts, or taste.

**Keep untouched:** `pkg/*` (the framework), `pkg/ui/components` + `utils`
(vendored — never hand-edited), `cmd/bespoke`, `platform/`, the docs
templates, `AGENTS.md`'s rules, and the generated-artifacts discipline.
The platform invariants in AGENTS.md are the product — changing them means
you wanted a different project.

## 0. Prerequisites (verify, don't assume)

Go 1.26+, `just`, `rsync`, ssh; a Tailscale tailnet; a domain on Cloudflare
(any registrar works if you swap the caddy DNS module); one edge host
already running Caddy; one app host (systemd, linger-capable). Optional but
worth it: GitHub Copilot subscription + CLI (LLM features; everything
degrades gracefully without), a [Lemonade](https://lemonade-server.ai)
server (local voice/TTS; stub mode without).

## 1. Identity

- **ASK:** their GitHub handle/module path. Replace the module everywhere:
  `go.mod` + every import: `grep -rl github.com/bketelsen/bespoke --include='*.go' --include='*.templ'`,
  sed to `github.com/THEM/bespoke`, then `just ui && just check`.
- Point `git remote` at their repo.

## 2. Deployment config

- **ASK:** domain, edge host (ssh + tailscale IPv4), app host (ssh +
  tailscale IPv4 + arch), ssh username. Copy
  [deploy/deploy.env.example](deploy/deploy.env.example) to
  `deploy/deploy.env` (gitignored) and fill in their values.
- Replace the username (`bjk`) in both [deploy/sudoers/](deploy/sudoers/)
  files.
- Walk [deploy/README.md](deploy/README.md) with them: custom Caddy build
  (`just caddy-push`), Cloudflare token drop-in, `import` line, wildcard
  DNS to THEIR edge tailscale IP, the tailnet ACL, linger on the app host.
- If they have Lemonade: set `BESPOKE_LEMONADE_URL` in the app host's env
  file and check the model names in `platform/audio.go` defaults against
  `GET <lemonade>/models`.

## 3. Apps

- Brian's apps are worked examples. **ASK** which to keep as references.
  Remove with `go run ./cmd/bespoke rm <slug> --force` (journal and todo
  reference each other's intents — remove or keep them together, and if
  removing journal alone, delete todo's "Journal it" banner in
  `apps/todo/views/home.templ`).
- Keep `apps/hello` until their first own app proves the whole loop
  end-to-end, then optionally remove it.
- Their first real app: use the [design-app](.agents/skills/design-app/SKILL.md)
  interview, then [new-app](.agents/skills/new-app/SKILL.md).

## 4. Theme

The look lives ONLY in [design/input.css](design/input.css) (oklch tokens,
light + dark). **ASK** about their taste — reference colors, warm/cool,
radius, density — then rewrite the token values (never the structure, never
component files), run `scripts/setup-tools.sh` once, `just ui`, and show
them screenshots. Iterate until it's theirs; the whole platform restyles
from this one file.

## 5. Docs

- Rewrite [README.md](README.md)'s pitch and
  [docs/design/vision.md](docs/design/vision.md) in their voice — Brian's
  are personal.
- [docs/plans/roadmap.md](docs/plans/roadmap.md): reset the status notes
  and one-shot log; their history starts now.
- **Leave the ADRs** (0001–0018): they explain why the code is shaped this
  way. They're inherited history — immutable as always; new decisions get
  new numbers. Add ADR-0019 "Forked from bketelsen/bespoke" if they want a
  clean marker.

## 6. Verify it's theirs

- `just check` passes; `just dev` up; dashboard shows THEIR identity via
  `BESPOKE_DEV_USER`, THEIR theme, THEIR apps.
- Deploy per the runbook; done when `https://hello.<their-domain>` renders
  THEIR Tailscale name — the same done-when this repo started with.
