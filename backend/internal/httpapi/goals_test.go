package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/store"
)

type fakeGoalStore struct {
	goals   map[string]store.Goal
	order   []string
	err     error
	deleted string
}

func newFakeGoalStore() *fakeGoalStore {
	return &fakeGoalStore{goals: map[string]store.Goal{}}
}

func (f *fakeGoalStore) CreateGoal(_ context.Context, goal store.Goal) (store.Goal, error) {
	if f.err != nil {
		return store.Goal{}, f.err
	}
	f.goals[goal.ID] = goal
	f.order = append(f.order, goal.ID)
	return goal, nil
}

func (f *fakeGoalStore) Goal(_ context.Context, id string) (store.Goal, error) {
	if f.err != nil {
		return store.Goal{}, f.err
	}
	goal, ok := f.goals[id]
	if !ok {
		return store.Goal{}, store.ErrNotFound
	}
	return goal, nil
}

func (f *fakeGoalStore) GoalsOn(_ context.Context, _, date string) ([]store.Goal, error) {
	if f.err != nil {
		return nil, f.err
	}
	goals := []store.Goal{}
	for _, id := range f.order {
		if goal := f.goals[id]; goal.GoalDate == date {
			goals = append(goals, goal)
		}
	}
	return goals, nil
}

func (f *fakeGoalStore) UpdateGoal(_ context.Context, goal store.Goal) (store.Goal, error) {
	if f.err != nil {
		return store.Goal{}, f.err
	}
	if _, ok := f.goals[goal.ID]; !ok {
		return store.Goal{}, store.ErrNotFound
	}
	f.goals[goal.ID] = goal
	return goal, nil
}

func (f *fakeGoalStore) SetGoalDone(_ context.Context, id string, done bool) (store.Goal, error) {
	if f.err != nil {
		return store.Goal{}, f.err
	}
	goal, ok := f.goals[id]
	if !ok {
		return store.Goal{}, store.ErrNotFound
	}
	if done {
		at := fixedNow
		goal.CompletedAt = &at
	} else {
		goal.CompletedAt = nil
	}
	f.goals[id] = goal
	return goal, nil
}

func (f *fakeGoalStore) DeleteGoal(_ context.Context, id string) error {
	if f.err != nil {
		return f.err
	}
	if _, ok := f.goals[id]; !ok {
		return store.ErrNotFound
	}
	delete(f.goals, id)
	f.deleted = id
	return nil
}

func newGoalServer(t *testing.T, goals GoalStore) *Server {
	t.Helper()

	s := newTestServerWithAll(time.Now(), newFakeDeviceStore(), &fakeSessionStore{}, goals)
	s.now = func() time.Time { return fixedNow }

	return s
}

func send(t *testing.T, s *Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	return rec
}

func createGoal(t *testing.T, s *Server, body string) goalResponse {
	t.Helper()

	rec := send(t, s, http.MethodPost, "/api/goals", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("creating goal: status %d, body %s", rec.Code, rec.Body)
	}

	var goal goalResponse
	if err := json.NewDecoder(rec.Body).Decode(&goal); err != nil {
		t.Fatalf("decoding goal: %v", err)
	}

	return goal
}

func TestCreateGoal(t *testing.T) {
	s := newGoalServer(t, newFakeGoalStore())

	goal := createGoal(t, s, `{"subject":"Mathematics","topic":"HCF and LCM","target_minutes":45,"goal_date":"2026-08-28"}`)

	if goal.Subject != "Mathematics" || goal.TargetMinutes != 45 {
		t.Errorf("goal = %+v, want the values sent", goal)
	}
	if goal.Done {
		t.Error("a new goal should not be done")
	}
}

func TestCreateGoalDefaultsToToday(t *testing.T) {
	s := newGoalServer(t, newFakeGoalStore())

	goal := createGoal(t, s, `{"subject":"Physics","target_minutes":60}`)

	if want := fixedNow.Format(store.DateLayout); goal.GoalDate != want {
		t.Errorf("goal_date = %q, want today (%q)", goal.GoalDate, want)
	}
}

