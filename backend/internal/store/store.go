// Package store reads and writes Deskbridge data in SQLite.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrNotFound lets callers handle a missing row without importing database/sql.
var ErrNotFound = errors.New("not found")

type Store struct {
	db  *sql.DB
	now func() time.Time
}

func New(db *sql.DB) *Store {
	return &Store{
		db:  db,
		now: func() time.Time { return time.Now().UTC() },
	}
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func parseTime(value string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing timestamp %q: %w", value, err)
	}
	return t, nil
}

func optionalTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}

	t, err := parseTime(value.String)
	if err != nil {
		return nil, err
	}

	return &t, nil
}

func optionalString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}
