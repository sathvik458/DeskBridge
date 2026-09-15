package insight

import (
	"testing"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/store"
)

var bahrain = time.FixedZone("Asia/Bahrain", 3*60*60)

// Mid-afternoon in Bahrain, so "now" is unambiguously on one local day.
var now = time.Date(2026, 9, 15, 11, 0, 0, 0, time.UTC)

func done(id, subject, startedAt string, mins int) store.Session {
	t, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		panic(err)
	}

	return store.Session{
		ID: id, Subject: subject, Status: store.SessionCompleted,
		StartedAt: t, AccumulatedSeconds: int64(mins) * 60,
	}
}

func goalOn(id, date string, finished bool) store.Goal {
	g := store.Goal{ID: id, GoalDate: date, Subject: "Physics", TargetMinutes: 60}
	if finished {
		at := now
		g.CompletedAt = &at
	}
	return g
}

func dayOf(r Report, date string) Day {
	for _, d := range r.Days {
		if d.Date == date {
			return d
		}
	}
	return Day{}
}

func TestEveryDayInTheRangeAppears(t *testing.T) {
	r := Build(nil, nil, now, 7, bahrain)

	if len(r.Days) != 7 {
		t.Fatalf("got %d days, want 7", len(r.Days))
	}
	if r.Days[6].Date != "2026-09-15" {
		t.Errorf("last day = %q, want today in Bahrain", r.Days[6].Date)
	}
	if r.Days[0].Date != "2026-09-09" {
		t.Errorf("first day = %q, want six days back", r.Days[0].Date)
	}
}

// A chart that skips empty days lies about consistency.
func TestQuietDaysAreStillListed(t *testing.T) {
	r := Build([]store.Session{done("s1", "Physics", "2026-09-15T08:00:00Z", 60)}, nil, now, 3, bahrain)

	if len(r.Days) != 3 {
		t.Fatalf("got %d days, want 3", len(r.Days))
	}
	for _, d := range r.Days[:2] {
		if d.Sessions != 0 || d.Seconds != 0 {
			t.Errorf("%s should be empty, got %+v", d.Date, d)
		}
	}
}

// 22:15 UTC is 01:15 the next morning in Bahrain. Bucketing on the raw UTC date
// would file this under the wrong day.
func TestALateSessionLandsOnTheLocalDay(t *testing.T) {
	late := done("s1", "Physics", "2026-09-11T22:15:00Z", 30)

	r := Build([]store.Session{late}, nil, now, 10, bahrain)

	if got := dayOf(r, "2026-09-12"); got.Sessions != 1 {
		t.Errorf("2026-09-12 has %d sessions, want the late one filed there", got.Sessions)
	}
	if got := dayOf(r, "2026-09-11"); got.Sessions != 0 {
		t.Errorf("2026-09-11 has %d sessions, want none", got.Sessions)
	}
}

// The same instant, read in UTC, belongs to the previous day.
func TestTheSameSessionMovesWithTheZone(t *testing.T) {
	late := done("s1", "Physics", "2026-09-11T22:15:00Z", 30)

	inUTC := Build([]store.Session{late}, nil, now, 10, time.UTC)

	if got := dayOf(inUTC, "2026-09-11"); got.Sessions != 1 {
		t.Errorf("in UTC the session should be on the 11th, got %+v", got)
	}
}

// Behind UTC as well as ahead, or the shift is only being handled one way.
func TestAnEarlySessionMovesBackwardsInAWesternZone(t *testing.T) {
	newYork := time.FixedZone("America/New_York", -4*60*60)
	early := done("s1", "Physics", "2026-09-12T02:00:00Z", 45)

	r := Build([]store.Session{early}, nil, now, 10, newYork)

	if got := dayOf(r, "2026-09-11"); got.Sessions != 1 {
		t.Errorf("02:00 UTC is the previous evening in New York, got %+v for the 11th", got)
	}
}

func TestSecondsAndCountsAddUp(t *testing.T) {
	sessions := []store.Session{
		done("s1", "Physics", "2026-09-15T06:00:00Z", 30),
		done("s2", "Physics", "2026-09-15T09:00:00Z", 45),
		done("s3", "Chemistry", "2026-09-14T09:00:00Z", 60),
	}

	r := Build(sessions, nil, now, 7, bahrain)

	if today := dayOf(r, "2026-09-15"); today.Seconds != 75*60 || today.Sessions != 2 {
		t.Errorf("today = %+v, want 75 minutes over 2 sessions", today)
	}
	if r.Total != 135*60 {
		t.Errorf("total = %d, want %d", r.Total, 135*60)
	}
	if r.SessionCnt != 3 {
		t.Errorf("session count = %d, want 3", r.SessionCnt)
	}
}

func TestSubjectsAreRankedByTime(t *testing.T) {
	sessions := []store.Session{
		done("s1", "Physics", "2026-09-15T06:00:00Z", 30),
		done("s2", "Chemistry", "2026-09-15T07:00:00Z", 90),
		done("s3", "Physics", "2026-09-14T06:00:00Z", 20),
	}

	r := Build(sessions, nil, now, 7, bahrain)

	if len(r.Subjects) != 2 {
		t.Fatalf("got %d subjects, want 2", len(r.Subjects))
	}
	if r.Subjects[0].Name != "Chemistry" || r.Subjects[0].Seconds != 90*60 {
		t.Errorf("first = %+v, want Chemistry on 90 minutes", r.Subjects[0])
	}
	if r.Subjects[1].Sessions != 2 {
		t.Errorf("Physics session count = %d, want 2", r.Subjects[1].Sessions)
	}
}

