CREATE TABLE study_sessions (
    id                  TEXT PRIMARY KEY,
    user_id             TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    subject             TEXT NOT NULL,
    topic               TEXT,
    status              TEXT NOT NULL CHECK (status IN ('active', 'paused', 'completed', 'abandoned')),
    started_at          TEXT NOT NULL,
    ended_at            TEXT,
    accumulated_seconds INTEGER NOT NULL DEFAULT 0,
    last_resumed_at     TEXT,
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL
) STRICT;

CREATE INDEX idx_study_sessions_user_id ON study_sessions (user_id);
CREATE INDEX idx_study_sessions_status ON study_sessions (status);
CREATE INDEX idx_study_sessions_started_at ON study_sessions (started_at);

CREATE TABLE study_goals (
    id             TEXT PRIMARY KEY,
    user_id        TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    session_id     TEXT REFERENCES study_sessions (id) ON DELETE SET NULL,
    subject        TEXT NOT NULL,
    topic          TEXT,
    target_minutes INTEGER NOT NULL CHECK (target_minutes > 0),
    goal_date      TEXT NOT NULL,
    completed_at   TEXT,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
) STRICT;

CREATE INDEX idx_study_goals_user_date ON study_goals (user_id, goal_date);

CREATE TABLE messages (
    id         TEXT PRIMARY KEY,
    sender_id  TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    session_id TEXT REFERENCES study_sessions (id) ON DELETE SET NULL,
    kind       TEXT NOT NULL CHECK (kind IN ('message', 'help_request')),
    body       TEXT NOT NULL,
    read_at    TEXT,
    created_at TEXT NOT NULL
) STRICT;

CREATE INDEX idx_messages_created_at ON messages (created_at);
CREATE INDEX idx_messages_unread ON messages (read_at) WHERE read_at IS NULL;

CREATE TABLE files (
    id              TEXT PRIMARY KEY,
    uploader_id     TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    category        TEXT NOT NULL CHECK (category IN ('notes', 'homework', 'images', 'documents', 'resources', 'other')),
    original_name   TEXT NOT NULL,
    stored_path     TEXT NOT NULL UNIQUE,
    size_bytes      INTEGER NOT NULL CHECK (size_bytes >= 0),
    checksum_sha256 TEXT NOT NULL,
    created_at      TEXT NOT NULL
) STRICT;

CREATE INDEX idx_files_category ON files (category);

CREATE TABLE events (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    type       TEXT NOT NULL,
    payload    TEXT NOT NULL DEFAULT '{}',
    device_id  TEXT REFERENCES devices (id) ON DELETE SET NULL,
    created_at TEXT NOT NULL
) STRICT;

CREATE INDEX idx_events_type ON events (type);
CREATE INDEX idx_events_created_at ON events (created_at);
