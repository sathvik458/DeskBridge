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
	presence := NewPresence(sweeper, nil, discardLogger(), time.Millisecond, 90*time.Second)

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
	presence := NewPresence(sweeper, nil, discardLogger(), time.Millisecond, time.Minute)

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
	presence := NewPresence(sweeper, nil, discardLogger(), time.Millisecond, time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go presence.Run(ctx)

	<-sweeper.called
	time.Sleep(50 * time.Millisecond)

	if sweeper.callCount() < 2 {
		t.Errorf("watcher swept %d times after an error, want it to keep going", sweeper.callCount())
	}
}

type fakeIndex struct {
	known map[string]bool
	err   error
}

func (f *fakeIndex) StoredPaths(_ context.Context) (map[string]bool, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.known, nil
}

type fakeBlobs struct {
	held     map[string]time.Time
	discards []string
	err      error
}

func (f *fakeBlobs) Stored() (map[string]time.Time, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.held, nil
}

func (f *fakeBlobs) Discard(rel string) error {
	f.discards = append(f.discards, rel)
	delete(f.held, rel)
	return nil
}

type fakeBuckets struct{ swept int }

func (f *fakeBuckets) SweepLimiters() int {
	f.swept++
	return 0
}

func sweepOnce(t *testing.T, files *fakeIndex, blobs *fakeBlobs, buckets *fakeBuckets) {
	t.Helper()

	p := NewPresence(&fakeSweeper{}, nil, discardLogger(), time.Millisecond, time.Minute)
	p.AlsoSweep(files, blobs, buckets)
	p.sweep(context.Background())
}

func TestAnOldBlobWithNoRowIsRemoved(t *testing.T) {
	long := time.Now().Add(-3 * time.Hour)

	files := &fakeIndex{known: map[string]bool{"ab/kept": true}}
	blobs := &fakeBlobs{held: map[string]time.Time{
		"ab/kept":   long,
		"cd/orphan": long,
	}}

	sweepOnce(t, files, blobs, &fakeBuckets{})

	if len(blobs.discards) != 1 || blobs.discards[0] != "cd/orphan" {
		t.Errorf("discarded %v, want only cd/orphan", blobs.discards)
	}
}

// A blob written seconds ago may belong to a request that has not committed its
// row yet. Deleting it would break a live upload.
func TestAFreshBlobIsLeftAloneEvenWithNoRow(t *testing.T) {
	files := &fakeIndex{known: map[string]bool{}}
	blobs := &fakeBlobs{held: map[string]time.Time{
		"cd/justnow": time.Now().Add(-30 * time.Second),
	}}

	sweepOnce(t, files, blobs, &fakeBuckets{})

	if len(blobs.discards) != 0 {
		t.Errorf("discarded %v, want a file this new left alone", blobs.discards)
	}
}

func TestNothingIsRemovedWhenEveryBlobHasARow(t *testing.T) {
	long := time.Now().Add(-3 * time.Hour)

	files := &fakeIndex{known: map[string]bool{"ab/one": true, "cd/two": true}}
	blobs := &fakeBlobs{held: map[string]time.Time{"ab/one": long, "cd/two": long}}

	sweepOnce(t, files, blobs, &fakeBuckets{})

	if len(blobs.discards) != 0 {
		t.Errorf("discarded %v from a consistent vault", blobs.discards)
	}
}

// If the database read fails we do not know what is orphaned, and guessing would
// delete real files.
func TestADatabaseFailureDeletesNothing(t *testing.T) {
	files := &fakeIndex{err: errors.New("database is down")}
	blobs := &fakeBlobs{held: map[string]time.Time{
		"cd/orphan": time.Now().Add(-3 * time.Hour),
	}}

	sweepOnce(t, files, blobs, &fakeBuckets{})

	if len(blobs.discards) != 0 {
		t.Errorf("discarded %v while the index was unreadable", blobs.discards)
	}
}

func TestTheSweepAlsoTrimsTheRateLimiters(t *testing.T) {
	buckets := &fakeBuckets{}

	sweepOnce(t, &fakeIndex{known: map[string]bool{}}, &fakeBlobs{held: map[string]time.Time{}}, buckets)

	if buckets.swept == 0 {
		t.Error("the limiter buckets were never swept, so they would grow forever")
	}
}

// The extra jobs are optional, so a worker built without them must still run.
func TestSweepingWithNothingWiredUpIsSafe(t *testing.T) {
	p := NewPresence(&fakeSweeper{}, nil, discardLogger(), time.Millisecond, time.Minute)
	p.sweep(context.Background())
}
