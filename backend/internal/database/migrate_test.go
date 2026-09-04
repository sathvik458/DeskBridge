package database

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"testing"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func tableNames(t *testing.T, db *sql.DB) map[string]bool {
	t.Helper()

	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatalf("listing tables: %v", err)
	}
	defer rows.Close()

	names := make(map[string]bool)

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scanning table name: %v", err)
		}
		names[name] = true
	}

	return names
}

func TestMigrateCreatesEverySchemaTable(t *testing.T) {
	db := newTestDB(t)

	if err := Migrate(context.Background(), db, discardLogger()); err != nil {
		t.Fatalf("Migrate() returned an unexpected error: %v", err)
	}

	names := tableNames(t, db)

	for _, want := range []string{
		"schema_migrations", "users", "devices", "study_sessions",
		"study_goals", "messages", "files", "events", "board_marks",
	} {
		if !names[want] {
			t.Errorf("table %q was not created", want)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	if err := Migrate(ctx, db, discardLogger()); err != nil {
		t.Fatalf("first Migrate() failed: %v", err)
	}

	var afterFirst int
	if err := db.QueryRow("SELECT count(*) FROM schema_migrations").Scan(&afterFirst); err != nil {
		t.Fatalf("counting migrations: %v", err)
	}

	if err := Migrate(ctx, db, discardLogger()); err != nil {
		t.Fatalf("second Migrate() failed: %v", err)
	}

	var afterSecond int
	if err := db.QueryRow("SELECT count(*) FROM schema_migrations").Scan(&afterSecond); err != nil {
		t.Fatalf("counting migrations: %v", err)
	}

	if afterFirst != afterSecond {
		t.Errorf("migration count changed on re-run: %d then %d", afterFirst, afterSecond)
	}

	if afterFirst == 0 {
		t.Error("no migrations were recorded")
	}
}

func TestForeignKeysAreEnforced(t *testing.T) {
	db := newTestDB(t)

	if err := Migrate(context.Background(), db, discardLogger()); err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	_, err := db.Exec(`
		INSERT INTO study_sessions (id, user_id, subject, status, started_at, created_at, updated_at)
		VALUES ('s1', 'nobody', 'Physics', 'active', '2026-08-12T10:00:00Z', '2026-08-12T10:00:00Z', '2026-08-12T10:00:00Z')`)

	if err == nil {
		t.Fatal("inserted a session referencing a non-existent user, want a foreign key error")
	}
}

func TestCheckConstraintsRejectBadEnums(t *testing.T) {
	db := newTestDB(t)

	if err := Migrate(context.Background(), db, discardLogger()); err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	_, err := db.Exec(`
		INSERT INTO users (id, username, display_name, role, created_at, updated_at)
		VALUES ('u1', 'ricky', 'Ricky', 'teacher', '2026-08-12T10:00:00Z', '2026-08-12T10:00:00Z')`)

	if err == nil {
		t.Fatal("inserted a user with role 'teacher', want a check constraint error")
	}
}

func TestLoadMigrationsAreOrderedAndUnique(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations() returned an unexpected error: %v", err)
	}

	if len(migrations) == 0 {
		t.Fatal("no migrations were embedded")
	}

	for i, m := range migrations {
		if m.version != i+1 {
			t.Errorf("migration %d has version %d, want %d - versions must be contiguous from 1", i, m.version, i+1)
		}
		if m.script == "" {
			t.Errorf("migration %d (%s) is empty", m.version, m.name)
		}
	}
}

func TestParseMigrationName(t *testing.T) {
	tests := []struct {
		filename    string
		wantVersion int
		wantName    string
		wantErr     bool
	}{
		{filename: "0001_initial_schema.sql", wantVersion: 1, wantName: "initial_schema"},
		{filename: "0042_add_widgets.sql", wantVersion: 42, wantName: "add_widgets"},
		{filename: "no_number.sql", wantErr: true},
		{filename: "0001.sql", wantErr: true},
		{filename: "0000_zero.sql", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.filename, func(t *testing.T) {
			version, name, err := parseMigrationName(tc.filename)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseMigrationName(%q) succeeded, want an error", tc.filename)
				}
				return
			}

			if err != nil {
				t.Fatalf("parseMigrationName(%q) returned an unexpected error: %v", tc.filename, err)
			}
			if version != tc.wantVersion {
				t.Errorf("version = %d, want %d", version, tc.wantVersion)
			}
			if name != tc.wantName {
				t.Errorf("name = %q, want %q", name, tc.wantName)
			}
		})
	}
}
