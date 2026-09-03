package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	ShelfNotes     = "notes"
	ShelfHomework  = "homework"
	ShelfImages    = "images"
	ShelfDocuments = "documents"
	ShelfResources = "resources"
	ShelfOther     = "other"
)

// Shelves is the whole set the check constraint on the table allows.
var Shelves = []string{ShelfNotes, ShelfHomework, ShelfImages, ShelfDocuments, ShelfResources, ShelfOther}

func IsShelf(name string) bool {
	for _, shelf := range Shelves {
		if shelf == name {
			return true
		}
	}
	return false
}

type File struct {
	ID           string
	UploaderID   string
	Category     string
	OriginalName string
	StoredPath   string
	SizeBytes    int64
	Checksum     string
	CreatedAt    time.Time
}

const fileColumns = `id, uploader_id, category, original_name, stored_path,
	size_bytes, checksum_sha256, created_at`

func (s *Store) RecordUpload(ctx context.Context, file File) (File, error) {
	file.CreatedAt = s.now()

	_, err := s.db.ExecContext(ctx,
		"INSERT INTO files ("+fileColumns+") VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		file.ID, file.UploaderID, file.Category, file.OriginalName, file.StoredPath,
		file.SizeBytes, file.Checksum, formatTime(file.CreatedAt),
	)
	if err != nil {
		return File{}, fmt.Errorf("recording upload: %w", err)
	}

	return s.File(ctx, file.ID)
}

func (s *Store) File(ctx context.Context, id string) (File, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+fileColumns+" FROM files WHERE id = ?", id)

	file, err := scanFile(row)
	if err != nil {
		return File{}, fmt.Errorf("looking up file %s: %w", id, err)
	}

	return file, nil
}

// An empty category means every shelf. Newest first, since the file someone just
// shared is the one the other person is looking for.
func (s *Store) Files(ctx context.Context, category string) ([]File, error) {
	query := "SELECT " + fileColumns + " FROM files"
	args := []any{}

	if category != "" {
		query += " WHERE category = ?"
		args = append(args, category)
	}

	query += " ORDER BY created_at DESC, id DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing files: %w", err)
	}
	defer rows.Close()

	files := []File{}

	for rows.Next() {
		file, err := scanFile(rows)
		if err != nil {
			return nil, fmt.Errorf("listing files: %w", err)
		}
		files = append(files, file)
	}

	return files, rows.Err()
}

// ForgetFile hands back the row it deleted, because the caller needs the stored path
// to remove the bytes and a separate lookup could race another delete.
func (s *Store) ForgetFile(ctx context.Context, id string) (File, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return File{}, fmt.Errorf("file %s: starting transaction: %w", id, err)
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(ctx, "SELECT "+fileColumns+" FROM files WHERE id = ?", id)

	file, err := scanFile(row)
	if err != nil {
		return File{}, fmt.Errorf("file %s: %w", id, err)
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM files WHERE id = ?", id); err != nil {
		return File{}, fmt.Errorf("deleting file %s: %w", id, err)
	}

	if err := tx.Commit(); err != nil {
		return File{}, fmt.Errorf("file %s: committing: %w", id, err)
	}

	return file, nil
}

func (s *Store) FileTally(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT category, COUNT(*) FROM files GROUP BY category")
	if err != nil {
		return nil, fmt.Errorf("counting files: %w", err)
	}
	defer rows.Close()

	tally := map[string]int{}

	for rows.Next() {
		var (
			shelf string
			count int
		)
		if err := rows.Scan(&shelf, &count); err != nil {
			return nil, fmt.Errorf("counting files: %w", err)
		}
		tally[shelf] = count
	}

	return tally, rows.Err()
}

func scanFile(src scanner) (File, error) {
	var (
		file      File
		createdAt string
	)

	err := src.Scan(&file.ID, &file.UploaderID, &file.Category, &file.OriginalName,
		&file.StoredPath, &file.SizeBytes, &file.Checksum, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return File{}, ErrNotFound
	}
	if err != nil {
		return File{}, err
	}

	if file.CreatedAt, err = parseTime(createdAt); err != nil {
		return File{}, err
	}

	return file, nil
}
