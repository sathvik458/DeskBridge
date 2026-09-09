ALTER TABLE users ADD COLUMN pass_hash TEXT;

CREATE TABLE logins (
    token      TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    label      TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    seen_at    TEXT NOT NULL,
    expires_at TEXT NOT NULL
) STRICT;

CREATE INDEX idx_logins_user_id ON logins (user_id);
CREATE INDEX idx_logins_expires_at ON logins (expires_at);