// Ordering has to be stable when two subjects tie, or the list reshuffles on every
// refresh for no reason.
func TestATieBetweenSubjectsBreaksOnName(t *testing.T) {
	sessions := []store.Session{
		done("s1", "Zoology", "2026-09-15T06:00:00Z", 30),
		done("s2", "Algebra", "2026-09-15T07:00:00Z", 30),
	}

	r := Build(sessions, nil, now, 7, bahrain)

	if r.Subjects[0].Name != "Algebra" {
		t.Errorf("first = %q, want Algebra to win the tie", r.Subjects[0].Name)
	}
}

func TestBestDay(t *testing.T) {
	sessions := []store.Session{
		done("s1", "Physics", "2026-09-13T06:00:00Z", 20),
		done("s2", "Physics", "2026-09-14T06:00:00Z", 90),
		done("s3", "Physics", "2026-09-15T06:00:00Z", 40),
	}

	r := Build(sessions, nil, now, 7, bahrain)

	if r.Best.Date != "2026-09-14" || r.Best.Seconds != 90*60 {
		t.Errorf("best = %+v, want the 14th on 90 minutes", r.Best)
	}
}

func TestSessionsOutsideTheWindowAreIgnored(t *testing.T) {
	sessions := []store.Session{
		done("old", "Physics", "2026-08-01T06:00:00Z", 600),
		done("s1", "Physics", "2026-09-15T06:00:00Z", 30),
	}

	r := Build(sessions, nil, now, 3, bahrain)

	if r.Total != 30*60 {
		t.Errorf("total = %d, want only the recent session counted", r.Total)
	}
	if r.SessionCnt != 1 {
		t.Errorf("counted %d sessions, want 1", r.SessionCnt)
	}
}

func TestStreakCountsBackFromToday(t *testing.T) {
	sessions := []store.Session{
		done("s1", "Physics", "2026-09-13T06:00:00Z", 30),
		done("s2", "Physics", "2026-09-14T06:00:00Z", 30),
		done("s3", "Physics", "2026-09-15T06:00:00Z", 30),
	}

	if r := Build(sessions, nil, now, 7, bahrain); r.Streak != 3 {
		t.Errorf("streak = %d, want 3", r.Streak)
	}
}

// The day is not over, so an empty today must not wipe out a real streak.
func TestAnEmptyTodayDoesNotBreakTheStreak(t *testing.T) {
	sessions := []store.Session{
		done("s1", "Physics", "2026-09-13T06:00:00Z", 30),
		done("s2", "Physics", "2026-09-14T06:00:00Z", 30),
	}

	if r := Build(sessions, nil, now, 7, bahrain); r.Streak != 2 {
		t.Errorf("streak = %d, want 2 with today still empty", r.Streak)
	}
}

func TestAGapBreaksTheStreak(t *testing.T) {
	sessions := []store.Session{
		done("s1", "Physics", "2026-09-11T06:00:00Z", 30),
		done("s2", "Physics", "2026-09-12T06:00:00Z", 30),
		done("s4", "Physics", "2026-09-15T06:00:00Z", 30),
	}

	if r := Build(sessions, nil, now, 7, bahrain); r.Streak != 1 {
		t.Errorf("streak = %d, want 1 - the earlier run does not carry across the gap", r.Streak)
	}
}

func TestNoSessionsMeansNoStreak(t *testing.T) {
	if r := Build(nil, nil, now, 7, bahrain); r.Streak != 0 {
		t.Errorf("streak = %d, want 0", r.Streak)
	}
}

func TestGoalsAreCountedInsideTheRange(t *testing.T) {
	goals := []store.Goal{
		goalOn("g1", "2026-09-14", true),
		goalOn("g2", "2026-09-15", false),
		goalOn("g3", "2026-08-01", true),
	}

	r := Build(nil, goals, now, 7, bahrain)

	if r.GoalsSet != 2 {
		t.Errorf("goals set = %d, want 2 inside the window", r.GoalsSet)
	}
	if r.GoalsDone != 1 {
		t.Errorf("goals done = %d, want 1", r.GoalsDone)
	}
}

// A running session has banked seconds plus the time since it resumed, and the
// report has to show that rather than waiting for it to end.
func TestARunningSessionCountsTimeSoFar(t *testing.T) {
	resumed := now.Add(-20 * time.Minute)

	live := store.Session{
		ID: "s1", Subject: "Physics", Status: store.SessionActive,
		StartedAt: now.Add(-time.Hour), AccumulatedSeconds: 600,
		LastResumedAt: &resumed,
	}

	r := Build([]store.Session{live}, nil, now, 7, bahrain)

	if want := int64(600 + 20*60); r.Total != want {
		t.Errorf("total = %d, want %d (banked plus the live stretch)", r.Total, want)
	}
}

func TestWindowReachesPastBothEnds(t *testing.T) {
	from, to := Window(now, 7, bahrain)

	if !from.Before(now.AddDate(0, 0, -7)) {
		t.Errorf("window starts at %s, want it reaching back past the seventh day", from)
	}
	if !to.After(now) {
		t.Errorf("window ends at %s, want it past now", to)
	}
	if !from.Before(to) {
		t.Error("the window is inverted")
	}
}

func TestOneDayIsAValidRange(t *testing.T) {
	r := Build([]store.Session{done("s1", "Physics", "2026-09-15T06:00:00Z", 30)}, nil, now, 1, bahrain)

	if len(r.Days) != 1 {
		t.Fatalf("got %d days, want 1", len(r.Days))
	}
	if r.Days[0].Seconds != 30*60 {
		t.Errorf("the single day = %+v, want 30 minutes", r.Days[0])
	}
}
