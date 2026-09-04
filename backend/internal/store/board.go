package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	MarkDraw  = "draw"
	MarkErase = "erase"
	MarkClear = "clear"
)

// Mark is one entry in the board's log. The board is never edited in place - a
// rubbed-out stroke is a later mark that points back at it.
type Mark struct {
	Seq       int64
	ID        string
	AuthorID  string
	Kind      string
	TargetID  *string
	Ink       *string
	Thickness *float64
	Path      []float64
	CreatedAt time.Time
}

const markColumns = `seq, id, author_id, kind, target_id, ink, thickness, path, created_at`

func (s *Store) AddMark(ctx context.Context, mark Mark) (Mark, error) {
	mark.CreatedAt = s.now()

	var encoded any
	if mark.Kind == MarkDraw {
		packed, err := packPath(mark.Path)
		if err != nil {
			return Mark{}, err
		}
		encoded = packed
	}

	result, err := s.db.ExecContext(ctx, `
		INSERT INTO board_marks (id, author_id, kind, target_id, ink, thickness, path, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		mark.ID, mark.AuthorID, mark.Kind, mark.TargetID, mark.Ink, mark.Thickness,
		encoded, formatTime(mark.CreatedAt),
	)
	if err != nil {
		return Mark{}, fmt.Errorf("adding a board mark: %w", err)
	}

	// The sequence is assigned by the database, so it has to be read back rather
	// than guessed - it is the cursor every client syncs against.
	seq, err := result.LastInsertId()
	if err != nil {
		return Mark{}, fmt.Errorf("reading the mark sequence: %w", err)
	}

	mark.Seq = seq

	return mark, nil
}

// MarksSince returns everything after a sequence number, oldest first, which is
// both the steady-state update and the catch-up after a disconnect.
func (s *Store) MarksSince(ctx context.Context, since int64, limit int) ([]Mark, error) {
	if limit <= 0 || limit > 5000 {
		limit = 2000
	}

	rows, err := s.db.QueryContext(ctx,
		"SELECT "+markColumns+" FROM board_marks WHERE seq > ? ORDER BY seq LIMIT ?", since, limit)
	if err != nil {
		return nil, fmt.Errorf("listing board marks: %w", err)
	}
	defer rows.Close()

	marks := []Mark{}

	for rows.Next() {
		mark, err := scanMark(rows)
		if err != nil {
			return nil, fmt.Errorf("listing board marks: %w", err)
		}
		marks = append(marks, mark)
	}

	return marks, rows.Err()
}

func (s *Store) LatestMarkSeq(ctx context.Context) (int64, error) {
	var seq int64

	err := s.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(seq), 0) FROM board_marks").Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("reading the latest mark sequence: %w", err)
	}

	return seq, nil
}

func (s *Store) MarkExists(ctx context.Context, id string) (bool, error) {
	var found int

	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM board_marks WHERE id = ? AND kind = ?", id, MarkDraw).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("looking up mark %s: %w", id, err)
	}

	return found > 0, nil
}

// Marks older than the newest clear can never be drawn again, so they are the one
// part of the log that is safe to actually delete.
func (s *Store) TrimBoard(ctx context.Context) (int, error) {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM board_marks
		WHERE seq < (SELECT COALESCE(MAX(seq), 0) FROM board_marks WHERE kind = ?)`, MarkClear)
	if err != nil {
		return 0, fmt.Errorf("trimming the board: %w", err)
	}

	removed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("trimming the board: %w", err)
	}

	return int(removed), nil
}

func packPath(points []float64) (string, error) {
	if len(points) == 0 {
		return "", errors.New("a stroke needs at least one point")
	}
	if len(points)%2 != 0 {
		return "", fmt.Errorf("a path needs an even number of numbers, got %d", len(points))
	}

	encoded, err := json.Marshal(points)
	if err != nil {
		return "", fmt.Errorf("encoding the path: %w", err)
	}

	return string(encoded), nil
}

func unpackPath(raw sql.NullString) ([]float64, error) {
	if !raw.Valid {
		return nil, nil
	}

	var points []float64
	if err := json.Unmarshal([]byte(raw.String), &points); err != nil {
		return nil, fmt.Errorf("decoding the path: %w", err)
	}

	return points, nil
}

func scanMark(src scanner) (Mark, error) {
	var (
		mark      Mark
		targetID  sql.NullString
		ink       sql.NullString
		thickness sql.NullFloat64
		path      sql.NullString
		createdAt string
	)

	err := src.Scan(&mark.Seq, &mark.ID, &mark.AuthorID, &mark.Kind, &targetID,
		&ink, &thickness, &path, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Mark{}, ErrNotFound
	}
	if err != nil {
		return Mark{}, err
	}

	mark.TargetID = optionalString(targetID)
	mark.Ink = optionalString(ink)

	if thickness.Valid {
		mark.Thickness = &thickness.Float64
	}

	if mark.Path, err = unpackPath(path); err != nil {
		return Mark{}, err
	}
	if mark.CreatedAt, err = parseTime(createdAt); err != nil {
		return Mark{}, err
	}

	return mark, nil
}
