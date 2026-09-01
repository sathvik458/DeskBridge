package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/store"
)

type fakeMessageStore struct {
	messages []store.Message
	err      error
	marked   int
}

func (f *fakeMessageStore) CreateMessage(_ context.Context, message store.Message) (store.Message, error) {
	if f.err != nil {
		return store.Message{}, f.err
	}
	message.CreatedAt = fixedNow
	f.messages = append(f.messages, message)
	return message, nil
}

func (f *fakeMessageStore) Messages(_ context.Context, limit int) ([]store.Message, error) {
	if f.err != nil {
		return nil, f.err
	}

	newest := []store.Message{}
	for i := len(f.messages) - 1; i >= 0; i-- {
		if limit > 0 && len(newest) == limit {
			break
		}
		newest = append(newest, f.messages[i])
	}

	return newest, nil
}

func (f *fakeMessageStore) UnreadFrom(_ context.Context, senderID string) ([]store.Message, error) {
	if f.err != nil {
		return nil, f.err
	}

	unread := []store.Message{}
	for _, message := range f.messages {
		if message.SenderID == senderID && message.ReadAt == nil {
			unread = append(unread, message)
		}
	}

	return unread, nil
}

func (f *fakeMessageStore) MarkMessageRead(_ context.Context, id string) (store.Message, error) {
	if f.err != nil {
		return store.Message{}, f.err
	}

	for i, message := range f.messages {
		if message.ID == id {
			at := fixedNow
			f.messages[i].ReadAt = &at
			return f.messages[i], nil
		}
	}

	return store.Message{}, store.ErrNotFound
}

func (f *fakeMessageStore) MarkAllReadFrom(_ context.Context, senderID string) (int, error) {
	if f.err != nil {
		return 0, f.err
	}

	for i, message := range f.messages {
		if message.SenderID == senderID && message.ReadAt == nil {
			at := fixedNow
			f.messages[i].ReadAt = &at
			f.marked++
		}
	}

	return f.marked, nil
}

func newMessageServer(t *testing.T, messages MessageStore) *Server {
	t.Helper()

	log := discardTestLogger()
	s := NewServer(log, "test", time.Now(), newFakeDeviceStore(), &fakeSessionStore{},
		newFakeGoalStore(), messages, "http://localhost:5173")
	s.now = func() time.Time { return fixedNow }

	return s
}

func sendMessage(t *testing.T, s *Server, body string) messageResponse {
	t.Helper()

	rec := send(t, s, http.MethodPost, "/api/messages", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("sending message: status %d, body %s", rec.Code, rec.Body)
	}

	var message messageResponse
	if err := json.NewDecoder(rec.Body).Decode(&message); err != nil {
		t.Fatalf("decoding message: %v", err)
	}

	return message
}

func TestSendMessage(t *testing.T) {
	s := newMessageServer(t, &fakeMessageStore{})

	message := sendMessage(t, s, `{"from":"supporter","kind":"message","body":"how is revision going?"}`)

	if message.From != "supporter" || message.Body != "how is revision going?" {
		t.Errorf("message = %+v, want the values sent", message)
	}
	if message.Read {
		t.Error("a new message should be unread")
	}
}

func TestSendHelpRequest(t *testing.T) {
	s := newMessageServer(t, &fakeMessageStore{})

	message := sendMessage(t, s, `{"from":"student","kind":"help_request","body":"stuck on question 4"}`)

	if message.Kind != store.MessageKindHelp {
		t.Errorf("kind = %q, want %q", message.Kind, store.MessageKindHelp)
	}
}

func TestOnlyTheStudentCanRaiseAHelpRequest(t *testing.T) {
	s := newMessageServer(t, &fakeMessageStore{})

	rec := send(t, s, http.MethodPost, "/api/messages",
		`{"from":"supporter","kind":"help_request","body":"help me"}`)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d. body: %s", rec.Code, http.StatusBadRequest, rec.Body)
	}
}

