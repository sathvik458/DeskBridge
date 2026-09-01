package httpapi

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func discardTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRequestLoggingRecordsOutcome(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	s := NewServer(log, "test", time.Now(), newFakeDeviceStore(), &fakeSessionStore{}, newFakeGoalStore(), &fakeMessageStore{}, "")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	line := buf.String()

	for _, want := range []string{`msg=request`, `method=GET`, `path=/health`, `status=200`} {
		if !strings.Contains(line, want) {
			t.Errorf("log line missing %q\ngot: %s", want, line)
		}
	}
}

func TestStatusRecorderCapturesNon200(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	s := NewServer(log, "test", time.Now(), newFakeDeviceStore(), &fakeSessionStore{}, newFakeGoalStore(), &fakeMessageStore{}, "")

	req := httptest.NewRequest(http.MethodGet, "/nope", nil)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	if !strings.Contains(buf.String(), "status=404") {
		t.Errorf("expected status=404 in log line, got: %s", buf.String())
	}
}

func TestStatusRecorderDefaultsTo200(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	})

	rec := &statusRecorder{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
	inner.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.status != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.status, http.StatusOK)
	}
	if rec.written != len("hello") {
		t.Errorf("written = %d, want %d", rec.written, len("hello"))
	}
}

func TestPreflightIsAnsweredForTheAllowedOrigin(t *testing.T) {
	s := NewServer(discardTestLogger(), "test", time.Now(), newFakeDeviceStore(), &fakeSessionStore{}, newFakeGoalStore(), &fakeMessageStore{}, "http://localhost:5173")

	req := httptest.NewRequest(http.MethodOptions, "/api/devices", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("Allow-Origin = %q, want the dev server origin", got)
	}
}

func TestOtherOriginsGetNoCORSHeaders(t *testing.T) {
	s := NewServer(discardTestLogger(), "test", time.Now(), newFakeDeviceStore(), &fakeSessionStore{}, newFakeGoalStore(), &fakeMessageStore{}, "http://localhost:5173")

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.Header.Set("Origin", "https://somewhere-else.example")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin = %q, want it absent for an unlisted origin", got)
	}
}
