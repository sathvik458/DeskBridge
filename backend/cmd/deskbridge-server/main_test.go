package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func quiet() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

var fakeAPI = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "api")
})

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestTheBuiltPageIsServedBesideTheAPI(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<p>deskbridge</p>"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := withFrontend(fakeAPI, dir, quiet())

	if body := get(h, "/").Body.String(); !strings.Contains(body, "deskbridge") {
		t.Errorf("/ served %q, want the built page", body)
	}
	if body := get(h, "/api/status").Body.String(); body != "api" {
		t.Errorf("/api/status served %q, want it passed to the API", body)
	}
	if body := get(h, "/health").Body.String(); body != "api" {
		t.Errorf("/health served %q, want it passed to the API", body)
	}
}

func TestNoBuildMeansTheAPIAlone(t *testing.T) {
	h := withFrontend(fakeAPI, filepath.Join(t.TempDir(), "missing"), quiet())

	if body := get(h, "/").Body.String(); body != "api" {
		t.Errorf("/ served %q, want everything to go to the API when there is no build", body)
	}
}

// http.Dir already refuses to climb out of its root; this keeps it that way.
func TestTheFrontendCannotReachOutsideItsFolder(t *testing.T) {
	outer := t.TempDir()
	dir := filepath.Join(outer, "dist")
	os.Mkdir(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("ok"), 0o644)
	os.WriteFile(filepath.Join(outer, "secret.txt"), []byte("keep out"), 0o644)

	h := withFrontend(fakeAPI, dir, quiet())

	if body := get(h, "/../secret.txt").Body.String(); strings.Contains(body, "keep out") {
		t.Error("a file outside the build folder was served")
	}
}
