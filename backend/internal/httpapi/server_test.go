package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestServer(startedAt time.Time) *Server {
	return newTestServerWithStore(startedAt, newFakeDeviceStore())
}

func newTestServerWithStore(startedAt time.Time, devices DeviceStore) *Server {
	return newTestServerWithStores(startedAt, devices, &fakeSessionStore{})
}

func newTestServerWithStores(startedAt time.Time, devices DeviceStore, sessions SessionStore) *Server {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewServer(log, "test", startedAt, devices, sessions, "http://localhost:5173")
}

func doRequest(t *testing.T, s *Server, method, path string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()

	s.Routes().ServeHTTP(rec, req)

	return rec
}

func TestHealthReturnsOK(t *testing.T) {
	s := newTestServer(time.Now())

	rec := doRequest(t, s, http.MethodGet, "/health")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var body healthResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}

	if body.Status != "ok" {
		t.Errorf("status field = %q, want %q", body.Status, "ok")
	}
}

func TestStatusReportsUptime(t *testing.T) {
	startedAt := time.Now().Add(-90 * time.Second)
	s := newTestServer(startedAt)

	rec := doRequest(t, s, http.MethodGet, "/api/status")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body statusResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}

	if body.Version != "test" {
		t.Errorf("version = %q, want %q", body.Version, "test")
	}

	if body.UptimeSeconds < 89 || body.UptimeSeconds > 92 {
		t.Errorf("uptime_seconds = %d, want roughly 90", body.UptimeSeconds)
	}

	if body.Uptime != "1m30s" {
		t.Errorf("uptime = %q, want %q", body.Uptime, "1m30s")
	}

	if body.GoVersion == "" {
		t.Error("go_version is empty")
	}
}

func TestUnknownRouteReturns404(t *testing.T) {
	s := newTestServer(time.Now())

	rec := doRequest(t, s, http.MethodGet, "/api/does-not-exist")

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestWrongMethodIsRejected(t *testing.T) {
	s := newTestServer(time.Now())

	rec := doRequest(t, s, http.MethodPost, "/health")

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}
