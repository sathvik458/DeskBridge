package httpapi

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/sathvik458/deskbridge/backend/internal/live"
	"github.com/sathvik458/deskbridge/backend/internal/store"
	"github.com/sathvik458/deskbridge/backend/internal/vault"
)

const (
	maxUploadBytes  = 25 << 20
	uploadWindow    = 5 * time.Minute
	maxUploadedName = 200
	uploadField     = "file"
	shelfField      = "category"
)

type fileResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Category  string `json:"category"`
	SizeBytes int64  `json:"size_bytes"`
	Checksum  string `json:"checksum"`
	CreatedAt string `json:"created_at"`
}

func newFileResponse(file store.File) fileResponse {
	return fileResponse{
		ID:        file.ID,
		Name:      file.OriginalName,
		Category:  file.Category,
		SizeBytes: file.SizeBytes,
		Checksum:  file.Checksum,
		CreatedAt: file.CreatedAt.Format(time.RFC3339),
	}
}

func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	shelf := r.URL.Query().Get("category")

	if shelf != "" && !store.IsShelf(shelf) {
		s.writeError(w, http.StatusBadRequest, "unknown category "+shelf)
		return
	}

	files, err := s.files.Files(r.Context(), shelf)
	if err != nil {
		s.log.Error("listing files", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not list the files")
		return
	}

	out := make([]fileResponse, 0, len(files))
	for _, file := range files {
		out = append(out, newFileResponse(file))
	}

	s.writeJSON(w, http.StatusOK, out)
}

// The body is read part by part rather than through ParseMultipartForm, which would
// spool the whole upload before the handler ever sees it.
func (s *Server) handleUploadFile(w http.ResponseWriter, r *http.Request) {
	// A slow 25 MB upload outlasts the server's 15s ReadTimeout, so this one
	// connection gets a longer window. A writer with no connection cannot, and need not.
	err := http.NewResponseController(w).SetReadDeadline(s.now().Add(uploadWindow))
	if err != nil && !errors.Is(err, errors.ErrUnsupported) {
		s.log.Error("extending the read deadline", "error", err)
		s.writeError(w, http.StatusInternalServerError, "this server cannot accept uploads")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+(1<<20))

	parts, err := r.MultipartReader()
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "expected a multipart/form-data body")
		return
	}

	shelf := store.ShelfOther
	var kept *store.File

	for {
		part, err := parts.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			s.rollBack(kept)
			s.writeError(w, http.StatusBadRequest, "the upload body ended early")
			return
		}

		switch part.FormName() {
		case shelfField:
			chosen, err := readShortField(part)
			part.Close()
			if err != nil {
				s.rollBack(kept)
				s.writeError(w, http.StatusBadRequest, "category is too long")
				return
			}
			if chosen != "" && !store.IsShelf(chosen) {
				s.rollBack(kept)
				s.writeError(w, http.StatusBadRequest, "unknown category "+chosen)
				return
			}
			if chosen != "" {
				shelf = chosen
			}

		case uploadField:
			if kept != nil {
				part.Close()
				s.rollBack(kept)
				s.writeError(w, http.StatusBadRequest, "send one file per request")
				return
			}

			file, status, err := s.absorb(part, shelf)
			part.Close()
			if err != nil {
				if status == http.StatusInternalServerError {
					s.log.Error("storing an upload", "error", err)
					s.writeError(w, status, "could not store the file")
					return
				}
				s.writeError(w, status, err.Error())
				return
			}
			kept = &file

		default:
			part.Close()
		}
	}

	if kept == nil {
		s.writeError(w, http.StatusBadRequest, "no file part in the request")
		return
	}

	// A category part after the file part still counts, so the row is only written
	// once the whole body has been read and the shelf is final.
	kept.Category = shelf

	saved, err := s.files.RecordUpload(r.Context(), *kept)
	if err != nil {
		s.rollBack(kept)
		s.log.Error("recording an upload", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not save the file")
		return
	}

	s.feed.Announce(live.FileAdded, map[string]any{
		"file_id": saved.ID, "name": saved.OriginalName, "category": saved.Category,
	})

	s.writeJSON(w, http.StatusCreated, newFileResponse(saved))
}

