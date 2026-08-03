# Personal Wiki

Brian's single-user reference wiki for jotting and looking up notes, browsing by tag, and linking related pages.

## Records
**Page**
- id
- title (unique, text)
- body (markdown/free-form text)
- tags (list of strings)
- created_at
- updated_at
- last_viewed_at (nullable)

## Views
- GET / — List of all pages (recent/alphabetical toggle), search bar on top, "Recently viewed" section
- GET /page/:id — View page: rendered body with [[links]] clickable, tag chips, "Pages that link here" backlink list, Edit/Delete buttons
- GET /page/:id/edit — Edit title, body, tags
- GET /new — Create page (title + body + tags)
- GET /tag/:tag — Pages filtered by tag

## Platform surfaces
- Dashboard card: shows recently viewed pages count + quick search
- Chat tools: create_page(title, body, tags), update_page(title, new_title?, body?, tags?), search_pages(query), get_page(title) — the LLM is a primary author of this wiki, so writing tools cover create and edit; delete stays in the UI
- Intents: "note that...", "look up...", "add to my wiki"

## Behavior
- Typing `[[Title]]` in body auto-creates a blank stub page for that title if it doesn't exist; clicking it opens/edits the stub
- Viewing a page updates last_viewed_at (drives "recently viewed" resurfacing)
- Edit/delete anytime, permanent delete (no trash)

## Non-goals
- No real-time collaboration or sharing
- No version history
- No full graph/network visualization (only simple backlink list)

## Later
- Full-text search ranking/highlighting
- Tag autocomplete
- Voice capture for quick notes
