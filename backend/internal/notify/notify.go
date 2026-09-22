// Package notify turns a live event into the line a notice shows, and decides
// when two events are really the same thing happening twice.
package notify

import (
	"fmt"
	"strings"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/live"
)

// Within is how close together two events of the same tag have to be before the
// second one bumps the first instead of stacking up beside it.
const Within = 5 * time.Minute

const snippetLen = 60

type Note struct {
	Line string
	Tag  string
}

// Compose returns the notice an event deserves. The second result is false for
// events that are only worth a live nudge: a pen stroke on the whiteboard should
// not still be waiting for you the next morning.
func Compose(kind string, body map[string]any) (Note, bool) {
	switch kind {
	case live.HelpRaised:
		return Note{Line: withSnippet("Asked for help", body), Tag: kind}, true

	case live.MessageSent:
		return Note{Line: withSnippet("New message", body), Tag: tagPerSender(kind, body)}, true

	case live.FileAdded:
		return Note{Line: withName("Shared a file", body), Tag: kind}, true

	case live.SessionStarted:
		return Note{Line: withName("Started studying", body), Tag: kind}, true

	case live.SessionEnded:
		return Note{Line: "Finished studying", Tag: kind}, true

	case live.FocusChanged:
		if text(body, "state") != "finished" {
			return Note{}, false
		}
		return Note{Line: "Finished a focus stretch", Tag: kind + ":finished"}, true
	}

	return Note{}, false
}

// Two people talking at once should not collapse into one notice, so the sender
// is part of what makes a message notice unique.
func tagPerSender(kind string, body map[string]any) string {
	if from := text(body, "from"); from != "" {
		return kind + ":" + from
	}
	return kind
}

func withSnippet(lead string, body map[string]any) string {
	said := clip(text(body, "body"))
	if said == "" {
		return lead
	}
	return lead + " — " + said
}

func withName(lead string, body map[string]any) string {
	for _, key := range []string{"subject", "name"} {
		if v := clip(text(body, key)); v != "" {
			return lead + " — " + v
		}
	}
	return lead
}

// Count is how the line reads once the same thing has happened more than once.
func Count(line string, times int) string {
	if times < 2 {
		return line
	}
	return fmt.Sprintf("%s (%d×)", line, times)
}

func clip(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= snippetLen {
		return s
	}
	return strings.TrimSpace(s[:snippetLen]) + "…"
}

func text(body map[string]any, key string) string {
	if body == nil {
		return ""
	}
	s, _ := body[key].(string)
	return s
}
