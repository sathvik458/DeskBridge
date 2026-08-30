package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/store"
)

const maxTargetMinutes = 24 * 60

type goalRequest struct {
	Subject       string  `json:"subject"`
	Topic         *string `json:"topic"`
	TargetMinutes int     `json:"target_minutes"`
	GoalDate      string  `json:"goal_date"`
}

func (req goalRequest) validate() error {
	if strings.TrimSpace(req.Subject) == "" {
		return errors.New("subject is required")
	}
	if len(req.Subject) > 120 {
		return errors.New("subject is too long")
	}
	if req.TargetMinutes < 1 || req.TargetMinutes > maxTargetMinutes {
		return fmt.Errorf("target_minutes must be between 1 and %d", maxTargetMinutes)
	}
	if _, err := time.Parse(store.DateLayout, req.GoalDate); err != nil {
		return errors.New("goal_date must look like 2026-08-28")
	}
	return nil
}

type goalResponse struct {
	ID            string  `json:"id"`
	Subject       string  `json:"subject"`
	Topic         *string `json:"topic"`
	TargetMinutes int     `json:"target_minutes"`
	GoalDate      string  `json:"goal_date"`
	Done          bool    `json:"done"`
	CompletedAt   *string `json:"completed_at"`
	SessionID     *string `json:"session_id"`
}

func newGoalResponse(goal store.Goal) goalResponse {
	response := goalResponse{
		ID:            goal.ID,
		Subject:       goal.Subject,
		Topic:         goal.Topic,
		TargetMinutes: goal.TargetMinutes,
		GoalDate:      goal.GoalDate,
		Done:          goal.Done(),
		SessionID:     goal.SessionID,
	}

	if goal.CompletedAt != nil {
		completed := goal.CompletedAt.Format(time.RFC3339)
		response.CompletedAt = &completed
	}

	return response
}

func (s *Server) handleListGoals(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		date = s.now().Format(store.DateLayout)
	}

	if _, err := time.Parse(store.DateLayout, date); err != nil {
		s.writeError(w, http.StatusBadRequest, "date must look like 2026-08-28")
		return
	}

	goals, err := s.goals.GoalsOn(r.Context(), defaultUserID, date)
	if err != nil {
		s.log.Error("listing goals", "date", date, "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not list goals")
		return
	}

	responses := make([]goalResponse, 0, len(goals))
	for _, goal := range goals {
		responses = append(responses, newGoalResponse(goal))
	}

	s.writeJSON(w, http.StatusOK, responses)
}

func (s *Server) handleCreateGoal(w http.ResponseWriter, r *http.Request) {
	var req goalRequest

	if err := decodeBody(w, r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.GoalDate == "" {
		req.GoalDate = s.now().Format(store.DateLayout)
	}

	if err := req.validate(); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	goal, err := s.goals.CreateGoal(r.Context(), store.Goal{
		ID:            newID(),
		UserID:        defaultUserID,
		Subject:       strings.TrimSpace(req.Subject),
		Topic:         trimmedOrNil(req.Topic),
		TargetMinutes: req.TargetMinutes,
		GoalDate:      req.GoalDate,
	})
	if err != nil {
		s.log.Error("creating goal", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not create the goal")
		return
	}

	s.writeJSON(w, http.StatusCreated, newGoalResponse(goal))
}

func (s *Server) handleUpdateGoal(w http.ResponseWriter, r *http.Request) {
	var req goalRequest

	if err := decodeBody(w, r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	id := r.PathValue("id")

	existing, err := s.goals.Goal(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		s.writeError(w, http.StatusNotFound, "goal not found")
		return
	}
	if err != nil {
		s.log.Error("looking up goal", "goal_id", id, "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not look up the goal")
		return
	}

	if req.GoalDate == "" {
		req.GoalDate = existing.GoalDate
	}

	if err := req.validate(); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	existing.Subject = strings.TrimSpace(req.Subject)
	existing.Topic = trimmedOrNil(req.Topic)
	existing.TargetMinutes = req.TargetMinutes
	existing.GoalDate = req.GoalDate

	updated, err := s.goals.UpdateGoal(r.Context(), existing)
	if errors.Is(err, store.ErrNotFound) {
		s.writeError(w, http.StatusNotFound, "goal not found")
		return
	}
	if err != nil {
		s.log.Error("updating goal", "goal_id", id, "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not update the goal")
		return
	}

	s.writeJSON(w, http.StatusOK, newGoalResponse(updated))
}

func (s *Server) handleCompleteGoal(w http.ResponseWriter, r *http.Request) {
	s.setGoalDone(w, r, true)
}

func (s *Server) handleReopenGoal(w http.ResponseWriter, r *http.Request) {
	s.setGoalDone(w, r, false)
}

func (s *Server) setGoalDone(w http.ResponseWriter, r *http.Request, done bool) {
	id := r.PathValue("id")

	goal, err := s.goals.SetGoalDone(r.Context(), id, done)
	if errors.Is(err, store.ErrNotFound) {
		s.writeError(w, http.StatusNotFound, "goal not found")
		return
	}
	if err != nil {
		s.log.Error("marking goal", "goal_id", id, "done", done, "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not update the goal")
		return
	}

	s.writeJSON(w, http.StatusOK, newGoalResponse(goal))
}

func (s *Server) handleDeleteGoal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	err := s.goals.DeleteGoal(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		s.writeError(w, http.StatusNotFound, "goal not found")
		return
	}
	if err != nil {
		s.log.Error("deleting goal", "goal_id", id, "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not delete the goal")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
