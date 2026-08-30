package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const DateLayout = "2006-01-02"

type Goal struct {
	ID            string
	UserID        string
	SessionID     *string
	Subject       string
	Topic         *string
	TargetMinutes int
	GoalDate      string
	CompletedAt   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (g Goal) Done() bool {
	return g.CompletedAt != nil
}

const goalColumns = `id, user_id, session_id, subject, topic, target_minutes,
	goal_date, completed_at, created_at, updated_at`

func (s *Store) CreateGoal(ctx context.Context, goal Goal) (Goal, error) {
	now := s.now()
	goal.CreatedAt = now
	goal.UpdatedAt = now

	_, err := s.db.ExecContext(ctx,
		"INSERT INTO study_goals ("+goalColumns+") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		goal.ID, goal.UserID, goal.SessionID, goal.Subject, goal.Topic,
		goal.TargetMinutes, goal.GoalDate, nil,
		formatTime(goal.CreatedAt), formatTime(goal.UpdatedAt),
	)
	if err != nil {
		return Goal{}, fmt.Errorf("creating goal: %w", err)
	}

	return s.Goal(ctx, goal.ID)
}

func (s *Store) Goal(ctx context.Context, id string) (Goal, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+goalColumns+" FROM study_goals WHERE id = ?", id)

	goal, err := scanGoal(row)
	if err != nil {
		return Goal{}, fmt.Errorf("looking up goal %s: %w", id, err)
	}

	return goal, nil
}

func (s *Store) GoalsOn(ctx context.Context, userID, date string) ([]Goal, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+goalColumns+` FROM study_goals
		 WHERE user_id = ? AND goal_date = ? ORDER BY created_at`,
		userID, date)
	if err != nil {
		return nil, fmt.Errorf("listing goals for %s: %w", date, err)
	}
	defer rows.Close()

	goals := []Goal{}

	for rows.Next() {
		goal, err := scanGoal(rows)
		if err != nil {
			return nil, fmt.Errorf("listing goals for %s: %w", date, err)
		}
		goals = append(goals, goal)
	}

	return goals, rows.Err()
}

func (s *Store) UpdateGoal(ctx context.Context, goal Goal) (Goal, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE study_goals
		SET subject = ?, topic = ?, target_minutes = ?, goal_date = ?, updated_at = ?
		WHERE id = ?`,
		goal.Subject, goal.Topic, goal.TargetMinutes, goal.GoalDate,
		formatTime(s.now()), goal.ID)
	if err != nil {
		return Goal{}, fmt.Errorf("updating goal %s: %w", goal.ID, err)
	}

	if err := mustHaveChangedARow(result, goal.ID); err != nil {
		return Goal{}, err
	}

	return s.Goal(ctx, goal.ID)
}

func (s *Store) SetGoalDone(ctx context.Context, id string, done bool) (Goal, error) {
	now := s.now()

	var completedAt any
	if done {
		completedAt = formatTime(now)
	}

	result, err := s.db.ExecContext(ctx,
		"UPDATE study_goals SET completed_at = ?, updated_at = ? WHERE id = ?",
		completedAt, formatTime(now), id)
	if err != nil {
		return Goal{}, fmt.Errorf("marking goal %s: %w", id, err)
	}

	if err := mustHaveChangedARow(result, id); err != nil {
		return Goal{}, err
	}

	return s.Goal(ctx, id)
}

func (s *Store) DeleteGoal(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM study_goals WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting goal %s: %w", id, err)
	}

	return mustHaveChangedARow(result, id)
}

func mustHaveChangedARow(result sql.Result, id string) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("goal %s: %w", id, err)
	}

	if affected == 0 {
		return fmt.Errorf("goal %s: %w", id, ErrNotFound)
	}

	return nil
}

func scanGoal(src scanner) (Goal, error) {
	var (
		goal        Goal
		sessionID   sql.NullString
		topic       sql.NullString
		completedAt sql.NullString
		createdAt   string
		updatedAt   string
	)

	err := src.Scan(&goal.ID, &goal.UserID, &sessionID, &goal.Subject, &topic,
		&goal.TargetMinutes, &goal.GoalDate, &completedAt, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Goal{}, ErrNotFound
	}
	if err != nil {
		return Goal{}, err
	}

	goal.SessionID = optionalString(sessionID)
	goal.Topic = optionalString(topic)

	if goal.CompletedAt, err = optionalTime(completedAt); err != nil {
		return Goal{}, err
	}
	if goal.CreatedAt, err = parseTime(createdAt); err != nil {
		return Goal{}, err
	}
	if goal.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return Goal{}, err
	}

	return goal, nil
}
