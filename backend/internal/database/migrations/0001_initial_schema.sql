CREATE TABLE users (
    id           TEXT PRIMARY KEY,
    username     TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    role         TEXT NOT NULL CHECK (role IN ('supporter', 'student')),
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
) STRICT;

CREATE TABLE devices (
    id           TEXT PRIMARY KEY,
    user_id      TEXT REFERENCES users (id) ON DELETE SET NULL,
    name         TEXT NOT NULL,
    kind         TEXT NOT NULL CHECK (kind IN ('server', 'laptop', 'phone', 'desktop')),
    status       TEXT NOT NULL DEFAULT 'unknown' CHECK (status IN ('online', 'offline', 'unknown')),
    last_seen_at TEXT,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
) STRICT;

CREATE INDEX idx_devices_user_id ON devices (user_id);
CREATE INDEX idx_devices_status ON devices (status);
