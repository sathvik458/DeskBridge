package live

import (
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

func quietLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func waitFor(t *testing.T, mail <-chan Event) Event {
	t.Helper()

	select {
	case event := <-mail:
		return event
	case <-time.After(time.Second):
		t.Fatal("no event arrived")
		return Event{}
	}
}

func TestEveryWatcherGetsTheEvent(t *testing.T) {
	feed := NewFeed(quietLog())

	first, stopFirst := feed.Watch()
	second, stopSecond := feed.Watch()
	defer stopFirst()
	defer stopSecond()

	if feed.Watching() != 2 {
		t.Fatalf("Watching() = %d, want 2", feed.Watching())
	}

	feed.Announce(HelpRaised, map[string]any{"message_id": "m1"})

	for name, mail := range map[string]<-chan Event{"first": first, "second": second} {
		event := waitFor(t, mail)
		if event.Kind != HelpRaised {
			t.Errorf("%s got kind %q, want %q", name, event.Kind, HelpRaised)
		}
		if event.Body["message_id"] != "m1" {
			t.Errorf("%s got body %v", name, event.Body)
		}
	}
}

func TestStoppingAWatchRemovesIt(t *testing.T) {
	feed := NewFeed(quietLog())

	mail, stop := feed.Watch()
	stop()

	if feed.Watching() != 0 {
		t.Errorf("Watching() = %d after stopping, want 0", feed.Watching())
	}

	if _, open := <-mail; open {
		t.Error("the mailbox should be closed once the watch stops")
	}
}

func TestStoppingTwiceIsHarmless(t *testing.T) {
	feed := NewFeed(quietLog())

	_, stop := feed.Watch()
	stop()
	stop()
}

func TestAnnounceWithNoWatchersDoesNothing(t *testing.T) {
	feed := NewFeed(quietLog())

	feed.Announce(MessageSent, nil)
}

// The point of the non-blocking send: a watcher that stops reading must not be able
// to stall the caller.
func TestASlowWatcherNeverBlocksTheAnnouncer(t *testing.T) {
	feed := NewFeed(quietLog())

	_, stop := feed.Watch()
	defer stop()

	finished := make(chan struct{})

	go func() {
		for i := 0; i < mailboxSize*5; i++ {
			feed.Announce(MessageSent, map[string]any{"n": i})
		}
		close(finished)
	}()

	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("Announce blocked on a watcher that was not reading")
	}
}

func TestAWatcherStartedLaterMissesEarlierEvents(t *testing.T) {
	feed := NewFeed(quietLog())

	feed.Announce(SessionStarted, map[string]any{"session_id": "s0"})

	mail, stop := feed.Watch()
	defer stop()

	feed.Announce(SessionEnded, map[string]any{"session_id": "s1"})

	event := waitFor(t, mail)
	if event.Kind != SessionEnded {
		t.Errorf("kind = %q, want %q - the feed is not a replayable log", event.Kind, SessionEnded)
	}
}

func TestConcurrentWatchAnnounceAndStop(t *testing.T) {
	feed := NewFeed(quietLog())

	var crowd sync.WaitGroup

	for i := 0; i < 20; i++ {
		crowd.Add(1)
		go func() {
			defer crowd.Done()
			mail, stop := feed.Watch()
			go func() {
				for range mail {
				}
			}()
			time.Sleep(time.Millisecond)
			stop()
		}()
	}

	for i := 0; i < 50; i++ {
		crowd.Add(1)
		go func() {
			defer crowd.Done()
			feed.Announce(DeviceChanged, map[string]any{"device_id": "d1"})
		}()
	}

	crowd.Wait()

	if feed.Watching() != 0 {
		t.Errorf("Watching() = %d after everyone left, want 0", feed.Watching())
	}
}
