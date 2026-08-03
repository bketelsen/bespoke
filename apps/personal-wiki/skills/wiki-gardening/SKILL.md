---
name: wiki-gardening
description: Periodic wiki maintenance — find stub pages to fill, tidy tags, strengthen links. Load when asked to review, clean up, or garden the wiki.
---

# Gardening the wiki

A healthy wiki has no long-lived empty stubs, consistent tags, and links
between related pages. Gardening is a review conversation: chat can CREATE
pages but cannot edit existing ones — for changes to existing pages,
propose the edit and link the owner to the page (`/page/<id>`, Edit button
there).

## Procedure

1. **Find stubs.** Stubs are pages auto-created by `[[links]]` with empty
   bodies. `get_page` on titles that look bare; a stub returns no body
   text. For each stub, either draft content and offer it to the owner, or
   note it as intentionally pending.
2. **Check tags.** Collect tags from `search_pages` results; flag
   near-duplicates ("recipe"/"recipes") and untagged pages. Propose a
   merged tag set — the owner applies it in the edit view.
3. **Strengthen links.** For pages that mention a concept another page
   covers without `[[linking]]` it, propose the added links. New pages you
   create during gardening should link back to the pages that prompted
   them.
4. **Report.** End with a short list: stubs filled or drafted, tag
   proposals, link proposals — each with the page id so the owner can jump
   straight to the edit view.

Keep gardening sessions small: a handful of pages per pass beats a wall of
proposals nobody applies.
