-- personal-wiki schema, applied by pkg/db in lexical order.
-- Add tables here; never edit an applied migration — add 0002_*.sql instead.

CREATE TABLE pages (
    id             INTEGER PRIMARY KEY,
    login          TEXT NOT NULL,
    title          TEXT NOT NULL,
    body           TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at     TEXT NOT NULL DEFAULT (datetime('now')),
    last_viewed_at TEXT,
    UNIQUE (login, title)
);
CREATE INDEX pages_login_title ON pages (login, title);
CREATE INDEX pages_login_last_viewed ON pages (login, last_viewed_at);

CREATE TABLE page_tags (
    page_id INTEGER NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
    tag     TEXT NOT NULL,
    PRIMARY KEY (page_id, tag)
);
CREATE INDEX page_tags_tag ON page_tags (tag);

