-- StartSession used to check and then insert as two separate steps, so two quick
-- clicks could leave a person with two live sessions. Any such pair already in a
-- database is settled first, or the unique index below would refuse to build.
-- The newest stays; the rest are abandoned without counting their open stretch,
-- so no study time is invented for a session nobody was really running.
UPDATE study_sessions
SET status          = 'abandoned',
    ended_at        = strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
    last_resumed_at = NULL,
    updated_at      = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
WHERE status IN ('active', 'paused')
  AND id NOT IN (
      SELECT id FROM (
          SELECT id, ROW_NUMBER() OVER (
              PARTITION BY user_id ORDER BY started_at DESC, id DESC
          ) AS place
          FROM study_sessions
          WHERE status IN ('active', 'paused')
      )
      WHERE place = 1
  );

CREATE UNIQUE INDEX idx_study_sessions_one_live
    ON study_sessions (user_id)
    WHERE status IN ('active', 'paused');
