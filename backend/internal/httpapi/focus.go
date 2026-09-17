package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/focus"
	"github.com/sathvik458/deskbridge/backend/internal/live"
	"github.com/sathvik458/deskbridge/backend/internal/store"
)

type startFocusRequest struct {
	Kind      string  `json:"kind"`
	SessionID *string `json:"session_id"`
}

type focusResponse struct {
	ID         string  `json:"id"`
	Kind       string  `json:"kind"`
	SessionID  *string `json:"session_id"`
	PlannedSec int64   `json:"planned_seconds"`
	LeftSec    int64   `json:"left_seconds"`
	StartedAt  string  `json:"started_at"`
	ServerTime string  `json:"server_time"`
	DoneToday  int     `json:"done_today"`
	IntoRound  int     `json:"into_round"`
	UpNext     string  `json:"up_next"`
}

type focusIdle struct {
	Running   bool   `json:"running"`
	DoneToday int    `json:"done_today"`
	IntoRound int    `json:"into_round"`
	UpNext    string `json:"up_next"`
}

// The same shape the session timer uses: the server says how much is left and when
// it measured that, and the browser only counts down locally from there.
func (s *Server) dressFocus(run store.FocusRun, now time.Time, doneToday int) focusResponse {
	return focusResponse{
		ID:         run.ID,
		Kind:       run.Kind,
		SessionID:  run.SessionID,
		PlannedSec: run.Planned,
		LeftSec:    int64(run.Left(now).Seconds()),
		StartedAt:  run.StartedAt.Format(time.RFC3339),
		ServerTime: now.Format(time.RFC3339),
		DoneToday:  doneToday,
		IntoRound:  focus.IntoRound(doneToday),
		UpNext:     focus.Next(run.Kind, doneToday),
	}
}

func (s *Server) handleStartFocus(w http.ResponseWriter, r *http.Request) {
	var req startFocusRequest

	if err := decodeBody(w, r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Kind == "" {
		req.Kind = store.FocusWork
	}
	if !focus.Known(req.Kind) {
		s.writeError(w, http.StatusBadRequest, "kind must be focus, short_break or long_break")
		return
	}

	who, err := s.studying(r)
	if err != nil {
		s.log.Error("finding whose study data this is", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not work out whose desk this is")
		return
	}

	run, err := s.focus.StartFocus(r.Context(), store.FocusRun{
		ID:        newID(),
		UserID:    who,
		SessionID: req.SessionID,
		Kind:      req.Kind,
		Planned:   int64(focus.Planned(req.Kind).Seconds()),
	})
	if errors.Is(err, store.ErrFocusRunning) {
		s.writeError(w, http.StatusConflict, "a focus timer is already running")
		return
	}
	if err != nil {
		s.log.Error("starting a focus run", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not start the timer")
		return
	}

	s.feed.Announce(live.FocusChanged, map[string]any{"kind": run.Kind, "state": "started"})

	done, _ := s.countFocusToday(r, who)

	s.writeJSON(w, http.StatusCreated, s.dressFocus(run, s.now(), done))
}

func (s *Server) handleCurrentFocus(w http.ResponseWriter, r *http.Request) {
	who, err := s.studying(r)
	if err != nil {
		s.log.Error("finding whose study data this is", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not work out whose desk this is")
		return
	}

	done, err := s.countFocusToday(r, who)
	if err != nil {
		s.log.Error("counting focus runs", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not read the timer")
		return
	}

	run, err := s.focus.CurrentFocus(r.Context(), who)
	if errors.Is(err, store.ErrNotFound) {
		s.writeJSON(w, http.StatusOK, focusIdle{
			DoneToday: done,
			IntoRound: focus.IntoRound(done),
			UpNext:    focus.Ahead(done),
		})
		return
	}
	if err != nil {
		s.log.Error("reading the current focus run", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not read the timer")
		return
	}

	out := s.dressFocus(run, s.now(), done)

	s.writeJSON(w, http.StatusOK, struct {
		Running bool `json:"running"`
		focusResponse
	}{true, out})
}

func (s *Server) handleFinishFocus(w http.ResponseWriter, r *http.Request) {
	s.closeFocus(w, r, store.FocusFinished)
}

func (s *Server) handleStopFocus(w http.ResponseWriter, r *http.Request) {
	s.closeFocus(w, r, store.FocusStopped)
}

func (s *Server) closeFocus(w http.ResponseWriter, r *http.Request, outcome string) {
	id := r.PathValue("id")

	run, err := s.focus.CloseFocus(r.Context(), id, outcome)
	if errors.Is(err, store.ErrNotFound) {
		s.writeError(w, http.StatusNotFound, "no timer to close")
		return
	}
	if err != nil {
		s.log.Error("closing a focus run", "focus_id", id, "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not stop the timer")
		return
	}

	s.feed.Announce(live.FocusChanged, map[string]any{"kind": run.Kind, "state": outcome})

	who, err := s.studying(r)
	if err != nil {
		s.log.Error("finding whose study data this is", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not work out whose desk this is")
		return
	}

	done, _ := s.countFocusToday(r, who)

	s.writeJSON(w, http.StatusOK, focusIdle{
		DoneToday: done,
		IntoRound: focus.IntoRound(done),
		UpNext:    focus.Next(run.Kind, done),
	})
}

// "Today" is the caller's local day, for the same reason the insights page cares:
// a stretch finished at one in the morning belongs to that morning.
func (s *Server) countFocusToday(r *http.Request, who string) (int, error) {
	zone, err := readZone(r.URL.Query().Get("zone"))
	if err != nil {
		zone = time.UTC
	}

	local := s.now().In(zone)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone)

	return s.focus.FinishedFocusBetween(r.Context(), who, start.UTC(), start.AddDate(0, 0, 1).UTC())
}
