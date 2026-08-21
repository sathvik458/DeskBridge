package httpapi

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRequestLoggingRecordsOutcome(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	s := NewServer(log, "test", time.Now(), newFakeDeviceStore())

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
	s := NewServer(log, "test", time.Now(), newFakeDeviceStore())

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
