CREATE TABLE briefs (
    login      TEXT PRIMARY KEY,
    name       TEXT NOT NULL DEFAULT '',
    brief      TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
