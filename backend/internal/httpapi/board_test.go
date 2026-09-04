package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/live"
	"github.com/sathvik458/deskbridge/backend/internal/store"
)

type fakeBoardStore struct {
	marks  []store.Mark
	nextID int64
	err    error
}

func newFakeBoardStore() *fakeBoardStore {
	return &fakeBoardStore{}
}

func (f *fakeBoardStore) AddMark(_ context.Context, mark store.Mark) (store.Mark, error) {
	if f.err != nil {
		return store.Mark{}, f.err
	}

	f.nextID++
	mark.Seq = f.nextID
	mark.CreatedAt = fixedNow
	f.marks = append(f.marks, mark)

	return mark, nil
}

func (f *fakeBoardStore) MarksSince(_ context.Context, since int64, limit int) ([]store.Mark, error) {
	if f.err != nil {
		return nil, f.err
	}

	newer := []store.Mark{}
	for _, mark := range f.marks {
		if mark.Seq > since {
			newer = append(newer, mark)
		}
		if limit > 0 && len(newer) == limit {
			break
		}
	}

	return newer, nil
}

func (f *fakeBoardStore) MarkExists(_ context.Context, id string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}

	for _, mark := range f.marks {
		if mark.ID == id && mark.Kind == store.MarkDraw {
			return true, nil
		}
	}

	return false, nil
}

func (f *fakeBoardStore) TrimBoard(_ context.Context) (int, error) {
	if f.err != nil {
		return 0, f.err
	}

	lastClear := int64(0)
	for _, mark := range f.marks {
		if mark.Kind == store.MarkClear && mark.Seq > lastClear {
			lastClear = mark.Seq
		}
	}

	kept := []store.Mark{}
	for _, mark := range f.marks {
		if mark.Seq >= lastClear {
			kept = append(kept, mark)
		}
	}

	removed := len(f.marks) - len(kept)
	f.marks = kept

	return removed, nil
}

func newBoardServer(t *testing.T, board BoardStore, feed *live.Feed) *Server {
	t.Helper()

	return NewServer(discardTestLogger(), "test", time.Now(), newFakeDeviceStore(),
		&fakeSessionStore{}, newFakeGoalStore(), &fakeMessageStore{}, newFakeFileStore(),
		board, newTestVault(t), feed, "http://localhost:5173")
}

func postJSON(t *testing.T, s *Server, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	return rec
}

