# Bookmarks

A quick-access dashboard of saved links, opened when Brian wants to jump straight to a site without hunting for it.

## Records
**Bookmark**
- id
- url (required)
- title (auto-fetched from page on add, editable)
- tags (optional, comma-separated or list)
- created_at

## Views
- GET / — Dashboard: reverse-chronological list of bookmarks (title + tag chips), tapping a bookmark opens the link directly in a new tab; tag chips filter the list in place (tap again to clear)
- GET /add — Paste URL, auto-fetch title, optional tags
- GET /bookmarks/:id/edit — Edit title/tags/URL, delete option

## Platform surfaces
- Dashboard card: shows most recent 3-5 bookmarks with titles, tap to open
- Chat tools: add_bookmark(url), list_bookmarks(tag?)
- Intents: "save this link", "show my bookmarks"

## Non-goals
- No folders or nested organization
- No sharing/export
- No detail/notes view — title + tags only

## Later
- Favicon display
- Pin/reorder favorites
- Broken-link detection
