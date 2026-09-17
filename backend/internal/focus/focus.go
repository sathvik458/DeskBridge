// Package focus holds the pomodoro rules: how long each stretch runs and what
// comes after it.
package focus

import (
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/store"
)

const (
	WorkFor       = 25 * time.Minute
	ShortBreakFor = 5 * time.Minute
	LongBreakFor  = 15 * time.Minute

	// A long break lands after this many finished work stretches.
	RoundLength = 4
)

func Planned(kind string) time.Duration {
	switch kind {
	case store.FocusShortBreak:
		return ShortBreakFor
	case store.FocusLongBreak:
		return LongBreakFor
	default:
		return WorkFor
	}
}

func Known(kind string) bool {
	switch kind {
	case store.FocusWork, store.FocusShortBreak, store.FocusLongBreak:
		return true
	}
	return false
}

// Next suggests what to start once the current stretch is over. Breaks always
// lead back to work; work leads to a break, long one every fourth time.
func Next(justFinished string, doneToday int) string {
	if justFinished != store.FocusWork {
		return store.FocusWork
	}

	if doneToday > 0 && doneToday%RoundLength == 0 {
		return store.FocusLongBreak
	}

	return store.FocusShortBreak
}

// Ahead is what to suggest when nothing is running at all.
func Ahead(doneToday int) string {
	if doneToday > 0 && doneToday%RoundLength == 0 {
		return store.FocusLongBreak
	}
	return store.FocusWork
}

// IntoRound is how far through the current set of four the student is, which is
// what the dots under the timer show.
func IntoRound(doneToday int) int {
	return doneToday % RoundLength
}
