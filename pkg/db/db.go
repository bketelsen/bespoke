// Package db opens an app's SQLite database per
// docs/adr/0007-sqlite-per-app-litestream.md: one file per app under the
// data directory, WAL mode, embedded migrations applied on open.
//
// The driver is modernc.org/sqlite (pure Go): deploys cross-compile with
// CGO_ENABLED=0 (docs/adr/0011-split-host-deployment.md).
package db

import (
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	_ "modernc.org/sqlite"
)

// Open opens data/<slug>.db, creating the data directory if needed, and
// applies any unapplied migrations from the app's embedded migrations FS
// (files named NNNN_*.sql, applied in lexical order, tracked via PRAGMA
// user_version). Pass nil for an app with no schema.
func Open(slug string, migrations fs.FS) (*sql.DB, error) {
	dir := os.Getenv("BESPOKE_DATA")
	if dir == "" {
		root := os.Getenv("BESPOKE_ROOT")
		if root == "" {
			root = "."
		}
		dir = filepath.Join(root, "data")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)",
		filepath.Join(dir, slug+".db"))
	sqldb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if migrations != nil {
		if err := migrate(sqldb, migrations); err != nil {
			sqldb.Close()
			return nil, fmt.Errorf("migrate %s: %w", slug, err)
		}
	}
	return sqldb, nil
}

func migrate(sqldb *sql.DB, migrations fs.FS) error {
	names, err := fs.Glob(migrations, "*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)

	var version int
	if err := sqldb.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	for i, name := range names {
		n := i + 1 // migration number = position in sorted order
		if n <= version {
			continue
		}
		src, err := fs.ReadFile(migrations, name)
		if err != nil {
			return err
		}
		tx, err := sqldb.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(src)); err != nil {
			tx.Rollback()
			return fmt.Errorf("%s: %w", name, err)
		}
		// PRAGMA doesn't take placeholders; n is an int we control.
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", n)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
