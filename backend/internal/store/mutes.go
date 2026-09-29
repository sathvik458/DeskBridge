package store

import (
	"context"
	"fmt"
)

// MutedBy answers "who does not want to hear about this", which is the question
// asked once per event. Asking each recipient separately would be a query per
// person; this is a query per event no matter how many people are in the house.
func (s *Store) MutedBy(ctx context.Context, kind string) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT user_id FROM notice_mutes WHERE kind = ?", kind)
	if err != nil {
		return nil, fmt.Errorf("reading who muted %s: %w", kind, err)
	}
	defer rows.Close()

	out := map[string]bool{}

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("reading who muted %s: %w", kind, err)
		}
		out[id] = true
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading who muted %s: %w", kind, err)
	}

	return out, nil
}

func (s *Store) Mutes(ctx context.Context, userID string) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT kind FROM notice_mutes WHERE user_id = ?", userID)
	if err != nil {
		return nil, fmt.Errorf("reading mutes for %s: %w", userID, err)
	}
	defer rows.Close()

	out := map[string]bool{}

	for rows.Next() {
		var kind string
		if err := rows.Scan(&kind); err != nil {
			return nil, fmt.Errorf("reading mutes for %s: %w", userID, err)
		}
		out[kind] = true
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading mutes for %s: %w", userID, err)
	}

	return out, nil
}

// SetMute is written so pressing the same switch twice is harmless: muting what is
// already muted inserts nothing, unmuting what was never muted deletes nothing.
func (s *Store) SetMute(ctx context.Context, userID, kind string, muted bool) error {
	var err error

	if muted {
		_, err = s.db.ExecContext(ctx,
			"INSERT INTO notice_mutes (user_id, kind) VALUES (?, ?) ON CONFLICT DO NOTHING",
			userID, kind)
	} else {
		_, err = s.db.ExecContext(ctx,
			"DELETE FROM notice_mutes WHERE user_id = ? AND kind = ?", userID, kind)
	}

	if err != nil {
		return fmt.Errorf("saving the %s switch for %s: %w", kind, userID, err)
	}

	return nil
}
