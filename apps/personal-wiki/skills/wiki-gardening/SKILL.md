---
name: wiki-gardening
description: Periodic wiki maintenance — fill stub pages, tidy tags, strengthen links. Load when asked to review, clean up, or garden the wiki.
---

# Gardening the wiki

A healthy wiki has no long-lived empty stubs, consistent tags, and links
between related pages. Chat is a primary author here: use `create_page`
and `update_page` to apply fixes directly. Deleting is the owner's call —
propose it with the page link (`/page/<id>`) instead.

## Procedure

1. **Find stubs.** Stubs are pages auto-created by `[[links]]` with empty
   bodies. `get_page` on titles that look bare; a stub returns no body
   text. Fill each stub with `update_page` — even two good sentences and
   a couple of `[[links]]` beat an empty page. If you lack the knowledge
   to fill one, say so rather than padding it.
2. **Tidy tags.** Collect tags from `search_pages` results; merge
   near-duplicates ("recipe"/"recipes") by updating the affected pages to
   the canonical tag, and tag untagged pages (1–3 lowercase tags,
   reusing existing ones).
3. **Strengthen links.** Where a page mentions a concept another page
   covers without `[[linking]]` it, update the body to add the link.
   Follow the page-authoring skill's conventions for any body you touch.
4. **Report.** End with a short list of what changed — stubs filled,
   tags merged, links added — each with the page id, plus anything you
   deliberately left for the owner (deletes, judgment calls).

Keep gardening sessions small: a handful of pages per pass, fully done,
beats a sweep of half-edits.
