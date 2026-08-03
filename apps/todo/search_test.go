package main

import (
	"context"
	"io/fs"
	"strings"
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

	t.Run("matches only the user's tasks with deep links", func(t *testing.T) {
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
		if !strings.HasPrefix(got[0].URL, "/#task-") {
			t.Errorf("want task-anchor deep-link url, got %q", got[0].URL)
		}
	})

	t.Run("LIKE wildcards match literally", func(t *testing.T) {
		got, err := searchTasks(ctx, sqldb, "me@x", "_")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Errorf("bare _ should match nothing, got %+v", got)
		}
	})

	t.Run("returns nil for whitespace query", func(t *testing.T) {
		got, err := searchTasks(ctx, sqldb, "me@x", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if got != nil {
			t.Errorf("want nil results, got %+v", got)
		}
	})
}
