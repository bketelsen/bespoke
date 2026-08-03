# Global Search Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a dashboard search box and a platform `search` MCP/chat tool that query every registered app's user-scoped data over HTTP fan-out, grouped by app, with best-effort deep links.

**Architecture:** A new optional app-contract endpoint `GET /_search?q=`, registered via `web.Search(mux, provider)`. platformd fans out to every app's endpoint in parallel (mirroring `platform/cards.go`), forwards the caller's identity, and groups results by app for both a dashboard results page and a platform-owned `search` tool exposed to dashboard chat and `/mcp`. No central index. Apps own their own SQLite queries and user scoping.

**Tech Stack:** Go 1.x, `net/http`, `modernc.org/sqlite` (CGO-free), templ views on `pkg/ui`, `github.com/modelcontextprotocol/go-sdk/mcp`.

## Global Constraints

- Module path is `github.com/bketelsen/bespoke`.
- Driver stays `modernc.org/sqlite`; everything must cross-compile with `CGO_ENABLED=0` (no cgo). No new dependencies.
- Identity only via `auth.FromContext`; never read `Tailscale-User-*` headers directly inside an app handler. platformd (fan-out code) forwards them, matching `platform/cards.go`.
- All app data is user-scoped by convention (`WHERE login=?`); every search query MUST filter by the caller's login.
- Search providers are cheap DB queries only — never LLM calls.
- Deep links are preferred/best-effort: return a specific item URL when available; a bare `/` home URL is an acceptable fallback.
- The `_`-prefixed path namespace is reserved for the platform.
- After changing any `.templ` file, run `just ui` and commit the generated `*_templ.go` and the compiled stylesheet.
- Run `just check` (vet + tests + golangci-lint + `go mod tidy -diff` + CGO-free linux cross-compile) before considering any change done.
- Contract JSON response shape (defined in Task 1, consumed everywhere): `{"results":[{"title":"...","snippet":"...","url":"/path","timestamp":"..."}]}`. Only `title` is required.

---

### Task 1: `web.Search` helper and result type

**Files:**
- Create: `pkg/web/search.go`
- Test: `pkg/web/search_test.go`

**Interfaces:**
- Consumes: `auth.User`, `auth.FromContext`, `auth.Middleware` (existing).
- Produces:
  - `type SearchResult struct { Title string \`json:"title"\`; Snippet string \`json:"snippet,omitempty"\`; URL string \`json:"url,omitempty"\`; Timestamp string \`json:"timestamp,omitempty"\` }`
  - `type SearchProvider func(ctx context.Context, user auth.User, q string) ([]SearchResult, error)`
  - `func Search(mux *http.ServeMux, provider SearchProvider)` — mounts `GET /_search`, reads `q` from the query string, calls the provider with `auth.FromContext`, and writes `{"results":[...]}` as JSON. A nil/empty slice serializes as `{"results":[]}`.

- [ ] **Step 1: Write the failing test**

```go
package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bketelsen/bespoke/pkg/auth"
)

func TestSearchEndpoint(t *testing.T) {
	inner := http.NewServeMux()
	Search(inner, func(ctx context.Context, user auth.User, q string) ([]SearchResult, error) {
		if user.Login != "test@example" {
			t.Errorf("provider got login %q", user.Login)
		}
		if q != "milk" {
			t.Errorf("provider got q %q", q)
		}
		return []SearchResult{{Title: "Buy milk", URL: "/task/1"}}, nil
	})
	mux := auth.Middleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/_search?q=milk", nil)
	req.Header.Set("Tailscale-User-Login", "test@example")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var got struct {
		Results []SearchResult `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("bad json: %v (%s)", err, rec.Body.String())
	}
	if len(got.Results) != 1 || got.Results[0].Title != "Buy milk" || got.Results[0].URL != "/task/1" {
		t.Fatalf("unexpected results: %+v", got.Results)
	}
}

