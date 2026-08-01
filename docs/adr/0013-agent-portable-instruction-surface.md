# 0013 — Agent-portable instruction surface

- **Status:** Accepted
- **Date:** 2026-08-01

## Context

Bespoke is maintained by whichever coding agent has quota today — Claude
Code as primary, with Copilot CLI, Codex, Gemini CLI, and others as
fallbacks when allotments run out. Each tool reads its own instruction file
(`CLAUDE.md`, `.github/copilot-instructions.md`, `GEMINI.md`, `AGENTS.md`)
and its own skills location (`.claude/skills/`). Duplicating conventions per
tool guarantees drift, and drift is fatal to one-shot reliability
([ADR-0002](0002-optimize-for-one-shot-agent-reliability.md)).

## Decision

One canonical surface, tool paths as symlinks:

- **`AGENTS.md`** (the emerging cross-tool standard) is the real file
  holding all conventions. `CLAUDE.md`, `GEMINI.md`, and
  `.github/copilot-instructions.md` are symlinks to it.
- **`.agents/skills/`** is the real skills directory
  (`<skill>/SKILL.md` with YAML frontmatter). `.claude/skills` is a symlink
  to it.
- Content is written tool-agnostically: plain markdown, no tool-specific
  directives; anything only one tool understands stays out of the shared
  files.

## Consequences

- Conventions and skills are edited in exactly one place; every agent sees
  the same law.
- Symlinks are committed to git — fine on Linux/macOS (the platform's world);
  GitHub's web renderer may show link targets rather than content, an
  accepted cosmetic cost.
- The skill format is the lowest common denominator (frontmatter +
  markdown steps); agents without native skill support are pointed at the
  directory from AGENTS.md.

## Alternatives considered

- **Per-tool copies kept in sync by convention:** guaranteed drift. Rejected.
- **Claude-only skills:** wastes the fallback agents exactly when they're
  needed (quota exhaustion). Rejected.

## References

- Builds on: [ADR-0002](0002-optimize-for-one-shot-agent-reliability.md)
- Shapes: [design/agent-layer.md](../design/agent-layer.md), `AGENTS.md`,
  [.agents/skills/](../../.agents/skills/)
