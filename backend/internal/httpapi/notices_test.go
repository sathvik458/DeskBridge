package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/live"
	"github.com/sathvik458/deskbridge/backend/internal/notify"
	"github.com/sathvik458/deskbridge/backend/internal/store"
)

var errNoticeStoreDown = errors.New("the notice table is unavailable")

type fakeNoticeStore struct {
	rows  []store.Notice
	muted map[string]map[string]bool
	err   error
}

func newFakeNoticeStore() *fakeNoticeStore {
	return &fakeNoticeStore{muted: map[string]map[string]bool{}}
}

func (f *fakeNoticeStore) MutedBy(_ context.Context, kind string) (map[string]bool, error) {
	if f.err != nil {
		return nil, f.err
	}

	out := map[string]bool{}
	for who, kinds := range f.muted {
		if kinds[kind] {
			out[who] = true
		}
	}

	return out, nil
}

func (f *fakeNoticeStore) Mutes(_ context.Context, userID string) (map[string]bool, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.muted[userID], nil
}

func (f *fakeNoticeStore) SetMute(_ context.Context, userID, kind string, muted bool) error {
	if f.err != nil {
		return f.err
	}

	if f.muted[userID] == nil {
		f.muted[userID] = map[string]bool{}
	}

	if muted {
		f.muted[userID][kind] = true
	} else {
		delete(f.muted[userID], kind)
	}

	return nil
}

func (f *fakeNoticeStore) Note(_ context.Context, n store.Notice, _ time.Duration) (store.Notice, error) {
	if f.err != nil {
		return store.Notice{}, f.err
	}

	n.Times = 1
	n.MadeAt = time.Now().UTC()
	f.rows = append(f.rows, n)

	return n, nil
}

func (f *fakeNoticeStore) Notices(_ context.Context, userID string, limit int) ([]store.Notice, error) {
	if f.err != nil {
		return nil, f.err
	}

	out := []store.Notice{}
	for _, n := range f.rows {
		if n.UserID == userID {
			out = append(out, n)
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].MadeAt.After(out[j].MadeAt) })

	if len(out) > limit {
		out = out[:limit]
	}

	return out, nil
}

func (f *fakeNoticeStore) UnseenNotices(_ context.Context, userID string) (int, error) {
	if f.err != nil {
		return 0, f.err
	}

	n := 0
	for _, row := range f.rows {
		if row.UserID == userID && row.SeenAt == nil {
			n++
		}
	}

	return n, nil
}

func (f *fakeNoticeStore) MarkNoticesSeen(_ context.Context, userID string) (int, error) {
	if f.err != nil {
		return 0, f.err
	}

	now := time.Now().UTC()
	marked := 0

	for i := range f.rows {
		if f.rows[i].UserID == userID && f.rows[i].SeenAt == nil {
			f.rows[i].SeenAt = &now
			marked++
		}
	}

	return marked, nil
}

func (f *fakeNoticeStore) forUser(id string) []store.Notice {
	out := []store.Notice{}
	for _, n := range f.rows {
		if n.UserID == id {
			out = append(out, n)
		}
	}
	return out
}

func newNoticeServer(t *testing.T, notices NoticeStore) *Server {
	t.Helper()

	log := discardTestLogger()

	return NewServer(log, "test", time.Now(), newFakeDeviceStore(), &fakeSessionStore{},
		newFakeGoalStore(), &fakeMessageStore{}, newFakeFileStore(), newFakeBoardStore(),
		newFakeUserStore(), newFakeLoginStore(), newFakeInsightStore(), newFakeFocusStore(),
		notices, newTestVault(t), live.NewFeed(log), "http://localhost:5173", false)
}

// The student raises the help request, so the supporter is the one who should
// find something waiting. Nobody should be told about their own doing.
func TestHelpLeavesANoticeForTheOtherPerson(t *testing.T) {
	kept := newFakeNoticeStore()
	s := newNoticeServer(t, kept)

	rec := postJSON(t, s, "/api/messages", `{"kind":"help_request","body":"stuck on question 4"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body)
	}

	if left := kept.forUser(helperID); len(left) != 1 {
		t.Fatalf("supporter has %d notices, want 1", len(left))
	} else if !strings.Contains(left[0].Body, "stuck on question 4") {
		t.Errorf("notice reads %q, expected the question in it", left[0].Body)
	}

	if mine := kept.forUser(studentID); len(mine) != 0 {
		t.Errorf("the student was told about their own help request: %+v", mine)
	}
}

// A pen stroke is worth a live nudge and nothing more.
func TestQuietEventsLeaveNothingBehind(t *testing.T) {
	kept := newFakeNoticeStore()
	s := newNoticeServer(t, kept)

	rec := postJSON(t, s, "/api/board/marks", `{"ink":"#1E1E1E","thickness":2,"path":[0.1,0.1,0.2,0.2]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body)
	}

	if len(kept.rows) != 0 {
		t.Errorf("a whiteboard stroke left %d notices behind, want none", len(kept.rows))
	}
}

