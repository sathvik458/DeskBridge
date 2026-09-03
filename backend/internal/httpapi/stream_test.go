package httpapi

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/live"
)

func startTestStream(t *testing.T) (*Server, *httptest.Server, *bufio.Reader, func()) {
	t.Helper()

	feed := live.NewFeed(discardTestLogger())
	s := newTestServerWithFeed(t, feed)

	web := httptest.NewServer(s.Routes())

	response, err := http.Get(web.URL + "/api/live")
	if err != nil {
		t.Fatalf("opening the stream: %v", err)
	}

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}

	if got := response.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", got)
	}

	reader := bufio.NewReader(response.Body)

	// The greeting comment arrives immediately, which is how we know the handler is
	// past its setup and has registered a watch.
	line, err := reader.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, ":") {
		t.Fatalf("first line = %q, err %v; want a comment", line, err)
	}
	reader.ReadString('\n')

	waitForWatchers(t, feed, 1)

	return s, web, reader, func() {
		response.Body.Close()
		web.Close()
	}
}

func waitForWatchers(t *testing.T, feed *live.Feed, want int) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if feed.Watching() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("Watching() = %d, want %d", feed.Watching(), want)
}

func readEvent(t *testing.T, reader *bufio.Reader) (string, string) {
	t.Helper()

	var kind, data string

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("reading the stream: %v", err)
		}

		line = strings.TrimRight(line, "\n")

		switch {
		case strings.HasPrefix(line, "event: "):
			kind = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		case line == "" && kind != "":
			return kind, data
		}
	}
}

func TestStreamDeliversAnEvent(t *testing.T) {
	s, _, reader, done := startTestStream(t)
	defer done()

	go s.feed.Announce(live.HelpRaised, map[string]any{"message_id": "m1"})

	kind, data := readEvent(t, reader)

	if kind != live.HelpRaised {
		t.Errorf("event kind = %q, want %q", kind, live.HelpRaised)
	}
	if !strings.Contains(data, `"message_id":"m1"`) {
		t.Errorf("data = %s, want it to carry the message id", data)
	}
}

func TestSendingAMessageReachesTheStream(t *testing.T) {
	s, web, reader, done := startTestStream(t)
	defer done()

	go func() {
		http.Post(web.URL+"/api/messages", "application/json",
			strings.NewReader(`{"from":"student","kind":"help_request","body":"stuck on question 4"}`))
	}()

	kind, data := readEvent(t, reader)

	if kind != live.HelpRaised {
		t.Errorf("kind = %q, want %q", kind, live.HelpRaised)
	}
	if !strings.Contains(data, "stuck on question 4") {
		t.Errorf("data = %s, want the body of the help request", data)
	}

	_ = s
}

func TestClosingTheConnectionEndsTheWatch(t *testing.T) {
	s, _, _, done := startTestStream(t)

	done()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s.feed.Watching() == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Errorf("Watching() = %d after the client left, want 0", s.feed.Watching())
}

func TestTwoStreamsBothGetTheEvent(t *testing.T) {
	feed := live.NewFeed(discardTestLogger())
	s := newTestServerWithFeed(t, feed)

	web := httptest.NewServer(s.Routes())
	defer web.Close()

	readers := make([]*bufio.Reader, 0, 2)

	for i := 0; i < 2; i++ {
		response, err := http.Get(web.URL + "/api/live")
		if err != nil {
			t.Fatalf("opening stream %d: %v", i, err)
		}
		defer response.Body.Close()

		reader := bufio.NewReader(response.Body)
		reader.ReadString('\n')
		reader.ReadString('\n')
		readers = append(readers, reader)
	}

	waitForWatchers(t, feed, 2)

	go feed.Announce(live.SessionStarted, map[string]any{"session_id": "s1"})

	for i, reader := range readers {
		kind, _ := readEvent(t, reader)
		if kind != live.SessionStarted {
			t.Errorf("stream %d got %q, want %q", i, kind, live.SessionStarted)
		}
	}
}
