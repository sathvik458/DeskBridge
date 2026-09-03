package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/live"
	"github.com/sathvik458/deskbridge/backend/internal/store"
	"github.com/sathvik458/deskbridge/backend/internal/vault"
)

type fakeFileStore struct {
	files []store.File
	err   error
}

func newFakeFileStore() *fakeFileStore {
	return &fakeFileStore{}
}

func (f *fakeFileStore) RecordUpload(_ context.Context, file store.File) (store.File, error) {
	if f.err != nil {
		return store.File{}, f.err
	}
	file.CreatedAt = fixedNow
	f.files = append(f.files, file)
	return file, nil
}

func (f *fakeFileStore) File(_ context.Context, id string) (store.File, error) {
	if f.err != nil {
		return store.File{}, f.err
	}
	for _, file := range f.files {
		if file.ID == id {
			return file, nil
		}
	}
	return store.File{}, store.ErrNotFound
}

func (f *fakeFileStore) Files(_ context.Context, category string) ([]store.File, error) {
	if f.err != nil {
		return nil, f.err
	}

	matching := []store.File{}
	for _, file := range f.files {
		if category == "" || file.Category == category {
			matching = append(matching, file)
		}
	}

	return matching, nil
}

func (f *fakeFileStore) ForgetFile(_ context.Context, id string) (store.File, error) {
	if f.err != nil {
		return store.File{}, f.err
	}

	for i, file := range f.files {
		if file.ID == id {
			f.files = append(f.files[:i], f.files[i+1:]...)
			return file, nil
		}
	}

	return store.File{}, store.ErrNotFound
}

func newFileServer(t *testing.T, files FileStore, shelf *vault.Vault, feed *live.Feed) *Server {
	t.Helper()

	log := discardTestLogger()

	return NewServer(log, "test", time.Now(), newFakeDeviceStore(), &fakeSessionStore{},
		newFakeGoalStore(), &fakeMessageStore{}, files, shelf, feed, "http://localhost:5173")
}

func uploadBody(t *testing.T, name, category, body string) (io.Reader, string) {
	t.Helper()

	buf := &bytes.Buffer{}
	form := multipart.NewWriter(buf)

	if category != "" {
		if err := form.WriteField("category", category); err != nil {
			t.Fatalf("writing the category field: %v", err)
		}
	}

	part, err := form.CreateFormFile("file", name)
	if err != nil {
		t.Fatalf("creating the file part: %v", err)
	}
	if _, err := io.WriteString(part, body); err != nil {
		t.Fatalf("writing the file part: %v", err)
	}

	if err := form.Close(); err != nil {
		t.Fatalf("closing the form: %v", err)
	}

	return buf, form.FormDataContentType()
}

func postUpload(t *testing.T, s *Server, name, category, body string) *httptest.ResponseRecorder {
	t.Helper()

	payload, contentType := uploadBody(t, name, category, body)

	req := httptest.NewRequest(http.MethodPost, "/api/files", payload)
	req.Header.Set("Content-Type", contentType)

	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	return rec
}

func decodeFile(t *testing.T, rec *httptest.ResponseRecorder) fileResponse {
	t.Helper()

	var body fileResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding the response: %v", err)
	}

	return body
}

func TestUploadStoresTheBytesAndTheRow(t *testing.T) {
	files := newFakeFileStore()
	shelf := newTestVault(t)
	s := newFileServer(t, files, shelf, live.NewFeed(discardTestLogger()))

	rec := postUpload(t, s, "thermo.pdf", store.ShelfNotes, "chapter four")

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body)
	}

	body := decodeFile(t, rec)

	if body.Name != "thermo.pdf" {
		t.Errorf("name = %q, want %q", body.Name, "thermo.pdf")
	}
	if body.Category != store.ShelfNotes {
		t.Errorf("category = %q, want %q", body.Category, store.ShelfNotes)
	}
	if body.SizeBytes != int64(len("chapter four")) {
		t.Errorf("size_bytes = %d, want %d", body.SizeBytes, len("chapter four"))
	}
	if len(body.Checksum) != 64 {
		t.Errorf("checksum = %q, want 64 hex characters", body.Checksum)
	}

	if len(files.files) != 1 {
		t.Fatalf("the store holds %d rows, want 1", len(files.files))
	}

	handle, err := shelf.Read(files.files[0].StoredPath)
	if err != nil {
		t.Fatalf("the bytes are not on disk: %v", err)
	}
	defer handle.Close()

	kept, err := io.ReadAll(handle)
	if err != nil {
		t.Fatalf("reading the stored file: %v", err)
	}
	if string(kept) != "chapter four" {
		t.Errorf("stored %q, want %q", kept, "chapter four")
	}
}

