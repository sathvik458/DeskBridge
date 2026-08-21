package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type fakeSweeper struct {
	mu     sync.Mutex
	calls  int
	silent time.Duration
	err    error
	called chan struct{}
}

func (f *fakeSweeper) MarkStaleDevicesOffline(ctx context.Context, silentFor time.Duration) (int, error) {
	f.mu.Lock()
	f.calls++
	f.silent = silentFor
	f.mu.Unlock()

	select {
	case f.called <- struct{}{}:
	default:
	}

	return 1, f.err
}

func (f *fakeSweeper) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestPresenceSweepsOnEveryTick(t *testing.T) {
	sweeper := &fakeSweeper{called: make(chan struct{}, 1)}
	presence := NewPresence(sweeper, discardLogger(), time.Millisecond, 90*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go presence.Run(ctx)

	select {
	case <-sweeper.called:
	case <-time.After(2 * time.Second):
		t.Fatal("the watcher never swept")
	}

	if got := sweeper.silent; got != 90*time.Second {
		t.Errorf("swept with timeout %s, want %s", got, 90*time.Second)
	}
}

func TestPresenceStopsWhenContextIsCancelled(t *testing.T) {
	sweeper := &fakeSweeper{called: make(chan struct{}, 1)}
	presence := NewPresence(sweeper, discardLogger(), time.Millisecond, time.Minute)

	ctx, cancel := context.WithCancel(context.Background())

	stopped := make(chan struct{})
	go func() {
		presence.Run(ctx)
		close(stopped)
	}()

	<-sweeper.called
	cancel()

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after the context was cancelled")
	}

	before := sweeper.callCount()
	time.Sleep(20 * time.Millisecond)

	if after := sweeper.callCount(); after != before {
		t.Errorf("watcher swept %d more times after stopping", after-before)
	}
}

func TestPresenceKeepsRunningAfterAFailedSweep(t *testing.T) {
	sweeper := &fakeSweeper{called: make(chan struct{}, 1), err: errors.New("database is busy")}
	presence := NewPresence(sweeper, discardLogger(), time.Millisecond, time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go presence.Run(ctx)

	<-sweeper.called
	time.Sleep(50 * time.Millisecond)

	if sweeper.callCount() < 2 {
		t.Errorf("watcher swept %d times after an error, want it to keep going", sweeper.callCount())
	}
}
