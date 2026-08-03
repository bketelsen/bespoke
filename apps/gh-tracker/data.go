package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"time"

	"github.com/bketelsen/bespoke/apps/gh-tracker/views"
)

const maxProjects = 7
const staleAfter = 15 * time.Minute

// ProjectGroup is a project plus its (possibly just-refreshed) items, ready
// for the view layer.
type ProjectGroup struct {
	ID            int64
	OwnerRepo     string
	Items         []Item
	LastFetchedAt time.Time
	FetchErr      string
}

func githubToken(ctx context.Context, db *sql.DB, login string) (string, error) {
	var tok sql.NullString
	err := db.QueryRowContext(ctx, "SELECT github_token FROM settings WHERE login = ?", login).Scan(&tok)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return tok.String, nil
}

// loadProjectGroups loads every configured project for login, refreshing
// any whose cache is stale (>15min or never fetched) from the GitHub API.
// A refresh failure keeps the last good cache and records an inline error
// (README rule: never blank the list).
func loadProjectGroups(ctx context.Context, db *sql.DB, login string) ([]ProjectGroup, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, owner_repo, last_fetched_at, cached_items
		FROM projects WHERE login = ? ORDER BY position, id`, login)
	if err != nil {
		return nil, err
	}
	type raw struct {
		id            int64
		ownerRepo     string
		lastFetchedAt sql.NullString
		cachedItems   string
	}
	var list []raw
	for rows.Next() {
		var r raw
		if err := rows.Scan(&r.id, &r.ownerRepo, &r.lastFetchedAt, &r.cachedItems); err != nil {
			rows.Close()
			return nil, err
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	token, err := githubToken(ctx, db, login)
	if err != nil {
		return nil, err
	}

	out := make([]ProjectGroup, 0, len(list))
	for _, r := range list {
		var items []Item
		_ = json.Unmarshal([]byte(r.cachedItems), &items)

		var lastFetched time.Time
		if r.lastFetchedAt.Valid {
			lastFetched, _ = time.Parse("2006-01-02 15:04:05", r.lastFetchedAt.String)
		}

		g := ProjectGroup{ID: r.id, OwnerRepo: r.ownerRepo, Items: items, LastFetchedAt: lastFetched}

		if time.Since(lastFetched) > staleAfter {
			fresh, ferr := fetchOpenItems(httpClient, r.ownerRepo, token)
			if ferr != nil {
				g.FetchErr = ferr.Error() // keep serving cached items
			} else {
				g.Items = fresh
				g.LastFetchedAt = time.Now().UTC()
				body, _ := json.Marshal(fresh)
				if _, err := db.ExecContext(ctx,
					"UPDATE projects SET cached_items = ?, last_fetched_at = datetime('now') WHERE id = ?",
					string(body), r.id); err != nil {
					return nil, err
				}
			}
		}
		out = append(out, g)
	}
	return out, nil
}

func toView(groups []ProjectGroup) []views.ProjectGroup {
	out := make([]views.ProjectGroup, 0, len(groups))
	for _, g := range groups {
		vg := views.ProjectGroup{
			ID:        g.ID,
			OwnerRepo: g.OwnerRepo,
			FetchErr:  g.FetchErr,
		}
		if !g.LastFetchedAt.IsZero() {
			vg.SyncedLabel = relativeTime(g.LastFetchedAt)
		}
		for _, it := range g.Items {
			vi := views.Item{
				Type:   it.Type,
				Title:  it.Title,
				URL:    it.URL,
				Number: it.Number,
			}
			if t, err := time.Parse(time.RFC3339, it.UpdatedAt); err == nil {
				vi.UpdatedLabel = relativeTime(t)
			}
			vg.Items = append(vg.Items, vi)
		}
		out = append(out, vg)
	}
	return out
}

// relativeTime humanizes t (in the past) against now — "just now", "12 min
// ago", "3 hr ago", "5 days ago".
func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + " min ago"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + " hr ago"
	default:
		return strconv.Itoa(int(d.Hours()/24)) + " days ago"
	}
}

// openCounts returns the total open PR/issue count across all of login's
// projects, from cache only (dashboard card — never triggers an API call).
func openCounts(ctx context.Context, db *sql.DB, login string) (int, error) {
	rows, err := db.QueryContext(ctx, "SELECT cached_items FROM projects WHERE login = ?", login)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	total := 0
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return 0, err
		}
		var items []Item
		_ = json.Unmarshal([]byte(raw), &items)
		total += len(items)
	}
	return total, rows.Err()
}
