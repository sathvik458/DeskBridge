package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	MessageKindPlain = "message"
	MessageKindHelp  = "help_request"
)

type Message struct {
	ID        string
	SenderID  string
	SessionID *string
	Kind      string
	Body      string
	ReadAt    *time.Time
	CreatedAt time.Time
}

func (m Message) IsRead() bool {
	return m.ReadAt != nil
}

const messageColumns = "id, sender_id, session_id, kind, body, read_at, created_at"

func (s *Store) CreateMessage(ctx context.Context, message Message) (Message, error) {
	message.CreatedAt = s.now()

	_, err := s.db.ExecContext(ctx,
		"INSERT INTO messages ("+messageColumns+") VALUES (?, ?, ?, ?, ?, ?, ?)",
		message.ID, message.SenderID, message.SessionID, message.Kind, message.Body,
		nil, formatTime(message.CreatedAt),
	)
	if err != nil {
		return Message{}, fmt.Errorf("creating message: %w", err)
	}

	return s.Message(ctx, message.ID)
}

func (s *Store) Message(ctx context.Context, id string) (Message, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+messageColumns+" FROM messages WHERE id = ?", id)

	message, err := scanMessage(row)
	if err != nil {
		return Message{}, fmt.Errorf("looking up message %s: %w", id, err)
	}

	return message, nil
}

// Newest first, because a limit is only useful if it keeps the most recent.
func (s *Store) Messages(ctx context.Context, limit int) ([]Message, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx,
		"SELECT "+messageColumns+" FROM messages ORDER BY created_at DESC, id DESC LIMIT ?", limit)
	if err != nil {
		return nil, fmt.Errorf("listing messages: %w", err)
	}
	defer rows.Close()

	messages := []Message{}

	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("listing messages: %w", err)
		}
		messages = append(messages, message)
	}

	return messages, rows.Err()
}

func (s *Store) UnreadFrom(ctx context.Context, senderID string) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+messageColumns+` FROM messages
		 WHERE sender_id = ? AND read_at IS NULL ORDER BY created_at`, senderID)
	if err != nil {
		return nil, fmt.Errorf("listing unread messages: %w", err)
	}
	defer rows.Close()

	messages := []Message{}

	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("listing unread messages: %w", err)
		}
		messages = append(messages, message)
	}

	return messages, rows.Err()
}

func (s *Store) MarkMessageRead(ctx context.Context, id string) (Message, error) {
	result, err := s.db.ExecContext(ctx,
		"UPDATE messages SET read_at = ? WHERE id = ? AND read_at IS NULL",
		formatTime(s.now()), id)
	if err != nil {
		return Message{}, fmt.Errorf("marking message %s read: %w", id, err)
	}

	if _, err := result.RowsAffected(); err != nil {
		return Message{}, fmt.Errorf("marking message %s read: %w", id, err)
	}

	// Reading it back covers both "already read" and "no such message", which the
	// row count alone cannot tell apart.
	return s.Message(ctx, id)
}

func (s *Store) MarkAllReadFrom(ctx context.Context, senderID string) (int, error) {
	result, err := s.db.ExecContext(ctx,
		"UPDATE messages SET read_at = ? WHERE sender_id = ? AND read_at IS NULL",
		formatTime(s.now()), senderID)
	if err != nil {
		return 0, fmt.Errorf("marking messages read: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("marking messages read: %w", err)
	}

	return int(affected), nil
}

func scanMessage(src scanner) (Message, error) {
	var (
		message   Message
		sessionID sql.NullString
		readAt    sql.NullString
		createdAt string
	)

	err := src.Scan(&message.ID, &message.SenderID, &sessionID, &message.Kind,
		&message.Body, &readAt, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Message{}, ErrNotFound
	}
	if err != nil {
		return Message{}, err
	}

	message.SessionID = optionalString(sessionID)

	if message.ReadAt, err = optionalTime(readAt); err != nil {
		return Message{}, err
	}
	if message.CreatedAt, err = parseTime(createdAt); err != nil {
		return Message{}, err
	}

	return message, nil
}