func TestCreateGoalRejectsBadInput(t *testing.T) {
	tests := []struct{ name, body string }{
		{"missing subject", `{"target_minutes":45}`},
		{"blank subject", `{"subject":"  ","target_minutes":45}`},
		{"zero minutes", `{"subject":"Maths","target_minutes":0}`},
		{"negative minutes", `{"subject":"Maths","target_minutes":-30}`},
		{"absurd minutes", `{"subject":"Maths","target_minutes":5000}`},
		{"bad date", `{"subject":"Maths","target_minutes":45,"goal_date":"28-08-2026"}`},
		{"unknown field", `{"subject":"Maths","target_minutes":45,"priority":"high"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newGoalServer(t, newFakeGoalStore())

			if rec := send(t, s, http.MethodPost, "/api/goals", tc.body); rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d. body: %s", rec.Code, http.StatusBadRequest, rec.Body)
			}
		})
	}
}

func TestListGoalsFiltersByDate(t *testing.T) {
	s := newGoalServer(t, newFakeGoalStore())

	createGoal(t, s, `{"subject":"Maths","target_minutes":45,"goal_date":"2026-08-28"}`)
	createGoal(t, s, `{"subject":"Physics","target_minutes":60,"goal_date":"2026-08-28"}`)
	createGoal(t, s, `{"subject":"Chemistry","target_minutes":45,"goal_date":"2026-08-29"}`)

	rec := send(t, s, http.MethodGet, "/api/goals?date=2026-08-28", "")

	var goals []goalResponse
	if err := json.NewDecoder(rec.Body).Decode(&goals); err != nil {
		t.Fatalf("decoding goals: %v", err)
	}

	if len(goals) != 2 {
		t.Fatalf("got %d goals for the 28th, want 2", len(goals))
	}
}

func TestListGoalsRejectsABadDate(t *testing.T) {
	s := newGoalServer(t, newFakeGoalStore())

	if rec := send(t, s, http.MethodGet, "/api/goals?date=yesterday", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestListGoalsReturnsEmptyArray(t *testing.T) {
	s := newGoalServer(t, newFakeGoalStore())

	rec := send(t, s, http.MethodGet, "/api/goals", "")

	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("body = %s, want []", got)
	}
}

func TestCompleteAndReopenGoal(t *testing.T) {
	s := newGoalServer(t, newFakeGoalStore())
	goal := createGoal(t, s, `{"subject":"Maths","target_minutes":45}`)

	rec := send(t, s, http.MethodPost, "/api/goals/"+goal.ID+"/complete", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("complete: status %d, body %s", rec.Code, rec.Body)
	}

	var done goalResponse
	json.NewDecoder(rec.Body).Decode(&done)
	if !done.Done || done.CompletedAt == nil {
		t.Errorf("after completing: %+v, want done with a timestamp", done)
	}

	rec = send(t, s, http.MethodPost, "/api/goals/"+goal.ID+"/reopen", "")
	var reopened goalResponse
	json.NewDecoder(rec.Body).Decode(&reopened)

	if reopened.Done || reopened.CompletedAt != nil {
		t.Errorf("after reopening: %+v, want not done", reopened)
	}
}

func TestUpdateGoal(t *testing.T) {
	s := newGoalServer(t, newFakeGoalStore())
	goal := createGoal(t, s, `{"subject":"Maths","topic":"HCF","target_minutes":45}`)

	rec := send(t, s, http.MethodPatch, "/api/goals/"+goal.ID,
		`{"subject":"Mathematics","topic":"LCM","target_minutes":30}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: status %d, body %s", rec.Code, rec.Body)
	}

	var updated goalResponse
	json.NewDecoder(rec.Body).Decode(&updated)

	if updated.Subject != "Mathematics" || updated.TargetMinutes != 30 {
		t.Errorf("updated = %+v, want the new values", updated)
	}
	if updated.GoalDate != goal.GoalDate {
		t.Errorf("goal_date = %q, want it unchanged at %q", updated.GoalDate, goal.GoalDate)
	}
}

func TestDeleteGoal(t *testing.T) {
	goals := newFakeGoalStore()
	s := newGoalServer(t, goals)
	goal := createGoal(t, s, `{"subject":"Maths","target_minutes":45}`)

	rec := send(t, s, http.MethodDelete, "/api/goals/"+goal.ID, "")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if goals.deleted != goal.ID {
		t.Errorf("deleted %q, want %q", goals.deleted, goal.ID)
	}
}

func TestGoalOperationsOnUnknownIdAre404(t *testing.T) {
	tests := []struct{ name, method, path, body string }{
		{"complete", http.MethodPost, "/api/goals/ghost/complete", ""},
		{"reopen", http.MethodPost, "/api/goals/ghost/reopen", ""},
		{"delete", http.MethodDelete, "/api/goals/ghost", ""},
		{"update", http.MethodPatch, "/api/goals/ghost", `{"subject":"Maths","target_minutes":30}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newGoalServer(t, newFakeGoalStore())

			if rec := send(t, s, tc.method, tc.path, tc.body); rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want %d. body: %s", rec.Code, http.StatusNotFound, rec.Body)
			}
		})
	}
}

func TestGoalStoreFailureBecomes500(t *testing.T) {
	goals := newFakeGoalStore()
	goals.err = errors.New("disk is full")
	s := newGoalServer(t, goals)

	rec := send(t, s, http.MethodGet, "/api/goals", "")

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if strings.Contains(rec.Body.String(), "disk is full") {
		t.Errorf("internal error leaked: %s", rec.Body)
	}
}
