package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/store"
)

const defaultUserID = "default-student"

type startSessionRequest struct {
	Subject string  `json:"subject"`
	Topic   *string `json:"topic"`
}

func (req startSessionRequest) validate() error {
	if strings.TrimSpace(req.Subject) == "" {
		return errors.New("subject is required")
	}
	if len(req.Subject) > 120 {
		return errors.New("subject is too long")
	}
	return nil
}

type sessionResponse struct {
	ID             string  `json:"id"`
	Subject        string  `json:"subject"`
	Topic          *string `json:"topic"`
	Status         string  `json:"status"`
	StartedAt      string  `json:"started_at"`
	EndedAt        *string `json:"ended_at"`
	ElapsedSeconds int64   `json:"elapsed_seconds"`
	ServerTime     string  `json:"server_time"`
}

// ElapsedSeconds and ServerTime ship together so a browser can tick forward from a
// known instant without ever deciding the elapsed time for itself.
func newSessionResponse(session store.Session, now time.Time) sessionResponse {
	response := sessionResponse{
		ID:             session.ID,
		Subject:        session.Subject,
		Topic:          session.Topic,
		Status:         session.Status,
		StartedAt:      session.StartedAt.Format(time.RFC3339),
		ElapsedSeconds: int64(session.Elapsed(now).Seconds()),
		ServerTime:     now.UTC().Format(time.RFC3339),
	}

	if session.EndedAt != nil {
		ended := session.EndedAt.Format(time.RFC3339)
		response.EndedAt = &ended
	}

	return response
}

func (s *Server) handleStartSession(w http.ResponseWriter, r *http.Request) {
	var req startSessionRequest

	if err := decodeBody(w, r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := req.validate(); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	session, err := s.sessions.StartSession(r.Context(), store.Session{
		ID:      newID(),
		UserID:  defaultUserID,
		Subject: strings.TrimSpace(req.Subject),
		Topic:   trimmedOrNil(req.Topic),
	})
	if errors.Is(err, store.ErrSessionRunning) {
		s.writeError(w, http.StatusConflict, "a session is already running")
		return
	}
	if err != nil {
		s.log.Error("starting session", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not start the session")
		return
	}

	s.writeJSON(w, http.StatusCreated, newSessionResponse(session, s.now()))
}

func (s *Server) handleCurrentSession(w http.ResponseWriter, r *http.Request) {
	session, err := s.sessions.CurrentSession(r.Context(), defaultUserID)
	if errors.Is(err, store.ErrNotFound) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		s.log.Error("looking up the current session", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not look up the session")
		return
	}

	s.writeJSON(w, http.StatusOK, newSessionResponse(session, s.now()))
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	sessions, err := s.sessions.Sessions(r.Context(), defaultUserID, limit)
	if err != nil {
		s.log.Error("listing sessions", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not list sessions")
		return
	}

	now := s.now()
	responses := make([]sessionResponse, 0, len(sessions))
	for _, session := range sessions {
		responses = append(responses, newSessionResponse(session, now))
	}

	s.writeJSON(w, http.StatusOK, responses)
}

func (s *Server) handlePauseSession(w http.ResponseWriter, r *http.Request) {
	s.applyTransition(w, r, s.sessions.PauseSession)
}

func (s *Server) handleResumeSession(w http.ResponseWriter, r *http.Request) {
	s.applyTransition(w, r, s.sessions.ResumeSession)
}

func (s *Server) handleEndSession(w http.ResponseWriter, r *http.Request) {
	s.applyTransition(w, r, s.sessions.EndSession)
}

func (s *Server) applyTransition(w http.ResponseWriter, r *http.Request, move sessionMove) {
	id := r.PathValue("id")

	session, err := move(r.Context(), id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		s.writeError(w, http.StatusNotFound, "session not found")
	case errors.Is(err, store.ErrBadTransition):
		s.writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		s.log.Error("changing session state", "session_id", id, "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not change the session")
	default:
		s.writeJSON(w, http.StatusOK, newSessionResponse(session, s.now()))
	}
}

func trimmedOrNil(value *string) *string {
	if value == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}
