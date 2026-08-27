package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/store"
)

var fixedNow = time.Date(2026, 8, 27, 16, 45, 0, 0, time.UTC)

type fakeSessionStore struct {
	current  *store.Session
	history  []store.Session
	err      error
	lastMove string
}

func (f *fakeSessionStore) StartSession(_ context.Context, session store.Session) (store.Session, error) {
	if f.err != nil {
		return store.Session{}, f.err
	}
	if f.current != nil {
		return store.Session{}, store.ErrSessionRunning
	}

	session.Status = store.SessionActive
	session.StartedAt = fixedNow.Add(-10 * time.Minute)
	resumed := session.StartedAt
	session.LastResumedAt = &resumed
	f.current = &session

	return session, nil
}

func (f *fakeSessionStore) CurrentSession(context.Context, string) (store.Session, error) {
	if f.err != nil {
		return store.Session{}, f.err
	}
	if f.current == nil {
		return store.Session{}, store.ErrNotFound
	}
	return *f.current, nil
}

func (f *fakeSessionStore) Sessions(context.Context, string, int) ([]store.Session, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.history, nil
}

func (f *fakeSessionStore) PauseSession(_ context.Context, id string) (store.Session, error) {
	return f.move("pause", id, store.SessionPaused)
}

func (f *fakeSessionStore) ResumeSession(_ context.Context, id string) (store.Session, error) {
	return f.move("resume", id, store.SessionActive)
}

func (f *fakeSessionStore) EndSession(_ context.Context, id string) (store.Session, error) {
	return f.move("end", id, store.SessionCompleted)
}

func (f *fakeSessionStore) move(name, id, status string) (store.Session, error) {
	if f.err != nil {
		return store.Session{}, f.err
	}
	if f.current == nil || f.current.ID != id {
		return store.Session{}, store.ErrNotFound
	}

	f.lastMove = name
	f.current.Status = status

	return *f.current, nil
}

func newSessionServer(t *testing.T, sessions SessionStore) *Server {
	t.Helper()

	s := newTestServerWithStores(time.Now(), newFakeDeviceStore(), sessions)
	s.now = func() time.Time { return fixedNow }

	return s
}

func TestStartSession(t *testing.T) {
	sessions := &fakeSessionStore{}
	s := newSessionServer(t, sessions)

	rec := post(t, s, "/api/sessions", `{"subject":"Physics","topic":"Ray Optics"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d. body: %s", rec.Code, http.StatusCreated, rec.Body)
	}

	var body sessionResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if body.Subject != "Physics" || body.Status != store.SessionActive {
		t.Errorf("response = %+v, want an active Physics session", body)
	}

	if body.ElapsedSeconds != 600 {
		t.Errorf("elapsed_seconds = %d, want 600", body.ElapsedSeconds)
	}

	if body.ServerTime == "" {
		t.Error("server_time is empty, so a client cannot interpolate safely")
	}
}

func TestStartSessionRejectsBadInput(t *testing.T) {
	tests := []struct{ name, body string }{
		{"missing subject", `{"topic":"Optics"}`},
		{"blank subject", `{"subject":"   "}`},
		{"unknown field", `{"subject":"Physics","duration":45}`},
		{"not json", `nope`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newSessionServer(t, &fakeSessionStore{})

			if rec := post(t, s, "/api/sessions", tc.body); rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d. body: %s", rec.Code, http.StatusBadRequest, rec.Body)
			}
		})
	}
}

func TestStartSessionWhileOneIsRunningIsAConflict(t *testing.T) {
	sessions := &fakeSessionStore{}
	s := newSessionServer(t, sessions)

	post(t, s, "/api/sessions", `{"subject":"Physics"}`)
	rec := post(t, s, "/api/sessions", `{"subject":"Chemistry"}`)

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestCurrentSessionIs204WhenNoneIsRunning(t *testing.T) {
	s := newSessionServer(t, &fakeSessionStore{})

	rec := doRequest(t, s, http.MethodGet, "/api/sessions/current")

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestSessionTransitions(t *testing.T) {
	for _, move := range []string{"pause", "resume", "end"} {
		t.Run(move, func(t *testing.T) {
			sessions := &fakeSessionStore{}
			s := newSessionServer(t, sessions)

			post(t, s, "/api/sessions", `{"subject":"Physics"}`)
			id := sessions.current.ID

			rec := post(t, s, "/api/sessions/"+id+"/"+move, "")

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d. body: %s", rec.Code, http.StatusOK, rec.Body)
			}
			if sessions.lastMove != move {
				t.Errorf("store saw %q, want %q", sessions.lastMove, move)
			}
		})
	}
}

func TestTransitionOnUnknownSessionIs404(t *testing.T) {
	s := newSessionServer(t, &fakeSessionStore{})

	if rec := post(t, s, "/api/sessions/ghost/pause", ""); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestBadTransitionIsAConflict(t *testing.T) {
	sessions := &fakeSessionStore{err: store.ErrBadTransition}
	s := newSessionServer(t, sessions)

	if rec := post(t, s, "/api/sessions/s1/resume", ""); rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestListSessionsReturnsEmptyArray(t *testing.T) {
	s := newSessionServer(t, &fakeSessionStore{})

	rec := doRequest(t, s, http.MethodGet, "/api/sessions")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body []sessionResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if body == nil {
		t.Error("body decoded to nil, want an empty array")
	}
}

func TestSessionStoreFailureBecomes500(t *testing.T) {
	sessions := &fakeSessionStore{err: errors.New("database is gone")}
	s := newSessionServer(t, sessions)

	rec := doRequest(t, s, http.MethodGet, "/api/sessions")

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if strings.Contains(rec.Body.String(), "database is gone") {
		t.Errorf("internal error leaked: %s", rec.Body)
	}
}
