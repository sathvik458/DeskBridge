package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/store"
)

const (
	supporterUserID = "default-supporter"
	maxMessageBody  = 4000
)

// Until authentication exists, the caller says who it is. Both ids are seeded by
// migrations, and this is the one place that mapping lives.
var sendersByRole = map[string]string{
	"supporter": supporterUserID,
	"student":   defaultUserID,
}

type createMessageRequest struct {
	From      string  `json:"from"`
	Kind      string  `json:"kind"`
	Body      string  `json:"body"`
	SessionID *string `json:"session_id"`
}

func (req createMessageRequest) validate() error {
	if _, ok := sendersByRole[req.From]; !ok {
		return errors.New("from must be supporter or student")
	}

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

	if req.Kind == store.MessageKindHelp && req.From != "student" {
		return errors.New("only the student can raise a help request")
	}

	return nil
}

type messageResponse struct {
	ID        string  `json:"id"`
	From      string  `json:"from"`
	Kind      string  `json:"kind"`
	Body      string  `json:"body"`
	SessionID *string `json:"session_id"`
	Read      bool    `json:"read"`
	CreatedAt string  `json:"created_at"`
}

func newMessageResponse(message store.Message) messageResponse {
	from := "student"
	if message.SenderID == supporterUserID {
		from = "supporter"
	}

	return messageResponse{
		ID:        message.ID,
		From:      from,
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

	s.writeJSON(w, http.StatusOK, toMessageResponses(messages))
}

func (s *Server) handleUnreadMessages(w http.ResponseWriter, r *http.Request) {
	messages, err := s.messages.UnreadFrom(r.Context(), defaultUserID)
	if err != nil {
		s.log.Error("listing unread messages", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not list unread messages")
		return
	}

	s.writeJSON(w, http.StatusOK, toMessageResponses(messages))
}

func (s *Server) handleCreateMessage(w http.ResponseWriter, r *http.Request) {
	var req createMessageRequest

	if err := decodeBody(w, r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Kind == "" {
		req.Kind = store.MessageKindPlain
	}

	if err := req.validate(); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	message, err := s.messages.CreateMessage(r.Context(), store.Message{
		ID:        newID(),
		SenderID:  sendersByRole[req.From],
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
	}

	s.writeJSON(w, http.StatusCreated, newMessageResponse(message))
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

	s.writeJSON(w, http.StatusOK, newMessageResponse(message))
}

func (s *Server) handleMarkAllRead(w http.ResponseWriter, r *http.Request) {
	marked, err := s.messages.MarkAllReadFrom(r.Context(), defaultUserID)
	if err != nil {
		s.log.Error("marking messages read", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not mark messages read")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]int{"marked": marked})
}

func toMessageResponses(messages []store.Message) []messageResponse {
	responses := make([]messageResponse, 0, len(messages))
	for _, message := range messages {
		responses = append(responses, newMessageResponse(message))
	}
	return responses
}
