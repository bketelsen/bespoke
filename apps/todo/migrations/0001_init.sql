CREATE TABLE tasks (
    id           INTEGER PRIMARY KEY,
    login        TEXT NOT NULL,
    parent_id    INTEGER REFERENCES tasks(id) ON DELETE CASCADE,
    description  TEXT NOT NULL,
    due          TEXT,                                   -- YYYY-MM-DD, local date
    priority     TEXT NOT NULL DEFAULT 'L' CHECK (priority IN ('L','M','H')),
    done         INTEGER NOT NULL DEFAULT 0,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    completed_at TEXT
);
CREATE INDEX tasks_login_parent ON tasks (login, parent_id);
CREATE INDEX tasks_login_due ON tasks (login, due);
