CREATE TABLE notices (
    id      TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind    TEXT NOT NULL,
    tag     TEXT NOT NULL,
    body    TEXT NOT NULL,
    times   INTEGER NOT NULL DEFAULT 1 CHECK (times > 0),
    made_at TEXT NOT NULL,
    seen_at TEXT
) STRICT;

CREATE INDEX idx_notices_user_made ON notices (user_id, made_at);

CREATE INDEX idx_notices_open ON notices (user_id, tag) WHERE seen_at IS NULL;
