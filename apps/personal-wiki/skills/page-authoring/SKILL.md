---
name: page-authoring
description: How to write a good page in this wiki — titles, [[links]], tags, structure. Load before creating pages or drafting content for one.
---

# Writing a wiki page

The goal is a densely linked reference wiki, not a pile of notes. When
creating a page (via `create_page`) or drafting content for the owner:

## Titles

- Noun phrases, singular, capitalized like a heading: "Sourdough Starter",
  "Caddy Reverse Proxy", "Trip Planning Checklist".
- The title is the link target — pick the phrase other pages would
  naturally use in a sentence. Prefer "SQLite Backup" over "How I back up
  my SQLite databases".
- Titles are unique per wiki; `create_page` fails on duplicates. If a page
  exists, say so and show what's there (`get_page`) instead of inventing a
  variant title.

## Linking

- Link liberally with `[[Title]]` anywhere in a body. Every concept that
  deserves its own page someday should be a link today — the wiki
  auto-creates an empty stub page for any `[[Title]]` that doesn't exist
  yet, and backlinks appear on the target page automatically.
- Before writing, `search_pages` for related pages and link to what
  already exists using their exact titles.

## Structure

- Open with one plain sentence saying what the page covers.
- Then short sections with `##` headings; lists over prose for reference
  material. Keep pages under a screen — split big topics into linked pages.
- Bodies are markdown (GFM). No HTML.

## Tags

- Lowercase, comma-free, few: 1–3 per page. Reuse existing tags —
  `search_pages` results show each page's tags; match them rather than
  minting near-duplicates ("recipe" vs "recipes").
