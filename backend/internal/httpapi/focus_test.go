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

	"github.com/sathvik458/deskbridge/backend/internal/focus"
	"github.com/sathvik458/deskbridge/backend/internal/live"
	"github.com/sathvik458/deskbridge/backend/internal/store"
)

type fakeFocusStore struct {
	runs []store.FocusRun
	err  error
	now  time.Time
}

func newFakeFocusStore() *fakeFocusStore {
	return &fakeFocusStore{now: focusNow}
}

func (f *fakeFocusStore) StartFocus(_ context.Context, run store.FocusRun) (store.FocusRun, error) {
	if f.err != nil {
		return store.FocusRun{}, f.err
	}

	for _, r := range f.runs {
		if r.EndedAt == nil {
			return store.FocusRun{}, store.ErrFocusRunning
		}
	}

	run.StartedAt = f.now
	f.runs = append(f.runs, run)

	return run, nil
}

func (f *fakeFocusStore) CurrentFocus(_ context.Context, _ string) (store.FocusRun, error) {
	if f.err != nil {
		return store.FocusRun{}, f.err
	}

	for _, r := range f.runs {
		if r.EndedAt == nil {
			return r, nil
		}
	}

	return store.FocusRun{}, store.ErrNotFound
}

func (f *fakeFocusStore) CloseFocus(_ context.Context, id, outcome string) (store.FocusRun, error) {
	if f.err != nil {
		return store.FocusRun{}, f.err
	}

	for i, r := range f.runs {
		if r.ID == id && r.EndedAt == nil {
			at := f.now
			f.runs[i].EndedAt = &at
			f.runs[i].Outcome = &outcome
			return f.runs[i], nil
		}
	}

	return store.FocusRun{}, store.ErrNotFound
}

func (f *fakeFocusStore) FinishedFocusBetween(_ context.Context, _ string, _, _ time.Time) (int, error) {
	if f.err != nil {
		return 0, f.err
	}

	n := 0
	for _, r := range f.runs {
		if r.Kind == store.FocusWork && r.Outcome != nil && *r.Outcome == store.FocusFinished {
			n++
		}
	}

	return n, nil
}

var focusNow = time.Date(2026, 9, 17, 9, 0, 0, 0, time.UTC)

func newFocusServer(t *testing.T, data *fakeFocusStore) *Server {
	t.Helper()

	log := discardTestLogger()
	s := NewServer(log, "test", focusNow, newFakeDeviceStore(), &fakeSessionStore{}, newFakeGoalStore(),
		&fakeMessageStore{}, newFakeFileStore(), newFakeBoardStore(), newFakeUserStore(),
		newFakeLoginStore(), newFakeInsightStore(), data, newFakeNoticeStore(), newTestVault(t), live.NewFeed(log),
		"http://localhost:5173", false)
	s.now = func() time.Time { return focusNow }

	return s
}

func startFocus(t *testing.T, s *Server, body string) focusResponse {
	t.Helper()

	rec := postJSON(t, s, "/api/focus", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("start status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body)
	}

	var out focusResponse
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decoding: %v", err)
	}

	return out
}

func TestStartingAFocusStretch(t *testing.T) {
	s := newFocusServer(t, newFakeFocusStore())

	out := startFocus(t, s, `{"kind":"focus"}`)

	if out.Kind != store.FocusWork {
		t.Errorf("kind = %q, want focus", out.Kind)
	}
	if out.PlannedSec != int64(focus.WorkFor.Seconds()) {
		t.Errorf("planned = %d, want %v", out.PlannedSec, focus.WorkFor.Seconds())
	}
	if out.LeftSec != out.PlannedSec {
		t.Errorf("left = %d at the start, want the full %d", out.LeftSec, out.PlannedSec)
	}
	if out.ServerTime == "" {
		t.Error("no server time, so the browser cannot count down safely")
	}
}

func TestKindDefaultsToWork(t *testing.T) {
	s := newFocusServer(t, newFakeFocusStore())

	if out := startFocus(t, s, `{}`); out.Kind != store.FocusWork {
		t.Errorf("kind = %q, want focus", out.Kind)
	}
}

func TestBreaksHaveTheirOwnLengths(t *testing.T) {
	for kind, want := range map[string]time.Duration{
		store.FocusShortBreak: focus.ShortBreakFor,
		store.FocusLongBreak:  focus.LongBreakFor,
	} {
		s := newFocusServer(t, newFakeFocusStore())

		out := startFocus(t, s, `{"kind":"`+kind+`"}`)
		if out.PlannedSec != int64(want.Seconds()) {
			t.Errorf("%s planned = %d, want %v", kind, out.PlannedSec, want.Seconds())
		}
	}
}

