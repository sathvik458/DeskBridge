package database

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()

	path := filepath.Join(t.TempDir(), "test.db")

	db, err := Open(context.Background(), path, time.Second)
	if err != nil {
		t.Fatalf("Open() returned an unexpected error: %v", err)
	}

	t.Cleanup(func() { db.Close() })

	return db
}

func TestOpenEnablesWAL(t *testing.T) {
	db := newTestDB(t)

	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("reading journal_mode: %v", err)
	}

	if mode != "wal" {
		t.Errorf("journal_mode = %q, want %q", mode, "wal")
	}
}

func TestOpenEnablesForeignKeysOnEveryConnection(t *testing.T) {
	db := newTestDB(t)

	for i := 0; i < 8; i++ {
		conn, err := db.Conn(context.Background())
		if err != nil {
			t.Fatalf("opening connection %d: %v", i, err)
		}

		var enabled int
		if err := conn.QueryRowContext(context.Background(), "PRAGMA foreign_keys").Scan(&enabled); err != nil {
			t.Fatalf("reading foreign_keys on connection %d: %v", i, err)
		}

		if enabled != 1 {
			t.Errorf("connection %d has foreign_keys off", i)
		}

		conn.Close()
	}
}

func TestOpenRejectsUnwritablePath(t *testing.T) {
	_, err := Open(context.Background(), filepath.Join(t.TempDir(), "no-such-dir", "test.db"), time.Second)
	if err == nil {
		t.Fatal("Open() succeeded on an unwritable path, want an error")
	}
}
