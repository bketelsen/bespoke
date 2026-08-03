-- gh-tracker schema, applied by pkg/db in lexical order.
-- Add tables here; never edit an applied migration — add 0002_*.sql instead.

CREATE TABLE projects (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	login           TEXT NOT NULL,
	owner_repo      TEXT NOT NULL,
	position        INTEGER NOT NULL DEFAULT 0,
	last_fetched_at DATETIME,
	cached_items    TEXT NOT NULL DEFAULT '[]', -- JSON list of {type,title,url,number,updated_at}
	created_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_projects_login ON projects (login, position, id);

CREATE TABLE settings (
	login        TEXT PRIMARY KEY,
	github_token TEXT
);
