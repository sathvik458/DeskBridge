// Package limit slows down callers that try the same thing too often.
package limit

import (
	"math"
	"sync"
	"time"
)

// A bucket holds tokens that refill over time. Taking one costs an attempt; an
// empty bucket means wait. Burst is the bucket size, so a few quick tries in a row
// are fine and a sustained hammering is not.
type bucket struct {
	tokens float64
	seen   time.Time
}

type Limiter struct {
	guard  sync.Mutex
	slots  map[string]*bucket
	burst  float64
	refill float64
	now    func() time.Time
}

// New builds a limiter allowing burst attempts up front, then one more every
// per. A refill of 0 or a burst below 1 would lock everyone out forever, so both
// are floored.
func New(burst int, per time.Duration) *Limiter {
	if burst < 1 {
		burst = 1
	}
	if per <= 0 {
		per = time.Second
	}

	return &Limiter{
		slots:  make(map[string]*bucket),
		burst:  float64(burst),
		refill: 1 / per.Seconds(),
		now:    time.Now,
	}
}

// Allow takes a token if one is there. The wait it returns is how long until the
// next token lands, which is what a Retry-After header wants.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	l.guard.Lock()
	defer l.guard.Unlock()

	now := l.now()

	b, known := l.slots[key]
	if !known {
		b = &bucket{tokens: l.burst, seen: now}
		l.slots[key] = b
	}

	// Refill for the time that has passed, capped at the burst so an account left
	// alone for a week does not bank a week of attempts.
	b.tokens = math.Min(l.burst, b.tokens+now.Sub(b.seen).Seconds()*l.refill)
	b.seen = now

	if b.tokens < 1 {
		short := 1 - b.tokens
		return false, time.Duration(short/l.refill*float64(time.Second)) + time.Second
	}

	b.tokens--

	return true, 0
}

// Forget drops a key, which is what a successful sign-in does so a legitimate user
// who fumbled their password twice is not still carrying it.
func (l *Limiter) Forget(key string) {
	l.guard.Lock()
	defer l.guard.Unlock()

	delete(l.slots, key)
}

// Sweep removes buckets that have refilled completely. They are indistinguishable
// from a caller that has never been seen, so keeping them only costs memory - and
// without this the map grows once per username anyone ever types.
func (l *Limiter) Sweep() int {
	l.guard.Lock()
	defer l.guard.Unlock()

	now := l.now()
	gone := 0

	for key, b := range l.slots {
		if b.tokens+now.Sub(b.seen).Seconds()*l.refill >= l.burst {
			delete(l.slots, key)
			gone++
		}
	}

	return gone
}

func (l *Limiter) Tracking() int {
	l.guard.Lock()
	defer l.guard.Unlock()

	return len(l.slots)
}
