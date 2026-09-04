package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/live"
	"github.com/sathvik458/deskbridge/backend/internal/store"
)

const (
	maxPointsPerStroke = 2000
	maxMarksPerPage    = 1000
	thinnestNib        = 1.0
	thickestNib        = 24.0
)

// A fixed palette rather than free hex, so a colour can never be a vector for
// something odd and both people see the same four pens.
var pens = map[string]bool{
	"#1E1E1E": true,
	"#C84B31": true,
	"#3B6E5B": true,
	"#1F4257": true,
}

type strokeRequest struct {
	Ink       string    `json:"ink"`
	Thickness float64   `json:"thickness"`
	Path      []float64 `json:"path"`
}

func (req strokeRequest) validate() error {
	if !pens[req.Ink] {
		return errors.New("ink must be one of the four board colours")
	}

	if req.Thickness < thinnestNib || req.Thickness > thickestNib {
		return errors.New("thickness is outside the usable range")
	}

	switch {
	case len(req.Path) == 0:
		return errors.New("a stroke needs at least one point")
	case len(req.Path)%2 != 0:
		return errors.New("path must hold pairs of coordinates")
	case len(req.Path) > maxPointsPerStroke*2:
		return errors.New("that stroke has too many points")
	}

	// Coordinates are fractions of the board, not pixels, so anything outside the
	// unit square is a client bug rather than a very long line.
	for _, coordinate := range req.Path {
		if coordinate < 0 || coordinate > 1 {
			return errors.New("coordinates must sit between 0 and 1")
		}
	}

	return nil
}

type markResponse struct {
	Seq       int64     `json:"seq"`
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	By        string    `json:"by"`
	TargetID  *string   `json:"target_id,omitempty"`
	Ink       *string   `json:"ink,omitempty"`
	Thickness *float64  `json:"thickness,omitempty"`
	Path      []float64 `json:"path,omitempty"`
	CreatedAt string    `json:"created_at"`
}

func newMarkResponse(mark store.Mark) markResponse {
	by := "student"
	if mark.AuthorID == supporterUserID {
		by = "supporter"
	}

	return markResponse{
		Seq:       mark.Seq,
		ID:        mark.ID,
		Kind:      mark.Kind,
		By:        by,
		TargetID:  mark.TargetID,
		Ink:       mark.Ink,
		Thickness: mark.Thickness,
		Path:      mark.Path,
		CreatedAt: mark.CreatedAt.Format(time.RFC3339),
	}
}

type boardPage struct {
	Marks  []markResponse `json:"marks"`
	Cursor int64          `json:"cursor"`
	More   bool           `json:"more"`
}

// The client sends the highest sequence it has, and gets back only what came after.
// Asking from zero rebuilds the whole board, so first load and reconnect are one path.
func (s *Server) handleBoardMarks(w http.ResponseWriter, r *http.Request) {
	since, err := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	if err != nil {
		since = 0
	}
	if since < 0 {
		s.writeError(w, http.StatusBadRequest, "since cannot be negative")
		return
	}

	marks, err := s.board.MarksSince(r.Context(), since, maxMarksPerPage)
	if err != nil {
		s.log.Error("listing board marks", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not read the board")
		return
	}

	page := boardPage{
		Marks:  make([]markResponse, 0, len(marks)),
		Cursor: since,
		More:   len(marks) == maxMarksPerPage,
	}

	for _, mark := range marks {
		page.Marks = append(page.Marks, newMarkResponse(mark))
		page.Cursor = mark.Seq
	}

	s.writeJSON(w, http.StatusOK, page)
}

func (s *Server) handleDrawStroke(w http.ResponseWriter, r *http.Request) {
	var req strokeRequest

	if err := decodeBody(w, r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := req.validate(); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	mark, err := s.board.AddMark(r.Context(), store.Mark{
		ID:        newID(),
		AuthorID:  supporterUserID,
		Kind:      store.MarkDraw,
		Ink:       &req.Ink,
		Thickness: &req.Thickness,
		Path:      req.Path,
	})
	if err != nil {
		s.log.Error("adding a stroke", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not add that stroke")
		return
	}

	s.announceMark(mark)

	s.writeJSON(w, http.StatusCreated, newMarkResponse(mark))
}

// Nothing is removed. Rubbing out a stroke appends a mark that points back at it,
// because a client replaying from an older cursor has to be told, not shown a gap.
func (s *Server) handleEraseStroke(w http.ResponseWriter, r *http.Request) {
	target := r.PathValue("id")

	drawn, err := s.board.MarkExists(r.Context(), target)
	if err != nil {
		s.log.Error("looking up a stroke", "mark_id", target, "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not read the board")
		return
	}
	if !drawn {
		s.writeError(w, http.StatusNotFound, "no such stroke")
		return
	}

	mark, err := s.board.AddMark(r.Context(), store.Mark{
		ID:       newID(),
		AuthorID: supporterUserID,
		Kind:     store.MarkErase,
		TargetID: &target,
	})
	if err != nil {
		s.log.Error("erasing a stroke", "mark_id", target, "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not rub that out")
		return
	}

	s.announceMark(mark)

	s.writeJSON(w, http.StatusCreated, newMarkResponse(mark))
}

func (s *Server) handleClearBoard(w http.ResponseWriter, r *http.Request) {
	mark, err := s.board.AddMark(r.Context(), store.Mark{
		ID:       newID(),
		AuthorID: supporterUserID,
		Kind:     store.MarkClear,
	})
	if err != nil {
		s.log.Error("clearing the board", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not clear the board")
		return
	}

	// Everything before a clear is unreachable, so this is the one moment the log
	// can shrink without any client losing something it still needs.
	if removed, err := s.board.TrimBoard(r.Context()); err != nil {
		s.log.Error("trimming the board", "error", err)
	} else if removed > 0 {
		s.log.Info("board trimmed", "removed", removed)
	}

	s.announceMark(mark)

	s.writeJSON(w, http.StatusCreated, newMarkResponse(mark))
}

func (s *Server) announceMark(mark store.Mark) {
	s.feed.Announce(live.BoardMarked, map[string]any{"seq": mark.Seq, "kind": mark.Kind})
}
