package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type User struct {
	ID          string
	Username    string
	DisplayName string
	Role        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

const userColumns = "id, username, display_name, role, created_at, updated_at"

func (s *Store) CreateUser(ctx context.Context, user User) (User, error) {
	now := s.now()
	user.CreatedAt = now
	user.UpdatedAt = now

	_, err := s.db.ExecContext(ctx,
		"INSERT INTO users ("+userColumns+") VALUES (?, ?, ?, ?, ?, ?)",
		user.ID, user.Username, user.DisplayName, user.Role,
		formatTime(user.CreatedAt), formatTime(user.UpdatedAt),
	)
	if err != nil {
		return User{}, fmt.Errorf("creating user %s: %w", user.Username, err)
	}

	return user, nil
}

func (s *Store) User(ctx context.Context, id string) (User, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+userColumns+" FROM users WHERE id = ?", id)

	user, err := scanUser(row)
	if err != nil {
		return User{}, fmt.Errorf("looking up user %s: %w", id, err)
	}

	return user, nil
}

func (s *Store) UserByUsername(ctx context.Context, username string) (User, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+userColumns+" FROM users WHERE username = ?", username)

	user, err := scanUser(row)
	if err != nil {
		return User{}, fmt.Errorf("looking up user %s: %w", username, err)
	}

	return user, nil
}

// PassHash is read on its own rather than hanging off User, so a hash cannot be
// handed to a JSON encoder by accident just because someone listed the users.
func (s *Store) PassHash(ctx context.Context, id string) (string, error) {
	var h sql.NullString

	err := s.db.QueryRowContext(ctx, "SELECT pass_hash FROM users WHERE id = ?", id).Scan(&h)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("reading the password for %s: %w", id, err)
	}

	return h.String, nil
}

func (s *Store) SetPassHash(ctx context.Context, id, hash string) error {
	res, err := s.db.ExecContext(ctx,
		"UPDATE users SET pass_hash = ?, updated_at = ? WHERE id = ?",
		hash, formatTime(s.now()), id)
	if err != nil {
		return fmt.Errorf("setting the password for %s: %w", id, err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("setting the password for %s: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("user %s: %w", id, ErrNotFound)
	}

	return nil
}

// ClaimPassword only writes when the account has never had one. Two people racing
// to claim a fresh account must not both succeed, so the condition lives in the
// WHERE clause and the row count decides the winner.
func (s *Store) ClaimPassword(ctx context.Context, id, hash string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		"UPDATE users SET pass_hash = ?, updated_at = ? WHERE id = ? AND pass_hash IS NULL",
		hash, formatTime(s.now()), id)
	if err != nil {
		return false, fmt.Errorf("claiming %s: %w", id, err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("claiming %s: %w", id, err)
	}

	return n == 1, nil
}

func (s *Store) Users(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+userColumns+" FROM users ORDER BY username")
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}
	defer rows.Close()

	users := []User{}

	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("listing users: %w", err)
		}
		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}

	return users, nil
}

// scanner is satisfied by both *sql.Row and *sql.Rows, so one scan function serves both.
type scanner interface {
	Scan(dest ...any) error
}

func scanUser(src scanner) (User, error) {
	var (
		user      User
		createdAt string
		updatedAt string
	)

	err := src.Scan(&user.ID, &user.Username, &user.DisplayName, &user.Role, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}

	if user.CreatedAt, err = parseTime(createdAt); err != nil {
		return User{}, err
	}
	if user.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return User{}, err
	}

	return user, nil
}

// OneStudent is the seam where a real supporter-to-student pairing will go. The
// app has exactly one student today, so the role is enough to find them.
func (s *Store) OneStudent(ctx context.Context) (User, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT "+userColumns+" FROM users WHERE role = 'student' ORDER BY created_at LIMIT 1")

	u, err := scanUser(row)
	if err != nil {
		return User{}, fmt.Errorf("looking up the student: %w", err)
	}

	return u, nil
}
