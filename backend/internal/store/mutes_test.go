package store

import (
	"context"
	"testing"
)

func TestNothingIsMutedToStartWith(t *testing.T) {
	s := newTestStore(t)
	seedUser(t, s, "u1", "mum", "supporter")

	muted, err := s.Mutes(context.Background(), "u1")
	if err != nil {
		t.Fatalf("Mutes() returned an unexpected error: %v", err)
	}
	if len(muted) != 0 {
		t.Errorf("a fresh account starts with %d things muted, want 0", len(muted))
	}
}

func TestMutingAndUnmuting(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedUser(t, s, "u1", "mum", "supporter")

	if err := s.SetMute(ctx, "u1", "file.added", true); err != nil {
		t.Fatalf("SetMute() returned an unexpected error: %v", err)
	}

	muted, _ := s.Mutes(ctx, "u1")
	if !muted["file.added"] {
		t.Fatalf("file.added is not muted, got %v", muted)
	}

	if err := s.SetMute(ctx, "u1", "file.added", false); err != nil {
		t.Fatalf("SetMute() returned an unexpected error: %v", err)
	}

	muted, _ = s.Mutes(ctx, "u1")
	if muted["file.added"] {
		t.Error("file.added is still muted after being switched back on")
	}
}

// The switch is a switch, not a counter: pressing it twice is the same as once.
func TestMutingTwiceIsHarmless(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedUser(t, s, "u1", "mum", "supporter")

	for range 3 {
		if err := s.SetMute(ctx, "u1", "help.raised", true); err != nil {
			t.Fatalf("SetMute() returned an unexpected error: %v", err)
		}
	}

	if err := s.SetMute(ctx, "u1", "help.raised", false); err != nil {
		t.Fatalf("SetMute() returned an unexpected error: %v", err)
	}

	muted, _ := s.Mutes(ctx, "u1")
	if muted["help.raised"] {
		t.Error("one unmute did not undo three mutes")
	}
}

func TestUnmutingSomethingNeverMuted(t *testing.T) {
	s := newTestStore(t)
	seedUser(t, s, "u1", "mum", "supporter")

	if err := s.SetMute(context.Background(), "u1", "message.sent", false); err != nil {
		t.Errorf("unmuting something that was never muted should be quiet, got %v", err)
	}
}

func TestMutedByFindsOnlyThePeopleWhoMutedIt(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedUser(t, s, "u1", "mum", "supporter")
	seedUser(t, s, "u2", "kid", "student")

	if err := s.SetMute(ctx, "u1", "file.added", true); err != nil {
		t.Fatalf("SetMute() returned an unexpected error: %v", err)
	}

	quiet, err := s.MutedBy(ctx, "file.added")
	if err != nil {
		t.Fatalf("MutedBy() returned an unexpected error: %v", err)
	}
	if !quiet["u1"] || quiet["u2"] {
		t.Errorf("MutedBy(file.added) = %v, want only u1", quiet)
	}

	other, _ := s.MutedBy(ctx, "help.raised")
	if len(other) != 0 {
		t.Errorf("MutedBy(help.raised) = %v, want nobody", other)
	}
}

// One person going quiet must not silence the other.
func TestAMuteBelongsToOnePerson(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedUser(t, s, "u1", "mum", "supporter")
	seedUser(t, s, "u2", "kid", "student")

	s.SetMute(ctx, "u1", "help.raised", true)

	theirs, _ := s.Mutes(ctx, "u2")
	if len(theirs) != 0 {
		t.Errorf("the student inherited %v from the supporter", theirs)
	}
}