func TestSearchEndpointEmpty(t *testing.T) {
	inner := http.NewServeMux()
	Search(inner, func(ctx context.Context, user auth.User, q string) ([]SearchResult, error) {
		return nil, nil
	})
	mux := auth.Middleware(inner)
	req := httptest.NewRequest(http.MethodGet, "/_search?q=x", nil)
	req.Header.Set("Tailscale-User-Login", "test@example")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if body := rec.Body.String(); body != "{\"results\":[]}\n" {
		t.Fatalf("empty search body = %q", body)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/web/ -run TestSearchEndpoint -v`
Expected: FAIL — `undefined: Search` / `undefined: SearchResult`.

- [ ] **Step 3: Write minimal implementation**

```go
// Package web: app-contract search endpoint (ADR-0028). GET /_search?q=
// returns the caller's user-scoped matches; platformd fans out to every
// app and groups the results.
package web

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/bketelsen/bespoke/pkg/auth"
)

// SearchResult is one hit an app returns for a query. Only Title is
// required; URL is app-relative and best-effort deep (a bare "/" home link
// is acceptable). See docs/specs/app-search.md.
type SearchResult struct {
	Title     string `json:"title"`
	Snippet   string `json:"snippet,omitempty"`
	URL       string `json:"url,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

// SearchProvider returns the user's results matching q. It MUST scope to the
// user and MUST be a cheap DB query — never an LLM call (platformd waits
// behind a short timeout).
type SearchProvider func(ctx context.Context, user auth.User, q string) ([]SearchResult, error)

// Search mounts GET /_search, the optional app-contract endpoint the
// dashboard search box and the platform `search` tool fan out to. Call
// inside web.Run's register.
func Search(mux *http.ServeMux, provider SearchProvider) {
	mux.HandleFunc("GET /_search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		results, err := provider(r.Context(), auth.FromContext(r.Context()), q)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if results == nil {
			results = []SearchResult{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"results": results})
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/web/ -run TestSearchEndpoint -v`
Expected: PASS (both `TestSearchEndpoint` and `TestSearchEndpointEmpty`).

- [ ] **Step 5: Commit**

```bash
git add pkg/web/search.go pkg/web/search_test.go
git commit -m "feat(web): add web.Search app-contract search endpoint (ADR-0028)"
```

---

### Task 2: platformd fan-out aggregator

**Files:**
- Create: `platform/search.go`
- Test: `platform/search_test.go`

**Interfaces:**
- Consumes: `manifest.App` (fields `Slug`, `Name`, `Port`), `web.SearchResult` (Task 1), `contextClient` (existing `*http.Client` in `platform/cards.go`).
- Produces:
  - `type SearchGroup struct { Slug, Name string; Results []web.SearchResult }`
  - `func aggregateSearch(ctx context.Context, login, name, q string, apps []manifest.App) []SearchGroup` — parallel GET each app's `/_search?q=`, identity forwarded, 900ms timeout via a package client, 32KB cap, skip misses. Returns groups only for apps with ≥1 result, in the input app order.

Note: use a dedicated `var searchClient = &http.Client{Timeout: 900 * time.Millisecond}` in `platform/search.go` (do not reuse `cardClient`, to keep timeouts independently tunable). URL resolution to app base URLs happens in the view/tool (Tasks 4/5), not here — this returns app-relative URLs untouched plus the slug/name needed to resolve them.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/bketelsen/bespoke/internal/manifest"
	"github.com/bketelsen/bespoke/pkg/web"
)

// startSearchApp spins a fake app server returning the given results for any
// query, and returns a manifest.App pointed at it.
func startSearchApp(t *testing.T, slug, name string, results []web.SearchResult) manifest.App {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_search" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Tailscale-User-Login") == "" {
			t.Error("identity not forwarded")
		}
		json.NewEncoder(w).Encode(map[string]any{"results": results})
	}))
	t.Cleanup(srv.Close)
	// httptest servers listen on 127.0.0.1:<port>; extract the port.
	port, _ := strconv.Atoi(srv.URL[strings.LastIndex(srv.URL, ":")+1:])
	return manifest.App{Slug: slug, Name: name, Port: port}
}

func TestAggregateSearch(t *testing.T) {
	t.Setenv("BESPOKE_BIND_IP", "127.0.0.1")
	notes := startSearchApp(t, "notes", "Notes", []web.SearchResult{{Title: "milk note", URL: "/#note-3"}})
	todo := startSearchApp(t, "todo", "Todo", []web.SearchResult{{Title: "buy milk", URL: "/task/7"}})
	empty := startSearchApp(t, "empty", "Empty", nil)

	groups := aggregateSearch(context.Background(), "me@x", "Me", "milk",
		[]manifest.App{notes, todo, empty})

	if len(groups) != 2 {
		t.Fatalf("want 2 groups, got %d: %+v", len(groups), groups)
	}
	if groups[0].Slug != "notes" || groups[1].Slug != "todo" {
		t.Fatalf("group order/slug wrong: %+v", groups)
	}
	if groups[0].Results[0].Title != "milk note" {
		t.Fatalf("notes result wrong: %+v", groups[0].Results)
	}
}
```

Note: the fake server ignores `BESPOKE_BIND_IP` since `aggregateSearch` builds `http://<host>:<port>/_search`; because httptest binds `127.0.0.1` and the default host is `127.0.0.1`, the port match reaches the fake server. Keep the `t.Setenv` line so the host is deterministic.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./platform/ -run TestAggregateSearch -v`
Expected: FAIL — `undefined: aggregateSearch`.

- [ ] **Step 3: Write minimal implementation**

