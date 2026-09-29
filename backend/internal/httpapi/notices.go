package httpapi

import (
	"net/http"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/notify"
	"github.com/sathvik458/deskbridge/backend/internal/store"
)

const noticePage = 50

type noticeResponse struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Line   string `json:"line"`
	Times  int    `json:"times"`
	MadeAt string `json:"made_at"`
	Seen   bool   `json:"seen"`
}

type noticeList struct {
	Notices []noticeResponse `json:"notices"`
	Unseen  int              `json:"unseen"`
}

type noticeSwitch struct {
	Kind string `json:"kind"`
	On   bool   `json:"on"`
}

type switchRequest struct {
	On bool `json:"on"`
}

// tell does both halves of "something happened": the live feed for whoever is
// watching right now, and a notice row for whoever is not.
//
// A notice failing is never allowed to fail the request that caused it. The event
// already happened; losing the reminder about it is the smaller problem.
func (s *Server) tell(r *http.Request, kind string, body map[string]any) {
	s.feed.Announce(kind, body)

	note, worth := notify.Compose(kind, body)
	if !worth {
		return
	}

	ctx := r.Context()
	me := userOf(r)

	people, err := s.users.Users(ctx)
	if err != nil {
		s.log.Error("working out who to tell", "kind", kind, "error", err)
		return
	}

	// One question per event rather than one per person: who has switched this off.
	quiet, err := s.notices.MutedBy(ctx, kind)
	if err != nil {
		s.log.Error("reading who muted this", "kind", kind, "error", err)
		quiet = nil
	}

	for _, p := range people {
		if p.ID == me.ID || quiet[p.ID] {
			continue
		}

		_, err := s.notices.Note(ctx, store.Notice{
			ID:     newID(),
			UserID: p.ID,
			Kind:   kind,
			Tag:    note.Tag,
			Body:   note.Line,
		}, notify.Within)
		if err != nil {
			s.log.Error("leaving a notice", "user_id", p.ID, "kind", kind, "error", err)
		}
	}
}

func (s *Server) handleListNotices(w http.ResponseWriter, r *http.Request) {
	me := userOf(r)
	ctx := r.Context()

	rows, err := s.notices.Notices(ctx, me.ID, noticePage)
	if err != nil {
		s.log.Error("listing notices", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not read your notices")
		return
	}

	unseen, err := s.notices.UnseenNotices(ctx, me.ID)
	if err != nil {
		s.log.Error("counting unseen notices", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not read your notices")
		return
	}

	out := make([]noticeResponse, 0, len(rows))

	for _, n := range rows {
		out = append(out, noticeResponse{
			ID:     n.ID,
			Kind:   n.Kind,
			Line:   notify.Count(n.Body, n.Times),
			Times:  n.Times,
			MadeAt: n.MadeAt.Format(time.RFC3339),
			Seen:   n.SeenAt != nil,
		})
	}

	s.writeJSON(w, http.StatusOK, noticeList{Notices: out, Unseen: unseen})
}

func (s *Server) handleMarkNoticesSeen(w http.ResponseWriter, r *http.Request) {
	marked, err := s.notices.MarkNoticesSeen(r.Context(), userOf(r).ID)
	if err != nil {
		s.log.Error("marking notices seen", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not clear your notices")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]int{"marked": marked})
}

func (s *Server) handleNoticeSettings(w http.ResponseWriter, r *http.Request) {
	muted, err := s.notices.Mutes(r.Context(), userOf(r).ID)
	if err != nil {
		s.log.Error("reading notice settings", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not read your settings")
		return
	}

	kinds := notify.Worth()
	out := make([]noticeSwitch, 0, len(kinds))

	// Absence means on, so a kind added later is on for everybody without a backfill.
	for _, kind := range kinds {
		out = append(out, noticeSwitch{Kind: kind, On: !muted[kind]})
	}

	s.writeJSON(w, http.StatusOK, map[string]any{"switches": out})
}

func (s *Server) handleSetNoticeSwitch(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")

	if !notify.Mutable(kind) {
		s.writeError(w, http.StatusBadRequest, "there is no switch for that")
		return
	}

	var req switchRequest

	if err := decodeBody(w, r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.notices.SetMute(r.Context(), userOf(r).ID, kind, !req.On); err != nil {
		s.log.Error("saving a notice switch", "kind", kind, "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not save that")
		return
	}

	s.writeJSON(w, http.StatusOK, noticeSwitch{Kind: kind, On: req.On})
}
