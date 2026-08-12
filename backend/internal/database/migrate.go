package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

const createMigrationsTable = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    applied_at TEXT NOT NULL
) STRICT;`

type migration struct {
	version int
	name    string
	script  string
}

// Migrate applies every migration that has not run yet, oldest first.
func Migrate(ctx context.Context, db *sql.DB, log *slog.Logger) error {
	if _, err := db.ExecContext(ctx, createMigrationsTable); err != nil {
		return fmt.Errorf("creating schema_migrations: %w", err)
	}

	migrations, err := loadMigrations()
	if err != nil {
		return err
	}

	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return err
	}

	for _, m := range migrations {
		if applied[m.version] {
			continue
		}

		if err := apply(ctx, db, m); err != nil {
			return err
		}

		log.Info("migration applied", "version", m.version, "name", m.name)
	}

	return nil
}

// The schema change and its bookkeeping row share one transaction so they cannot disagree.
func apply(ctx context.Context, db *sql.DB, m migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("migration %d: starting transaction: %w", m.version, err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, m.script); err != nil {
		return fmt.Errorf("migration %d (%s): %w", m.version, m.name, err)
	}

	appliedAt := time.Now().UTC().Format(time.RFC3339)

	_, err = tx.ExecContext(ctx,
		"INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)",
		m.version, m.name, appliedAt,
	)
	if err != nil {
		return fmt.Errorf("migration %d: recording version: %w", m.version, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("migration %d: committing: %w", m.version, err)
	}

	return nil
}

func appliedVersions(ctx context.Context, db *sql.DB) (map[int]bool, error) {
	rows, err := db.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("reading applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]bool)

	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scanning applied migration: %w", err)
		}
		applied[version] = true
	}

	return applied, rows.Err()
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, fmt.Errorf("reading migrations directory: %w", err)
	}

	var migrations []migration
	seen := make(map[int]string)

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		version, name, err := parseMigrationName(entry.Name())
		if err != nil {
			return nil, err
		}

		if clash, ok := seen[version]; ok {
			return nil, fmt.Errorf("two migrations share version %d: %s and %s", version, clash, entry.Name())
		}
		seen[version] = entry.Name()

		script, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", entry.Name(), err)
		}

		migrations = append(migrations, migration{version, name, string(script)})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	return migrations, nil
}

func parseMigrationName(filename string) (int, string, error) {
	base := strings.TrimSuffix(filename, ".sql")

	prefix, name, found := strings.Cut(base, "_")
	if !found || name == "" {
		return 0, "", fmt.Errorf("migration %q must be named NNNN_description.sql", filename)
	}

	version, err := strconv.Atoi(prefix)
	if err != nil {
		return 0, "", fmt.Errorf("migration %q has a non-numeric version: %w", filename, err)
	}

	if version < 1 {
		return 0, "", fmt.Errorf("migration %q must have a version of 1 or more", filename)
	}

	return version, name, nil
}
