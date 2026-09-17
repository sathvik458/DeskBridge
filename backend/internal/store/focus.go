package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	FocusWork       = "focus"
	FocusShortBreak = "short_break"
	FocusLongBreak  = "long_break"

	FocusFinished = "finished"
	FocusStopped  = "stopped"
)

var ErrFocusRunning = errors.New("a focus run is already going")

type FocusRun struct {
	ID        string
	UserID    string
	SessionID *string
	Kind      string
	Planned   int64
	StartedAt time.Time
	EndedAt   *time.Time
	Outcome   *string
}

func (f FocusRun) Running() bool {
	return f.EndedAt == nil
}

// Left is how much of the planned stretch remains. It never goes negative, so a
// run left open overnight reads as done rather than as a huge negative number.
func (f FocusRun) Left(now time.Time) time.Duration {
	if !f.Running() {
		return 0
	}

	spent := now.Sub(f.StartedAt)
	if spent < 0 {
		spent = 0
	}

	left := time.Duration(f.Planned)*time.Second - spent
	if left < 0 {
		return 0
	}

	return left
}

const focusCols = "id, user_id, session_id, kind, planned_secs, started_at, ended_at, outcome"

// StartFocus refuses while one is already running, the same way StartSession does.
func (s *Store) StartFocus(ctx context.Context, run FocusRun) (FocusRun, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return FocusRun{}, fmt.Errorf("starting a focus run: %w", err)
	}
	defer tx.Rollback()

	var open int
	err = tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM focus_runs WHERE user_id = ? AND ended_at IS NULL", run.UserID).Scan(&open)
	if err != nil {
		return FocusRun{}, fmt.Errorf("checking for an open focus run: %w", err)
	}
	if open > 0 {
		return FocusRun{}, ErrFocusRunning
	}

	run.StartedAt = s.now()

	_, err = tx.ExecContext(ctx,
		"INSERT INTO focus_runs ("+focusCols+") VALUES (?, ?, ?, ?, ?, ?, NULL, NULL)",
		run.ID, run.UserID, run.SessionID, run.Kind, run.Planned, formatTime(run.StartedAt))
	if err != nil {
		return FocusRun{}, fmt.Errorf("starting a focus run: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return FocusRun{}, fmt.Errorf("starting a focus run: %w", err)
	}

	return run, nil
}

func (s *Store) CurrentFocus(ctx context.Context, userID string) (FocusRun, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT "+focusCols+" FROM focus_runs WHERE user_id = ? AND ended_at IS NULL", userID)

	run, err := scanFocus(row)
	if err != nil {
		return FocusRun{}, fmt.Errorf("looking up the current focus run: %w", err)
	}

	return run, nil
}

// CloseFocus is one statement so two clicks cannot close the same run twice. The
// row count is what decides whether this caller was the one that closed it.
func (s *Store) CloseFocus(ctx context.Context, id, outcome string) (FocusRun, error) {
	res, err := s.db.ExecContext(ctx,
		"UPDATE focus_runs SET ended_at = ?, outcome = ? WHERE id = ? AND ended_at IS NULL",
		formatTime(s.now()), outcome, id)
	if err != nil {
		return FocusRun{}, fmt.Errorf("closing focus run %s: %w", id, err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return FocusRun{}, fmt.Errorf("closing focus run %s: %w", id, err)
	}
	if n == 0 {
		return FocusRun{}, fmt.Errorf("focus run %s: %w", id, ErrNotFound)
	}

	return s.FocusRun(ctx, id)
}

func (s *Store) FocusRun(ctx context.Context, id string) (FocusRun, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+focusCols+" FROM focus_runs WHERE id = ?", id)

	run, err := scanFocus(row)
	if err != nil {
		return FocusRun{}, fmt.Errorf("looking up focus run %s: %w", id, err)
	}

	return run, nil
}

func (s *Store) FinishedFocusBetween(ctx context.Context, userID string, from, to time.Time) (int, error) {
	var n int

	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM focus_runs
		WHERE user_id = ? AND kind = ? AND outcome = ? AND started_at >= ? AND started_at < ?`,
		userID, FocusWork, FocusFinished, formatTime(from), formatTime(to)).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("counting finished focus runs: %w", err)
	}

	return n, nil
}

func scanFocus(src scanner) (FocusRun, error) {
	var (
		run       FocusRun
		sessionID sql.NullString
		startedAt string
		endedAt   sql.NullString
		outcome   sql.NullString
	)

	err := src.Scan(&run.ID, &run.UserID, &sessionID, &run.Kind, &run.Planned,
		&startedAt, &endedAt, &outcome)
	if errors.Is(err, sql.ErrNoRows) {
		return FocusRun{}, ErrNotFound
	}
	if err != nil {
		return FocusRun{}, err
	}

	run.SessionID = optionalString(sessionID)
	run.Outcome = optionalString(outcome)

	if run.StartedAt, err = parseTime(startedAt); err != nil {
		return FocusRun{}, err
	}
	if run.EndedAt, err = optionalTime(endedAt); err != nil {
		return FocusRun{}, err
	}

	return run, nil
}
