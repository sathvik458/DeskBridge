package notify

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/sathvik458/deskbridge/backend/internal/live"
)

func TestComposeSkipsNoiseEvents(t *testing.T) {
	quiet := []string{live.BoardMarked, live.DeviceChanged, live.GoalChanged, live.SessionPaused}

	for _, kind := range quiet {
		if _, worth := Compose(kind, nil); worth {
			t.Errorf("%s should not leave a notice behind", kind)
		}
	}
}

func TestComposeCarriesWhatWasSaid(t *testing.T) {
	note, worth := Compose(live.HelpRaised, map[string]any{"body": "stuck on question 4"})
	if !worth {
		t.Fatal("a help request is worth a notice")
	}

	if !strings.Contains(note.Line, "stuck on question 4") {
		t.Errorf("line lost the message: %q", note.Line)
	}
	if note.Tag != live.HelpRaised {
		t.Errorf("tag = %q, want %q", note.Tag, live.HelpRaised)
	}
}

func TestComposeClipsLongBodies(t *testing.T) {
	note, _ := Compose(live.MessageSent, map[string]any{"body": strings.Repeat("a", 200)})

	if len(note.Line) > 100 {
		t.Errorf("line is %d chars, expected it to be clipped: %q", len(note.Line), note.Line)
	}
	if !strings.HasSuffix(note.Line, "…") {
		t.Errorf("clipped line should say it was clipped: %q", note.Line)
	}
}

// Two people talking at once are two notices, not one that keeps overwriting itself.
func TestMessageTagsSplitBySender(t *testing.T) {
	mum, _ := Compose(live.MessageSent, map[string]any{"from": "supporter"})
	kid, _ := Compose(live.MessageSent, map[string]any{"from": "student"})

	if mum.Tag == kid.Tag {
		t.Errorf("both senders share tag %q", mum.Tag)
	}
}

func TestOnlyFinishedFocusIsWorthANotice(t *testing.T) {
	if _, worth := Compose(live.FocusChanged, map[string]any{"state": "started"}); worth {
		t.Error("starting a timer should not notify anyone")
	}

	if _, worth := Compose(live.FocusChanged, map[string]any{"state": "finished"}); !worth {
		t.Error("finishing a stretch is worth knowing about")
	}
}

func TestCountOnlyShowsOnceItRepeats(t *testing.T) {
	if got := Count("Asked for help", 1); got != "Asked for help" {
		t.Errorf("single notice should read plainly, got %q", got)
	}

	if got := Count("Asked for help", 3); !strings.Contains(got, "3") {
		t.Errorf("repeated notice should show the count, got %q", got)
	}
}

func TestComposeSurvivesAMissingBody(t *testing.T) {
	note, worth := Compose(live.HelpRaised, nil)
	if !worth || note.Line == "" {
		t.Fatalf("a help request with no body still needs a line, got %q", note.Line)
	}
}

// Every kind on the mutable list has to be a kind Compose actually recognises,
// or the settings screen would offer a switch that does nothing.
func TestEveryMutableKindComposes(t *testing.T) {
	for _, kind := range Worth() {
		if _, ok := Compose(kind, map[string]any{"state": "finished"}); !ok {
			t.Errorf("%s is offered as mutable but Compose ignores it", kind)
		}
	}
}

func TestSomethingOffTheListIsNotMutable(t *testing.T) {
	if Mutable(live.BoardMarked) {
		t.Error("a whiteboard stroke should not be something you can mute, it never notifies")
	}
}

// Worth hands out a copy, so a caller cannot reorder or empty the real list.
func TestWorthCannotBeEditedFromOutside(t *testing.T) {
	first := Worth()
	first[0] = "tampered"

	if Worth()[0] == "tampered" {
		t.Error("Worth handed out the underlying slice")
	}
}

func TestClipNeverBreaksALetter(t *testing.T) {
	note, _ := Compose(live.HelpRaised, map[string]any{"body": strings.Repeat("సహాయం కావాలి ", 20)})

	if !utf8.ValidString(note.Line) {
		t.Errorf("clipped line is not valid UTF-8: %q", note.Line)
	}
}
