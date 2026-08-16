package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	StatusOnline  = "online"
	StatusOffline = "offline"
	StatusUnknown = "unknown"
)

type Device struct {
	ID         string
	UserID     *string
	Name       string
	Kind       string
	Status     string
	LastSeenAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

const deviceColumns = "id, user_id, name, kind, status, last_seen_at, created_at, updated_at"

// Registering is an upsert because a device re-registers every time it restarts.
func (s *Store) RegisterDevice(ctx context.Context, device Device) (Device, error) {
	now := s.now()
	device.UpdatedAt = now
	device.LastSeenAt = &now
	device.Status = StatusOnline

	if device.CreatedAt.IsZero() {
		device.CreatedAt = now
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO devices (`+deviceColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			user_id      = excluded.user_id,
			name         = excluded.name,
			kind         = excluded.kind,
			status       = excluded.status,
			last_seen_at = excluded.last_seen_at,
			updated_at   = excluded.updated_at`,
		device.ID, device.UserID, device.Name, device.Kind, device.Status,
		formatTime(now), formatTime(device.CreatedAt), formatTime(device.UpdatedAt),
	)
	if err != nil {
		return Device{}, fmt.Errorf("registering device %s: %w", device.ID, err)
	}

	return s.Device(ctx, device.ID)
}

func (s *Store) Device(ctx context.Context, id string) (Device, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+deviceColumns+" FROM devices WHERE id = ?", id)

	device, err := scanDevice(row)
	if err != nil {
		return Device{}, fmt.Errorf("looking up device %s: %w", id, err)
	}

	return device, nil
}

func (s *Store) Devices(ctx context.Context) ([]Device, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+deviceColumns+" FROM devices ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("listing devices: %w", err)
	}
	defer rows.Close()

	devices := []Device{}

	for rows.Next() {
		device, err := scanDevice(rows)
		if err != nil {
			return nil, fmt.Errorf("listing devices: %w", err)
		}
		devices = append(devices, device)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing devices: %w", err)
	}

	return devices, nil
}

func (s *Store) RecordHeartbeat(ctx context.Context, id string) error {
	now := formatTime(s.now())

	result, err := s.db.ExecContext(ctx,
		"UPDATE devices SET status = ?, last_seen_at = ?, updated_at = ? WHERE id = ?",
		StatusOnline, now, now, id,
	)
	if err != nil {
		return fmt.Errorf("recording heartbeat for device %s: %w", id, err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("recording heartbeat for device %s: %w", id, err)
	}

	if affected == 0 {
		return fmt.Errorf("recording heartbeat for device %s: %w", id, ErrNotFound)
	}

	return nil
}

// MarkStaleDevicesOffline is how a device is noticed to be gone: nothing announces a
// power cut, so silence past the cutoff is the only available signal.
func (s *Store) MarkStaleDevicesOffline(ctx context.Context, silentFor time.Duration) (int, error) {
	now := s.now()
	cutoff := formatTime(now.Add(-silentFor))

	result, err := s.db.ExecContext(ctx, `
		UPDATE devices
		SET status = ?, updated_at = ?
		WHERE status = ? AND (last_seen_at IS NULL OR last_seen_at < ?)`,
		StatusOffline, formatTime(now), StatusOnline, cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("marking stale devices offline: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("marking stale devices offline: %w", err)
	}

	return int(affected), nil
}

func scanDevice(src scanner) (Device, error) {
	var (
		device     Device
		userID     sql.NullString
		lastSeenAt sql.NullString
		createdAt  string
		updatedAt  string
	)

	err := src.Scan(&device.ID, &userID, &device.Name, &device.Kind, &device.Status,
		&lastSeenAt, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	if err != nil {
		return Device{}, err
	}

	device.UserID = optionalString(userID)

	if device.LastSeenAt, err = optionalTime(lastSeenAt); err != nil {
		return Device{}, err
	}
	if device.CreatedAt, err = parseTime(createdAt); err != nil {
		return Device{}, err
	}
	if device.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return Device{}, err
	}

	return device, nil
}