```go
// Global search fan-out (ADR-0028): query each app's /_search in parallel
// with the caller's identity forwarded, group results by app. Any miss
// (absent endpoint, error, slow, oversized) is simply skipped — a broken
// app can't break search. Mirrors cards.go.
package main

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/bketelsen/bespoke/internal/manifest"
	"github.com/bketelsen/bespoke/pkg/web"
)

var searchClient = &http.Client{Timeout: 900 * time.Millisecond}

// SearchGroup is one app's hits for a query, in the dashboard's grouped view.
type SearchGroup struct {
	Slug    string
	Name    string
	Results []web.SearchResult
}

// aggregateSearch fans out q to every app's /_search, forwarding identity.
// Groups are returned only for apps that produced at least one result, in
// the input order (fan-out yields no cross-app ranking).
func aggregateSearch(ctx context.Context, login, name, q string, apps []manifest.App) []SearchGroup {
	host := cmp.Or(os.Getenv("BESPOKE_BIND_IP"), "127.0.0.1")

	type slot struct {
		group SearchGroup
		ok    bool
	}
	slots := make([]slot, len(apps))
	var wg sync.WaitGroup
	for i, app := range apps {
		wg.Add(1)
		go func(i int, app manifest.App) {
			defer wg.Done()
			u := fmt.Sprintf("http://%s:%d/_search?q=%s", host, app.Port, url.QueryEscape(q))
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
			if err != nil {
				return
			}
			req.Header.Set("Tailscale-User-Login", login)
			req.Header.Set("Tailscale-User-Name", name)
			resp, err := searchClient.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return
			}
			var payload struct {
				Results []web.SearchResult `json:"results"`
			}
			if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<10)).Decode(&payload); err != nil {
				return
			}
			if len(payload.Results) == 0 {
				return
			}
			slots[i] = slot{SearchGroup{Slug: app.Slug, Name: app.Name, Results: payload.Results}, true}
		}(i, app)
	}
	wg.Wait()

	var groups []SearchGroup
	for _, s := range slots {
		if s.ok {
			groups = append(groups, s.group)
		}
	}
	return groups
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./platform/ -run TestAggregateSearch -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add platform/search.go platform/search_test.go
git commit -m "feat(platform): fan-out search aggregator over app /_search (ADR-0028)"
```

---

### Task 3: Notes search endpoint with deep-link anchors

**Files:**
- Modify: `apps/notes/main.go` (add `web.Search` registration; extend `loadNotes` to select `id`)
- Modify: `apps/notes/views/home.templ` (add `ID` to `Note`, render `id="note-<id>"` anchor)
- Test: `apps/notes/search_test.go`

**Interfaces:**
- Consumes: `web.Search`, `web.SearchResult` (Task 1), existing `sqldb`.
- Produces: an app-level `searchNotes(ctx, sqldb, login, q string) ([]web.SearchResult, error)` returning notes whose `body` matches `q` (substring, case-insensitive), scoped by login, `url = "/#note-<id>"`, `title` = first line, `snippet` = body.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"context"
	"io/fs"
	"testing"

	"github.com/bketelsen/bespoke/pkg/db"
)

