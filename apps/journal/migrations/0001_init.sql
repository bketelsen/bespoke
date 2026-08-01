CREATE TABLE entries (
    id         INTEGER PRIMARY KEY,
    login      TEXT NOT NULL,
    body       TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX entries_login_created ON entries (login, created_at DESC);

CREATE TABLE summaries (
    id          INTEGER PRIMARY KEY,
    login       TEXT NOT NULL,
    range_start TEXT NOT NULL,
    range_end   TEXT NOT NULL,
    body        TEXT NOT NULL,
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX summaries_login_created ON summaries (login, created_at DESC);
