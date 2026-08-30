package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

const testDate = "2026-08-28"

func seedGoal(t *testing.T, s *Store, id, subject, date string, minutes int) Goal {
	t.Helper()

	goal, err := s.CreateGoal(context.Background(), Goal{
		ID: id, UserID: testUserID, Subject: subject, TargetMinutes: minutes, GoalDate: date,
	})
	if err != nil {
		t.Fatalf("CreateGoal(%s) returned an unexpected error: %v", subject, err)
	}

	return goal
}

func TestCreateAndReadGoal(t *testing.T) {
	s := newTestStore(t)
	topic := "HCF and LCM"

	created, err := s.CreateGoal(context.Background(), Goal{
		ID: "g1", UserID: testUserID, Subject: "Mathematics", Topic: &topic,
		TargetMinutes: 45, GoalDate: testDate,
	})
	if err != nil {
		t.Fatalf("CreateGoal() returned an unexpected error: %v", err)
	}

	if created.Done() {
		t.Error("a new goal should not be done")
	}
	if created.Topic == nil || *created.Topic != topic {
		t.Errorf("Topic = %v, want %q", created.Topic, topic)
	}

	found, err := s.Goal(context.Background(), "g1")
	if err != nil {
		t.Fatalf("Goal() returned: %v", err)
	}
	if found.TargetMinutes != 45 {
		t.Errorf("TargetMinutes = %d, want 45", found.TargetMinutes)
	}
}

func TestGoalsAreScopedToTheirDate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	seedGoal(t, s, "g1", "Mathematics", testDate, 45)
	seedGoal(t, s, "g2", "Physics", testDate, 60)
	seedGoal(t, s, "g3", "Chemistry", "2026-08-29", 45)

	today, err := s.GoalsOn(ctx, testUserID, testDate)
	if err != nil {
		t.Fatalf("GoalsOn() returned: %v", err)
	}
	if len(today) != 2 {
		t.Fatalf("got %d goals for %s, want 2", len(today), testDate)
	}

	// Ordered by creation, so the plan reads in the order it was written.
	if today[0].Subject != "Mathematics" || today[1].Subject != "Physics" {
		t.Errorf("order = %s, %s; want Mathematics, Physics", today[0].Subject, today[1].Subject)
	}

	tomorrow, err := s.GoalsOn(ctx, testUserID, "2026-08-29")
	if err != nil {
		t.Fatalf("GoalsOn() returned: %v", err)
	}
	if len(tomorrow) != 1 {
		t.Errorf("got %d goals for the 29th, want 1", len(tomorrow))
	}
}

func TestGoalsOnReturnsEmptySliceNotNil(t *testing.T) {
	s := newTestStore(t)

	goals, err := s.GoalsOn(context.Background(), testUserID, testDate)
	if err != nil {
		t.Fatalf("GoalsOn() returned: %v", err)
	}
	if goals == nil {
		t.Error("GoalsOn() returned nil, want an empty slice")
	}
}

func TestCompleteAndReopenGoal(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedGoal(t, s, "g1", "Mathematics", testDate, 45)

	later := testClock.Add(2 * time.Hour)
	s.setClock(later)

	done, err := s.SetGoalDone(ctx, "g1", true)
	if err != nil {
		t.Fatalf("SetGoalDone(true) returned: %v", err)
	}
	if !done.Done() {
		t.Fatal("goal is not marked done")
	}
	if !done.CompletedAt.Equal(later) {
		t.Errorf("CompletedAt = %s, want %s", done.CompletedAt, later)
	}

	reopened, err := s.SetGoalDone(ctx, "g1", false)
	if err != nil {
		t.Fatalf("SetGoalDone(false) returned: %v", err)
	}
	if reopened.Done() {
		t.Error("goal is still marked done after reopening")
	}
	if reopened.CompletedAt != nil {
		t.Error("CompletedAt should be cleared when a goal is reopened")
	}
}

func TestUpdateGoalKeepsCompletion(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	goal := seedGoal(t, s, "g1", "Mathematics", testDate, 45)

	if _, err := s.SetGoalDone(ctx, goal.ID, true); err != nil {
		t.Fatalf("SetGoalDone() returned: %v", err)
	}

	goal, err := s.Goal(ctx, goal.ID)
	if err != nil {
		t.Fatalf("Goal() returned: %v", err)
	}

	goal.Subject = "Maths"
	goal.TargetMinutes = 30

	updated, err := s.UpdateGoal(ctx, goal)
	if err != nil {
		t.Fatalf("UpdateGoal() returned: %v", err)
	}

	if updated.Subject != "Maths" || updated.TargetMinutes != 30 {
		t.Errorf("updated = %+v, want the new values", updated)
	}
	if !updated.Done() {
		t.Error("editing a goal should not un-complete it")
	}
}

func TestDeleteGoal(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedGoal(t, s, "g1", "Mathematics", testDate, 45)

	if err := s.DeleteGoal(ctx, "g1"); err != nil {
		t.Fatalf("DeleteGoal() returned: %v", err)
	}

	if _, err := s.Goal(ctx, "g1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestGoalOperationsOnUnknownId(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.Goal(ctx, "ghost"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Goal(): error = %v, want ErrNotFound", err)
	}
	if _, err := s.SetGoalDone(ctx, "ghost", true); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetGoalDone(): error = %v, want ErrNotFound", err)
	}
	if err := s.DeleteGoal(ctx, "ghost"); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteGoal(): error = %v, want ErrNotFound", err)
	}
	if _, err := s.UpdateGoal(ctx, Goal{ID: "ghost", Subject: "Maths", TargetMinutes: 30, GoalDate: testDate}); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateGoal(): error = %v, want ErrNotFound", err)
	}
}

func TestGoalRejectsNonPositiveTarget(t *testing.T) {
	s := newTestStore(t)

	_, err := s.CreateGoal(context.Background(), Goal{
		ID: "g1", UserID: testUserID, Subject: "Mathematics", TargetMinutes: 0, GoalDate: testDate,
	})

	if err == nil {
		t.Error("CreateGoal() accepted a target of 0, want the check constraint to reject it")
	}
}
