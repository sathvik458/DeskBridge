package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/live"
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

func (f *fakeMessageStore) UnreadNotFrom(_ context.Context, viewerID string) ([]store.Message, error) {
	if f.err != nil {
		return nil, f.err
	}

	unread := []store.Message{}
	for _, message := range f.messages {
		if message.SenderID != viewerID && message.ReadAt == nil {
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

func (f *fakeMessageStore) MarkAllReadNotFrom(_ context.Context, viewerID string) (int, error) {
	if f.err != nil {
		return 0, f.err
	}

	for i, message := range f.messages {
		if message.SenderID != viewerID && message.ReadAt == nil {
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
		newFakeGoalStore(), messages, newFakeFileStore(), newFakeBoardStore(), newFakeUserStore(), newFakeLoginStore(), newTestVault(t),
		live.NewFeed(log), "http://localhost:5173", false)
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

// sendAs posts a message as somebody other than the default student.
func sendAs(t *testing.T, s *Server, token, body string) {
	t.Helper()

	req := signedInAs(httptest.NewRequest(http.MethodPost, "/api/messages", strings.NewReader(body)), token)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("sending as %s: status %d, body %s", token, rec.Code, rec.Body)
	}
}

func TestSendMessage(t *testing.T) {
	s := newMessageServer(t, &fakeMessageStore{})

	message := sendMessage(t, s, `{"kind":"message","body":"how is revision going?"}`)

	if message.From != "student" || message.Body != "how is revision going?" {
		t.Errorf("message = %+v, want the values sent", message)
	}
	if message.Read {
		t.Error("a new message should be unread")
	}
}

func TestSendHelpRequest(t *testing.T) {
	s := newMessageServer(t, &fakeMessageStore{})

	message := sendMessage(t, s, `{"kind":"help_request","body":"stuck on question 4"}`)

	if message.Kind != store.MessageKindHelp {
		t.Errorf("kind = %q, want %q", message.Kind, store.MessageKindHelp)
	}
}

// The rule reads off the signed-in account now, so the supporter is refused even
// though the request body looks identical to one the student may send.
func TestOnlyTheStudentCanRaiseAHelpRequest(t *testing.T) {
	s := newMessageServer(t, &fakeMessageStore{})

	req := signedInAs(httptest.NewRequest(http.MethodPost, "/api/messages",
		strings.NewReader(`{"kind":"help_request","body":"help me"}`)), helperToken)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("supporter status = %d, want %d. body: %s", rec.Code, http.StatusBadRequest, rec.Body)
	}

	// ...and the student, signed in, may.
	ok := send(t, s, http.MethodPost, "/api/messages", `{"kind":"help_request","body":"help me"}`)
	if ok.Code != http.StatusCreated {
		t.Errorf("student status = %d, want %d. body: %s", ok.Code, http.StatusCreated, ok.Body)
	}
}

func TestSendMessageRejectsBadInput(t *testing.T) {
	tests := []struct{ name, body string }{
		{"missing body", `{"kind":"message"}`},
		{"blank body", `{"body":"   "}`},
		{"unknown kind", `{"kind":"shout","body":"hello"}`},
		{"unknown field", `{"body":"hello","urgent":true}`},
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

	message := sendMessage(t, s, `{"body":"just checking in"}`)

	if message.Kind != store.MessageKindPlain {
		t.Errorf("kind = %q, want %q", message.Kind, store.MessageKindPlain)
	}
}

func TestListMessagesIsNewestFirst(t *testing.T) {
	s := newMessageServer(t, &fakeMessageStore{})

	sendMessage(t, s, `{"body":"first"}`)
	sendMessage(t, s, `{"body":"second"}`)

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

// "Unread" is relative to whoever is asking: my own messages are never unread to
// me, so the same endpoint serves both sides of the conversation.
func TestUnreadIsWhatTheOtherPersonSent(t *testing.T) {
	s := newMessageServer(t, &fakeMessageStore{})

	sendMessage(t, s, `{"body":"mine, sent as the student"}`)
	sendAs(t, s, helperToken, `{"body":"theirs, sent as the supporter"}`)

	rec := send(t, s, http.MethodGet, "/api/messages/unread", "")

	var messages []messageResponse
	if err := json.NewDecoder(rec.Body).Decode(&messages); err != nil {
		t.Fatalf("decoding: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("got %d unread, want only what the supporter sent", len(messages))
	}
	if messages[0].SenderID != helperID {
		t.Errorf("unread from %q, want the supporter", messages[0].SenderID)
	}
}

// The mirror image: signed in as the supporter, the student's message is the
// unread one. Same handler, opposite answer.
func TestUnreadFlipsWithWhoIsAsking(t *testing.T) {
	s := newMessageServer(t, &fakeMessageStore{})

	sendMessage(t, s, `{"body":"from the student"}`)
	sendAs(t, s, helperToken, `{"body":"from the supporter"}`)

	req := signedInAs(httptest.NewRequest(http.MethodGet, "/api/messages/unread", nil), helperToken)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	var messages []messageResponse
	if err := json.NewDecoder(rec.Body).Decode(&messages); err != nil {
		t.Fatalf("decoding: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("got %d unread, want only what the student sent", len(messages))
	}
	if messages[0].SenderID != studentID {
		t.Errorf("unread from %q, want the student", messages[0].SenderID)
	}
}

func TestMarkMessageRead(t *testing.T) {
	s := newMessageServer(t, &fakeMessageStore{})
	message := sendMessage(t, s, `{"body":"hello"}`)

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

	sendAs(t, s, helperToken, `{"body":"one from them"}`)
	sendAs(t, s, helperToken, `{"body":"two from them"}`)
	sendMessage(t, s, `{"body":"mine, already read by definition"}`)

	rec := send(t, s, http.MethodPost, "/api/messages/read", "")

	var body map[string]int
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding: %v", err)
	}

	if body["marked"] != 2 {
		t.Errorf("marked = %d, want 2 (my own message is not mine to read)", body["marked"])
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