func TestAnUnknownKindIsRejected(t *testing.T) {
	s := newFocusServer(t, newFakeFocusStore())

	if rec := postJSON(t, s, "/api/focus", `{"kind":"nap"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestStartingWhileOneIsRunningIsAConflict(t *testing.T) {
	s := newFocusServer(t, newFakeFocusStore())
	startFocus(t, s, `{"kind":"focus"}`)

	if rec := postJSON(t, s, "/api/focus", `{"kind":"focus"}`); rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestNothingRunningReportsWhatIsNext(t *testing.T) {
	s := newFocusServer(t, newFakeFocusStore())

	rec := doRequest(t, s, http.MethodGet, "/api/focus/current")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var idle focusIdle
	if err := json.NewDecoder(rec.Body).Decode(&idle); err != nil {
		t.Fatalf("decoding: %v", err)
	}

	if idle.Running {
		t.Error("running = true with nothing started")
	}
	if idle.UpNext != store.FocusWork {
		t.Errorf("up next = %q, want focus", idle.UpNext)
	}
}

func TestCurrentReportsTheRunningStretch(t *testing.T) {
	s := newFocusServer(t, newFakeFocusStore())
	started := startFocus(t, s, `{"kind":"focus"}`)

	rec := doRequest(t, s, http.MethodGet, "/api/focus/current")

	var out struct {
		Running bool `json:"running"`
		focusResponse
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decoding: %v", err)
	}

	if !out.Running {
		t.Error("running = false while a stretch is going")
	}
	if out.ID != started.ID {
		t.Errorf("id = %q, want %q", out.ID, started.ID)
	}
}

func TestFinishingAStretch(t *testing.T) {
	data := newFakeFocusStore()
	s := newFocusServer(t, data)
	run := startFocus(t, s, `{"kind":"focus"}`)

	rec := postJSON(t, s, "/api/focus/"+run.ID+"/finish", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body)
	}

	var idle focusIdle
	if err := json.NewDecoder(rec.Body).Decode(&idle); err != nil {
		t.Fatalf("decoding: %v", err)
	}

	if idle.DoneToday != 1 {
		t.Errorf("done today = %d, want 1", idle.DoneToday)
	}
	if idle.UpNext != store.FocusShortBreak {
		t.Errorf("up next = %q, want a short break", idle.UpNext)
	}
}

// Stopping early is not an achievement, so it must not move the counter.
func TestStoppingEarlyDoesNotCount(t *testing.T) {
	s := newFocusServer(t, newFakeFocusStore())
	run := startFocus(t, s, `{"kind":"focus"}`)

	rec := postJSON(t, s, "/api/focus/"+run.ID+"/stop", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var idle focusIdle
	if err := json.NewDecoder(rec.Body).Decode(&idle); err != nil {
		t.Fatalf("decoding: %v", err)
	}

	if idle.DoneToday != 0 {
		t.Errorf("done today = %d after stopping early, want 0", idle.DoneToday)
	}
}

func TestAFourthFinishedStretchEarnsALongBreak(t *testing.T) {
	s := newFocusServer(t, newFakeFocusStore())

	var last focusIdle
	for i := 0; i < 4; i++ {
		run := startFocus(t, s, `{"kind":"focus"}`)

		rec := postJSON(t, s, "/api/focus/"+run.ID+"/finish", "")
		if err := json.NewDecoder(rec.Body).Decode(&last); err != nil {
			t.Fatalf("decoding: %v", err)
		}
	}

	if last.DoneToday != 4 {
		t.Fatalf("done today = %d, want 4", last.DoneToday)
	}
	if last.UpNext != store.FocusLongBreak {
		t.Errorf("up next = %q, want a long break after four", last.UpNext)
	}
	if last.IntoRound != 0 {
		t.Errorf("into round = %d, want it wrapped to 0", last.IntoRound)
	}
}

func TestClosingSomethingThatIsNotRunning(t *testing.T) {
	s := newFocusServer(t, newFakeFocusStore())

	for _, path := range []string{"/api/focus/ghost/finish", "/api/focus/ghost/stop"} {
		if rec := postJSON(t, s, path, ""); rec.Code != http.StatusNotFound {
			t.Errorf("%s = %d, want %d", path, rec.Code, http.StatusNotFound)
		}
	}
}

func TestStartingAnnouncesOnTheFeed(t *testing.T) {
	log := discardTestLogger()
	feed := live.NewFeed(log)
	events, stop := feed.Watch()
	defer stop()

	s := NewServer(log, "test", focusNow, newFakeDeviceStore(), &fakeSessionStore{}, newFakeGoalStore(),
		&fakeMessageStore{}, newFakeFileStore(), newFakeBoardStore(), newFakeUserStore(),
		newFakeLoginStore(), newFakeInsightStore(), newFakeFocusStore(), newFakeNoticeStore(), newTestVault(t), feed,
		"http://localhost:5173", false)
	s.now = func() time.Time { return focusNow }

	startFocus(t, s, `{"kind":"focus"}`)

	select {
	case e := <-events:
		if e.Kind != live.FocusChanged {
			t.Errorf("kind = %q, want %q", e.Kind, live.FocusChanged)
		}
	case <-time.After(time.Second):
		t.Fatal("no event after starting a stretch")
	}
}

func TestFocusNeedsSigningIn(t *testing.T) {
	s := newFocusServer(t, newFakeFocusStore())

	req := guarded(httptest.NewRequest(http.MethodPost, "/api/focus", strings.NewReader("{}")))
	req.Header.Set("Content-Type", "application/json")

	if rec := fire(t, s, req); rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAFocusStoreFailureBecomes500(t *testing.T) {
	data := newFakeFocusStore()
	data.err = errors.New("database is on fire")

	s := newFocusServer(t, data)

	if rec := doRequest(t, s, http.MethodGet, "/api/focus/current"); rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
