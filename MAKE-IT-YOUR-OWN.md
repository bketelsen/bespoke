---
name: make-it-your-own
description: Turn a fork of Brian's Bespoke into YOUR Bespoke — swap identity, apps, theme, and deployment for your own while keeping the platform. Point your coding agent at this file right after forking and it will interview you and do the work.
---

# Make it your own

You forked a *personal* platform — most of what's here is the platform
(keep it) wearing Brian's specifics (replace them). This skill walks an
agent through the swap. Agent: work top to bottom, one phase per commit,
`just check` between phases. Interview the new owner where marked — never
guess their domain, hosts, or taste. (All paths in this file are relative
to the **repo root** — the canonical file is `MAKE-IT-YOUR-OWN.md` there;
`.agents/skills/make-it-your-own/SKILL.md` is a symlink to it.)

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
server (speech both ways: real Whisper transcription in, kokoro TTS out —
without it, voice input degrades to clearly-marked stub transcriptions and
TTS is simply unavailable).

## 1. Identity

- **ASK:** their GitHub handle/module path. Replace the module everywhere:
  `go.mod` + every import: `grep -rl github.com/bketelsen/bespoke --include='*.go' --include='*.templ'`,
  sed to `github.com/THEM/bespoke`, then `just ui && just check`.
- Non-Go references the grep misses — fix each: `.templui.json`
  (`moduleName` — stale, it silently breaks the imports `templui add`
  writes), `README.md` (CI badge URL), `SECURITY.md` (advisory URL),
  `.github/ISSUE_TEMPLATE/config.yml` (MAKE-IT-YOUR-OWN link).
- Point `git remote` at their repo.

## 2. Deployment config

- **ASK:** domain, edge host (ssh + tailscale IPv4), app host (ssh +
  tailscale IPv4 + arch), ssh username. Copy
  [deploy/deploy.env.example](deploy/deploy.env.example) to
  `deploy/deploy.env` (gitignored) and fill in their values.
- Replace the username (`bjk`) in both [deploy/sudoers/](deploy/sudoers/)
  files (it also appears in [deploy/README.md](deploy/README.md)'s prose).
- Walk [deploy/README.md](deploy/README.md) with them: custom Caddy build
  (`just caddy-push`), Cloudflare token drop-in, `import` line, wildcard
  DNS to THEIR edge tailscale IP, the tailnet ACL, linger on the app host.
- If they have Lemonade: set `BESPOKE_LEMONADE_URL` in the app host's env
  file and check the model names in `platform/audio.go` defaults against
  `GET <lemonade>/models`.

## 3. Inference provider

Bespoke runs LLM inference on **GitHub Copilot out of convenience, not
conviction** — it's simply what Brian already had (ADR-0009). Nothing else
in the platform knows that: apps speak only `pkg/llm`
(`Complete`/`CompleteJSON`/`Classify`, `WithUser`, `WithTools`), which
talks to one gateway endpoint on platformd. The provider lives in exactly
one file — `platform/llm.go` — behind that interface.

- **ASK:** what do they already pay for or run? Copilot subscription →
  keep as-is (just install + sign in the `copilot` CLI on the app host).
  Anthropic/OpenAI API keys → reimplement the gateway's `complete()`
  against that API. Local hardware → any OpenAI-compatible server works
  (Ollama, Lemonade, llama.cpp server); note a Lemonade install already
  slots in for voice, and its chat models can serve text too.
- Requirements to preserve: the brief injection (ADR-0019) is plain
  system-prompt text — any provider handles it; **agentic chat and MCP
  (ADR-0021) need a model + API with tool calling**, so check that before
  choosing a local model.
- Whatever they pick, zero app code changes — that seam is the point.
  Record their choice as an ADR superseding/refining 0009.

## 4. Apps

- Brian's apps are worked examples. **ASK** which to keep as references.
  Remove with `go run ./cmd/bespoke rm <slug> --force` (journal and todo
  reference each other's intents — remove or keep them together, and if
  removing journal alone, delete todo's "Journal it" banner in
  `apps/todo/views/home.templ`).
- Keep `apps/hello` until their first own app proves the whole loop
  end-to-end, then optionally remove it.
- Their first real app: use the [design-app](.agents/skills/design-app/SKILL.md)
  interview, then [new-app](.agents/skills/new-app/SKILL.md).

## 5. Theme

The look lives ONLY in [design/input.css](design/input.css) (oklch tokens,
light + dark). **ASK** about their taste — reference colors, warm/cool,
radius, density — then rewrite the token values (never the structure, never
component files), run `scripts/setup-tools.sh` once, `just ui`, and show
them screenshots. Iterate until it's theirs; the whole platform restyles
from this one file.

## 6. Docs

- Rewrite [README.md](README.md)'s pitch and
  [docs/design/vision.md](docs/design/vision.md) in their voice — Brian's
  are personal.
- The community files are Brian-specific too: [CONTRIBUTING.md](CONTRIBUTING.md)
  ("the apps in this repo are mine"), [SECURITY.md](SECURITY.md) (advisory
  URL), [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) (contact email), and the
  issue templates under `.github/ISSUE_TEMPLATE/`. Rewrite or remove —
  don't ship a fork that routes security reports to Brian.
- [docs/plans/roadmap.md](docs/plans/roadmap.md): reset the status notes
  and one-shot log; their history starts now.
- **Leave the ADRs**: they explain why the code is shaped this
  way. They're inherited history — immutable as always; new decisions get
  the next numbers. An ADR "Forked from bketelsen/bespoke" makes a clean
  first marker (and the inference-provider choice from §3 belongs in one).

## 7. Verify it's theirs

- `just check` passes locally AND CI is green on their remote (the
  inherited workflow runs the identical recipe — a red fork badge means
  the swap isn't done).
- `just dev` up; dashboard shows THEIR identity via `BESPOKE_DEV_USER`,
  THEIR theme, THEIR apps.
- Deploy per the runbook; done when `https://hello.<their-domain>` renders
  THEIR Tailscale name — the same done-when this repo started with.
