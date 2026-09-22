package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/live"
	"github.com/sathvik458/deskbridge/backend/internal/store"
)

const maxMessageBody = 4000

type createMessageRequest struct {
	Kind      string  `json:"kind"`
	Body      string  `json:"body"`
	SessionID *string `json:"session_id"`
}

func (req createMessageRequest) validate(role string) error {
	if req.Kind != store.MessageKindPlain && req.Kind != store.MessageKindHelp {
		return errors.New("kind must be message or help_request")
	}

	body := strings.TrimSpace(req.Body)
	if body == "" {
		return errors.New("body is required")
	}
	if len(body) > maxMessageBody {
		return errors.New("body is too long")
	}

	// The rule is about who is asking for help, so it hangs off the signed-in
	// account rather than a field the caller could set to anything.
	if req.Kind == store.MessageKindHelp && role != "student" {
		return errors.New("only the student can raise a help request")
	}

	return nil
}

type messageResponse struct {
	ID        string  `json:"id"`
	From      string  `json:"from"`
	SenderID  string  `json:"sender_id"`
	Kind      string  `json:"kind"`
	Body      string  `json:"body"`
	SessionID *string `json:"session_id"`
	Read      bool    `json:"read"`
	CreatedAt string  `json:"created_at"`
}

func newMessageResponse(message store.Message, role string) messageResponse {
	return messageResponse{
		ID:        message.ID,
		From:      role,
		SenderID:  message.SenderID,
		Kind:      message.Kind,
		Body:      message.Body,
		SessionID: message.SessionID,
		Read:      message.IsRead(),
		CreatedAt: message.CreatedAt.Format(time.RFC3339),
	}
}

func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	messages, err := s.messages.Messages(r.Context(), limit)
	if err != nil {
		s.log.Error("listing messages", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not list messages")
		return
	}

	s.writeJSON(w, http.StatusOK, s.dress(messages, userOf(r)))
}

func (s *Server) handleUnreadMessages(w http.ResponseWriter, r *http.Request) {
	messages, err := s.messages.UnreadNotFrom(r.Context(), userOf(r).ID)
	if err != nil {
		s.log.Error("listing unread messages", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not list unread messages")
		return
	}

	s.writeJSON(w, http.StatusOK, s.dress(messages, userOf(r)))
}

func (s *Server) handleCreateMessage(w http.ResponseWriter, r *http.Request) {
	var req createMessageRequest

	if err := decodeBody(w, r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	me := userOf(r)

	if req.Kind == "" {
		req.Kind = store.MessageKindPlain
	}

	if err := req.validate(me.Role); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	message, err := s.messages.CreateMessage(r.Context(), store.Message{
		ID:        newID(),
		SenderID:  me.ID,
		SessionID: req.SessionID,
		Kind:      req.Kind,
		Body:      strings.TrimSpace(req.Body),
	})
	if err != nil {
		s.log.Error("creating message", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not send the message")
		return
	}

	if message.Kind == store.MessageKindHelp {
		s.log.Info("help requested", "message_id", message.ID, "session_id", message.SessionID)
		s.tell(r, live.HelpRaised, map[string]any{"message_id": message.ID, "body": message.Body})
	} else {
		s.tell(r, live.MessageSent, map[string]any{"message_id": message.ID, "from": me.Role, "body": message.Body})
	}

	s.writeJSON(w, http.StatusCreated, newMessageResponse(message, me.Role))
}

func (s *Server) handleMarkMessageRead(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	message, err := s.messages.MarkMessageRead(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		s.writeError(w, http.StatusNotFound, "message not found")
		return
	}
	if err != nil {
		s.log.Error("marking message read", "message_id", id, "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not mark the message read")
		return
	}

	s.writeJSON(w, http.StatusOK, newMessageResponse(message, roleOfSender(message, userOf(r))))
}

func (s *Server) handleMarkAllRead(w http.ResponseWriter, r *http.Request) {
	marked, err := s.messages.MarkAllReadNotFrom(r.Context(), userOf(r).ID)
	if err != nil {
		s.log.Error("marking messages read", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not mark messages read")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]int{"marked": marked})
}

// Only two people ever talk here, so a message is either mine or theirs and the
// viewer's own role is enough to label it without another query per row.
func roleOfSender(m store.Message, viewer store.User) string {
	if m.SenderID == viewer.ID {
		return viewer.Role
	}
	if viewer.Role == "student" {
		return "supporter"
	}
	return "student"
}

func (s *Server) dress(messages []store.Message, viewer store.User) []messageResponse {
	out := make([]messageResponse, 0, len(messages))
	for _, m := range messages {
		out = append(out, newMessageResponse(m, roleOfSender(m, viewer)))
	}
	return out
}
