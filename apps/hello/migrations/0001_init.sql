CREATE TABLE visits (
    login      TEXT NOT NULL,
    visited_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX visits_login ON visits (login);
