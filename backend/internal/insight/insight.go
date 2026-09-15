// Package insight turns raw sessions and goals into the numbers a dashboard shows.
package insight

import (
	"sort"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/store"
)

const DayLayout = "2006-01-02"

type Day struct {
	Date     string
	Seconds  int64
	Sessions int
}

type Subject struct {
	Name     string
	Seconds  int64
	Sessions int
}

type Report struct {
	Days       []Day
	Subjects   []Subject
	Total      int64
	Best       Day
	Streak     int
	GoalsSet   int
	GoalsDone  int
	Zone       string
	SessionCnt int
}

// Window is the UTC span to ask the database for. It reaches a day past each end
// because a local day can start before or finish after the matching UTC day.
func Window(now time.Time, days int, zone *time.Location) (time.Time, time.Time) {
	last := now.In(zone)
	first := last.AddDate(0, 0, -(days - 1))

	start := time.Date(first.Year(), first.Month(), first.Day(), 0, 0, 0, 0, zone)
	end := time.Date(last.Year(), last.Month(), last.Day(), 0, 0, 0, 0, zone).AddDate(0, 0, 1)

	return start.UTC().Add(-24 * time.Hour), end.UTC().Add(24 * time.Hour)
}

// Build fills every day in the range, including the empty ones, so a chart does
// not silently skip the days nobody studied.
func Build(sessions []store.Session, goals []store.Goal, now time.Time, days int, zone *time.Location) Report {
	local := now.In(zone)
	last := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone)

	slots := map[string]*Day{}
	order := make([]string, 0, days)

	for i := days - 1; i >= 0; i-- {
		key := last.AddDate(0, 0, -i).Format(DayLayout)
		slots[key] = &Day{Date: key}
		order = append(order, key)
	}

	subjects := map[string]*Subject{}
	report := Report{Zone: zone.String()}

	for _, s := range sessions {
		key := s.StartedAt.In(zone).Format(DayLayout)

		slot, inRange := slots[key]
		if !inRange {
			continue
		}

		secs := int64(s.Elapsed(now).Seconds())

		slot.Seconds += secs
		slot.Sessions++

		sub, seen := subjects[s.Subject]
		if !seen {
			sub = &Subject{Name: s.Subject}
			subjects[s.Subject] = sub
		}
		sub.Seconds += secs
		sub.Sessions++

		report.Total += secs
		report.SessionCnt++
	}

	for _, key := range order {
		d := *slots[key]
		report.Days = append(report.Days, d)
		if d.Seconds > report.Best.Seconds {
			report.Best = d
		}
	}

	for _, sub := range subjects {
		report.Subjects = append(report.Subjects, *sub)
	}

	sort.Slice(report.Subjects, func(i, j int) bool {
		if report.Subjects[i].Seconds != report.Subjects[j].Seconds {
			return report.Subjects[i].Seconds > report.Subjects[j].Seconds
		}
		return report.Subjects[i].Name < report.Subjects[j].Name
	})

	from := order[0]
	to := order[len(order)-1]

	for _, g := range goals {
		if g.GoalDate < from || g.GoalDate > to {
			continue
		}
		report.GoalsSet++
		if g.Done() {
			report.GoalsDone++
		}
	}

	report.Streak = streak(report.Days)

	return report
}

// A streak counts back from the most recent day. Today being empty does not break
// it, because the day is not over yet - yesterday being empty does.
func streak(days []Day) int {
	if len(days) == 0 {
		return 0
	}

	run := 0

	for i := len(days) - 1; i >= 0; i-- {
		if days[i].Seconds > 0 {
			run++
			continue
		}
		if i == len(days)-1 {
			continue
		}
		break
	}

	return run
}
