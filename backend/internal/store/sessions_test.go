package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Seeded by migration 0003, so tests use it rather than creating a second user
// with the same username.
const testUserID = "default-student"

func startTestSession(t *testing.T, s *Store) Session {
	t.Helper()

	topic := "Ray Optics"
	session, err := s.StartSession(context.Background(), Session{
		ID: "sess1", UserID: testUserID, Subject: "Physics", Topic: &topic,
	})
	if err != nil {
		t.Fatalf("StartSession() returned an unexpected error: %v", err)
	}

	return session
}

func (s *Store) setClock(at time.Time) {
	s.now = func() time.Time { return at }
}

func TestStartSessionBeginsRunning(t *testing.T) {
	s := newTestStore(t)
	session := startTestSession(t, s)

	if session.Status != SessionActive {
		t.Errorf("Status = %q, want %q", session.Status, SessionActive)
	}
	if session.AccumulatedSeconds != 0 {
		t.Errorf("AccumulatedSeconds = %d, want 0", session.AccumulatedSeconds)
	}
	if session.LastResumedAt == nil {
		t.Fatal("LastResumedAt is nil, want the start time")
	}
	if !session.LastResumedAt.Equal(testClock) {
		t.Errorf("LastResumedAt = %s, want %s", session.LastResumedAt, testClock)
	}
}

func TestElapsedGrowsWhileActiveAndFreezesWhenPaused(t *testing.T) {
	s := newTestStore(t)
	session := startTestSession(t, s)

	if got := session.Elapsed(testClock.Add(90 * time.Second)); got != 90*time.Second {
		t.Errorf("elapsed after 90s = %s, want 1m30s", got)
	}

	s.setClock(testClock.Add(5 * time.Minute))

	paused, err := s.PauseSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("PauseSession() returned an unexpected error: %v", err)
	}

	// An hour of wall clock passes; a paused session must not have moved.
	frozen := paused.Elapsed(testClock.Add(time.Hour))
	if frozen != 5*time.Minute {
		t.Errorf("elapsed while paused = %s, want 5m0s", frozen)
	}
}

func TestPauseResumeEndArithmetic(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	session := startTestSession(t, s)

	s.setClock(testClock.Add(25 * time.Minute))
	if _, err := s.PauseSession(ctx, session.ID); err != nil {
		t.Fatalf("PauseSession() failed: %v", err)
	}

	// Ten minutes of break must not be counted.
	s.setClock(testClock.Add(35 * time.Minute))
	if _, err := s.ResumeSession(ctx, session.ID); err != nil {
		t.Fatalf("ResumeSession() failed: %v", err)
	}

	s.setClock(testClock.Add(50 * time.Minute))
	ended, err := s.EndSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("EndSession() failed: %v", err)
	}

	// 25 minutes before the pause, 15 minutes after the resume.
	want := 40 * time.Minute
	if got := ended.Elapsed(testClock.Add(time.Hour)); got != want {
		t.Errorf("final elapsed = %s, want %s", got, want)
	}
	if ended.Status != SessionCompleted {
		t.Errorf("Status = %q, want %q", ended.Status, SessionCompleted)
	}
	if ended.EndedAt == nil {
		t.Error("EndedAt is nil, want the end time")
	}
	if ended.LastResumedAt != nil {
		t.Error("LastResumedAt should be cleared once the session has ended")
	}
}

// The point of the two-column model: a restart mid-session loses nothing, because
// elapsed time is derived from stored fields rather than a counter held in memory.
func TestElapsedSurvivesAReload(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	session := startTestSession(t, s)

	s.setClock(testClock.Add(20 * time.Minute))
	if _, err := s.PauseSession(ctx, session.ID); err != nil {
		t.Fatalf("PauseSession() failed: %v", err)
	}
	if _, err := s.ResumeSession(ctx, session.ID); err != nil {
		t.Fatalf("ResumeSession() failed: %v", err)
	}

	reloaded, err := s.Session(ctx, session.ID)
	if err != nil {
		t.Fatalf("Session() failed: %v", err)
	}

	if got := reloaded.Elapsed(testClock.Add(30 * time.Minute)); got != 30*time.Minute {
		t.Errorf("elapsed after reload = %s, want 30m0s", got)
	}
}

func TestOnlyOneSessionRunsAtATime(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	startTestSession(t, s)

	_, err := s.StartSession(ctx, Session{ID: "sess2", UserID: testUserID, Subject: "Chemistry"})

	if !errors.Is(err, ErrSessionRunning) {
		t.Errorf("error = %v, want ErrSessionRunning", err)
	}
}

