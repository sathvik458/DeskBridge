package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	SessionActive    = "active"
	SessionPaused    = "paused"
	SessionCompleted = "completed"
	SessionAbandoned = "abandoned"
)

var (
	ErrSessionRunning = errors.New("a session is already running")
	ErrBadTransition  = errors.New("not allowed in this state")
)

type Session struct {
	ID                 string
	UserID             string
	Subject            string
	Topic              *string
	Status             string
	StartedAt          time.Time
	EndedAt            *time.Time
	AccumulatedSeconds int64
	LastResumedAt      *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Elapsed is what the whole design turns on: banked seconds, plus the time since
// the last resume only while the session is actually running.
func (s Session) Elapsed(now time.Time) time.Duration {
	elapsed := time.Duration(s.AccumulatedSeconds) * time.Second

	if s.Status == SessionActive && s.LastResumedAt != nil {
		if extra := now.Sub(*s.LastResumedAt); extra > 0 {
			elapsed += extra
		}
	}

	return elapsed
}

func (s Session) IsLive() bool {
	return s.Status == SessionActive || s.Status == SessionPaused
}

const sessionColumns = `id, user_id, subject, topic, status, started_at, ended_at,
	accumulated_seconds, last_resumed_at, created_at, updated_at`

func (s *Store) StartSession(ctx context.Context, session Session) (Session, error) {
	if _, err := s.CurrentSession(ctx, session.UserID); err == nil {
		return Session{}, ErrSessionRunning
	} else if !errors.Is(err, ErrNotFound) {
		return Session{}, err
	}

	now := s.now()
	session.Status = SessionActive
	session.StartedAt = now
	session.LastResumedAt = &now
	session.AccumulatedSeconds = 0
	session.CreatedAt = now
	session.UpdatedAt = now

	_, err := s.db.ExecContext(ctx,
		"INSERT INTO study_sessions ("+sessionColumns+") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		session.ID, session.UserID, session.Subject, session.Topic, session.Status,
		formatTime(session.StartedAt), nil, session.AccumulatedSeconds, formatTime(now),
		formatTime(session.CreatedAt), formatTime(session.UpdatedAt),
	)
	if err != nil {
		return Session{}, fmt.Errorf("starting session: %w", err)
	}

	return s.Session(ctx, session.ID)
}

func (s *Store) Session(ctx context.Context, id string) (Session, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+sessionColumns+" FROM study_sessions WHERE id = ?", id)

	session, err := scanSession(row)
	if err != nil {
		return Session{}, fmt.Errorf("looking up session %s: %w", id, err)
	}

	return session, nil
}

func (s *Store) CurrentSession(ctx context.Context, userID string) (Session, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT "+sessionColumns+` FROM study_sessions
		 WHERE user_id = ? AND status IN (?, ?) ORDER BY started_at DESC LIMIT 1`,
		userID, SessionActive, SessionPaused)

	session, err := scanSession(row)
	if err != nil {
		return Session{}, fmt.Errorf("looking up the current session: %w", err)
	}

	return session, nil
}

func (s *Store) Sessions(ctx context.Context, userID string, limit int) ([]Session, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx,
		"SELECT "+sessionColumns+" FROM study_sessions WHERE user_id = ? ORDER BY started_at DESC LIMIT ?",
		userID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}
	defer rows.Close()

	sessions := []Session{}

	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("listing sessions: %w", err)
		}
		sessions = append(sessions, session)
	}

	return sessions, rows.Err()
}

func (s *Store) PauseSession(ctx context.Context, id string) (Session, error) {
	return s.transition(ctx, id, func(session *Session, now time.Time) error {
		if session.Status != SessionActive {
			return fmt.Errorf("pausing a %s session: %w", session.Status, ErrBadTransition)
		}

		session.AccumulatedSeconds = int64(session.Elapsed(now).Seconds())
		session.LastResumedAt = nil
		session.Status = SessionPaused

		return nil
	})
}

func (s *Store) ResumeSession(ctx context.Context, id string) (Session, error) {
	return s.transition(ctx, id, func(session *Session, now time.Time) error {
		if session.Status != SessionPaused {
			return fmt.Errorf("resuming a %s session: %w", session.Status, ErrBadTransition)
		}

		session.LastResumedAt = &now
		session.Status = SessionActive

		return nil
	})
}

func (s *Store) EndSession(ctx context.Context, id string) (Session, error) {
	return s.transition(ctx, id, func(session *Session, now time.Time) error {
		if !session.IsLive() {
			return fmt.Errorf("ending a %s session: %w", session.Status, ErrBadTransition)
		}

		session.AccumulatedSeconds = int64(session.Elapsed(now).Seconds())
		session.LastResumedAt = nil
		session.EndedAt = &now
		session.Status = SessionCompleted

		return nil
	})
}

// The row is read and written inside one transaction so two clicks arriving together
// cannot both bank the same stretch of time.
func (s *Store) transition(ctx context.Context, id string, apply func(*Session, time.Time) error) (Session, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Session{}, fmt.Errorf("session %s: starting transaction: %w", id, err)
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(ctx, "SELECT "+sessionColumns+" FROM study_sessions WHERE id = ?", id)

	session, err := scanSession(row)
	if err != nil {
		return Session{}, fmt.Errorf("session %s: %w", id, err)
	}

	now := s.now()
	if err := apply(&session, now); err != nil {
		return Session{}, err
	}

	session.UpdatedAt = now

	var endedAt, lastResumedAt any
	if session.EndedAt != nil {
		endedAt = formatTime(*session.EndedAt)
	}
	if session.LastResumedAt != nil {
		lastResumedAt = formatTime(*session.LastResumedAt)
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE study_sessions
		SET status = ?, ended_at = ?, accumulated_seconds = ?, last_resumed_at = ?, updated_at = ?
		WHERE id = ?`,
		session.Status, endedAt, session.AccumulatedSeconds, lastResumedAt,
		formatTime(session.UpdatedAt), id)
	if err != nil {
		return Session{}, fmt.Errorf("session %s: updating: %w", id, err)
	}

	if err := tx.Commit(); err != nil {
		return Session{}, fmt.Errorf("session %s: committing: %w", id, err)
	}

	return session, nil
}

func scanSession(src scanner) (Session, error) {
	var (
		session       Session
		topic         sql.NullString
		endedAt       sql.NullString
		lastResumedAt sql.NullString
		startedAt     string
		createdAt     string
		updatedAt     string
	)

	err := src.Scan(&session.ID, &session.UserID, &session.Subject, &topic, &session.Status,
		&startedAt, &endedAt, &session.AccumulatedSeconds, &lastResumedAt, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, err
	}

	session.Topic = optionalString(topic)

	if session.EndedAt, err = optionalTime(endedAt); err != nil {
		return Session{}, err
	}
	if session.LastResumedAt, err = optionalTime(lastResumedAt); err != nil {
		return Session{}, err
	}
	if session.StartedAt, err = parseTime(startedAt); err != nil {
		return Session{}, err
	}
	if session.CreatedAt, err = parseTime(createdAt); err != nil {
		return Session{}, err
	}
	if session.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return Session{}, err
	}

	return session, nil
}
