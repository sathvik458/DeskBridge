package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/live"
	"github.com/sathvik458/deskbridge/backend/internal/store"
)

type fakeInsightStore struct {
	sessions []store.Session
	goals    []store.Goal
	err      error
	from, to time.Time
}

func newFakeInsightStore() *fakeInsightStore {
	return &fakeInsightStore{}
}

func (f *fakeInsightStore) SessionsBetween(_ context.Context, _ string, from, to time.Time) ([]store.Session, error) {
	if f.err != nil {
		return nil, f.err
	}

	f.from, f.to = from, to

	out := []store.Session{}
	for _, s := range f.sessions {
		if !s.StartedAt.Before(from) && s.StartedAt.Before(to) {
			out = append(out, s)
		}
	}

	return out, nil
}

func (f *fakeInsightStore) GoalsBetween(_ context.Context, _, from, to string) ([]store.Goal, error) {
	if f.err != nil {
		return nil, f.err
	}

	out := []store.Goal{}
	for _, g := range f.goals {
		if g.GoalDate >= from && g.GoalDate <= to {
			out = append(out, g)
		}
	}

	return out, nil
}

func newInsightServer(t *testing.T, data *fakeInsightStore, at time.Time) *Server {
	t.Helper()

	log := discardTestLogger()
	s := NewServer(log, "test", at, newFakeDeviceStore(), &fakeSessionStore{}, newFakeGoalStore(),
		&fakeMessageStore{}, newFakeFileStore(), newFakeBoardStore(), newFakeUserStore(),
		newFakeLoginStore(), data, newTestVault(t), live.NewFeed(log), "http://localhost:5173", false)
	s.now = func() time.Time { return at }

	return s
}

func readInsights(t *testing.T, s *Server, query string) insightsOut {
	t.Helper()

	rec := doRequest(t, s, http.MethodGet, "/api/insights"+query)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body)
	}

	var out insightsOut
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decoding: %v", err)
	}

	return out
}

func sessionAt(id, subject, startedAt string, mins int) store.Session {
	at, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		panic(err)
	}

	return store.Session{
		ID: id, Subject: subject, Status: store.SessionCompleted,
		StartedAt: at, AccumulatedSeconds: int64(mins) * 60,
	}
}

var reportNow = time.Date(2026, 9, 15, 11, 0, 0, 0, time.UTC)

func TestInsightsDefaultToTwoWeeks(t *testing.T) {
	s := newInsightServer(t, newFakeInsightStore(), reportNow)

	out := readInsights(t, s, "")

	if len(out.Days) != defaultSpan {
		t.Errorf("got %d days, want %d", len(out.Days), defaultSpan)
	}
	if out.Zone != "UTC" {
		t.Errorf("zone = %q, want UTC when none is asked for", out.Zone)
	}
}

func TestInsightsRespectTheDayCount(t *testing.T) {
	s := newInsightServer(t, newFakeInsightStore(), reportNow)

	if out := readInsights(t, s, "?days=5"); len(out.Days) != 5 {
		t.Errorf("got %d days, want 5", len(out.Days))
	}
}

func TestAnAbsurdDayCountIsCapped(t *testing.T) {
	s := newInsightServer(t, newFakeInsightStore(), reportNow)

	if out := readInsights(t, s, "?days=9999"); len(out.Days) != maxSpan {
		t.Errorf("got %d days, want it capped at %d", len(out.Days), maxSpan)
	}
}

func TestANonsenseDayCountFallsBackToTheDefault(t *testing.T) {
	s := newInsightServer(t, newFakeInsightStore(), reportNow)

	for _, q := range []string{"?days=banana", "?days=0", "?days=-5"} {
		if out := readInsights(t, s, q); len(out.Days) != defaultSpan {
			t.Errorf("%s gave %d days, want the default %d", q, len(out.Days), defaultSpan)
		}
	}
}

