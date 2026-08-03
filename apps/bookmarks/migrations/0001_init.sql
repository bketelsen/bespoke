-- bookmarks schema, applied by pkg/db in lexical order.
-- Add tables here; never edit an applied migration — add 0002_*.sql instead.
CREATE TABLE bookmarks (
    id         INTEGER PRIMARY KEY,
    login      TEXT NOT NULL,
    url        TEXT NOT NULL,
    title      TEXT NOT NULL,
    tags       TEXT NOT NULL DEFAULT '', -- comma-separated, no spaces around commas
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX bookmarks_login_created ON bookmarks (login, created_at DESC);