func drawStroke(t *testing.T, s *Server, ink string, path string) markResponse {
	t.Helper()

	rec := postJSON(t, s, "/api/board/marks",
		fmt.Sprintf(`{"ink":%q,"thickness":3,"path":%s}`, ink, path))

	if rec.Code != http.StatusCreated {
		t.Fatalf("draw status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body)
	}

	var mark markResponse
	if err := json.NewDecoder(rec.Body).Decode(&mark); err != nil {
		t.Fatalf("decoding the stroke: %v", err)
	}

	return mark
}

func readBoard(t *testing.T, s *Server, since int64) boardPage {
	t.Helper()

	rec := doRequest(t, s, http.MethodGet, fmt.Sprintf("/api/board/marks?since=%d", since))

	if rec.Code != http.StatusOK {
		t.Fatalf("board status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body)
	}

	var page boardPage
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
		t.Fatalf("decoding the board: %v", err)
	}

	return page
}

func TestDrawStoresTheStroke(t *testing.T) {
	board := newFakeBoardStore()
	s := newBoardServer(t, board, live.NewFeed(discardTestLogger()))

	mark := drawStroke(t, s, "#C84B31", "[0.1,0.2,0.3,0.4]")

	if mark.Kind != store.MarkDraw {
		t.Errorf("kind = %q, want %q", mark.Kind, store.MarkDraw)
	}
	if mark.Seq <= 0 {
		t.Errorf("seq = %d, want a positive sequence", mark.Seq)
	}
	if len(mark.Path) != 4 {
		t.Errorf("path has %d numbers, want 4", len(mark.Path))
	}
	if mark.Ink == nil || *mark.Ink != "#C84B31" {
		t.Errorf("ink = %v, want #C84B31", mark.Ink)
	}
	if len(board.marks) != 1 {
		t.Errorf("the store holds %d marks, want 1", len(board.marks))
	}
}

func TestDrawRejectsAColourOffThePalette(t *testing.T) {
	s := newBoardServer(t, newFakeBoardStore(), live.NewFeed(discardTestLogger()))

	rec := postJSON(t, s, "/api/board/marks", `{"ink":"#FF00FF","thickness":3,"path":[0.1,0.2]}`)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDrawRejectsCoordinatesOutsideTheBoard(t *testing.T) {
	s := newBoardServer(t, newFakeBoardStore(), live.NewFeed(discardTestLogger()))

	for _, path := range []string{"[1.5,0.2]", "[0.1,-0.2]", "[0.1,0.2,900,0.4]"} {
		rec := postJSON(t, s, "/api/board/marks",
			fmt.Sprintf(`{"ink":"#1E1E1E","thickness":3,"path":%s}`, path))

		if rec.Code != http.StatusBadRequest {
			t.Errorf("path %s gave status %d, want %d", path, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestDrawRejectsAnOddPath(t *testing.T) {
	s := newBoardServer(t, newFakeBoardStore(), live.NewFeed(discardTestLogger()))

	rec := postJSON(t, s, "/api/board/marks", `{"ink":"#1E1E1E","thickness":3,"path":[0.1,0.2,0.3]}`)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDrawRejectsAnEmptyPath(t *testing.T) {
	s := newBoardServer(t, newFakeBoardStore(), live.NewFeed(discardTestLogger()))

	rec := postJSON(t, s, "/api/board/marks", `{"ink":"#1E1E1E","thickness":3,"path":[]}`)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDrawRejectsAnAbsurdlyLongStroke(t *testing.T) {
	s := newBoardServer(t, newFakeBoardStore(), live.NewFeed(discardTestLogger()))

	points := make([]string, 0, (maxPointsPerStroke+10)*2)
	for i := 0; i < (maxPointsPerStroke+10)*2; i++ {
		points = append(points, "0.5")
	}

	rec := postJSON(t, s, "/api/board/marks",
		fmt.Sprintf(`{"ink":"#1E1E1E","thickness":3,"path":[%s]}`, strings.Join(points, ",")))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDrawRejectsABadThickness(t *testing.T) {
	s := newBoardServer(t, newFakeBoardStore(), live.NewFeed(discardTestLogger()))

	for _, thickness := range []string{"0", "-3", "500"} {
		rec := postJSON(t, s, "/api/board/marks",
			fmt.Sprintf(`{"ink":"#1E1E1E","thickness":%s,"path":[0.1,0.2]}`, thickness))

		if rec.Code != http.StatusBadRequest {
			t.Errorf("thickness %s gave status %d, want %d", thickness, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestBoardOnlyReturnsWhatIsNewerThanTheCursor(t *testing.T) {
	s := newBoardServer(t, newFakeBoardStore(), live.NewFeed(discardTestLogger()))

	drawStroke(t, s, "#1E1E1E", "[0.1,0.1]")
	second := drawStroke(t, s, "#1E1E1E", "[0.2,0.2]")
	third := drawStroke(t, s, "#1E1E1E", "[0.3,0.3]")

	page := readBoard(t, s, second.Seq)

	if len(page.Marks) != 1 {
		t.Fatalf("got %d marks, want only the one after seq %d", len(page.Marks), second.Seq)
	}
	if page.Marks[0].ID != third.ID {
		t.Errorf("got mark %s, want %s", page.Marks[0].ID, third.ID)
	}
	if page.Cursor != third.Seq {
		t.Errorf("cursor = %d, want %d", page.Cursor, third.Seq)
	}
}

// A client that has drifted out of date sends the cursor it still holds, and the
// same endpoint that serves live updates brings it fully back.
func TestBoardFromZeroRebuildsEverything(t *testing.T) {
	s := newBoardServer(t, newFakeBoardStore(), live.NewFeed(discardTestLogger()))

	drawStroke(t, s, "#1E1E1E", "[0.1,0.1]")
	drawStroke(t, s, "#C84B31", "[0.2,0.2]")

	page := readBoard(t, s, 0)

	if len(page.Marks) != 2 {
		t.Fatalf("got %d marks from zero, want the whole board", len(page.Marks))
	}
	if page.Marks[0].Seq >= page.Marks[1].Seq {
		t.Error("marks came back newest first, want replay order")
	}
}

func TestAnEmptyBoardKeepsTheCursorWhereItWas(t *testing.T) {
	s := newBoardServer(t, newFakeBoardStore(), live.NewFeed(discardTestLogger()))

	drawStroke(t, s, "#1E1E1E", "[0.1,0.1]")
	page := readBoard(t, s, 1)

	if len(page.Marks) != 0 {
		t.Fatalf("got %d marks, want none", len(page.Marks))
	}
	// Moving the cursor backwards here would make the client re-draw the board.
	if page.Cursor != 1 {
		t.Errorf("cursor = %d, want it left at 1", page.Cursor)
	}
}

func TestBoardMarksEncodeAsAnArray(t *testing.T) {
	s := newBoardServer(t, newFakeBoardStore(), live.NewFeed(discardTestLogger()))

	rec := doRequest(t, s, http.MethodGet, "/api/board/marks")

	if !strings.Contains(rec.Body.String(), `"marks":[]`) {
		t.Errorf("body = %s, want an empty array not null", rec.Body.String())
	}
}

func TestBoardRejectsANegativeCursor(t *testing.T) {
	s := newBoardServer(t, newFakeBoardStore(), live.NewFeed(discardTestLogger()))

	rec := doRequest(t, s, http.MethodGet, "/api/board/marks?since=-5")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAGarbledCursorIsTreatedAsAFreshLoad(t *testing.T) {
	s := newBoardServer(t, newFakeBoardStore(), live.NewFeed(discardTestLogger()))
	drawStroke(t, s, "#1E1E1E", "[0.1,0.1]")

	rec := doRequest(t, s, http.MethodGet, "/api/board/marks?since=banana")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var page boardPage
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
		t.Fatalf("decoding the board: %v", err)
	}
	if len(page.Marks) != 1 {
		t.Errorf("got %d marks, want the board rebuilt from scratch", len(page.Marks))
	}
}

// This is the design in one test: erasing appends, and the stroke it removes is
// still in the log for anyone replaying from further back.
func TestErasingAppendsAndLeavesTheStrokeInTheLog(t *testing.T) {
	board := newFakeBoardStore()
	s := newBoardServer(t, board, live.NewFeed(discardTestLogger()))

	stroke := drawStroke(t, s, "#1E1E1E", "[0.1,0.1]")

	rec := doRequest(t, s, http.MethodDelete, "/api/board/marks/"+stroke.ID)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body)
	}

	var erase markResponse
	if err := json.NewDecoder(rec.Body).Decode(&erase); err != nil {
		t.Fatalf("decoding the erase: %v", err)
	}

	if erase.Kind != store.MarkErase {
		t.Errorf("kind = %q, want %q", erase.Kind, store.MarkErase)
	}
	if erase.TargetID == nil || *erase.TargetID != stroke.ID {
		t.Errorf("target = %v, want %s", erase.TargetID, stroke.ID)
	}
	if erase.Seq <= stroke.Seq {
		t.Error("the erase must sort after the stroke it removes")
	}

	page := readBoard(t, s, 0)
	if len(page.Marks) != 2 {
		t.Fatalf("board holds %d marks, want the stroke and the erase", len(page.Marks))
	}
	if page.Marks[0].ID != stroke.ID {
		t.Error("the original stroke was removed from the log")
	}
}

func TestErasingSomethingThatIsNotThere(t *testing.T) {
	s := newBoardServer(t, newFakeBoardStore(), live.NewFeed(discardTestLogger()))

	rec := doRequest(t, s, http.MethodDelete, "/api/board/marks/ghost")

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestClearAppendsAndTrimsWhatCameBefore(t *testing.T) {
	board := newFakeBoardStore()
	s := newBoardServer(t, board, live.NewFeed(discardTestLogger()))

	drawStroke(t, s, "#1E1E1E", "[0.1,0.1]")
	drawStroke(t, s, "#C84B31", "[0.2,0.2]")

	rec := postJSON(t, s, "/api/board/clear", "")

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body)
	}

	var clear markResponse
	if err := json.NewDecoder(rec.Body).Decode(&clear); err != nil {
		t.Fatalf("decoding the clear: %v", err)
	}
	if clear.Kind != store.MarkClear {
		t.Errorf("kind = %q, want %q", clear.Kind, store.MarkClear)
	}

	// The clear itself must remain, so a client behind the cursor still learns the
	// board was wiped rather than seeing an empty page and keeping its strokes.
	page := readBoard(t, s, 0)
	if len(page.Marks) != 1 {
		t.Fatalf("board holds %d marks after a clear, want just the clear", len(page.Marks))
	}
	if page.Marks[0].Kind != store.MarkClear {
		t.Errorf("kept a %q, want the clear", page.Marks[0].Kind)
	}
}

func TestClearingKeepsSequenceNumbersClimbing(t *testing.T) {
	s := newBoardServer(t, newFakeBoardStore(), live.NewFeed(discardTestLogger()))

	drawStroke(t, s, "#1E1E1E", "[0.1,0.1]")
	rec := postJSON(t, s, "/api/board/clear", "")

	var clear markResponse
	if err := json.NewDecoder(rec.Body).Decode(&clear); err != nil {
		t.Fatalf("decoding the clear: %v", err)
	}

	after := drawStroke(t, s, "#1E1E1E", "[0.3,0.3]")

	if after.Seq <= clear.Seq {
		t.Errorf("stroke after the clear has seq %d, want it above %d", after.Seq, clear.Seq)
	}
}

func TestDrawAnnouncesOnTheFeed(t *testing.T) {
	feed := live.NewFeed(discardTestLogger())
	events, stop := feed.Watch()
	defer stop()

	s := newBoardServer(t, newFakeBoardStore(), feed)
	drawStroke(t, s, "#1E1E1E", "[0.1,0.1]")

	select {
	case event := <-events:
		if event.Kind != live.BoardMarked {
			t.Errorf("kind = %q, want %q", event.Kind, live.BoardMarked)
		}
		if event.Body["seq"] == nil {
			t.Error("the event carries no sequence for the client to compare against")
		}
	case <-time.After(time.Second):
		t.Fatal("no event arrived after a stroke")
	}
}

func TestBoardFailureIsReportedAsAServerError(t *testing.T) {
	board := newFakeBoardStore()
	board.err = errors.New("disk gave up")
	s := newBoardServer(t, board, live.NewFeed(discardTestLogger()))

	rec := doRequest(t, s, http.MethodGet, "/api/board/marks")

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