func TestSendMessageRejectsBadInput(t *testing.T) {
	tests := []struct{ name, body string }{
		{"missing from", `{"kind":"message","body":"hello"}`},
		{"unknown from", `{"from":"teacher","body":"hello"}`},
		{"missing body", `{"from":"student"}`},
		{"blank body", `{"from":"student","body":"   "}`},
		{"unknown kind", `{"from":"student","kind":"shout","body":"hello"}`},
		{"unknown field", `{"from":"student","body":"hello","urgent":true}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newMessageServer(t, &fakeMessageStore{})

			if rec := send(t, s, http.MethodPost, "/api/messages", tc.body); rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d. body: %s", rec.Code, http.StatusBadRequest, rec.Body)
			}
		})
	}
}

func TestKindDefaultsToPlainMessage(t *testing.T) {
	s := newMessageServer(t, &fakeMessageStore{})

	message := sendMessage(t, s, `{"from":"student","body":"just checking in"}`)

	if message.Kind != store.MessageKindPlain {
		t.Errorf("kind = %q, want %q", message.Kind, store.MessageKindPlain)
	}
}

func TestListMessagesIsNewestFirst(t *testing.T) {
	s := newMessageServer(t, &fakeMessageStore{})

	sendMessage(t, s, `{"from":"supporter","body":"first"}`)
	sendMessage(t, s, `{"from":"student","body":"second"}`)

	rec := send(t, s, http.MethodGet, "/api/messages", "")

	var messages []messageResponse
	if err := json.NewDecoder(rec.Body).Decode(&messages); err != nil {
		t.Fatalf("decoding messages: %v", err)
	}

	if len(messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(messages))
	}
	if messages[0].Body != "second" {
		t.Errorf("first message = %q, want the most recent one", messages[0].Body)
	}
}

func TestListMessagesReturnsEmptyArray(t *testing.T) {
	s := newMessageServer(t, &fakeMessageStore{})

	rec := send(t, s, http.MethodGet, "/api/messages", "")

	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("body = %s, want []", got)
	}
}

func TestUnreadOnlyIncludesTheStudent(t *testing.T) {
	s := newMessageServer(t, &fakeMessageStore{})

	sendMessage(t, s, `{"from":"supporter","body":"from me"}`)
	sendMessage(t, s, `{"from":"student","body":"from them"}`)

	rec := send(t, s, http.MethodGet, "/api/messages/unread", "")

	var messages []messageResponse
	json.NewDecoder(rec.Body).Decode(&messages)

	if len(messages) != 1 {
		t.Fatalf("got %d unread, want only the student's", len(messages))
	}
	if messages[0].From != "student" {
		t.Errorf("unread from %q, want student", messages[0].From)
	}
}

func TestMarkMessageRead(t *testing.T) {
	s := newMessageServer(t, &fakeMessageStore{})
	message := sendMessage(t, s, `{"from":"student","body":"hello"}`)

	rec := send(t, s, http.MethodPost, "/api/messages/"+message.ID+"/read", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var read messageResponse
	json.NewDecoder(rec.Body).Decode(&read)

	if !read.Read {
		t.Error("message is still unread after marking it")
	}
}

func TestMarkUnknownMessageReadIs404(t *testing.T) {
	s := newMessageServer(t, &fakeMessageStore{})

	if rec := send(t, s, http.MethodPost, "/api/messages/ghost/read", ""); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestMarkAllRead(t *testing.T) {
	s := newMessageServer(t, &fakeMessageStore{})

	sendMessage(t, s, `{"from":"student","body":"one"}`)
	sendMessage(t, s, `{"from":"student","body":"two"}`)
	sendMessage(t, s, `{"from":"supporter","body":"mine"}`)

	rec := send(t, s, http.MethodPost, "/api/messages/read", "")

	var body map[string]int
	json.NewDecoder(rec.Body).Decode(&body)

	if body["marked"] != 2 {
		t.Errorf("marked = %d, want 2 (only the student's)", body["marked"])
	}
}

func TestMessageStoreFailureBecomes500(t *testing.T) {
	s := newMessageServer(t, &fakeMessageStore{err: errors.New("database is on fire")})

	rec := send(t, s, http.MethodGet, "/api/messages", "")

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if strings.Contains(rec.Body.String(), "on fire") {
		t.Errorf("internal error leaked: %s", rec.Body)
	}
}