func TestListingNoticesCountsTheUnseen(t *testing.T) {
	kept := newFakeNoticeStore()
	s := newNoticeServer(t, kept)

	postJSON(t, s, "/api/messages", `{"kind":"help_request","body":"stuck"}`)

	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, signedInAs(httptest.NewRequest(http.MethodGet, "/api/notices", nil), helperToken))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body)
	}

	var body noticeList
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}

	if body.Unseen != 1 || len(body.Notices) != 1 {
		t.Errorf("got %d notices and %d unseen, want 1 and 1", len(body.Notices), body.Unseen)
	}
}

func TestMarkingSeenEmptiesTheCount(t *testing.T) {
	kept := newFakeNoticeStore()
	s := newNoticeServer(t, kept)

	postJSON(t, s, "/api/messages", `{"kind":"help_request","body":"stuck"}`)

	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, signedInAs(httptest.NewRequest(http.MethodPost, "/api/notices/seen", nil), helperToken))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body)
	}

	left, _ := kept.UnseenNotices(context.Background(), helperID)
	if left != 0 {
		t.Errorf("%d notices still unseen, want 0", left)
	}
}

// The event already happened; a broken notice table must not undo it.
func TestARequestStillSucceedsWhenTheNoticeFails(t *testing.T) {
	kept := newFakeNoticeStore()
	kept.err = errNoticeStoreDown
	s := newNoticeServer(t, kept)

	rec := postJSON(t, s, "/api/messages", `{"kind":"help_request","body":"stuck"}`)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want the message to go through anyway", rec.Code)
	}
}

func TestAMutedKindLeavesNoNotice(t *testing.T) {
	kept := newFakeNoticeStore()
	if err := kept.SetMute(context.Background(), helperID, "help.raised", true); err != nil {
		t.Fatalf("setting up the mute: %v", err)
	}

	s := newNoticeServer(t, kept)

	rec := postJSON(t, s, "/api/messages", `{"kind":"help_request","body":"stuck"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body)
	}

	if left := kept.forUser(helperID); len(left) != 0 {
		t.Errorf("a muted kind still left %d notices", len(left))
	}
}

// Muting one thing must not go quiet on everything else.
func TestMutingOneKindLeavesTheRestAlone(t *testing.T) {
	kept := newFakeNoticeStore()
	kept.SetMute(context.Background(), helperID, "file.added", true)

	s := newNoticeServer(t, kept)

	postJSON(t, s, "/api/messages", `{"kind":"help_request","body":"stuck"}`)

	if left := kept.forUser(helperID); len(left) != 1 {
		t.Errorf("supporter has %d notices, want the unmuted one to still arrive", len(left))
	}
}

func TestSettingsStartAllOn(t *testing.T) {
	s := newNoticeServer(t, newFakeNoticeStore())

	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, signedIn(httptest.NewRequest(http.MethodGet, "/api/notices/settings", nil)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body)
	}

	var body struct {
		Switches []noticeSwitch `json:"switches"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}

	if len(body.Switches) != len(notify.Worth()) {
		t.Fatalf("got %d switches, want one per mutable kind (%d)", len(body.Switches), len(notify.Worth()))
	}

	for _, sw := range body.Switches {
		if !sw.On {
			t.Errorf("%s starts switched off, want everything on by default", sw.Kind)
		}
	}
}

func TestFlippingASwitchSticks(t *testing.T) {
	kept := newFakeNoticeStore()
	s := newNoticeServer(t, kept)

	rec := putJSON(t, s, "/api/notices/settings/file.added", `{"on":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body)
	}

	muted, _ := kept.Mutes(context.Background(), studentID)
	if !muted["file.added"] {
		t.Errorf("the switch did not stick, mutes = %v", muted)
	}
}

func TestThereIsNoSwitchForSomethingThatNeverNotifies(t *testing.T) {
	s := newNoticeServer(t, newFakeNoticeStore())

	rec := putJSON(t, s, "/api/notices/settings/board.marked", `{"on":false}`)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d for a kind that never notifies", rec.Code, http.StatusBadRequest)
	}
}

func putJSON(t *testing.T, s *Server, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := signedIn(httptest.NewRequest(http.MethodPut, path, strings.NewReader(body)))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	return rec
}
