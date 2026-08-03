CREATE TABLE notes (
    id         INTEGER PRIMARY KEY,
    login      TEXT NOT NULL,
    body       TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX notes_login_created ON notes(login, created_at DESC);
