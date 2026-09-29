-- Only the exceptions are stored. A kind nobody has muted is on, so a new event
-- kind starts working for everyone without a backfill.
CREATE TABLE notice_mutes (
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind    TEXT NOT NULL,

    PRIMARY KEY (user_id, kind)
) STRICT;

CREATE INDEX idx_notice_mutes_kind ON notice_mutes (kind);
