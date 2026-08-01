package db

import (
	"testing"
	"testing/fstest"
)

func TestOpenMigrates(t *testing.T) {
	t.Setenv("BESPOKE_DATA", t.TempDir())
	migrations := fstest.MapFS{
		"0001_init.sql": {Data: []byte(`CREATE TABLE things (name TEXT NOT NULL);`)},
		"0002_more.sql": {Data: []byte(`ALTER TABLE things ADD COLUMN note TEXT;`)},
	}

	sqldb, err := Open("testapp", migrations)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sqldb.Exec(`INSERT INTO things (name, note) VALUES ('a', 'b')`); err != nil {
		t.Fatalf("schema not migrated: %v", err)
	}
	var version int
	if err := sqldb.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 2 {
		t.Fatalf("user_version = %d, want 2", version)
	}
	sqldb.Close()

	// Reopening must be a no-op, not a re-run of migrations.
	sqldb, err = Open("testapp", migrations)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer sqldb.Close()
	var count int
	if err := sqldb.QueryRow("SELECT count(*) FROM things").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1 (data lost on reopen?)", count)
	}
}
