package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func startRun(t *testing.T, s *Store, id, kind string, secs int64) FocusRun {
	t.Helper()

	run, err := s.StartFocus(context.Background(), FocusRun{
		ID: id, UserID: testUserID, Kind: kind, Planned: secs,
	})
	if err != nil {
		t.Fatalf("StartFocus(%s) returned: %v", id, err)
	}

	return run
}

func TestStartAndReadAFocusRun(t *testing.T) {
	s := newTestStore(t)

	run := startRun(t, s, "f1", FocusWork, 1500)

	if !run.Running() {
		t.Error("a new run should be running")
	}
	if !run.StartedAt.Equal(testClock) {
		t.Errorf("StartedAt = %s, want %s", run.StartedAt, testClock)
	}

	found, err := s.CurrentFocus(context.Background(), testUserID)
	if err != nil {
		t.Fatalf("CurrentFocus() returned: %v", err)
	}
	if found.ID != "f1" || found.Kind != FocusWork {
		t.Errorf("found %+v, want f1 on a work stretch", found)
	}
}

func TestOnlyOneRunAtATime(t *testing.T) {
	s := newTestStore(t)
	startRun(t, s, "f1", FocusWork, 1500)

	_, err := s.StartFocus(context.Background(), FocusRun{
		ID: "f2", UserID: testUserID, Kind: FocusShortBreak, Planned: 300,
	})

	if !errors.Is(err, ErrFocusRunning) {
		t.Errorf("error = %v, want ErrFocusRunning", err)
	}
}

func TestAnotherRunCanStartOnceTheFirstIsClosed(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	startRun(t, s, "f1", FocusWork, 1500)

	if _, err := s.CloseFocus(ctx, "f1", FocusFinished); err != nil {
		t.Fatalf("CloseFocus() returned: %v", err)
	}

	if _, err := s.StartFocus(ctx, FocusRun{
		ID: "f2", UserID: testUserID, Kind: FocusShortBreak, Planned: 300,
	}); err != nil {
		t.Errorf("StartFocus() after closing returned: %v", err)
	}
}

func TestNothingRunning(t *testing.T) {
	s := newTestStore(t)

	if _, err := s.CurrentFocus(context.Background(), testUserID); !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestClosingARunRecordsTheOutcome(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	startRun(t, s, "f1", FocusWork, 1500)

	later := testClock.Add(25 * time.Minute)
	s.setClock(later)

	done, err := s.CloseFocus(ctx, "f1", FocusFinished)
	if err != nil {
		t.Fatalf("CloseFocus() returned: %v", err)
	}

	if done.Running() {
		t.Error("the run is still marked as running")
	}
	if done.Outcome == nil || *done.Outcome != FocusFinished {
		t.Errorf("outcome = %v, want finished", done.Outcome)
	}
	if done.EndedAt == nil || !done.EndedAt.Equal(later) {
		t.Errorf("EndedAt = %v, want %s", done.EndedAt, later)
	}
}

// Two clicks on stop must not both succeed, or the second would rewrite an
// outcome that is already settled.
func TestClosingTwiceOnlyWorksOnce(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	startRun(t, s, "f1", FocusWork, 1500)

	if _, err := s.CloseFocus(ctx, "f1", FocusFinished); err != nil {
		t.Fatalf("first CloseFocus() returned: %v", err)
	}

	if _, err := s.CloseFocus(ctx, "f1", FocusStopped); !errors.Is(err, ErrNotFound) {
		t.Errorf("second CloseFocus() error = %v, want ErrNotFound", err)
	}

	still, err := s.FocusRun(ctx, "f1")
	if err != nil {
		t.Fatalf("FocusRun() returned: %v", err)
	}
	if *still.Outcome != FocusFinished {
		t.Errorf("outcome = %q, want the first one to stand", *still.Outcome)
	}
}

func TestClosingSomethingThatIsNotThere(t *testing.T) {
	s := newTestStore(t)

	if _, err := s.CloseFocus(context.Background(), "ghost", FocusFinished); !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestTimeLeftCountsDown(t *testing.T) {
	s := newTestStore(t)
	run := startRun(t, s, "f1", FocusWork, 1500)

	if left := run.Left(testClock); left != 25*time.Minute {
		t.Errorf("left at the start = %s, want 25m", left)
	}
	if left := run.Left(testClock.Add(10 * time.Minute)); left != 15*time.Minute {
		t.Errorf("left after 10m = %s, want 15m", left)
	}
}

// A run left open overnight has to read as zero, not as a large negative number.
func TestTimeLeftNeverGoesNegative(t *testing.T) {
	s := newTestStore(t)
	run := startRun(t, s, "f1", FocusWork, 1500)

	if left := run.Left(testClock.Add(9 * time.Hour)); left != 0 {
		t.Errorf("left after nine hours = %s, want 0", left)
	}
}

func TestAClosedRunHasNoTimeLeft(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	startRun(t, s, "f1", FocusWork, 1500)

	done, err := s.CloseFocus(ctx, "f1", FocusStopped)
	if err != nil {
		t.Fatalf("CloseFocus() returned: %v", err)
	}

	if left := done.Left(testClock); left != 0 {
		t.Errorf("left on a closed run = %s, want 0", left)
	}
}

// Only finished work counts. Breaks are not achievements and an abandoned stretch
// is not one either.
func TestOnlyFinishedWorkIsCounted(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	add := func(id, kind, outcome string) {
		t.Helper()
		startRun(t, s, id, kind, 1500)
		if _, err := s.CloseFocus(ctx, id, outcome); err != nil {
			t.Fatalf("CloseFocus(%s) returned: %v", id, err)
		}
	}

	add("f1", FocusWork, FocusFinished)
	add("f2", FocusWork, FocusStopped)
	add("f3", FocusShortBreak, FocusFinished)
	add("f4", FocusWork, FocusFinished)

	n, err := s.FinishedFocusBetween(ctx, testUserID, testClock.Add(-time.Hour), testClock.Add(time.Hour))
	if err != nil {
		t.Fatalf("FinishedFocusBetween() returned: %v", err)
	}

	if n != 2 {
		t.Errorf("counted %d, want 2 finished work stretches", n)
	}
}

func TestCountIsScopedToTheWindow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	startRun(t, s, "f1", FocusWork, 1500)
	if _, err := s.CloseFocus(ctx, "f1", FocusFinished); err != nil {
		t.Fatalf("CloseFocus() returned: %v", err)
	}

	n, err := s.FinishedFocusBetween(ctx, testUserID, testClock.Add(48*time.Hour), testClock.Add(72*time.Hour))
	if err != nil {
		t.Fatalf("FinishedFocusBetween() returned: %v", err)
	}

	if n != 0 {
		t.Errorf("counted %d outside the window, want 0", n)
	}
}

func TestAFocusRunRejectsAnUnknownKind(t *testing.T) {
	s := newTestStore(t)

	_, err := s.StartFocus(context.Background(), FocusRun{
		ID: "f1", UserID: testUserID, Kind: "nap", Planned: 1500,
	})

	if err == nil {
		t.Error("StartFocus() accepted an unknown kind")
	}
}
