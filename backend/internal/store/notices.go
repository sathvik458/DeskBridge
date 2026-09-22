package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// keepNotices is how long a notice stays around before the sweeper drops it. Long
// enough to come back from a weekend, short enough that the table stays small.
const keepNotices = 30 * 24 * time.Hour

type Notice struct {
	ID     string
	UserID string
	Kind   string
	Tag    string
	Body   string
	Times  int
	MadeAt time.Time
	SeenAt *time.Time
}

const noticeCols = "id, user_id, kind, tag, body, times, made_at, seen_at"

// Note either writes a new notice or bumps the last unseen one carrying the same
// tag. Five help requests in a minute should be one line saying it happened five
// times, not five identical lines.
//
// The lookup and the write are not one statement, so two events landing at the
// same instant can both insert. That costs a duplicate notice and nothing else,
// which is cheaper than holding a write lock open for every event.
func (s *Store) Note(ctx context.Context, n Notice, within time.Duration) (Notice, error) {
	now := s.now()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Notice{}, fmt.Errorf("leaving a notice: %w", err)
	}
	defer tx.Rollback()

	var hit string

	err = tx.QueryRowContext(ctx, `
		SELECT id FROM notices
		WHERE user_id = ? AND tag = ? AND seen_at IS NULL AND made_at >= ?
		ORDER BY made_at DESC LIMIT 1`,
		n.UserID, n.Tag, formatTime(now.Add(-within))).Scan(&hit)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		n.Times = 1
		n.MadeAt = now

		_, err = tx.ExecContext(ctx,
			"INSERT INTO notices ("+noticeCols+") VALUES (?, ?, ?, ?, ?, ?, ?, NULL)",
			n.ID, n.UserID, n.Kind, n.Tag, n.Body, n.Times, formatTime(n.MadeAt))
		if err != nil {
			return Notice{}, fmt.Errorf("leaving a notice: %w", err)
		}

	case err != nil:
		return Notice{}, fmt.Errorf("looking for a notice to bump: %w", err)

	default:
		// The newest wording wins, so the line reflects the most recent event.
		_, err = tx.ExecContext(ctx,
			"UPDATE notices SET times = times + 1, body = ?, made_at = ? WHERE id = ?",
			n.Body, formatTime(now), hit)
		if err != nil {
			return Notice{}, fmt.Errorf("bumping notice %s: %w", hit, err)
		}

		n.ID = hit
	}

	if err := tx.Commit(); err != nil {
		return Notice{}, fmt.Errorf("leaving a notice: %w", err)
	}

	return s.Notice(ctx, n.ID)
}

func (s *Store) Notice(ctx context.Context, id string) (Notice, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+noticeCols+" FROM notices WHERE id = ?", id)

	n, err := scanNotice(row)
	if err != nil {
		return Notice{}, fmt.Errorf("looking up notice %s: %w", id, err)
	}

	return n, nil
}

func (s *Store) Notices(ctx context.Context, userID string, limit int) ([]Notice, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+noticeCols+" FROM notices WHERE user_id = ? ORDER BY made_at DESC LIMIT ?",
		userID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing notices: %w", err)
	}
	defer rows.Close()

	out := []Notice{}

	for rows.Next() {
		n, err := scanNotice(rows)
		if err != nil {
			return nil, fmt.Errorf("listing notices: %w", err)
		}
		out = append(out, n)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing notices: %w", err)
	}

	return out, nil
}

func (s *Store) UnseenNotices(ctx context.Context, userID string) (int, error) {
	var n int

	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM notices WHERE user_id = ? AND seen_at IS NULL", userID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("counting unseen notices: %w", err)
	}

	return n, nil
}

func (s *Store) MarkNoticesSeen(ctx context.Context, userID string) (int, error) {
	res, err := s.db.ExecContext(ctx,
		"UPDATE notices SET seen_at = ? WHERE user_id = ? AND seen_at IS NULL",
		formatTime(s.now()), userID)
	if err != nil {
		return 0, fmt.Errorf("marking notices seen: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("marking notices seen: %w", err)
	}

	return int(n), nil
}

// TrimNotices drops anything old enough that nobody is coming back for it.
func (s *Store) TrimNotices(ctx context.Context) (int, error) {
	res, err := s.db.ExecContext(ctx,
		"DELETE FROM notices WHERE made_at < ?", formatTime(s.now().Add(-keepNotices)))
	if err != nil {
		return 0, fmt.Errorf("trimming notices: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("trimming notices: %w", err)
	}

	return int(n), nil
}

func scanNotice(src scanner) (Notice, error) {
	var (
		n      Notice
		madeAt string
		seenAt sql.NullString
	)

	err := src.Scan(&n.ID, &n.UserID, &n.Kind, &n.Tag, &n.Body, &n.Times, &madeAt, &seenAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Notice{}, ErrNotFound
	}
	if err != nil {
		return Notice{}, err
	}

	if n.MadeAt, err = parseTime(madeAt); err != nil {
		return Notice{}, err
	}
	if n.SeenAt, err = optionalTime(seenAt); err != nil {
		return Notice{}, err
	}

	return n, nil
}
