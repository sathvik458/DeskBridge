package store

import (
	"context"
	"testing"
	"time"
)

func leave(t *testing.T, s *Store, id, userID, tag, body string) Notice {
	t.Helper()

	n, err := s.Note(context.Background(), Notice{
		ID:     id,
		UserID: userID,
		Kind:   "help.raised",
		Tag:    tag,
		Body:   body,
	}, 5*time.Minute)
	if err != nil {
		t.Fatalf("leaving notice %s: %v", id, err)
	}

	return n
}

func TestANoticeIsWaitingAfterwards(t *testing.T) {
	s := newTestStore(t)
	seedUser(t, s, "u1", "mum", "supporter")

	left := leave(t, s, "n1", "u1", "help.raised", "stuck on question 4")

	if left.Times != 1 || left.SeenAt != nil {
		t.Errorf("first notice = %+v, want one unseen", left)
	}

	unseen, err := s.UnseenNotices(context.Background(), "u1")
	if err != nil {
		t.Fatalf("UnseenNotices() returned an unexpected error: %v", err)
	}
	if unseen != 1 {
		t.Errorf("unseen = %d, want 1", unseen)
	}
}

func TestTheSameThingTwiceIsOneNotice(t *testing.T) {
	s := newTestStore(t)
	seedUser(t, s, "u1", "mum", "supporter")

	first := leave(t, s, "n1", "u1", "help.raised", "stuck")
	again := leave(t, s, "n2", "u1", "help.raised", "still stuck")

	if again.ID != first.ID {
		t.Fatalf("second notice made a new row %s, want the first one bumped", again.ID)
	}
	if again.Times != 2 {
		t.Errorf("times = %d, want 2", again.Times)
	}
	if again.Body != "still stuck" {
		t.Errorf("body = %q, want the newest wording", again.Body)
	}
}

func TestDifferentTagsStayApart(t *testing.T) {
	s := newTestStore(t)
	seedUser(t, s, "u1", "mum", "supporter")

	leave(t, s, "n1", "u1", "help.raised", "stuck")
	leave(t, s, "n2", "u1", "message.sent", "hello")

	list, err := s.Notices(context.Background(), "u1", 10)
	if err != nil {
		t.Fatalf("Notices() returned an unexpected error: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("got %d notices, want 2", len(list))
	}
}

// Past the window the same thing is news again rather than a bigger count.
func TestAnOldNoticeIsNotBumped(t *testing.T) {
	s := newTestStore(t)
	seedUser(t, s, "u1", "mum", "supporter")

	leave(t, s, "n1", "u1", "help.raised", "stuck")

	s.now = func() time.Time { return testClock.Add(time.Hour) }
	later := leave(t, s, "n2", "u1", "help.raised", "stuck again")

	if later.ID != "n2" || later.Times != 1 {
		t.Errorf("late notice = %+v, want a fresh row", later)
	}
}

// A notice already read is left alone, so the bell does not go back up.
func TestASeenNoticeIsNotBumped(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedUser(t, s, "u1", "mum", "supporter")

	leave(t, s, "n1", "u1", "help.raised", "stuck")

	if _, err := s.MarkNoticesSeen(ctx, "u1"); err != nil {
		t.Fatalf("MarkNoticesSeen() returned an unexpected error: %v", err)
	}

	next := leave(t, s, "n2", "u1", "help.raised", "stuck again")
	if next.ID != "n2" {
		t.Errorf("bumped a notice that had already been read: %+v", next)
	}
}

func TestNoticesAreOnlyYourOwn(t *testing.T) {
	s := newTestStore(t)
	seedUser(t, s, "u1", "mum", "supporter")
	seedUser(t, s, "u2", "kid", "student")

	leave(t, s, "n1", "u1", "help.raised", "stuck")

	count, err := s.UnseenNotices(context.Background(), "u2")
	if err != nil {
		t.Fatalf("UnseenNotices() returned an unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("the student sees %d of the supporter's notices, want 0", count)
	}
}

func TestMarkingSeenClearsTheCount(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedUser(t, s, "u1", "mum", "supporter")

	leave(t, s, "n1", "u1", "help.raised", "stuck")
	leave(t, s, "n2", "u1", "message.sent", "hello")

	marked, err := s.MarkNoticesSeen(ctx, "u1")
	if err != nil {
		t.Fatalf("MarkNoticesSeen() returned an unexpected error: %v", err)
	}
	if marked != 2 {
		t.Errorf("marked = %d, want 2", marked)
	}

	unseen, _ := s.UnseenNotices(ctx, "u1")
	if unseen != 0 {
		t.Errorf("unseen = %d after marking, want 0", unseen)
	}
}

func TestOldNoticesAreSweptAway(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedUser(t, s, "u1", "mum", "supporter")

	leave(t, s, "n1", "u1", "help.raised", "stuck")

	s.now = func() time.Time { return testClock.Add(keepNotices + time.Hour) }

	dropped, err := s.TrimNotices(ctx)
	if err != nil {
		t.Fatalf("TrimNotices() returned an unexpected error: %v", err)
	}
	if dropped != 1 {
		t.Errorf("dropped = %d, want 1", dropped)
	}

	list, _ := s.Notices(ctx, "u1", 10)
	if len(list) != 0 {
		t.Errorf("%d notices survived the sweep, want none", len(list))
	}
}
