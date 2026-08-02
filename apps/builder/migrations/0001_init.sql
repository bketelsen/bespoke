-- builder schema, applied by pkg/db in lexical order.

CREATE TABLE runs (
    id         TEXT PRIMARY KEY,              -- r<unix-ms>, shared with the spool
    login      TEXT NOT NULL,
    idea       TEXT NOT NULL,
    slug       TEXT NOT NULL DEFAULT '',
    spec       TEXT NOT NULL DEFAULT '',
    status     TEXT NOT NULL DEFAULT 'interviewing', -- interviewing|ready|building|deploying|live|failed
    detail     TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE messages (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id     TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    role       TEXT NOT NULL,                 -- user|assistant
    body       TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX messages_run ON messages(run_id, id);

-- Mirror of the spool's events.jsonl (docs/design/builder-plane.md); seq is
-- the line number, so tailing is idempotent.
CREATE TABLE run_events (
    run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    seq    INTEGER NOT NULL,
    ts     TEXT NOT NULL,
    kind   TEXT NOT NULL,
    body   TEXT NOT NULL,
    PRIMARY KEY (run_id, seq)
);
