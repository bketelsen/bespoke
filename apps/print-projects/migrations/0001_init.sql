CREATE TABLE projects (
    id         INTEGER PRIMARY KEY,
    login      TEXT NOT NULL,
    name       TEXT NOT NULL,
    source_url TEXT NOT NULL DEFAULT '',
    notes      TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX projects_login_created ON projects(login, created_at DESC);

CREATE TABLE print_history (
    id          INTEGER PRIMARY KEY,
    project_id  INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    login       TEXT NOT NULL,
    printed_on  TEXT NOT NULL,
    notes       TEXT NOT NULL DEFAULT '',
    image       BLOB,
    image_type  TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    CHECK (image IS NULL OR length(image) <= 8388608)
);

CREATE INDEX history_project_date ON print_history(login, project_id, printed_on DESC, id DESC);
