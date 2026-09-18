CREATE INDEX idx_study_sessions_user_started ON study_sessions (user_id, started_at);

DROP INDEX idx_study_sessions_user_id;
DROP INDEX idx_study_sessions_started_at;