func TestSearchNotes(t *testing.T) {
	t.Setenv("BESPOKE_DATA", t.TempDir())
	migrations, _ := fs.Sub(migrationFS, "migrations")
	sqldb, err := db.Open("notes", migrations)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := addNote(ctx, sqldb, "me@x", "buy milk and bread"); err != nil {
		t.Fatal(err)
	}
	if err := addNote(ctx, sqldb, "me@x", "call the dentist"); err != nil {
		t.Fatal(err)
	}
	if err := addNote(ctx, sqldb, "other@x", "milk for other user"); err != nil {
		t.Fatal(err)
	}

	got, err := searchNotes(ctx, sqldb, "me@x", "MILK")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 hit, got %d: %+v", len(got), got)
	}
	if got[0].Title != "buy milk and bread" {
		t.Errorf("title = %q", got[0].Title)
	}
	if got[0].URL == "" || got[0].URL == "/" {
		t.Errorf("want deep-link url, got %q", got[0].URL)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./apps/notes/ -run TestSearchNotes -v`
Expected: FAIL — `undefined: searchNotes`.

- [ ] **Step 3: Add `searchNotes` and register it**

Add to `apps/notes/main.go` (import `"fmt"`, `"strings"` already present; add `"github.com/bketelsen/bespoke/pkg/web"` already imported):

```go
// searchNotes returns the user's notes whose body contains q (case-
// insensitive substring), newest first, deep-linked to the note anchor.
func searchNotes(ctx context.Context, sqldb *sql.DB, login, q string) ([]web.SearchResult, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	rows, err := sqldb.QueryContext(ctx,
		"SELECT id, body FROM notes WHERE login=? AND body LIKE '%'||?||'%' COLLATE NOCASE ORDER BY id DESC LIMIT 20",
		login, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []web.SearchResult
	for rows.Next() {
		var id int64
		var body string
		if err := rows.Scan(&id, &body); err != nil {
			return nil, err
		}
		title := body
		if i := strings.IndexByte(title, '\n'); i >= 0 {
			title = title[:i]
		}
		out = append(out, web.SearchResult{
			Title:   title,
			Snippet: body,
			URL:     fmt.Sprintf("/#note-%d", id),
		})
	}
	return out, rows.Err()
}
```

Register it inside `web.Run("notes", ...)` register func, next to `DashboardCard`:

```go
		web.Search(mux, func(ctx context.Context, user auth.User, q string) ([]web.SearchResult, error) {
			return searchNotes(ctx, sqldb, user.Login, q)
		})
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./apps/notes/ -run TestSearchNotes -v`
Expected: PASS.

- [ ] **Step 5: Add the deep-link anchor to the view**

In `apps/notes/views/home.templ`, extend the `Note` type and render the anchor id. Replace the type line:

```go
type Note struct { ID int64; Body, CreatedAt string }
```

In `NotesLive`, wrap each note card with an anchor id (change the `for` body):

```go
		for _, n := range notes {
			<div id={ fmt.Sprintf("note-%d", n.ID) }>
				@card.Card() {
					@card.Content() {
						<div class="select-text pt-6">@ui.Markdown(n.Body)</div>
						<p class="mt-3 text-xs text-muted-foreground">{ n.CreatedAt }</p>
					}
				}
			</div>
		}
```

Add `"fmt"` to the import block in `home.templ`.

- [ ] **Step 6: Populate `Note.ID` in `loadNotes`**

In `apps/notes/main.go`, change the `loadNotes` query and scan to include `id`:

```go
	rows, err := sqldb.QueryContext(ctx, "SELECT id, body, created_at FROM notes WHERE login=? ORDER BY id DESC LIMIT ?", login, limit)
```

```go
		if err := rows.Scan(&n.ID, &n.Body, &n.CreatedAt); err != nil {
			return nil, err
		}
```

- [ ] **Step 7: Regenerate templ and build**

Run: `just ui && go build ./apps/notes/...`
Expected: no errors.

- [ ] **Step 8: Run the app's tests**

Run: `go test ./apps/notes/...`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add apps/notes/
git commit -m "feat(notes): /_search endpoint with per-note deep-link anchors (ADR-0028)"
```

---

### Task 4: Todo search endpoint with per-task deep link

**Files:**
- Modify: `apps/todo/main.go` (add `searchTasks`, register `web.Search`, add `GET /tasks/{id}` deep-link route)
- Test: `apps/todo/search_test.go`

**Interfaces:**
- Consumes: `web.Search`, `web.SearchResult` (Task 1), existing `sqldb`, `dueLabel`.
- Produces: `searchTasks(ctx, sqldb, login, q string) ([]web.SearchResult, error)` — tasks whose `description` matches `q`, scoped by login, `url = "/tasks/<id>"` (a real route added in this task), `title` = description, `snippet` = state + due + priority.

Note: Todo already has `/tasks/{id}/edit`. Add a plain `GET /tasks/{id}` that renders the home page scrolled/highlighted to that task is heavier than needed; instead point the deep link at the existing edit route target is wrong (edit is a form). Simplest honest deep link: add `id="task-<id>"` anchors to the task list (like notes) and link to `/#task-<id>`. Do that rather than a new route — it is a real deep link, lower risk, and matches the notes exemplar.

Revised: `url = "/#task-<id>"`; add the anchor id in the todo list view.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"context"
	"io/fs"
	"testing"

	"github.com/bketelsen/bespoke/pkg/db"
)

func TestSearchTasks(t *testing.T) {
	t.Setenv("BESPOKE_DATA", t.TempDir())
	migrations, _ := fs.Sub(migrationFS, "migrations")
	sqldb, err := db.Open("todo", migrations)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := sqldb.ExecContext(ctx, "INSERT INTO tasks (login, description) VALUES (?, ?)", "me@x", "buy milk"); err != nil {
		t.Fatal(err)
	}
	if _, err := sqldb.ExecContext(ctx, "INSERT INTO tasks (login, description) VALUES (?, ?)", "me@x", "walk dog"); err != nil {
		t.Fatal(err)
	}
	if _, err := sqldb.ExecContext(ctx, "INSERT INTO tasks (login, description) VALUES (?, ?)", "other@x", "milk run"); err != nil {
		t.Fatal(err)
	}

	got, err := searchTasks(ctx, sqldb, "me@x", "milk")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 hit, got %d: %+v", len(got), got)
	}
	if got[0].Title != "buy milk" {
		t.Errorf("title = %q", got[0].Title)
	}
	if got[0].URL == "" || got[0].URL == "/" {
		t.Errorf("want deep-link url, got %q", got[0].URL)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./apps/todo/ -run TestSearchTasks -v`
Expected: FAIL — `undefined: searchTasks`.

- [ ] **Step 3: Add `searchTasks` and register it**

Add to `apps/todo/main.go`:

```go
// searchTasks returns the user's tasks whose description contains q (case-
// insensitive substring), deep-linked to the task anchor.
func searchTasks(ctx context.Context, sqldb *sql.DB, login, q string) ([]web.SearchResult, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	rows, err := sqldb.QueryContext(ctx, `
		SELECT id, description, COALESCE(due,''), priority, done
		FROM tasks WHERE login=? AND description LIKE '%'||?||'%' COLLATE NOCASE
		ORDER BY done, id DESC LIMIT 20`, login, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []web.SearchResult
	for rows.Next() {
		var id int64
		var desc, due, prio string
		var done int
		if err := rows.Scan(&id, &desc, &due, &prio, &done); err != nil {
			return nil, err
		}
		state := "open"
		if done == 1 {
			state = "done"
		}
		snippet := fmt.Sprintf("%s · priority %s", state, prio)
		if due != "" {
			snippet += " · due " + due
		}
		out = append(out, web.SearchResult{
			Title:   desc,
			Snippet: snippet,
			URL:     fmt.Sprintf("/#task-%d", id),
		})
	}
	return out, rows.Err()
}
```

Register inside `web.Run("todo", ...)`, next to `DashboardCard`:

```go
		web.Search(mux, func(ctx context.Context, user auth.User, q string) ([]web.SearchResult, error) {
			return searchTasks(ctx, sqldb, user.Login, q)
		})
```

`"fmt"`, `"strings"`, `"database/sql"`, and `web` are already imported in `apps/todo/main.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./apps/todo/ -run TestSearchTasks -v`
Expected: PASS.

- [ ] **Step 5: Add `task-<id>` anchors to the task list view**

Inspect `apps/todo/views/home.templ` (and any `TasksLive` fragment) and wrap each top-level task row with `id={ fmt.Sprintf("task-%d", t.ID) }` (add `"fmt"` to the templ import block if absent). The `views.Task` type already has an `ID` field (used by `loadTasks`). Keep the change minimal: add the id attribute to the existing outer element of each task, do not restructure.

- [ ] **Step 6: Regenerate templ and build**

Run: `just ui && go build ./apps/todo/...`
Expected: no errors.

- [ ] **Step 7: Run the app's tests**

Run: `go test ./apps/todo/...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/todo/
git commit -m "feat(todo): /_search endpoint with per-task deep-link anchors (ADR-0028)"
```

---

### Task 5: Dashboard search box, results view, and route

**Files:**
- Create: `platform/views/search.templ`
- Modify: `platform/views/dashboard.templ` (add a search form to the dashboard)
- Modify: `platform/main.go` (add `GET /search` handler)
- Test: `platform/search_view_test.go` (route wiring smoke test)

**Interfaces:**
- Consumes: `aggregateSearch`, `SearchGroup` (Task 2), `manifest.LoadAll`, `auth.FromContext`, `appURL` (from `platform/views/helpers.go`), `ui.AppShell`.
- Produces: `views.SearchResults(user auth.User, dev bool, domain string, q string, groups []SearchGroup)` templ component; a `GET /search?q=` route on platformd.

URL resolution: results carry app-relative URLs. In the view, build each result's absolute href as `string(appURL(dev, domain, app)) ` (which ends in `/`) joined with the result URL trimmed of its leading `/`. For a `/#note-3` result under app `notes`, the href is `http://localhost:4102/#note-3` (dev) or `https://notes.<domain>/#note-3` (prod).

- [ ] **Step 1: Write the search results view**

Create `platform/views/search.templ`:

```go
package views

import (
	"strings"

	"github.com/bketelsen/bespoke/pkg/auth"
	"github.com/bketelsen/bespoke/pkg/ui"
	"github.com/bketelsen/bespoke/pkg/ui/components/card"
)

// resultHref joins an app's base URL with a result's app-relative URL.
func resultHref(base, rel string) string {
	if rel == "" {
		rel = "/"
	}
	return strings.TrimSuffix(base, "/") + rel
}

templ SearchResults(user auth.User, dev bool, domain string, q string, groups []Group) {
	@ui.AppShell(ui.ShellProps{Title: "Search", User: user}) {
		<form method="get" action="/search" class="mb-6">
			<input
				type="search"
				name="q"
				value={ q }
				placeholder="Search all apps…"
				class="w-full rounded-md border border-input bg-background px-3 py-2 text-base"
				autofocus
			/>
		</form>
		if q == "" {
			<p class="text-muted-foreground">Type a query to search across your apps.</p>
		} else if len(groups) == 0 {
			<p class="text-muted-foreground">No results for "{ q }".</p>
		} else {
			<div class="space-y-6">
				for _, g := range groups {
					<section>
						<h2 class="mb-2 text-sm font-semibold text-muted-foreground">{ g.Name } ({ len(g.Results) })</h2>
						<div class="space-y-2">
							for _, res := range g.Results {
								@card.Card() {
									@card.Content(card.ContentProps{Class: "py-3"}) {
										<a href={ templ.SafeURL(resultHref(g.Base, res.URL)) } class="block">
											<span class="font-medium">{ res.Title }</span>
											if res.Snippet != "" {
												<span class="mt-1 block text-sm text-muted-foreground">{ res.Snippet }</span>
											}
										</a>
									}
								}
							}
						</div>
					</section>
				}
			</div>
		}
	}
}
```

Note the view uses a local `Group` type with a pre-resolved `Base` so templ files import no `main` types. Define it in the same file:

```go
type Result struct{ Title, Snippet, URL string }
type Group struct {
	Name    string
	Base    string
	Results []Result
}
```

- [ ] **Step 2: Add the dashboard search box**

In `platform/views/dashboard.templ`, add a search form at the top of the `Dashboard` templ body, before the `data-init` card grid div:

```go
		<form method="get" action="/search" class="mb-6">
			<input
				type="search"
				name="q"
				placeholder="Search all apps…"
				class="w-full rounded-md border border-input bg-background px-3 py-2 text-base"
			/>
		</form>
```

- [ ] **Step 3: Add the `GET /search` route**

In `platform/main.go`, inside the `web.Serve("platformd", ...)` register func, add after the `GET /{$}` handler:

```go
		mux.HandleFunc("GET /search", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			q := strings.TrimSpace(r.URL.Query().Get("q"))
			dev := os.Getenv("BESPOKE_DEV_USER") != ""
			var groups []views.Group
			if q != "" {
				apps, _, err := manifest.LoadAll(root)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				for _, g := range aggregateSearch(r.Context(), user.Login, user.Name, q, apps) {
					vg := views.Group{Name: g.Name}
					// Resolve the app's base URL for deep links.
					for _, a := range apps {
						if a.Slug == g.Slug {
							vg.Base = string(views.AppBase(dev, domain, a))
							break
						}
					}
					for _, res := range g.Results {
						vg.Results = append(vg.Results, views.Result{Title: res.Title, Snippet: res.Snippet, URL: res.URL})
					}
					groups = append(groups, vg)
				}
			}
			views.SearchResults(user, dev, domain, q, groups).Render(r.Context(), w)
		})
```

- [ ] **Step 4: Export a base-URL helper from views**

`appURL` in `platform/views/helpers.go` is unexported and returns `templ.SafeURL`. Add an exported sibling that the `main` package can call:

```go
// AppBase is appURL exported for the search handler, which resolves each
// result's app-relative URL against its app's base.
func AppBase(dev bool, domain string, app manifest.App) templ.SafeURL {
	return appURL(dev, domain, app)
}
```

- [ ] **Step 5: Regenerate templ and build**

Run: `just ui && go build ./platform/...`
Expected: no errors. Fix any templ import issues (the `Group`/`Result` types live in `views`).

- [ ] **Step 6: Write a route smoke test**

Create `platform/search_view_test.go` verifying the results view renders groups and the empty state:

```go
package main

import (
	"context"
	"strings"
	"testing"

	"github.com/bketelsen/bespoke/pkg/auth"
	"github.com/bketelsen/bespoke/platform/views"
)

func TestSearchResultsView(t *testing.T) {
	var sb strings.Builder
	g := views.Group{Name: "Notes", Base: "http://localhost:4102/", Results: []views.Result{{Title: "milk", URL: "/#note-1"}}}
	err := views.SearchResults(auth.User{Login: "me@x"}, true, "x.example", "milk", []views.Group{g}).Render(context.Background(), &sb)
	if err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	if !strings.Contains(out, "Notes (1)") {
		t.Errorf("missing group heading: %s", out)
	}
	if !strings.Contains(out, "http://localhost:4102/#note-1") {
		t.Errorf("missing resolved deep link: %s", out)
	}
}
```

- [ ] **Step 7: Run tests and build**

Run: `go test ./platform/... && go build ./platform/...`
Expected: PASS, no build errors.

- [ ] **Step 8: Commit**

```bash
git add platform/ 
git commit -m "feat(platform): dashboard search box, results view, /search route (ADR-0028)"
```

---

### Task 6: Platform `search` MCP/chat tool

**Files:**
- Modify: `platform/mcp.go` (add the platform-owned `search` tool to the per-request MCP server)
- Modify: `platform/main.go` (add `search` to the dashboard chat's tool list)
- Test: `platform/search_tool_test.go`

**Interfaces:**
- Consumes: `aggregateSearch`, `SearchGroup` (Task 2), `manifest.LoadAll`.
- Produces:
  - `func searchTool(ctx context.Context, root, login, name, q string) string` — runs the fan-out and formats grouped results as text (app heading + each `title` — `url`), suitable for an LLM to cite. Returns "(no results)" when empty.
  - The `search` tool registered on the MCP server (`mcp.AddTool`) and added to the dashboard chat `[]llm.Tool`.

The tool's input schema: `{"type":"object","properties":{"q":{"type":"string","description":"search query"}},"required":["q"]}`. Description: `"Search across all your apps' data. Returns matches grouped by app with deep links."` Not destructive — no marking needed.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/bketelsen/bespoke/internal/manifest"
	"github.com/bketelsen/bespoke/pkg/web"
)

func TestSearchToolFormatting(t *testing.T) {
	t.Setenv("BESPOKE_BIND_IP", "127.0.0.1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"results": []web.SearchResult{
			{Title: "buy milk", URL: "/#task-3"},
		}})
	}))
	defer srv.Close()
	port, _ := strconv.Atoi(srv.URL[strings.LastIndex(srv.URL, ":")+1:])
	apps := []manifest.App{{Slug: "todo", Name: "Todo", Port: port}}

	out := formatSearchGroups(aggregateSearch(context.Background(), "me@x", "Me", "milk", apps))
	if !strings.Contains(out, "Todo") || !strings.Contains(out, "buy milk") || !strings.Contains(out, "/#task-3") {
		t.Fatalf("bad tool output: %q", out)
	}

	if got := formatSearchGroups(nil); !strings.Contains(got, "no results") {
		t.Errorf("empty output = %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./platform/ -run TestSearchToolFormatting -v`
Expected: FAIL — `undefined: formatSearchGroups`.

- [ ] **Step 3: Add the formatter to `platform/search.go`**

```go
// formatSearchGroups renders grouped results as text for the search tool —
// each app's heading then "title — url" lines, so an LLM can cite deep links.
func formatSearchGroups(groups []SearchGroup) string {
	if len(groups) == 0 {
		return "(no results)"
	}
	var b strings.Builder
	for _, g := range groups {
		fmt.Fprintf(&b, "## %s\n", g.Name)
		for _, r := range g.Results {
			if r.URL != "" {
				fmt.Fprintf(&b, "- %s — %s\n", r.Title, r.URL)
			} else {
				fmt.Fprintf(&b, "- %s\n", r.Title)
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}
```

Add `"strings"` to the `platform/search.go` import block.

- [ ] **Step 4: Run the formatter test**

Run: `go test ./platform/ -run TestSearchToolFormatting -v`
Expected: PASS.

- [ ] **Step 5: Register the tool on the MCP server**

In `platform/mcp.go`, inside `mcpHandler`'s per-request server construction, after the `for _, t := range allAppTools(...)` loop and before `return s`, add the platform-owned search tool:

```go
		s.AddTool(&mcp.Tool{
			Name:        "search",
			Description: "Search across all your apps' data. Returns matches grouped by app with deep links.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"q": map[string]any{"type": "string", "description": "search query"},
				},
				"required": []string{"q"},
			},
		}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var args struct {
				Q string `json:"q"`
			}
			if req.Params.Arguments != nil {
				_ = json.Unmarshal(req.Params.Arguments, &args)
			}
			apps, _, err := manifest.LoadAll(root)
			if err != nil {
				return nil, err
			}
			out := formatSearchGroups(aggregateSearch(ctx, login, login, args.Q, apps))
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: out}}}, nil
		})
```

(`login` is already in scope in `mcpHandler`; `json` and `manifest` are already imported.)

- [ ] **Step 6: Add `search` to dashboard chat tools**

In `platform/main.go`, the `EnableChatWithTools` tools callback currently returns only `allAppTools`. The chat gateway calls tools back over HTTP by URL (`llm.Tool.URL`), so a URL-less tool won't execute there. To expose search to dashboard chat with the same mechanism, mount a platform-local HTTP tool endpoint and advertise it.

Add a small authenticated handler in the `web.Serve` register func:

```go
		mux.HandleFunc("POST /_tools/search", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			var args struct{ Q string `json:"q"` }
			body, _ := io.ReadAll(io.LimitReader(r.Body, 8<<10))
			_ = json.Unmarshal(body, &args)
			apps, _, err := manifest.LoadAll(root)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			out := formatSearchGroups(aggregateSearch(r.Context(), user.Login, user.Name, args.Q, apps))
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte(out))
		})
```

Then in the tools callback, append the platform search tool with its self URL. platformd's self base in dev/prod is `http://127.0.0.1:4000`; reuse the same host as fan-out:

```go
			func(ctx context.Context, user auth.User) []llm.Tool {
				var tools []llm.Tool
				for _, t := range allAppTools(ctx, root) {
					tools = append(tools, llm.Tool{
						Name:        t.Slug + "_" + t.Name,
						Description: fmt.Sprintf("[%s] %s", t.Slug, t.Description),
						Schema:      t.Schema,
						URL:         t.URL,
					})
				}
				host := cmp.Or(os.Getenv("BESPOKE_BIND_IP"), "127.0.0.1")
				tools = append(tools, llm.Tool{
					Name:        "search",
					Description: "Search across all your apps' data. Returns matches grouped by app with deep links.",
					Schema: map[string]any{
						"type":       "object",
						"properties": map[string]any{"q": map[string]any{"type": "string", "description": "search query"}},
						"required":   []string{"q"},
					},
					URL: fmt.Sprintf("http://%s:4000/_tools/search", host),
				})
				return tools
			},
```

Add imports `"io"` and `"cmp"` to `platform/main.go` if not present (`cmp` is already imported; add `"io"`).

- [ ] **Step 7: Build and test**

Run: `go build ./platform/... && go test ./platform/...`
Expected: no build errors, tests PASS.

- [ ] **Step 8: Commit**

```bash
git add platform/
git commit -m "feat(platform): expose search as MCP + dashboard-chat tool (ADR-0028)"
```

---

### Task 7: Full verification and docs closeout

**Files:**
- Modify: none required beyond confirming the already-committed docs (ADR-0028, `docs/specs/app-search.md`, `internal-services.md`, `app-manifest.md`, `AGENTS.md`, `docs/README.md`) are consistent with the shipped code.

- [ ] **Step 1: Confirm docs match the implementation**

Re-read `docs/specs/app-search.md` and confirm the JSON shape, the `web.Search(mux, provider)` signature, and the deep-link/fallback language match what Tasks 1–6 shipped. Fix any drift inline (e.g., if the provider signature differs). Verify cross-links resolve.

- [ ] **Step 2: Run the full check**

Run: `just check`
Expected: PASS — vet, all tests, golangci-lint, `go mod tidy -diff`, and the CGO-free linux cross-compile all succeed.

- [ ] **Step 3: Manual dev smoke (optional but recommended)**

Run: `just dev`, open the dashboard, type a query in the search box, confirm grouped results appear with working deep links into Notes and Todo. Stop dev.

- [ ] **Step 4: Commit any doc fixes**

```bash
git add docs/ AGENTS.md
git commit -m "docs: reconcile global-search docs with implementation (ADR-0028)"
```

---

## Self-Review

**Spec coverage** (against `docs/specs/app-search.md` and the design doc):
- `GET /_search?q=` contract + `web.Search` helper → Task 1. ✓
- User-scoping, cheap-query rule → Tasks 3, 4 (`WHERE login=?`, no LLM). ✓
- JSON `{results:[{title,snippet?,url?,timestamp?}]}` → Task 1 type + test. ✓
- Fan-out, identity forwarding, timeout, 32KB cap, drop misses, group by app → Task 2. ✓
- Deep links preferred/best-effort; both Notes and Todo ship real deep links → Tasks 3, 4. ✓
- Dashboard search box + grouped results view, plain GET → Task 5. ✓
- MCP + dashboard-chat `search` tool, same grouped structure → Task 6. ✓
- Docs (ADR-0028, spec, catalog, manifest, AGENTS.md, index) → committed pre-plan; reconciled in Task 7. ✓

**Placeholder scan:** No TBD/TODO/"handle edge cases" left; every code step shows complete code. ✓

**Type consistency:** `web.SearchResult`/`web.SearchProvider` (Task 1) used verbatim in Tasks 2–4. `SearchGroup` (Task 2) consumed in Tasks 5–6. `views.Group`/`views.Result` (Task 5) are the view-layer mirror populated by the `/search` handler; kept distinct from `main.SearchGroup` on purpose so templ imports no `main` types. `aggregateSearch` signature identical across Tasks 2, 5, 6. `formatSearchGroups` defined in Task 6 Step 3, used in Task 6 Steps 5–6. ✓

**One risk flagged for the implementer:** Task 6 Step 6 assumes platformd listens on `:4000` for the self tool URL. If `BESPOKE_LISTEN` overrides the port in some environment, prefer deriving the port from that env var rather than hardcoding `4000`. Verify against the deployed unit before relying on it; the MCP path (Task 6 Step 5) has no such dependency and is the primary surface.