func TestTheZoneChangesWhichDayASessionLandsOn(t *testing.T) {
	data := newFakeInsightStore()
	data.sessions = []store.Session{sessionAt("s1", "Physics", "2026-09-11T22:15:00Z", 30)}

	s := newInsightServer(t, data, reportNow)

	pick := func(out insightsOut, date string) dayOut {
		for _, d := range out.Days {
			if d.Date == date {
				return d
			}
		}
		return dayOut{}
	}

	inUTC := readInsights(t, s, "?days=10")
	if pick(inUTC, "2026-09-11").Sessions != 1 {
		t.Error("in UTC the session should sit on the 11th")
	}

	inBahrain := readInsights(t, s, "?days=10&zone=Asia/Bahrain")
	if pick(inBahrain, "2026-09-12").Sessions != 1 {
		t.Error("in Bahrain the same session should sit on the 12th")
	}
}

func TestAnUnknownZoneIsRejected(t *testing.T) {
	s := newInsightServer(t, newFakeInsightStore(), reportNow)

	rec := doRequest(t, s, http.MethodGet, "/api/insights?zone=Mars/Olympus_Mons")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// The window asked of the database reaches past both ends, because a local day
// can start before the matching UTC day does.
func TestTheQueryWindowIsWiderThanTheReport(t *testing.T) {
	data := newFakeInsightStore()
	s := newInsightServer(t, data, reportNow)

	readInsights(t, s, "?days=7&zone=Asia/Bahrain")

	if !data.from.Before(reportNow.AddDate(0, 0, -7)) {
		t.Errorf("window from %s, want it reaching back past seven days", data.from)
	}
	if !data.to.After(reportNow) {
		t.Errorf("window to %s, want it past now", data.to)
	}
}

func TestTotalsAndSubjectsComeThrough(t *testing.T) {
	data := newFakeInsightStore()
	data.sessions = []store.Session{
		sessionAt("s1", "Physics", "2026-09-15T06:00:00Z", 30),
		sessionAt("s2", "Chemistry", "2026-09-15T08:00:00Z", 90),
		sessionAt("s3", "Physics", "2026-09-14T08:00:00Z", 15),
	}

	out := readInsights(t, newInsightServer(t, data, reportNow), "?days=7")

	if out.Total != 135*60 {
		t.Errorf("total = %d, want %d", out.Total, 135*60)
	}
	if out.Sessions != 3 {
		t.Errorf("session count = %d, want 3", out.Sessions)
	}
	if len(out.Subjects) != 2 || out.Subjects[0].Name != "Chemistry" {
		t.Errorf("subjects = %+v, want Chemistry first", out.Subjects)
	}
	if out.Best.Date != "2026-09-15" {
		t.Errorf("best day = %q, want the 15th", out.Best.Date)
	}
}

func TestGoalsAreReported(t *testing.T) {
	at := reportNow

	data := newFakeInsightStore()
	data.goals = []store.Goal{
		{ID: "g1", GoalDate: "2026-09-14", Subject: "Physics", CompletedAt: &at},
		{ID: "g2", GoalDate: "2026-09-15", Subject: "Physics"},
	}

	out := readInsights(t, newInsightServer(t, data, reportNow), "?days=7")

	if out.GoalsSet != 2 || out.GoalsDone != 1 {
		t.Errorf("goals set/done = %d/%d, want 2/1", out.GoalsSet, out.GoalsDone)
	}
}

func TestEmptyListsEncodeAsArrays(t *testing.T) {
	s := newInsightServer(t, newFakeInsightStore(), reportNow)

	rec := doRequest(t, s, http.MethodGet, "/api/insights")
	body := rec.Body.String()

	if !strings.Contains(body, `"subjects":[]`) {
		t.Errorf("subjects encoded as null, want []: %s", body)
	}
}

func TestInsightsNeedSigningIn(t *testing.T) {
	s := newInsightServer(t, newFakeInsightStore(), reportNow)

	rec := fire(t, s, httptest.NewRequest(http.MethodGet, "/api/insights", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAStoreFailureBecomes500(t *testing.T) {
	data := newFakeInsightStore()
	data.err = errors.New("database is on fire")

	s := newInsightServer(t, data, reportNow)

	rec := doRequest(t, s, http.MethodGet, "/api/insights")

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
