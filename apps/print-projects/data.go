package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/bketelsen/bespoke/apps/print-projects/views"
)

func loadProjects(ctx context.Context, db *sql.DB, login string) ([]views.Project, error) {
	rows, err := db.QueryContext(ctx, `SELECT p.id,p.name,p.source_url,p.notes,
		(SELECT count(*) FROM print_history h WHERE h.project_id=p.id AND h.login=p.login)
		FROM projects p WHERE p.login=? ORDER BY 5=0 DESC,p.created_at DESC,p.id DESC`, login)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []views.Project
	for rows.Next() {
		var p views.Project
		if err := rows.Scan(&p.ID, &p.Name, &p.SourceURL, &p.Notes, &p.PrintCount); err != nil {
			return nil, err
		}
		h, err := db.QueryContext(ctx, `SELECT id,printed_on,notes,image IS NOT NULL FROM print_history
			WHERE project_id=? AND login=? ORDER BY printed_on DESC,id DESC`, p.ID, login)
		if err != nil {
			return nil, err
		}
		for h.Next() {
			var x views.Print
			if err := h.Scan(&x.ID, &x.PrintedOn, &x.Notes, &x.HasImage); err != nil {
				h.Close()
				return nil, err
			}
			p.Prints = append(p.Prints, x)
		}
		if err := h.Close(); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func chatContext(ctx context.Context, db *sql.DB, login string) (string, error) {
	projects, err := loadProjects(ctx, db, login)
	if err != nil {
		return "", err
	}
	if len(projects) == 0 {
		return "No print projects yet.", nil
	}
	var b strings.Builder
	for _, p := range projects {
		fmt.Fprintf(&b, "#%d %s (%d prints)\n", p.ID, p.Name, p.PrintCount)
		if p.SourceURL != "" {
			fmt.Fprintf(&b, "Source: %s\n", p.SourceURL)
		}
		if p.Notes != "" {
			fmt.Fprintf(&b, "Notes: %s\n", p.Notes)
		}
		for _, h := range p.Prints {
			fmt.Fprintf(&b, "- %s: %s\n", h.PrintedOn, h.Notes)
		}
	}
	return b.String(), nil
}
