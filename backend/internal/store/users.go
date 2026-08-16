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
