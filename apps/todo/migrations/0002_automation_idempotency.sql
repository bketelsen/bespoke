CREATE TABLE tool_idempotency (
    login TEXT NOT NULL,
    tool TEXT NOT NULL,
    key TEXT NOT NULL,
    result TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (login, tool, key)
);
