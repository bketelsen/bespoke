# Bespoke

Bespoke is my personal app platform: software that exists for exactly one
user — me. A journal that talks back. A todo list that knows what my week
looks like. Whatever I want next, described to an AI agent in a sentence or
two, and live on my own hardware a few minutes later, at its own URL, behind
my tailnet, styled like everything else I run.

The bet is that LLMs changed the economics of one-person software, and the
right response isn't more apps — it's a framework with opinions strong
enough that an agent can one-shot the next one. So Bespoke is mostly
conventions: one auth model (Tailscale identity at the edge — no login
screen will ever exist here), one storage pattern (SQLite file per app), one
design system, one deploy command, and a docs tree written for agents as
much as for me. In exchange, every app gets the expensive things for one
line of code each: chat grounded in its own data, voice in and out running
on my GPU (my words never leave the house), a live card on the dashboard,
and cross-app intents — highlight text in the journal and "Create Todo" just
appears.

This repo went from [an idea file](docs/design/vision.md) to a deployed
platform with eighteen architecture decision records and three running apps
in a single day — and two of those apps were built by describing them, not
by writing them. Steal the idea before you steal the code: the whole point
is that yours should be bespoke too.

## Start here

- **The docs:** [docs/README.md](docs/README.md) — every decision (ADRs),
  design doc, spec, and plan, cross-linked.
- **The architecture:** [docs/design/architecture.md](docs/design/architecture.md)
- **How agents build apps here:** [AGENTS.md](AGENTS.md) and
  [.agents/skills/](.agents/skills/)
- **What's next:** [docs/plans/roadmap.md](docs/plans/roadmap.md)
