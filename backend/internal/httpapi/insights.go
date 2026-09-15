package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/insight"
)

const (
	defaultSpan = 14
	maxSpan     = 120
)

type dayOut struct {
	Date     string `json:"date"`
	Seconds  int64  `json:"seconds"`
	Sessions int    `json:"sessions"`
}

type subjectOut struct {
	Name     string `json:"name"`
	Seconds  int64  `json:"seconds"`
	Sessions int    `json:"sessions"`
}

type insightsOut struct {
	Days      []dayOut     `json:"days"`
	Subjects  []subjectOut `json:"subjects"`
	Total     int64        `json:"total_seconds"`
	Best      dayOut       `json:"best_day"`
	Streak    int          `json:"streak"`
	Sessions  int          `json:"session_count"`
	GoalsSet  int          `json:"goals_set"`
	GoalsDone int          `json:"goals_done"`
	Zone      string       `json:"zone"`
}

func (s *Server) handleInsights(w http.ResponseWriter, r *http.Request) {
	span, err := strconv.Atoi(r.URL.Query().Get("days"))
	if err != nil || span < 1 {
		span = defaultSpan
	}
	if span > maxSpan {
		span = maxSpan
	}

	zone, err := readZone(r.URL.Query().Get("zone"))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "unknown time zone")
		return
	}

	who, err := s.studying(r)
	if err != nil {
		s.log.Error("finding whose study data this is", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not work out whose desk this is")
		return
	}

	now := s.now()
	from, to := insight.Window(now, span, zone)

	sessions, err := s.insights.SessionsBetween(r.Context(), who, from, to)
	if err != nil {
		s.log.Error("reading sessions for insights", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not read the study history")
		return
	}

	goals, err := s.insights.GoalsBetween(r.Context(), who,
		from.In(zone).Format(insight.DayLayout), to.In(zone).Format(insight.DayLayout))
	if err != nil {
		s.log.Error("reading goals for insights", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not read the study history")
		return
	}

	s.writeJSON(w, http.StatusOK, dress(insight.Build(sessions, goals, now, span, zone)))
}

// The browser sends its own zone name. An unknown one is a client bug worth a 400
// rather than quietly reporting the wrong days in UTC.
func readZone(name string) (*time.Location, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return time.UTC, nil
	}

	return time.LoadLocation(name)
}

func dress(r insight.Report) insightsOut {
	out := insightsOut{
		Days:      make([]dayOut, 0, len(r.Days)),
		Subjects:  make([]subjectOut, 0, len(r.Subjects)),
		Total:     r.Total,
		Best:      dayOut{Date: r.Best.Date, Seconds: r.Best.Seconds, Sessions: r.Best.Sessions},
		Streak:    r.Streak,
		Sessions:  r.SessionCnt,
		GoalsSet:  r.GoalsSet,
		GoalsDone: r.GoalsDone,
		Zone:      r.Zone,
	}

	for _, d := range r.Days {
		out.Days = append(out.Days, dayOut{Date: d.Date, Seconds: d.Seconds, Sessions: d.Sessions})
	}
	for _, sub := range r.Subjects {
		out.Subjects = append(out.Subjects, subjectOut{Name: sub.Name, Seconds: sub.Seconds, Sessions: sub.Sessions})
	}

	return out
}
