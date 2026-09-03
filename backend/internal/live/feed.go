// Package live carries server events out to whoever is watching.
package live

import (
	"log/slog"
	"sync"
	"time"
)

const (
	SessionStarted = "session.started"
	SessionPaused  = "session.paused"
	SessionResumed = "session.resumed"
	SessionEnded   = "session.ended"
	GoalChanged    = "goal.changed"
	MessageSent    = "message.sent"
	HelpRaised     = "help.raised"
	DeviceChanged  = "device.changed"
)

type Event struct {
	Kind string         `json:"kind"`
	At   time.Time      `json:"at"`
	Body map[string]any `json:"body,omitempty"`
}

const mailboxSize = 16

type watcher struct {
	mailbox chan Event
	missed  int
}

type Feed struct {
	guard    sync.Mutex
	watchers map[int]*watcher
	nextTag  int
	log      *slog.Logger
}

func NewFeed(log *slog.Logger) *Feed {
	return &Feed{
		watchers: make(map[int]*watcher),
		log:      log,
	}
}

// Watch returns a channel of events and a function that stops the watch. The
// caller must call the stop function or the feed leaks a mailbox per connection.
func (f *Feed) Watch() (<-chan Event, func()) {
	f.guard.Lock()
	defer f.guard.Unlock()

	tag := f.nextTag
	f.nextTag++

	seat := &watcher{mailbox: make(chan Event, mailboxSize)}
	f.watchers[tag] = seat

	return seat.mailbox, func() { f.drop(tag) }
}

func (f *Feed) drop(tag int) {
	f.guard.Lock()
	defer f.guard.Unlock()

	seat, watching := f.watchers[tag]
	if !watching {
		return
	}

	delete(f.watchers, tag)
	close(seat.mailbox)

	if seat.missed > 0 {
		f.log.Warn("watcher fell behind", "missed", seat.missed)
	}
}

// Announce is deliberately non-blocking. A browser that has stopped reading must
// never be able to stall a request handler, so a full mailbox drops the event and
// counts it instead. The dropped update is recovered by the client's next poll.
func (f *Feed) Announce(kind string, body map[string]any) {
	event := Event{Kind: kind, At: time.Now().UTC(), Body: body}

	f.guard.Lock()
	defer f.guard.Unlock()

	for _, seat := range f.watchers {
		select {
		case seat.mailbox <- event:
		default:
			seat.missed++
		}
	}
}

func (f *Feed) Watching() int {
	f.guard.Lock()
	defer f.guard.Unlock()

	return len(f.watchers)
}
