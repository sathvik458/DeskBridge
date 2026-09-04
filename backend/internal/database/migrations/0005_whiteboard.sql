CREATE TABLE board_marks (
    seq        INTEGER PRIMARY KEY AUTOINCREMENT,
    id         TEXT NOT NULL UNIQUE,
    author_id  TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind       TEXT NOT NULL CHECK (kind IN ('draw', 'erase', 'clear')),
    target_id  TEXT,
    ink        TEXT,
    thickness  REAL CHECK (thickness IS NULL OR thickness > 0),
    path       TEXT,
    created_at TEXT NOT NULL,

    CHECK (kind <> 'draw'  OR (path IS NOT NULL AND ink IS NOT NULL AND thickness IS NOT NULL)),
    CHECK (kind <> 'erase' OR target_id IS NOT NULL),
    CHECK (kind <> 'clear' OR (target_id IS NULL AND path IS NULL))
) STRICT;

CREATE INDEX idx_board_marks_created_at ON board_marks (created_at);
