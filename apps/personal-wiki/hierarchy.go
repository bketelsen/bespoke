package main

// The pseudo-hierarchy (spec: README.md): "Index" is the required root
// page — auto-created, undeletable, unrenameable. A page's children are
// the pages its body [[links]] to, and a page's place in the wiki is its
// shortest link path from Index. Nothing extra is stored: the hierarchy
// IS the link graph, so curating it means editing page bodies.

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/bketelsen/bespoke/apps/personal-wiki/views"
)

const indexTitle = "Index"

const indexSeed = `This is the wiki's table of contents: every page should be reachable from here.

Organize it with [[links]] — top-level links are your sections, and each section page links onward to its children.`

// ensureIndex guarantees the root page exists for this user.
func ensureIndex(ctx context.Context, sqldb *sql.DB, login string) error {
	_, err := sqldb.ExecContext(ctx,
		"INSERT OR IGNORE INTO pages (login, title, body) VALUES (?, ?, ?)",
		login, indexTitle, indexSeed)
	return err
}

// node is one page in the link graph; links keep body order, so the
// hierarchy inherits the order pages list their children in.
type node struct {
	id    int64
	title string
	links []string
}

// loadGraph reads the user's whole link graph. A personal wiki is small;
// one scan beats bookkeeping (backlinks already work this way).
func loadGraph(ctx context.Context, sqldb *sql.DB, login string) (map[string]node, error) {
	rows, err := sqldb.QueryContext(ctx, "SELECT id, title, body FROM pages WHERE login = ?", login)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	nodes := map[string]node{}
	for rows.Next() {
		var n node
		var body string
		if err := rows.Scan(&n.id, &n.title, &body); err != nil {
			return nil, err
		}
		n.links = wikiLinkTitles(body)
		nodes[n.title] = n
	}
	return nodes, rows.Err()
}

// breadcrumb returns the shortest link path Index → … → parent for a page
// (excluding the page itself), or nil for the Index and for orphans.
func breadcrumb(nodes map[string]node, target string) []views.PageSummary {
	if target == indexTitle {
		return nil
	}
	parent := map[string]string{indexTitle: ""}
	queue := []string{indexTitle}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == target {
			var path []views.PageSummary
			for t := parent[target]; t != ""; t = parent[t] {
				n := nodes[t]
				path = append([]views.PageSummary{{ID: n.id, Title: n.title}}, path...)
			}
			return path
		}
		for _, l := range nodes[cur].links {
			if _, seen := parent[l]; seen {
				continue
			}
			if _, ok := nodes[l]; !ok {
				continue
			}
			parent[l] = cur
			queue = append(queue, l)
		}
	}
	return nil
}

// reachable returns every title reachable from Index via links.
func reachable(nodes map[string]node) map[string]bool {
	seen := map[string]bool{}
	queue := []string{indexTitle}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if seen[cur] {
			continue
		}
		if _, ok := nodes[cur]; !ok {
			continue
		}
		seen[cur] = true
		queue = append(queue, nodes[cur].links...)
	}
	return seen
}

// orphanPages lists pages not reachable from the Index, sorted by title —
// the home page surfaces them so gardening can attach them.
func orphanPages(nodes map[string]node) []views.PageSummary {
	in := reachable(nodes)
	var out []views.PageSummary
	for t, n := range nodes {
		if !in[t] {
			out = append(out, views.PageSummary{ID: n.id, Title: n.title})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out
}

// hierarchyText renders the tree for the chat context: children indented
// under their first-reached parent, cycles and repeats cut, orphans
// appended — the map the LLM authors against.
func hierarchyText(nodes map[string]node) string {
	var b strings.Builder
	seen := map[string]bool{}
	var walk func(title string, depth int)
	walk = func(title string, depth int) {
		n, ok := nodes[title]
		if !ok || seen[title] {
			return
		}
		seen[title] = true
		fmt.Fprintf(&b, "%s- %s (#%d)\n", strings.Repeat("  ", depth), title, n.id)
		for _, l := range n.links {
			walk(l, depth+1)
		}
	}
	walk(indexTitle, 0)
	if orphans := orphanPages(nodes); len(orphans) > 0 {
		b.WriteString("Orphans (not reachable from Index — attach or absorb these):\n")
		for _, o := range orphans {
			fmt.Fprintf(&b, "- %s (#%d)\n", o.Title, o.ID)
		}
	}
	return b.String()
}
