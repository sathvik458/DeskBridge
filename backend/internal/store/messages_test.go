package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

const supporterID = "default-supporter"

func sendTestMessage(t *testing.T, s *Store, id, sender, kind, body string) Message {
	t.Helper()

	message, err := s.CreateMessage(context.Background(), Message{
		ID: id, SenderID: sender, Kind: kind, Body: body,
	})
	if err != nil {
		t.Fatalf("CreateMessage(%s) returned an unexpected error: %v", id, err)
	}

	return message
}

func TestCreateAndReadMessage(t *testing.T) {
	s := newTestStore(t)

	created := sendTestMessage(t, s, "m1", testUserID, MessageKindHelp, "stuck on question 4")

	if created.IsRead() {
		t.Error("a new message should be unread")
	}
	if !created.CreatedAt.Equal(testClock) {
		t.Errorf("CreatedAt = %s, want %s", created.CreatedAt, testClock)
	}

	found, err := s.Message(context.Background(), "m1")
	if err != nil {
		t.Fatalf("Message() returned: %v", err)
	}
	if found.Kind != MessageKindHelp {
		t.Errorf("Kind = %q, want %q", found.Kind, MessageKindHelp)
	}
}

func TestMessagesAreNewestFirst(t *testing.T) {
	s := newTestStore(t)

	sendTestMessage(t, s, "m1", supporterID, MessageKindPlain, "first")
	s.setClock(testClock.Add(time.Minute))
	sendTestMessage(t, s, "m2", testUserID, MessageKindPlain, "second")

	messages, err := s.Messages(context.Background(), 0)
	if err != nil {
		t.Fatalf("Messages() returned: %v", err)
	}

	if len(messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(messages))
	}
	if messages[0].Body != "second" {
		t.Errorf("first = %q, want the most recent message", messages[0].Body)
	}
}

func TestMessagesRespectsTheLimit(t *testing.T) {
	s := newTestStore(t)

	for i, body := range []string{"one", "two", "three"} {
		s.setClock(testClock.Add(time.Duration(i) * time.Minute))
		sendTestMessage(t, s, body, testUserID, MessageKindPlain, body)
	}

	messages, err := s.Messages(context.Background(), 2)
	if err != nil {
		t.Fatalf("Messages() returned: %v", err)
	}

	if len(messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(messages))
	}
	if messages[0].Body != "three" {
		t.Errorf("a limit should keep the newest, got %q first", messages[0].Body)
	}
}

func TestMessagesReturnsEmptySliceNotNil(t *testing.T) {
	s := newTestStore(t)

	messages, err := s.Messages(context.Background(), 0)
	if err != nil {
		t.Fatalf("Messages() returned: %v", err)
	}
	if messages == nil {
		t.Error("Messages() returned nil, want an empty slice")
	}
}

func TestUnreadIsScopedToOneSender(t *testing.T) {
	s := newTestStore(t)

	sendTestMessage(t, s, "m1", supporterID, MessageKindPlain, "from the supporter")
	sendTestMessage(t, s, "m2", testUserID, MessageKindPlain, "from the student")
	sendTestMessage(t, s, "m3", testUserID, MessageKindHelp, "help please")

	unread, err := s.UnreadFrom(context.Background(), testUserID)
	if err != nil {
		t.Fatalf("UnreadFrom() returned: %v", err)
	}

	if len(unread) != 2 {
		t.Fatalf("got %d unread from the student, want 2", len(unread))
	}
}

func TestMarkMessageRead(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	sendTestMessage(t, s, "m1", testUserID, MessageKindPlain, "hello")

	later := testClock.Add(5 * time.Minute)
	s.setClock(later)

	read, err := s.MarkMessageRead(ctx, "m1")
	if err != nil {
		t.Fatalf("MarkMessageRead() returned: %v", err)
	}

	if !read.IsRead() {
		t.Fatal("message is still unread")
	}
	if !read.ReadAt.Equal(later) {
		t.Errorf("ReadAt = %s, want %s", read.ReadAt, later)
	}
}

// Marking a message read twice must not move the timestamp - it records when it was
// first seen, not the last time something touched it.
func TestMarkingReadTwiceKeepsTheFirstTimestamp(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	sendTestMessage(t, s, "m1", testUserID, MessageKindPlain, "hello")

	first := testClock.Add(5 * time.Minute)
	s.setClock(first)
	if _, err := s.MarkMessageRead(ctx, "m1"); err != nil {
		t.Fatalf("first MarkMessageRead() returned: %v", err)
	}

	s.setClock(testClock.Add(time.Hour))
	again, err := s.MarkMessageRead(ctx, "m1")
	if err != nil {
		t.Fatalf("second MarkMessageRead() returned: %v", err)
	}

	if !again.ReadAt.Equal(first) {
		t.Errorf("ReadAt = %s, want it unchanged at %s", again.ReadAt, first)
	}
}

func TestMarkUnknownMessageRead(t *testing.T) {
	s := newTestStore(t)

	_, err := s.MarkMessageRead(context.Background(), "ghost")

	if !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestMarkAllReadOnlyTouchesOneSender(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	sendTestMessage(t, s, "m1", testUserID, MessageKindPlain, "one")
	sendTestMessage(t, s, "m2", testUserID, MessageKindHelp, "two")
	sendTestMessage(t, s, "m3", supporterID, MessageKindPlain, "mine")

	marked, err := s.MarkAllReadFrom(ctx, testUserID)
	if err != nil {
		t.Fatalf("MarkAllReadFrom() returned: %v", err)
	}

	if marked != 2 {
		t.Errorf("marked = %d, want 2", marked)
	}

	mine, err := s.Message(ctx, "m3")
	if err != nil {
		t.Fatalf("Message() returned: %v", err)
	}
	if mine.IsRead() {
		t.Error("the supporter's own message was marked read")
	}
}

func TestMarkAllReadIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	sendTestMessage(t, s, "m1", testUserID, MessageKindPlain, "one")

	if _, err := s.MarkAllReadFrom(ctx, testUserID); err != nil {
		t.Fatalf("first sweep returned: %v", err)
	}

	marked, err := s.MarkAllReadFrom(ctx, testUserID)
	if err != nil {
		t.Fatalf("second sweep returned: %v", err)
	}

	if marked != 0 {
		t.Errorf("second sweep marked %d, want 0", marked)
	}
}

func TestMessageRejectsUnknownKind(t *testing.T) {
	s := newTestStore(t)

	_, err := s.CreateMessage(context.Background(), Message{
		ID: "m1", SenderID: testUserID, Kind: "shout", Body: "hello",
	})

	if err == nil {
		t.Error("CreateMessage() accepted an unknown kind, want the check constraint to reject it")
	}
}
