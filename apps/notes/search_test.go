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