func (s *Server) absorb(part *multipart.Part, shelf string) (store.File, int, error) {
	name := tidyName(part.FileName())
	if name == "" {
		return store.File{}, http.StatusBadRequest, errors.New("the file needs a name")
	}

	id := newID()

	receipt, err := s.vault.Keep(id, part, maxUploadBytes)
	if errors.Is(err, vault.ErrTooBig) {
		return store.File{}, http.StatusRequestEntityTooLarge, fmt.Errorf("files must be under %d MB", maxUploadBytes>>20)
	}
	if err != nil {
		return store.File{}, http.StatusInternalServerError, err
	}

	if receipt.Size == 0 {
		s.vault.Discard(receipt.Path)
		return store.File{}, http.StatusBadRequest, errors.New("the file is empty")
	}

	return store.File{
		ID:           id,
		UploaderID:   defaultUserID,
		Category:     shelf,
		OriginalName: name,
		StoredPath:   receipt.Path,
		SizeBytes:    receipt.Size,
		Checksum:     receipt.Checksum,
	}, 0, nil
}

func (s *Server) handleDownloadFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	file, err := s.files.File(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		s.writeError(w, http.StatusNotFound, "file not found")
		return
	}
	if err != nil {
		s.log.Error("looking up a file", "file_id", id, "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not read the file")
		return
	}

	handle, err := s.vault.Read(file.StoredPath)
	if err != nil {
		s.log.Error("opening a stored file", "file_id", id, "error", err)
		s.writeError(w, http.StatusInternalServerError, "the file is missing from disk")
		return
	}
	defer handle.Close()

	w.Header().Set("Content-Type", guessType(file.OriginalName))
	w.Header().Set("Content-Disposition", attachmentHeader(file.OriginalName))
	w.Header().Set("X-Checksum-Sha256", file.Checksum)

	// ServeContent brings range requests and conditional GETs along for free, which
	// matters on a phone tethering over a patchy connection.
	http.ServeContent(w, r, file.OriginalName, file.CreatedAt, handle)
}

type verifyResponse struct {
	ID       string `json:"id"`
	Intact   bool   `json:"intact"`
	Expected string `json:"expected"`
	Found    string `json:"found"`
}

func (s *Server) handleVerifyFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	file, err := s.files.File(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		s.writeError(w, http.StatusNotFound, "file not found")
		return
	}
	if err != nil {
		s.log.Error("looking up a file", "file_id", id, "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not read the file")
		return
	}

	found, err := s.vault.Checksum(file.StoredPath)
	if err != nil {
		s.log.Error("hashing a stored file", "file_id", id, "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not read the file from disk")
		return
	}

	intact := found == file.Checksum
	if !intact {
		s.log.Warn("checksum mismatch", "file_id", id, "expected", file.Checksum, "found", found)
	}

	s.writeJSON(w, http.StatusOK, verifyResponse{
		ID: file.ID, Intact: intact, Expected: file.Checksum, Found: found,
	})
}

// The row goes first. An orphaned blob is a wasted megabyte that a sweep can find
// later; a row pointing at nothing is a 500 every time someone clicks download.
func (s *Server) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	file, err := s.files.ForgetFile(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		s.writeError(w, http.StatusNotFound, "file not found")
		return
	}
	if err != nil {
		s.log.Error("deleting a file", "file_id", id, "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not delete the file")
		return
	}

	if err := s.vault.Discard(file.StoredPath); err != nil {
		s.log.Error("removing a stored file", "file_id", id, "path", file.StoredPath, "error", err)
	}

	s.feed.Announce(live.FileRemoved, map[string]any{"file_id": file.ID, "name": file.OriginalName})

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) rollBack(file *store.File) {
	if file == nil {
		return
	}
	if err := s.vault.Discard(file.StoredPath); err != nil {
		s.log.Error("cleaning up an abandoned upload", "path", file.StoredPath, "error", err)
	}
}

func readShortField(part *multipart.Part) (string, error) {
	raw, err := io.ReadAll(io.LimitReader(part, 64))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

// The browser picks this name, so it is text to display and never a path. Control
// characters go too, because a filename ends up in a response header.
func tidyName(raw string) string {
	name := filepath.Base(strings.ReplaceAll(strings.TrimSpace(raw), `\`, "/"))

	if name == "." || name == ".." || name == "/" {
		return ""
	}

	cleaned := strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)

	if len(cleaned) > maxUploadedName {
		cleaned = cleaned[:maxUploadedName]
	}

	return strings.TrimSpace(cleaned)
}

func guessType(name string) string {
	if kind := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); kind != "" {
		return kind
	}
	return "application/octet-stream"
}

// Two spellings of the name: a plain ASCII one that every client understands and an
// escaped UTF-8 one for names with accents or scripts outside ASCII.
func attachmentHeader(name string) string {
	ascii := strings.Map(func(r rune) rune {
		if r > unicode.MaxASCII || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, name)

	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, ascii, url.PathEscape(name))
}