func TestUploadDefaultsToTheOtherShelf(t *testing.T) {
	files := newFakeFileStore()
	s := newFileServer(t, files, newTestVault(t), live.NewFeed(discardTestLogger()))

	rec := postUpload(t, s, "mystery.bin", "", "some bytes")

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body)
	}

	if got := decodeFile(t, rec).Category; got != store.ShelfOther {
		t.Errorf("category = %q, want %q", got, store.ShelfOther)
	}
}

func TestUploadRejectsAnUnknownShelf(t *testing.T) {
	s := newFileServer(t, newFakeFileStore(), newTestVault(t), live.NewFeed(discardTestLogger()))

	rec := postUpload(t, s, "cat.png", "memes", "meow")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUploadRejectsAnEmptyFile(t *testing.T) {
	files := newFakeFileStore()
	s := newFileServer(t, files, newTestVault(t), live.NewFeed(discardTestLogger()))

	rec := postUpload(t, s, "empty.txt", store.ShelfNotes, "")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if len(files.files) != 0 {
		t.Error("an empty upload was recorded, want it rejected before the row")
	}
}

func TestUploadNeedsAFilePart(t *testing.T) {
	buf := &bytes.Buffer{}
	form := multipart.NewWriter(buf)
	if err := form.WriteField("category", store.ShelfNotes); err != nil {
		t.Fatalf("writing the category field: %v", err)
	}
	form.Close()

	s := newFileServer(t, newFakeFileStore(), newTestVault(t), live.NewFeed(discardTestLogger()))

	req := httptest.NewRequest(http.MethodPost, "/api/files", buf)
	req.Header.Set("Content-Type", form.FormDataContentType())
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUploadRejectsAPlainJSONBody(t *testing.T) {
	s := newFileServer(t, newFakeFileStore(), newTestVault(t), live.NewFeed(discardTestLogger()))

	req := httptest.NewRequest(http.MethodPost, "/api/files", strings.NewReader(`{"name":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// The name comes from the browser, so a client that sends a path must not be able to
// steer where the bytes land or what the download header says.
func TestUploadStripsAPathOutOfTheName(t *testing.T) {
	files := newFakeFileStore()
	s := newFileServer(t, files, newTestVault(t), live.NewFeed(discardTestLogger()))

	rec := postUpload(t, s, "../../etc/passwd", store.ShelfNotes, "root:x:0:0")

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body)
	}

	if got := decodeFile(t, rec).Name; got != "passwd" {
		t.Errorf("name = %q, want %q", got, "passwd")
	}

	if path := files.files[0].StoredPath; strings.Contains(path, "..") || strings.Contains(path, "passwd") {
		t.Errorf("stored path = %q, want it derived from the id and not the name", path)
	}
}

func TestUploadDoesNotLeaveTheBytesWhenTheRowFails(t *testing.T) {
	files := newFakeFileStore()
	files.err = errors.New("database is on fire")
	shelf := newTestVault(t)
	s := newFileServer(t, files, shelf, live.NewFeed(discardTestLogger()))

	rec := postUpload(t, s, "thermo.pdf", store.ShelfNotes, "chapter four")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	// Nothing recorded the path, so the only way to find a leaked blob is to look.
	leaked := countInVault(t, shelf)

	if leaked != 0 {
		t.Errorf("%d files left in the vault, want the failed upload cleaned up", leaked)
	}
}

func TestUploadAnnouncesOnTheFeed(t *testing.T) {
	feed := live.NewFeed(discardTestLogger())
	events, stop := feed.Watch()
	defer stop()

	s := newFileServer(t, newFakeFileStore(), newTestVault(t), feed)

	if rec := postUpload(t, s, "thermo.pdf", store.ShelfNotes, "chapter four"); rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	select {
	case event := <-events:
		if event.Kind != live.FileAdded {
			t.Errorf("kind = %q, want %q", event.Kind, live.FileAdded)
		}
	case <-time.After(time.Second):
		t.Fatal("no event arrived after an upload")
	}
}

func TestListFilesCanBeNarrowed(t *testing.T) {
	files := newFakeFileStore()
	s := newFileServer(t, files, newTestVault(t), live.NewFeed(discardTestLogger()))

	postUpload(t, s, "thermo.pdf", store.ShelfNotes, "one")
	postUpload(t, s, "sheet.docx", store.ShelfHomework, "two")

	rec := doRequest(t, s, http.MethodGet, "/api/files?category=notes")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body []fileResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding the response: %v", err)
	}

	if len(body) != 1 || body[0].Name != "thermo.pdf" {
		t.Errorf("got %+v, want only thermo.pdf", body)
	}
}

func TestListFilesRejectsAnUnknownShelf(t *testing.T) {
	s := newFileServer(t, newFakeFileStore(), newTestVault(t), live.NewFeed(discardTestLogger()))

	rec := doRequest(t, s, http.MethodGet, "/api/files?category=memes")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestListFilesEncodesAsAnArray(t *testing.T) {
	s := newFileServer(t, newFakeFileStore(), newTestVault(t), live.NewFeed(discardTestLogger()))

	rec := doRequest(t, s, http.MethodGet, "/api/files")

	if trimmed := strings.TrimSpace(rec.Body.String()); trimmed != "[]" {
		t.Errorf("body = %s, want []", trimmed)
	}
}

func TestDownloadReturnsTheBytes(t *testing.T) {
	files := newFakeFileStore()
	s := newFileServer(t, files, newTestVault(t), live.NewFeed(discardTestLogger()))

	created := decodeFile(t, postUpload(t, s, "thermo.pdf", store.ShelfNotes, "chapter four"))

	rec := doRequest(t, s, http.MethodGet, "/api/files/"+created.ID+"/download")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "chapter four" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "chapter four")
	}

	disposition := rec.Header().Get("Content-Disposition")
	if !strings.Contains(disposition, `filename="thermo.pdf"`) {
		t.Errorf("Content-Disposition = %q, want it to name the file", disposition)
	}
	if got := rec.Header().Get("X-Checksum-Sha256"); got != created.Checksum {
		t.Errorf("checksum header = %q, want %q", got, created.Checksum)
	}
}

func TestDownloadHandlesANameWithAccents(t *testing.T) {
	s := newFileServer(t, newFakeFileStore(), newTestVault(t), live.NewFeed(discardTestLogger()))

	created := decodeFile(t, postUpload(t, s, "révision été.pdf", store.ShelfNotes, "notes"))

	rec := doRequest(t, s, http.MethodGet, "/api/files/"+created.ID+"/download")

	disposition := rec.Header().Get("Content-Disposition")
	if !strings.Contains(disposition, "filename*=UTF-8''") {
		t.Errorf("Content-Disposition = %q, want a UTF-8 spelling too", disposition)
	}

	// Everything in a header has to be printable ASCII or the response is invalid.
	for _, r := range disposition {
		if r > 127 {
			t.Fatalf("Content-Disposition carries a raw non-ASCII rune: %q", disposition)
		}
	}
}

func TestDownloadUnknownFile(t *testing.T) {
	s := newFileServer(t, newFakeFileStore(), newTestVault(t), live.NewFeed(discardTestLogger()))

	rec := doRequest(t, s, http.MethodGet, "/api/files/ghost/download")

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestVerifySaysTheFileIsIntact(t *testing.T) {
	s := newFileServer(t, newFakeFileStore(), newTestVault(t), live.NewFeed(discardTestLogger()))

	created := decodeFile(t, postUpload(t, s, "thermo.pdf", store.ShelfNotes, "chapter four"))

	rec := doRequest(t, s, http.MethodPost, "/api/files/"+created.ID+"/verify")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body verifyResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding the response: %v", err)
	}

	if !body.Intact {
		t.Errorf("intact = false for an untouched file, expected %q found %q", body.Expected, body.Found)
	}
}

func TestVerifyNoticesAChangedFile(t *testing.T) {
	files := newFakeFileStore()
	shelf := newTestVault(t)
	s := newFileServer(t, files, shelf, live.NewFeed(discardTestLogger()))

	created := decodeFile(t, postUpload(t, s, "thermo.pdf", store.ShelfNotes, "chapter four"))

	rewriteInVault(t, shelf, files.files[0].StoredPath, "chapter five")

	rec := doRequest(t, s, http.MethodPost, "/api/files/"+created.ID+"/verify")

	var body verifyResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding the response: %v", err)
	}

	if body.Intact {
		t.Error("intact = true after the bytes changed underneath")
	}
	if body.Found == body.Expected {
		t.Error("the reported checksums match, want them to differ")
	}
}

func TestDeleteRemovesTheRowAndTheBytes(t *testing.T) {
	files := newFakeFileStore()
	shelf := newTestVault(t)
	s := newFileServer(t, files, shelf, live.NewFeed(discardTestLogger()))

	created := decodeFile(t, postUpload(t, s, "thermo.pdf", store.ShelfNotes, "chapter four"))
	storedPath := files.files[0].StoredPath

	rec := doRequest(t, s, http.MethodDelete, "/api/files/"+created.ID)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("204 came with a body: %s", rec.Body)
	}

	if len(files.files) != 0 {
		t.Error("the row survived the delete")
	}
	if _, err := shelf.Read(storedPath); err == nil {
		t.Error("the bytes survived the delete")
	}
}

func TestDeleteUnknownFile(t *testing.T) {
	s := newFileServer(t, newFakeFileStore(), newTestVault(t), live.NewFeed(discardTestLogger()))

	rec := doRequest(t, s, http.MethodDelete, "/api/files/ghost")

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDeleteAnnouncesOnTheFeed(t *testing.T) {
	feed := live.NewFeed(discardTestLogger())
	files := newFakeFileStore()
	s := newFileServer(t, files, newTestVault(t), feed)

	created := decodeFile(t, postUpload(t, s, "thermo.pdf", store.ShelfNotes, "chapter four"))

	events, stop := feed.Watch()
	defer stop()

	if rec := doRequest(t, s, http.MethodDelete, "/api/files/"+created.ID); rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	select {
	case event := <-events:
		if event.Kind != live.FileRemoved {
			t.Errorf("kind = %q, want %q", event.Kind, live.FileRemoved)
		}
	case <-time.After(time.Second):
		t.Fatal("no event arrived after a delete")
	}
}

func TestTidyName(t *testing.T) {
	cases := map[string]string{
		"thermo.pdf":           "thermo.pdf",
		"../../etc/passwd":     "passwd",
		`C:\Users\me\notes.md`: "notes.md",
		"  spaced.txt  ":       "spaced.txt",
		"bad\x00name.txt":      "badname.txt",
		"..":                   "",
		"/":                    "",
		"":                     "",
	}

	for raw, want := range cases {
		if got := tidyName(raw); got != want {
			t.Errorf("tidyName(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestTidyNameCapsTheLength(t *testing.T) {
	long := strings.Repeat("a", 500) + ".pdf"

	if got := tidyName(long); len(got) > maxUploadedName {
		t.Errorf("tidyName kept %d characters, want at most %d", len(got), maxUploadedName)
	}
}

func TestGuessType(t *testing.T) {
	if got := guessType("board.png"); !strings.HasPrefix(got, "image/png") {
		t.Errorf("guessType(png) = %q, want an image type", got)
	}
	if got := guessType("mystery.zzz"); got != "application/octet-stream" {
		t.Errorf("guessType(unknown) = %q, want application/octet-stream", got)
	}
}

func countInVault(t *testing.T, shelf *vault.Vault) int {
	t.Helper()

	stored := 0

	err := filepath.WalkDir(shelf.Root(), func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			stored++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the vault: %v", err)
	}

	return stored
}

func rewriteInVault(t *testing.T, shelf *vault.Vault, relative, body string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(shelf.Root(), relative), []byte(body), 0o600); err != nil {
		t.Fatalf("rewriting %s: %v", relative, err)
	}
}
