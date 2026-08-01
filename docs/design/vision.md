# Vision

It's so easy now to build bespoke applications JUST FOR ME — one-off apps that
only I need or want; apps I don't intend to share. Bespoke is the foundational
framework that makes each next app nearly free.

The product is not any single app. It is the platform + conventions + agent
skills that make **"build me an app that X" a reliable one-shot prompt**
([ADR-0002](../adr/0002-optimize-for-one-shot-agent-reliability.md)). I'm not
building nine apps; I'm building the thing that makes the tenth app a prompt.

## Shape

- Everything lives on the tailnet behind Caddy at `*.bespoke.example.com`
  (domain TBD — plenty available).
- A dashboard at the apex lists every app (name, icon, description) from a
  per-app manifest.
- One shared framework (`pkg/*`), one design system, one deploy path, one
  storage pattern. Apps are small and boring by design.
- LLM inference rides my existing Copilot subscription.

See [architecture.md](architecture.md) for the concrete design.

## Prior art

- **[Smallweb](https://www.smallweb.run/)** — folder = app at a subdomain,
  zero-deploy, Deno-based. Closest existing project in spirit; differs in being
  TypeScript-centric with no shared services layer.
- **[Windmill](https://windmill.dev)** — scripts become APIs + auto-generated
  UIs. Solves internal-tools, not bespoke-frontends.
- **[PocketBase](https://pocketbase.io)** — single-binary Go BaaS, embeddable.
  Benched as an alternative shared backend
  ([ADR-0006](../adr/0006-library-first-shared-services.md)).
- **["My Personal Software Journey"](https://metedata.substack.com/p/016-my-personal-software-journey)**
  (metedata, 2026) — a lived-in version of this exact premise on a Mac mini:
  launcher app, one-folder-per-app monorepo, SQLite per app, unified design
  system, resident maintenance agent. Independent convergence on most of our
  choices; diverges on React/Docker/Cloudflare Access. Worth stealing: security
  guardrails the agent treats as law, generated app icons, resident cron agent.
