package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/database"
)

var testClock = time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC)

func newTestStore(t *testing.T) *Store {
	t.Helper()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "store_test.db")

	db, err := database.Open(ctx, path, time.Second)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := database.Migrate(ctx, db, discardLogger()); err != nil {
		t.Fatalf("migrating database: %v", err)
	}

	s := New(db)
	s.now = func() time.Time { return testClock }

	return s
}

func seedUser(t *testing.T, s *Store, id, username, role string) User {
	t.Helper()

	user, err := s.CreateUser(context.Background(), User{
		ID:          id,
		Username:    username,
		DisplayName: username,
		Role:        role,
	})
	if err != nil {
		t.Fatalf("seeding user %s: %v", username, err)
	}

	return user
}

func TestCreateAndReadUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	created := seedUser(t, s, "u1", "ricky", "supporter")

	if !created.CreatedAt.Equal(testClock) {
		t.Errorf("CreatedAt = %s, want %s", created.CreatedAt, testClock)
	}

	found, err := s.User(ctx, "u1")
	if err != nil {
		t.Fatalf("User() returned an unexpected error: %v", err)
	}

	if found.Username != "ricky" || found.Role != "supporter" {
		t.Errorf("read back %+v, want username ricky and role supporter", found)
	}

	if !found.CreatedAt.Equal(testClock) {
		t.Errorf("CreatedAt round-tripped as %s, want %s", found.CreatedAt, testClock)
	}
}

func TestUserNotFound(t *testing.T) {
	s := newTestStore(t)

	_, err := s.User(context.Background(), "nobody")

	if !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want it to wrap ErrNotFound", err)
	}
}

func TestUserByUsername(t *testing.T) {
	s := newTestStore(t)
	seedUser(t, s, "u1", "ricky", "supporter")

	found, err := s.UserByUsername(context.Background(), "ricky")
	if err != nil {
		t.Fatalf("UserByUsername() returned an unexpected error: %v", err)
	}

	if found.ID != "u1" {
		t.Errorf("ID = %q, want %q", found.ID, "u1")
	}
}

func TestDuplicateUsernameIsRejected(t *testing.T) {
	s := newTestStore(t)
	seedUser(t, s, "u1", "ricky", "supporter")

	_, err := s.CreateUser(context.Background(), User{
		ID: "u2", Username: "ricky", DisplayName: "Impostor", Role: "student",
	})

	if err == nil {
		t.Fatal("CreateUser() accepted a duplicate username, want an error")
	}
}

func TestUsersAreOrderedByUsername(t *testing.T) {
	s := newTestStore(t)
	seedUser(t, s, "u1", "ricky", "supporter")
	seedUser(t, s, "u2", "arjun", "student")

	users, err := s.Users(context.Background())
	if err != nil {
		t.Fatalf("Users() returned an unexpected error: %v", err)
	}

	// Asserting relative order rather than the whole list, so seeding another user
	// in a future migration cannot break a test that is really about ORDER BY.
	positions := map[string]int{}
	for i, user := range users {
		positions[user.Username] = i
	}

	arjun, ok := positions["arjun"]
	if !ok {
		t.Fatal("arjun is missing from the list")
	}

	ricky, ok := positions["ricky"]
	if !ok {
		t.Fatal("ricky is missing from the list")
	}

	if arjun > ricky {
		t.Errorf("arjun is at %d and ricky at %d, want arjun first", arjun, ricky)
	}

	for i := 1; i < len(users); i++ {
		if users[i-1].Username > users[i].Username {
			t.Errorf("not sorted: %q came before %q", users[i-1].Username, users[i].Username)
		}
	}
}

func TestUsersReturnsASliceNotNil(t *testing.T) {
	s := newTestStore(t)

	users, err := s.Users(context.Background())
	if err != nil {
		t.Fatalf("Users() returned an unexpected error: %v", err)
	}

	if users == nil {
		t.Error("Users() returned nil, want a slice so it encodes as [] not null")
	}
}
