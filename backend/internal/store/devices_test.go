package store

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRegisterDeviceCreatesIt(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	user := seedUser(t, s, "u1", "arjun", "student")

	device, err := s.RegisterDevice(ctx, Device{
		ID: "d1", UserID: &user.ID, Name: "Student Laptop", Kind: "laptop",
	})
	if err != nil {
		t.Fatalf("RegisterDevice() returned an unexpected error: %v", err)
	}

	if device.Status != StatusOnline {
		t.Errorf("Status = %q, want %q", device.Status, StatusOnline)
	}

	if device.LastSeenAt == nil {
		t.Fatal("LastSeenAt is nil, want the registration time")
	}

	if !device.LastSeenAt.Equal(testClock) {
		t.Errorf("LastSeenAt = %s, want %s", device.LastSeenAt, testClock)
	}
}

func TestRegisterDeviceTwiceUpdatesWithoutLosingCreatedAt(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	first, err := s.RegisterDevice(ctx, Device{ID: "d1", Name: "Old Name", Kind: "laptop"})
	if err != nil {
		t.Fatalf("first RegisterDevice() failed: %v", err)
	}

	later := testClock.Add(2 * time.Hour)
	s.now = func() time.Time { return later }

	second, err := s.RegisterDevice(ctx, Device{ID: "d1", Name: "New Name", Kind: "laptop"})
	if err != nil {
		t.Fatalf("second RegisterDevice() failed: %v", err)
	}

	if second.Name != "New Name" {
		t.Errorf("Name = %q, want %q", second.Name, "New Name")
	}

	if !second.CreatedAt.Equal(first.CreatedAt) {
		t.Errorf("CreatedAt changed from %s to %s, want it preserved", first.CreatedAt, second.CreatedAt)
	}

	if !second.UpdatedAt.Equal(later) {
		t.Errorf("UpdatedAt = %s, want %s", second.UpdatedAt, later)
	}

	devices, err := s.Devices(ctx)
	if err != nil {
		t.Fatalf("Devices() failed: %v", err)
	}
	if len(devices) != 1 {
		t.Errorf("got %d devices after re-registering, want 1", len(devices))
	}
}

func TestRegisterDeviceWithoutUser(t *testing.T) {
	s := newTestStore(t)

	device, err := s.RegisterDevice(context.Background(), Device{
		ID: "phone1", Name: "Phone Camera", Kind: "phone",
	})
	if err != nil {
		t.Fatalf("RegisterDevice() returned an unexpected error: %v", err)
	}

	if device.UserID != nil {
		t.Errorf("UserID = %v, want nil", *device.UserID)
	}
}

func TestRegisterDeviceRejectsUnknownUser(t *testing.T) {
	s := newTestStore(t)
	ghost := "no-such-user"

	_, err := s.RegisterDevice(context.Background(), Device{
		ID: "d1", UserID: &ghost, Name: "Laptop", Kind: "laptop",
	})

	if err == nil {
		t.Fatal("RegisterDevice() accepted an unknown user id, want a foreign key error")
	}
}

func TestDeviceNotFound(t *testing.T) {
	s := newTestStore(t)

	_, err := s.Device(context.Background(), "ghost")

	if !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want it to wrap ErrNotFound", err)
	}
}

func TestRecordHeartbeatUpdatesLastSeen(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.RegisterDevice(ctx, Device{ID: "d1", Name: "Laptop", Kind: "laptop"}); err != nil {
		t.Fatalf("RegisterDevice() failed: %v", err)
	}

	later := testClock.Add(30 * time.Second)
	s.now = func() time.Time { return later }

	if err := s.RecordHeartbeat(ctx, "d1"); err != nil {
		t.Fatalf("RecordHeartbeat() returned an unexpected error: %v", err)
	}

	device, err := s.Device(ctx, "d1")
	if err != nil {
		t.Fatalf("Device() failed: %v", err)
	}

	if !device.LastSeenAt.Equal(later) {
		t.Errorf("LastSeenAt = %s, want %s", device.LastSeenAt, later)
	}
}

func TestRecordHeartbeatForUnknownDevice(t *testing.T) {
	s := newTestStore(t)

	err := s.RecordHeartbeat(context.Background(), "ghost")

	if !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want it to wrap ErrNotFound", err)
	}
}

func TestMarkStaleDevicesOffline(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.RegisterDevice(ctx, Device{ID: "quiet", Name: "Old Phone", Kind: "phone"}); err != nil {
		t.Fatalf("RegisterDevice() failed: %v", err)
	}

	later := testClock.Add(10 * time.Minute)
	s.now = func() time.Time { return later }

	if _, err := s.RegisterDevice(ctx, Device{ID: "chatty", Name: "Laptop", Kind: "laptop"}); err != nil {
		t.Fatalf("RegisterDevice() failed: %v", err)
	}

	marked, err := s.MarkStaleDevicesOffline(ctx, 5*time.Minute)
	if err != nil {
		t.Fatalf("MarkStaleDevicesOffline() returned an unexpected error: %v", err)
	}

	if marked != 1 {
		t.Errorf("marked %d devices offline, want 1", marked)
	}

	quiet, err := s.Device(ctx, "quiet")
	if err != nil {
		t.Fatalf("Device() failed: %v", err)
	}
	if quiet.Status != StatusOffline {
		t.Errorf("quiet device status = %q, want %q", quiet.Status, StatusOffline)
	}

	chatty, err := s.Device(ctx, "chatty")
	if err != nil {
		t.Fatalf("Device() failed: %v", err)
	}
	if chatty.Status != StatusOnline {
		t.Errorf("chatty device status = %q, want %q", chatty.Status, StatusOnline)
	}
}

func TestMarkStaleDevicesOfflineIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.RegisterDevice(ctx, Device{ID: "quiet", Name: "Old Phone", Kind: "phone"}); err != nil {
		t.Fatalf("RegisterDevice() failed: %v", err)
	}

	s.now = func() time.Time { return testClock.Add(time.Hour) }

	if _, err := s.MarkStaleDevicesOffline(ctx, time.Minute); err != nil {
		t.Fatalf("first sweep failed: %v", err)
	}

	marked, err := s.MarkStaleDevicesOffline(ctx, time.Minute)
	if err != nil {
		t.Fatalf("second sweep failed: %v", err)
	}

	if marked != 0 {
		t.Errorf("second sweep marked %d devices, want 0", marked)
	}
}
