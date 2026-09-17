package focus

import (
	"testing"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/store"
)

func TestPlannedLengths(t *testing.T) {
	cases := map[string]time.Duration{
		store.FocusWork:       WorkFor,
		store.FocusShortBreak: ShortBreakFor,
		store.FocusLongBreak:  LongBreakFor,
	}

	for kind, want := range cases {
		if got := Planned(kind); got != want {
			t.Errorf("Planned(%s) = %s, want %s", kind, got, want)
		}
	}
}

// An unknown kind never reaches here from a validated handler, but falling back to
// a work stretch beats returning zero and starting a timer that is instantly over.
func TestAnUnknownKindFallsBackToWork(t *testing.T) {
	if got := Planned("nap"); got != WorkFor {
		t.Errorf("Planned(nap) = %s, want %s", got, WorkFor)
	}
}

func TestKnown(t *testing.T) {
	for _, kind := range []string{store.FocusWork, store.FocusShortBreak, store.FocusLongBreak} {
		if !Known(kind) {
			t.Errorf("Known(%s) = false", kind)
		}
	}

	for _, kind := range []string{"", "nap", "FOCUS", "work"} {
		if Known(kind) {
			t.Errorf("Known(%q) = true", kind)
		}
	}
}

func TestABreakAlwaysLeadsBackToWork(t *testing.T) {
	for _, kind := range []string{store.FocusShortBreak, store.FocusLongBreak} {
		for _, done := range []int{0, 1, 4, 8} {
			if got := Next(kind, done); got != store.FocusWork {
				t.Errorf("Next(%s, %d) = %s, want work", kind, done, got)
			}
		}
	}
}

func TestWorkLeadsToAShortBreak(t *testing.T) {
	for _, done := range []int{1, 2, 3, 5, 6, 7} {
		if got := Next(store.FocusWork, done); got != store.FocusShortBreak {
			t.Errorf("after %d finished, Next = %s, want a short break", done, got)
		}
	}
}

func TestEveryFourthWorkStretchEarnsALongBreak(t *testing.T) {
	for _, done := range []int{4, 8, 12} {
		if got := Next(store.FocusWork, done); got != store.FocusLongBreak {
			t.Errorf("after %d finished, Next = %s, want a long break", done, got)
		}
	}
}

// Zero finished is not a completed round, so it must not hand out a long break
// before any work has been done.
func TestNoWorkYetDoesNotEarnALongBreak(t *testing.T) {
	if got := Next(store.FocusWork, 0); got != store.FocusShortBreak {
		t.Errorf("Next(work, 0) = %s, want a short break", got)
	}
	if got := Ahead(0); got != store.FocusWork {
		t.Errorf("Ahead(0) = %s, want work", got)
	}
}

func TestAheadOffersALongBreakOnARoundBoundary(t *testing.T) {
	if got := Ahead(4); got != store.FocusLongBreak {
		t.Errorf("Ahead(4) = %s, want a long break", got)
	}
	if got := Ahead(3); got != store.FocusWork {
		t.Errorf("Ahead(3) = %s, want work", got)
	}
}

func TestIntoRoundWrapsEveryFour(t *testing.T) {
	want := map[int]int{0: 0, 1: 1, 2: 2, 3: 3, 4: 0, 5: 1, 9: 1}

	for done, expected := range want {
		if got := IntoRound(done); got != expected {
			t.Errorf("IntoRound(%d) = %d, want %d", done, got, expected)
		}
	}
}
