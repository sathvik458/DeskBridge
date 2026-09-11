package limit

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func frozen(t *testing.T, burst int, per time.Duration) (*Limiter, func(time.Duration)) {
	t.Helper()

	at := time.Date(2026, 9, 11, 9, 0, 0, 0, time.UTC)
	l := New(burst, per)
	l.now = func() time.Time { return at }

	return l, func(d time.Duration) { at = at.Add(d) }
}

func TestBurstIsAllowedThenRefused(t *testing.T) {
	l, _ := frozen(t, 3, time.Minute)

	for i := 1; i <= 3; i++ {
		if ok, _ := l.Allow("ricky"); !ok {
			t.Fatalf("attempt %d was refused, want the first 3 allowed", i)
		}
	}

	ok, wait := l.Allow("ricky")
	if ok {
		t.Fatal("the fourth attempt was allowed, want it refused")
	}
	if wait <= 0 {
		t.Error("a refusal came back with no wait to report")
	}
}

func TestTokensComeBackWithTime(t *testing.T) {
	l, tick := frozen(t, 2, time.Minute)

	l.Allow("ricky")
	l.Allow("ricky")

	if ok, _ := l.Allow("ricky"); ok {
		t.Fatal("expected to be empty")
	}

	tick(time.Minute)

	if ok, _ := l.Allow("ricky"); !ok {
		t.Error("a minute later the bucket is still empty, want one token back")
	}
	if ok, _ := l.Allow("ricky"); ok {
		t.Error("two tokens came back after one refill period")
	}
}

// An account nobody touches for a week must not bank a week of attempts.
func TestRefillIsCappedAtTheBurst(t *testing.T) {
	l, tick := frozen(t, 3, time.Minute)

	l.Allow("ricky")
	tick(7 * 24 * time.Hour)

	for i := 1; i <= 3; i++ {
		if ok, _ := l.Allow("ricky"); !ok {
			t.Fatalf("attempt %d refused after a long idle, want the full burst back", i)
		}
	}

	if ok, _ := l.Allow("ricky"); ok {
		t.Error("more than the burst was banked while idle")
	}
}

func TestKeysAreIndependent(t *testing.T) {
	l, _ := frozen(t, 1, time.Minute)

	if ok, _ := l.Allow("ricky"); !ok {
		t.Fatal("first key refused")
	}
	if ok, _ := l.Allow("ricky"); ok {
		t.Fatal("first key should be empty")
	}

	// Someone else hammering their own account must not lock me out of mine.
	if ok, _ := l.Allow("arjun"); !ok {
		t.Error("a second key was refused because the first was exhausted")
	}
}

func TestForgetClearsAKey(t *testing.T) {
	l, _ := frozen(t, 1, time.Minute)

	l.Allow("ricky")
	if ok, _ := l.Allow("ricky"); ok {
		t.Fatal("expected to be empty")
	}

	l.Forget("ricky")

	if ok, _ := l.Allow("ricky"); !ok {
		t.Error("the key is still limited after Forget")
	}
}

func TestTheWaitShrinksAsTimePasses(t *testing.T) {
	l, tick := frozen(t, 1, 10*time.Minute)

	l.Allow("ricky")

	_, first := l.Allow("ricky")
	tick(5 * time.Minute)
	_, second := l.Allow("ricky")

	if second >= first {
		t.Errorf("wait went from %s to %s, want it shorter after time passed", first, second)
	}
}

func TestSweepDropsOnlyRefilledBuckets(t *testing.T) {
	l, tick := frozen(t, 2, time.Minute)

	l.Allow("idle")
	l.Allow("busy")
	l.Allow("busy")

	tick(time.Minute)

	// "idle" used one token and has had a minute, so it is full again. "busy"
	// used both and is still one short.
	if gone := l.Sweep(); gone != 1 {
		t.Errorf("swept %d, want 1", gone)
	}
	if l.Tracking() != 1 {
		t.Errorf("tracking %d keys after the sweep, want 1", l.Tracking())
	}

	// The one that survived must keep its state, not silently reset.
	if ok, _ := l.Allow("busy"); !ok {
		t.Fatal("busy had one token back and was refused")
	}
	if ok, _ := l.Allow("busy"); ok {
		t.Error("busy was reset by the sweep instead of keeping its count")
	}
}

func TestSweepKeepsMemoryFromGrowingForever(t *testing.T) {
	l, tick := frozen(t, 1, time.Second)

	for i := 0; i < 500; i++ {
		l.Allow(fmt.Sprintf("someone-%d", i))
	}

	if l.Tracking() != 500 {
		t.Fatalf("tracking %d, want 500", l.Tracking())
	}

	tick(time.Hour)

	l.Sweep()

	if l.Tracking() != 0 {
		t.Errorf("tracking %d after everything refilled, want 0", l.Tracking())
	}
}

func TestSillyValuesDoNotLockEveryoneOut(t *testing.T) {
	for _, c := range []struct {
		burst int
		per   time.Duration
	}{{0, time.Minute}, {-5, time.Minute}, {3, 0}, {3, -time.Second}} {
		l := New(c.burst, c.per)
		if ok, _ := l.Allow("ricky"); !ok {
			t.Errorf("New(%d, %s) refused the very first attempt", c.burst, c.per)
		}
	}
}

// The limiter sits in front of a handler, so several requests hit it at once.
func TestConcurrentCallersDoNotRace(t *testing.T) {
	l := New(1000, time.Millisecond)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				l.Allow(fmt.Sprintf("key-%d", n%5))
				l.Sweep()
			}
		}(i)
	}
	wg.Wait()
}