func TestAnotherSessionCanStartOnceTheFirstEnds(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	session := startTestSession(t, s)

	s.setClock(testClock.Add(10 * time.Minute))
	if _, err := s.EndSession(ctx, session.ID); err != nil {
		t.Fatalf("EndSession() failed: %v", err)
	}

	if _, err := s.StartSession(ctx, Session{ID: "sess2", UserID: testUserID, Subject: "Chemistry"}); err != nil {
		t.Errorf("StartSession() after ending the first returned: %v", err)
	}
}

func TestIllegalTransitions(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*Store, string)
		move  func(*Store, context.Context, string) (Session, error)
	}{
		{
			name:  "pause an already paused session",
			setup: func(s *Store, id string) { s.PauseSession(context.Background(), id) },
			move:  (*Store).PauseSession,
		},
		{
			name:  "resume a running session",
			setup: func(*Store, string) {},
			move:  (*Store).ResumeSession,
		},
		{
			name: "end a completed session",
			setup: func(s *Store, id string) {
				s.EndSession(context.Background(), id)
			},
			move: (*Store).EndSession,
		},
		{
			name: "pause a completed session",
			setup: func(s *Store, id string) {
				s.EndSession(context.Background(), id)
			},
			move: (*Store).PauseSession,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			session := startTestSession(t, s)

			tc.setup(s, session.ID)

			_, err := tc.move(s, context.Background(), session.ID)
			if !errors.Is(err, ErrBadTransition) {
				t.Errorf("error = %v, want ErrBadTransition", err)
			}
		})
	}
}

func TestTransitionOnUnknownSession(t *testing.T) {
	s := newTestStore(t)

	_, err := s.PauseSession(context.Background(), "nope")

	if !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestCurrentSessionIsGoneOnceEnded(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	session := startTestSession(t, s)

	if _, err := s.CurrentSession(ctx, testUserID); err != nil {
		t.Fatalf("CurrentSession() before ending returned: %v", err)
	}

	s.setClock(testClock.Add(time.Minute))
	if _, err := s.EndSession(ctx, session.ID); err != nil {
		t.Fatalf("EndSession() failed: %v", err)
	}

	if _, err := s.CurrentSession(ctx, testUserID); !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestCurrentSessionIncludesPausedOnes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	session := startTestSession(t, s)

	s.setClock(testClock.Add(time.Minute))
	if _, err := s.PauseSession(ctx, session.ID); err != nil {
		t.Fatalf("PauseSession() failed: %v", err)
	}

	current, err := s.CurrentSession(ctx, testUserID)
	if err != nil {
		t.Fatalf("CurrentSession() returned: %v", err)
	}
	if current.Status != SessionPaused {
		t.Errorf("Status = %q, want %q", current.Status, SessionPaused)
	}
}

func TestSessionsAreNewestFirst(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	session := startTestSession(t, s)

	s.setClock(testClock.Add(time.Minute))
	if _, err := s.EndSession(ctx, session.ID); err != nil {
		t.Fatalf("EndSession() failed: %v", err)
	}

	s.setClock(testClock.Add(2 * time.Hour))
	if _, err := s.StartSession(ctx, Session{ID: "sess2", UserID: testUserID, Subject: "Chemistry"}); err != nil {
		t.Fatalf("StartSession() failed: %v", err)
	}

	sessions, err := s.Sessions(ctx, testUserID, 0)
	if err != nil {
		t.Fatalf("Sessions() returned: %v", err)
	}

	if len(sessions) != 2 {
		t.Fatalf("got %d sessions, want 2", len(sessions))
	}
	if sessions[0].Subject != "Chemistry" {
		t.Errorf("first session = %q, want the most recent one", sessions[0].Subject)
	}
}

func TestSessionsReturnsEmptySliceNotNil(t *testing.T) {
	s := newTestStore(t)
	sessions, err := s.Sessions(context.Background(), testUserID, 0)
	if err != nil {
		t.Fatalf("Sessions() returned: %v", err)
	}
	if sessions == nil {
		t.Error("Sessions() returned nil, want an empty slice")
	}
}

func TestTopicIsOptional(t *testing.T) {
	s := newTestStore(t)
	session, err := s.StartSession(context.Background(), Session{
		ID: "sess1", UserID: testUserID, Subject: "Mathematics",
	})
	if err != nil {
		t.Fatalf("StartSession() returned: %v", err)
	}

	if session.Topic != nil {
		t.Errorf("Topic = %v, want nil", *session.Topic)
	}
}
