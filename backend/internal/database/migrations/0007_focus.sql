CREATE TABLE focus_runs (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    session_id  TEXT REFERENCES study_sessions (id) ON DELETE SET NULL,
    kind        TEXT NOT NULL CHECK (kind IN ('focus', 'short_break', 'long_break')),
    planned_secs INTEGER NOT NULL CHECK (planned_secs > 0),
    started_at  TEXT NOT NULL,
    ended_at    TEXT,
    outcome     TEXT CHECK (outcome IS NULL OR outcome IN ('finished', 'stopped')),

    CHECK ((ended_at IS NULL) = (outcome IS NULL))
) STRICT;

CREATE INDEX idx_focus_runs_user_started ON focus_runs (user_id, started_at);
CREATE INDEX idx_focus_runs_open ON focus_runs (user_id) WHERE ended_at IS NULL;
