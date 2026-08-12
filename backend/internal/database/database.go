// Package database opens the SQLite database and applies schema migrations.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Open connects to the SQLite file at path and checks the pragmas took effect.
func Open(ctx context.Context, path string, busyTimeout time.Duration) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn(path, busyTimeout))
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}

	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(time.Hour)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connecting to %s: %w", path, err)
	}

	if err := checkPragmas(ctx, db); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

// Pragmas go in the connection string because they are per-connection and the pool opens many.
func dsn(path string, busyTimeout time.Duration) string {
	return fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(%d)",
		path, busyTimeout.Milliseconds(),
	)
}

func checkPragmas(ctx context.Context, db *sql.DB) error {
	var journalMode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		return fmt.Errorf("reading journal_mode: %w", err)
	}
	if journalMode != "wal" {
		return fmt.Errorf("journal_mode is %q, want wal", journalMode)
	}

	var foreignKeys int
	if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		return fmt.Errorf("reading foreign_keys: %w", err)
	}
	if foreignKeys != 1 {
		return fmt.Errorf("foreign keys are not enabled")
	}

	return nil
}
