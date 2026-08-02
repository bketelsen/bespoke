-- family-walks schema, applied by pkg/db in lexical order.
-- Add tables here; never edit an applied migration — add 0002_*.sql instead.

CREATE TABLE walk_days (
    id         INTEGER PRIMARY KEY,
    login      TEXT NOT NULL,
    date       TEXT NOT NULL,                        -- YYYY-MM-DD, local date
    walked     INTEGER NOT NULL,                      -- 1 = yes, 0 = no
    note       TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (login, date)
);
CREATE INDEX walk_days_login_date ON walk_days (login, date);
