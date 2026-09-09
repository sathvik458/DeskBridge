package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func openLogin(t *testing.T, s *Store, tok, uid string, life time.Duration) Login {
	t.Helper()

	l, err := s.StartLogin(context.Background(), Login{
		Token: tok, UserID: uid, Label: "mac", ExpiresAt: s.now().Add(life),
	})
	if err != nil {
		t.Fatalf("StartLogin(%s) returned: %v", tok, err)
	}

	return l
}

func TestStartAndResolveALogin(t *testing.T) {
	s := newTestStore(t)

	openLogin(t, s, "t1", testUserID, time.Hour)

	u, l, err := s.WhoIs(context.Background(), "t1")
	if err != nil {
		t.Fatalf("WhoIs() returned: %v", err)
	}

	if u.ID != testUserID {
		t.Errorf("user = %q, want %q", u.ID, testUserID)
	}
	if u.Username == "" {
		t.Error("the joined user came back empty")
	}
	if l.Label != "mac" {
		t.Errorf("label = %q, want mac", l.Label)
	}
	if !l.CreatedAt.Equal(testClock) {
		t.Errorf("CreatedAt = %s, want %s", l.CreatedAt, testClock)
	}
}

func TestUnknownTokenIsNotFound(t *testing.T) {
	s := newTestStore(t)

	if _, _, err := s.WhoIs(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

// An expired token has to be indistinguishable from a bogus one, so a caller
// cannot use the difference to learn that a token was once real.
func TestExpiredTokenIsNotFound(t *testing.T) {
	s := newTestStore(t)
	openLogin(t, s, "t1", testUserID, time.Hour)

	s.setClock(testClock.Add(2 * time.Hour))

	if _, _, err := s.WhoIs(context.Background(), "t1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestATokenIsLiveRightUpToItsExpiry(t *testing.T) {
	s := newTestStore(t)
	l := openLogin(t, s, "t1", testUserID, time.Hour)

	if !l.Live(l.ExpiresAt.Add(-time.Second)) {
		t.Error("a token expired a second early")
	}
	if l.Live(l.ExpiresAt) {
		t.Error("a token is still live at the instant it expires")
	}
}

func TestEndLogin(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	openLogin(t, s, "t1", testUserID, time.Hour)

	if err := s.EndLogin(ctx, "t1"); err != nil {
		t.Fatalf("EndLogin() returned: %v", err)
	}

	if _, _, err := s.WhoIs(ctx, "t1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("the token still resolves after sign out: %v", err)
	}
}

func TestEndingAnUnknownLoginIsNotAnError(t *testing.T) {
	s := newTestStore(t)

	if err := s.EndLogin(context.Background(), "never-existed"); err != nil {
		t.Errorf("EndLogin() on a missing token returned: %v", err)
	}
}

func TestEndAllLoginsClearsEveryBrowser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	openLogin(t, s, "t1", testUserID, time.Hour)
	openLogin(t, s, "t2", testUserID, time.Hour)
	openLogin(t, s, "t3", supporterID, time.Hour)

	n, err := s.EndAllLogins(ctx, testUserID)
	if err != nil {
		t.Fatalf("EndAllLogins() returned: %v", err)
	}
	if n != 2 {
		t.Errorf("ended %d logins, want 2", n)
	}

	// The other account must be untouched.
	if _, _, err := s.WhoIs(ctx, "t3"); err != nil {
		t.Errorf("the other user was signed out too: %v", err)
	}
}

func TestSweepRemovesOnlyExpiredLogins(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	openLogin(t, s, "stale", testUserID, time.Hour)
	openLogin(t, s, "fresh", testUserID, 48*time.Hour)

	s.setClock(testClock.Add(2 * time.Hour))

	n, err := s.SweepLogins(ctx)
	if err != nil {
		t.Fatalf("SweepLogins() returned: %v", err)
	}
	if n != 1 {
		t.Errorf("swept %d, want 1", n)
	}

	if _, _, err := s.WhoIs(ctx, "fresh"); err != nil {
		t.Errorf("the live token was swept: %v", err)
	}
}

func TestTouchOnlyWritesOnceTheTimeIsStale(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	openLogin(t, s, "t1", testUserID, 48*time.Hour)

	// Barely any time has passed, so this should be a no-op.
	s.setClock(testClock.Add(time.Minute))
	if err := s.TouchLogin(ctx, "t1", time.Hour); err != nil {
		t.Fatalf("TouchLogin() returned: %v", err)
	}

	_, l, err := s.WhoIs(ctx, "t1")
	if err != nil {
		t.Fatalf("WhoIs() returned: %v", err)
	}
	if !l.SeenAt.Equal(testClock) {
		t.Errorf("SeenAt moved to %s on a fresh token, want it left at %s", l.SeenAt, testClock)
	}

	later := testClock.Add(3 * time.Hour)
	s.setClock(later)
	if err := s.TouchLogin(ctx, "t1", time.Hour); err != nil {
		t.Fatalf("TouchLogin() returned: %v", err)
	}

	_, l, err = s.WhoIs(ctx, "t1")
	if err != nil {
		t.Fatalf("WhoIs() returned: %v", err)
	}
	if !l.SeenAt.Equal(later) {
		t.Errorf("SeenAt = %s, want it moved to %s", l.SeenAt, later)
	}
}

func TestLoginsForIsNewestFirst(t *testing.T) {
	s := newTestStore(t)

	openLogin(t, s, "old", testUserID, time.Hour)
	s.setClock(testClock.Add(time.Minute))
	openLogin(t, s, "new", testUserID, time.Hour)
	openLogin(t, s, "theirs", supporterID, time.Hour)

	out, err := s.LoginsFor(context.Background(), testUserID)
	if err != nil {
		t.Fatalf("LoginsFor() returned: %v", err)
	}

	if len(out) != 2 {
		t.Fatalf("got %d logins, want 2", len(out))
	}
	if out[0].Token != "new" {
		t.Errorf("first = %q, want the newest", out[0].Token)
	}
}

func TestLoginsForReturnsEmptySliceNotNil(t *testing.T) {
	s := newTestStore(t)

	out, err := s.LoginsFor(context.Background(), testUserID)
	if err != nil {
		t.Fatalf("LoginsFor() returned: %v", err)
	}
	if out == nil {
		t.Error("LoginsFor() returned nil, want an empty slice")
	}
}

func TestTwoLoginsCannotShareAToken(t *testing.T) {
	s := newTestStore(t)
	openLogin(t, s, "t1", testUserID, time.Hour)

	_, err := s.StartLogin(context.Background(), Login{
		Token: "t1", UserID: supporterID, ExpiresAt: s.now().Add(time.Hour),
	})

	if err == nil {
		t.Error("StartLogin() accepted a duplicate token")
	}
}

func TestALoginNeedsARealUser(t *testing.T) {
	s := newTestStore(t)

	_, err := s.StartLogin(context.Background(), Login{
		Token: "t1", UserID: "ghost", ExpiresAt: s.now().Add(time.Hour),
	})

	if err == nil {
		t.Error("StartLogin() accepted a token for a user that does not exist")
	}
}
