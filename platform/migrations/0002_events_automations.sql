CREATE TABLE events (
    source TEXT NOT NULL, id TEXT NOT NULL, login TEXT NOT NULL,
    type TEXT NOT NULL, subject_id TEXT NOT NULL DEFAULT '', occurred_at TEXT NOT NULL,
    accepted_at TEXT NOT NULL DEFAULT (datetime('now')), data_json TEXT NOT NULL,
    causation_id TEXT, correlation_id TEXT NOT NULL, depth INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (source, id)
);
CREATE INDEX events_login_accepted ON events(login, accepted_at DESC);

CREATE TABLE notifications (
    id TEXT PRIMARY KEY, event_id TEXT NOT NULL, source TEXT NOT NULL, login TEXT NOT NULL,
    title TEXT NOT NULL, body TEXT NOT NULL DEFAULT '', app_slug TEXT NOT NULL DEFAULT '',
    path TEXT NOT NULL DEFAULT '', group_key TEXT NOT NULL DEFAULT '', dedup_key TEXT UNIQUE,
    created_at TEXT NOT NULL DEFAULT (datetime('now')), read_at TEXT, dismissed_at TEXT
);
CREATE INDEX notifications_login_created ON notifications(login, created_at DESC, id DESC);

CREATE TABLE automation_rules (
    id TEXT PRIMARY KEY, login TEXT NOT NULL, name TEXT NOT NULL, enabled INTEGER NOT NULL DEFAULT 0,
    source TEXT NOT NULL, event_type TEXT NOT NULL, conditions_json TEXT NOT NULL DEFAULT '[]',
    steps_json TEXT NOT NULL, revision INTEGER NOT NULL DEFAULT 1, enabled_at TEXT,
    deleted_at TEXT, created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX automation_rules_login ON automation_rules(login, updated_at DESC);

CREATE TABLE automation_runs (
    id TEXT PRIMARY KEY, login TEXT NOT NULL, rule_id TEXT NOT NULL, event_id TEXT NOT NULL,
    event_source TEXT NOT NULL, rule_revision INTEGER NOT NULL, rule_json TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending', next_attempt_at TEXT, lease_owner TEXT,
    lease_expires_at TEXT, created_at TEXT NOT NULL DEFAULT (datetime('now')),
    started_at TEXT, finished_at TEXT,
    UNIQUE(rule_id, event_source, event_id)
);
CREATE INDEX automation_runs_pending ON automation_runs(status, next_attempt_at, lease_expires_at);

CREATE TABLE automation_steps (
    id TEXT PRIMARY KEY, run_id TEXT NOT NULL, name TEXT NOT NULL, position INTEGER NOT NULL,
    type TEXT NOT NULL, definition_json TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'pending',
    attempt INTEGER NOT NULL DEFAULT 0, next_attempt_at TEXT, input_json TEXT, output_json TEXT,
    error TEXT, started_at TEXT, finished_at TEXT, UNIQUE(run_id, position)
);
