package store

import (
	"context"
	"errors"
	"testing"
)

const fakeHash = "pbkdf2-sha256$210000$c2FsdHk$a2V5c3R1ZmY"

// Nothing ships with a password. An account nobody has claimed cannot be signed
// into, which is the whole reason there is no default credential in a migration.
func TestSeededAccountsStartWithNoPassword(t *testing.T) {
	s := newTestStore(t)

	for _, id := range []string{testUserID, supporterID} {
		h, err := s.PassHash(context.Background(), id)
		if err != nil {
			t.Fatalf("PassHash(%s) returned: %v", id, err)
		}
		if h != "" {
			t.Errorf("%s ships with a password already set", id)
		}
	}
}

func TestPassHashForAnUnknownUser(t *testing.T) {
	s := newTestStore(t)

	if _, err := s.PassHash(context.Background(), "ghost"); !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestSetAndReadPassHash(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.SetPassHash(ctx, testUserID, fakeHash); err != nil {
		t.Fatalf("SetPassHash() returned: %v", err)
	}

	h, err := s.PassHash(ctx, testUserID)
	if err != nil {
		t.Fatalf("PassHash() returned: %v", err)
	}
	if h != fakeHash {
		t.Errorf("hash = %q, want %q", h, fakeHash)
	}
}

func TestSetPassHashForAnUnknownUser(t *testing.T) {
	s := newTestStore(t)

	if err := s.SetPassHash(context.Background(), "ghost", fakeHash); !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestClaimingWorksOnceAndOnlyOnce(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	first, err := s.ClaimPassword(ctx, testUserID, fakeHash)
	if err != nil {
		t.Fatalf("ClaimPassword() returned: %v", err)
	}
	if !first {
		t.Fatal("the first claim on a fresh account was refused")
	}

	// Whoever gets there second must lose, or anyone could take over an account
	// that has already been set up.
	second, err := s.ClaimPassword(ctx, testUserID, "pbkdf2-sha256$210000$b3RoZXI$b3RoZXJrZXk")
	if err != nil {
		t.Fatalf("second ClaimPassword() returned: %v", err)
	}
	if second {
		t.Fatal("a second claim succeeded on an account that already had a password")
	}

	h, err := s.PassHash(ctx, testUserID)
	if err != nil {
		t.Fatalf("PassHash() returned: %v", err)
	}
	if h != fakeHash {
		t.Error("the second claim overwrote the first password")
	}
}

func TestClaimingAnUnknownUserJustFails(t *testing.T) {
	s := newTestStore(t)

	ok, err := s.ClaimPassword(context.Background(), "ghost", fakeHash)
	if err != nil {
		t.Fatalf("ClaimPassword() returned: %v", err)
	}
	if ok {
		t.Error("claimed a password for a user that does not exist")
	}
}

// Changing a password is a different operation from claiming one and must work
// even though the account already has a hash.
func TestSetPassHashOverwritesWhereClaimWouldNot(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	const other = "pbkdf2-sha256$210000$b3RoZXI$b3RoZXJrZXk"

	if _, err := s.ClaimPassword(ctx, testUserID, fakeHash); err != nil {
		t.Fatalf("ClaimPassword() returned: %v", err)
	}

	if err := s.SetPassHash(ctx, testUserID, other); err != nil {
		t.Fatalf("SetPassHash() returned: %v", err)
	}

	h, err := s.PassHash(ctx, testUserID)
	if err != nil {
		t.Fatalf("PassHash() returned: %v", err)
	}
	if h != other {
		t.Error("SetPassHash did not replace the existing password")
	}
}

// The hash must not travel on the User struct, or a handler that encodes a user
// list would put every password hash on the wire.
func TestUserStructCarriesNoHash(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.SetPassHash(ctx, testUserID, fakeHash); err != nil {
		t.Fatalf("SetPassHash() returned: %v", err)
	}

	u, err := s.User(ctx, testUserID)
	if err != nil {
		t.Fatalf("User() returned: %v", err)
	}

	for _, f := range []string{u.ID, u.Username, u.DisplayName, u.Role} {
		if f == fakeHash {
			t.Fatal("a password hash came back on the User struct")
		}
	}
}
