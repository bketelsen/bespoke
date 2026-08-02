---
name: design-app
description: Turn a thin app idea ("build me a journal") into a half-page spec through a short, focused interview — then hand off to new-app. Use BEFORE building whenever the request is a bare noun or one sentence; skip when the user already provided specifics or explicitly says "just build it".
---

# Design an app before building it

A one-line prompt hides a dozen decisions. Don't silently guess them and
don't interrogate exhaustively — run a short interview, then write the spec.
Budget: **5–8 questions, then stop.** If the user says "just build it" at
any point, write the spec from what you have and move on.

## Interview rules

- **One question per message.** Never a questionnaire.
- **Offer 2–4 concrete options with a recommendation** whenever possible —
  picking beats composing. Open-ended questions only where options would be
  fake (e.g. "walk me through the moment you'd open this app").
- Anchor in a real usage moment, not features: "It's Tuesday 9pm, you open
  it — what do you want to see first?"
- Challenge scope once: "What's the smallest version you'd actually use this
  week?" Everything else goes to a Later list, not the build.
- Skip questions the idea already answers. Stop early when you can write
  every spec section below without guessing.

## Angles to cover (pick what's undecided, in roughly this order)

1. **The moment of use** — when/where/why it gets opened; capture-heavy vs
   review-heavy.
2. **The record** — what one entry is: fields, size, structure. This drives
   the schema.
3. **The first screen** — what renders at `GET /`; what the 2–3 views are.
4. **Leverage** — anything from the
   [internal services catalog](../../../docs/design/internal-services.md)?
   (LLM summarize/classify at ~1.5s/call, future audio/voice input,
   privacy-sensitive → note the Lemonade/local candidate.) Don't invent
   service needs the user didn't imply.
5. **Lifecycle** — edit/delete? retention? does old data ever resurface
   (weekly summary, on-this-day)?
6. **Integrations** — check other apps' `[[intents]]` and this app's
   events: what text/objects should flow between apps? ("completing X
   offers journaling it"). One question, options from the actual registry.
7. **Non-goals** — name 2–3 things it deliberately won't do (sharing,
   mobile app, export…), confirmed with the user.
7. **Identity** — display name, slug, Lucide icon, one-line dashboard
   description.

## Output: the spec

Write `apps/<slug>/README.md` (after `just new <slug>`, or hand the text to
new-app step 1 if building immediately):

```markdown
# <Name>

<One sentence: who opens it when, to do what.>

## Records
<Table or bullets: entities and their fields.>

## Views
- `GET /` — <first screen>
- <other routes>

## Services
<pkg/llm, future audio, or "none". Note latency-driven UX choices.>

## Non-goals
<The 2–3 confirmed exclusions.>

## Later
<Deferred ideas from the interview — parked, not promised.>
```

Keep it under a page. It is the contract for the build and the anchor for
every future change ("check the app's README before modifying it").

## Hand-off

Confirm the spec with the user in one message ("building this — object
now"), then follow [new-app](../new-app/SKILL.md). Deviations discovered
during the build update the README in the same commit.
