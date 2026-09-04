package store

import (
	"context"
	"testing"
	"time"
)

func inkAt(colour string) *string { return &colour }

func widthOf(w float64) *float64 { return &w }

func drawOn(t *testing.T, s *Store, id string, path ...float64) Mark {
	t.Helper()

	mark, err := s.AddMark(context.Background(), Mark{
		ID: id, AuthorID: testUserID, Kind: MarkDraw,
		Ink: inkAt("#1E1E1E"), Thickness: widthOf(3), Path: path,
	})
	if err != nil {
		t.Fatalf("AddMark(%s) returned: %v", id, err)
	}

	return mark
}

func TestAddMarkAssignsAClimbingSequence(t *testing.T) {
	s := newTestStore(t)

	first := drawOn(t, s, "m1", 0.1, 0.1, 0.2, 0.2)
	second := drawOn(t, s, "m2", 0.3, 0.3, 0.4, 0.4)

	if first.Seq <= 0 {
		t.Errorf("first seq = %d, want a positive number from the database", first.Seq)
	}
	if second.Seq <= first.Seq {
		t.Errorf("second seq = %d, want it above the first at %d", second.Seq, first.Seq)
	}
	if !first.CreatedAt.Equal(testClock) {
		t.Errorf("CreatedAt = %s, want %s", first.CreatedAt, testClock)
	}
}

func TestMarksSinceOnlyReturnsWhatIsNew(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	drawOn(t, s, "m1", 0.1, 0.1)
	second := drawOn(t, s, "m2", 0.2, 0.2)
	drawOn(t, s, "m3", 0.3, 0.3)

	marks, err := s.MarksSince(ctx, second.Seq, 0)
	if err != nil {
		t.Fatalf("MarksSince() returned: %v", err)
	}

	if len(marks) != 1 {
		t.Fatalf("got %d marks, want only the one after seq %d", len(marks), second.Seq)
	}
	if marks[0].ID != "m3" {
		t.Errorf("got %q, want m3", marks[0].ID)
	}
}

// A client that has never seen the board asks from zero and must get everything,
// so the catch-up path and the first-load path are the same code.
func TestMarksSinceZeroReturnsTheWholeBoard(t *testing.T) {
	s := newTestStore(t)

	drawOn(t, s, "m1", 0.1, 0.1)
	drawOn(t, s, "m2", 0.2, 0.2)

	marks, err := s.MarksSince(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("MarksSince() returned: %v", err)
	}

	if len(marks) != 2 {
		t.Fatalf("got %d marks, want 2", len(marks))
	}
}

func TestMarksSinceIsOldestFirst(t *testing.T) {
	s := newTestStore(t)

	drawOn(t, s, "m1", 0.1, 0.1)
	s.setClock(testClock.Add(time.Second))
	drawOn(t, s, "m2", 0.2, 0.2)

	marks, err := s.MarksSince(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("MarksSince() returned: %v", err)
	}

	// Replay order is the whole point: strokes drawn later sit on top.
	if marks[0].ID != "m1" || marks[1].ID != "m2" {
		t.Errorf("order = %s then %s, want m1 then m2", marks[0].ID, marks[1].ID)
	}
}

func TestMarksSinceReturnsEmptySliceNotNil(t *testing.T) {
	s := newTestStore(t)

	marks, err := s.MarksSince(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("MarksSince() returned: %v", err)
	}
	if marks == nil {
		t.Error("MarksSince() returned nil, want an empty slice")
	}
}

func TestMarksSinceRespectsTheLimit(t *testing.T) {
	s := newTestStore(t)

	for _, id := range []string{"m1", "m2", "m3"} {
		drawOn(t, s, id, 0.1, 0.1)
	}

	marks, err := s.MarksSince(context.Background(), 0, 2)
	if err != nil {
		t.Fatalf("MarksSince() returned: %v", err)
	}

	if len(marks) != 2 {
		t.Fatalf("got %d marks, want 2", len(marks))
	}
	// A capped page must start at the oldest, or a client would skip marks forever.
	if marks[0].ID != "m1" {
		t.Errorf("page starts at %q, want m1", marks[0].ID)
	}
}

func TestPathSurvivesTheRoundTrip(t *testing.T) {
	s := newTestStore(t)
	path := []float64{0.125, 0.25, 0.5, 0.75, 0.9375, 0.0625}

	drawOn(t, s, "m1", path...)

	marks, err := s.MarksSince(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("MarksSince() returned: %v", err)
	}

	got := marks[0].Path
	if len(got) != len(path) {
		t.Fatalf("got %d numbers back, want %d", len(got), len(path))
	}
	for i := range path {
		if got[i] != path[i] {
			t.Errorf("point %d = %v, want %v", i, got[i], path[i])
		}
	}
}

func TestDrawKeepsItsInkAndThickness(t *testing.T) {
	s := newTestStore(t)

	_, err := s.AddMark(context.Background(), Mark{
		ID: "m1", AuthorID: testUserID, Kind: MarkDraw,
		Ink: inkAt("#C84B31"), Thickness: widthOf(6.5), Path: []float64{0.1, 0.2},
	})
	if err != nil {
		t.Fatalf("AddMark() returned: %v", err)
	}

	marks, err := s.MarksSince(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("MarksSince() returned: %v", err)
	}

	if marks[0].Ink == nil || *marks[0].Ink != "#C84B31" {
		t.Errorf("Ink = %v, want #C84B31", marks[0].Ink)
	}
	if marks[0].Thickness == nil || *marks[0].Thickness != 6.5 {
		t.Errorf("Thickness = %v, want 6.5", marks[0].Thickness)
	}
}

func TestEraseIsStoredAsItsOwnMark(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	drawn := drawOn(t, s, "m1", 0.1, 0.1)

	target := "m1"
	erase, err := s.AddMark(ctx, Mark{
		ID: "m2", AuthorID: testUserID, Kind: MarkErase, TargetID: &target,
	})
	if err != nil {
		t.Fatalf("AddMark(erase) returned: %v", err)
	}

	if erase.Seq <= drawn.Seq {
		t.Error("the erase should sit after the stroke it removes")
	}

	marks, err := s.MarksSince(ctx, 0, 0)
	if err != nil {
		t.Fatalf("MarksSince() returned: %v", err)
	}

	// The original stroke has to survive, or a client replaying the log from an
	// older cursor would draw it and never learn it was rubbed out.
	if len(marks) != 2 {
		t.Fatalf("got %d marks, want the draw and the erase to both remain", len(marks))
	}
	if marks[1].TargetID == nil || *marks[1].TargetID != "m1" {
		t.Errorf("erase target = %v, want m1", marks[1].TargetID)
	}
}

func TestClearIsAlsoJustAMark(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	drawOn(t, s, "m1", 0.1, 0.1)

	if _, err := s.AddMark(ctx, Mark{ID: "m2", AuthorID: testUserID, Kind: MarkClear}); err != nil {
		t.Fatalf("AddMark(clear) returned: %v", err)
	}

	marks, err := s.MarksSince(ctx, 0, 0)
	if err != nil {
		t.Fatalf("MarksSince() returned: %v", err)
	}

	if len(marks) != 2 {
		t.Fatalf("got %d marks, want 2", len(marks))
	}
	if marks[1].Kind != MarkClear {
		t.Errorf("kind = %q, want %q", marks[1].Kind, MarkClear)
	}
}

func TestLatestMarkSeq(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	empty, err := s.LatestMarkSeq(ctx)
	if err != nil {
		t.Fatalf("LatestMarkSeq() returned: %v", err)
	}
	if empty != 0 {
		t.Errorf("an empty board reports seq %d, want 0", empty)
	}

	drawOn(t, s, "m1", 0.1, 0.1)
	newest := drawOn(t, s, "m2", 0.2, 0.2)

	latest, err := s.LatestMarkSeq(ctx)
	if err != nil {
		t.Fatalf("LatestMarkSeq() returned: %v", err)
	}
	if latest != newest.Seq {
		t.Errorf("LatestMarkSeq() = %d, want %d", latest, newest.Seq)
	}
}

func TestMarkExistsOnlyFindsStrokes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	drawOn(t, s, "m1", 0.1, 0.1)

	if _, err := s.AddMark(ctx, Mark{ID: "m2", AuthorID: testUserID, Kind: MarkClear}); err != nil {
		t.Fatalf("AddMark(clear) returned: %v", err)
	}

	drawn, err := s.MarkExists(ctx, "m1")
	if err != nil {
		t.Fatalf("MarkExists() returned: %v", err)
	}
	if !drawn {
		t.Error("MarkExists(m1) = false, want true")
	}

	// Erasing a clear is meaningless, so only strokes count as erasable.
	cleared, err := s.MarkExists(ctx, "m2")
	if err != nil {
		t.Fatalf("MarkExists() returned: %v", err)
	}
	if cleared {
		t.Error("MarkExists(m2) = true for a clear, want false")
	}

	ghost, err := s.MarkExists(ctx, "nope")
	if err != nil {
		t.Fatalf("MarkExists() returned: %v", err)
	}
	if ghost {
		t.Error("MarkExists(nope) = true, want false")
	}
}

func TestTrimBoardDropsEverythingBeforeTheLastClear(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	drawOn(t, s, "m1", 0.1, 0.1)
	drawOn(t, s, "m2", 0.2, 0.2)
	if _, err := s.AddMark(ctx, Mark{ID: "m3", AuthorID: testUserID, Kind: MarkClear}); err != nil {
		t.Fatalf("AddMark(clear) returned: %v", err)
	}
	drawOn(t, s, "m4", 0.4, 0.4)

	removed, err := s.TrimBoard(ctx)
	if err != nil {
		t.Fatalf("TrimBoard() returned: %v", err)
	}
	if removed != 2 {
		t.Errorf("removed %d marks, want 2", removed)
	}

	marks, err := s.MarksSince(ctx, 0, 0)
	if err != nil {
		t.Fatalf("MarksSince() returned: %v", err)
	}
	if len(marks) != 2 {
		t.Fatalf("got %d marks, want the clear and what came after it", len(marks))
	}
	if marks[0].Kind != MarkClear || marks[1].ID != "m4" {
		t.Errorf("kept %s then %s, want the clear then m4", marks[0].ID, marks[1].ID)
	}
}

// Trimming must not renumber anything, because a client is holding one of those
// sequence numbers as its cursor.
func TestTrimBoardLeavesSequenceNumbersAlone(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	drawOn(t, s, "m1", 0.1, 0.1)
	if _, err := s.AddMark(ctx, Mark{ID: "m2", AuthorID: testUserID, Kind: MarkClear}); err != nil {
		t.Fatalf("AddMark(clear) returned: %v", err)
	}
	kept := drawOn(t, s, "m3", 0.3, 0.3)

	if _, err := s.TrimBoard(ctx); err != nil {
		t.Fatalf("TrimBoard() returned: %v", err)
	}

	after, err := s.LatestMarkSeq(ctx)
	if err != nil {
		t.Fatalf("LatestMarkSeq() returned: %v", err)
	}
	if after != kept.Seq {
		t.Errorf("latest seq = %d after trimming, want it unchanged at %d", after, kept.Seq)
	}
}

func TestTrimBoardOnABoardWithNoClearKeepsEverything(t *testing.T) {
	s := newTestStore(t)

	drawOn(t, s, "m1", 0.1, 0.1)
	drawOn(t, s, "m2", 0.2, 0.2)

	removed, err := s.TrimBoard(context.Background())
	if err != nil {
		t.Fatalf("TrimBoard() returned: %v", err)
	}
	if removed != 0 {
		t.Errorf("removed %d marks from a board that was never cleared, want 0", removed)
	}
}

func TestAddMarkRejectsAnOddPath(t *testing.T) {
	s := newTestStore(t)

	_, err := s.AddMark(context.Background(), Mark{
		ID: "m1", AuthorID: testUserID, Kind: MarkDraw,
		Ink: inkAt("#1E1E1E"), Thickness: widthOf(3), Path: []float64{0.1, 0.2, 0.3},
	})

	if err == nil {
		t.Error("AddMark() accepted a path with an odd number of coordinates")
	}
}

func TestAddMarkRejectsAnEmptyPath(t *testing.T) {
	s := newTestStore(t)

	_, err := s.AddMark(context.Background(), Mark{
		ID: "m1", AuthorID: testUserID, Kind: MarkDraw,
		Ink: inkAt("#1E1E1E"), Thickness: widthOf(3), Path: []float64{},
	})

	if err == nil {
		t.Error("AddMark() accepted a stroke with no points")
	}
}

func TestAddMarkRejectsAnUnknownKind(t *testing.T) {
	s := newTestStore(t)

	_, err := s.AddMark(context.Background(), Mark{
		ID: "m1", AuthorID: testUserID, Kind: "scribble",
	})

	if err == nil {
		t.Error("AddMark() accepted an unknown kind, want the check constraint to reject it")
	}
}

func TestTwoMarksCannotShareAnID(t *testing.T) {
	s := newTestStore(t)
	drawOn(t, s, "m1", 0.1, 0.1)

	_, err := s.AddMark(context.Background(), Mark{
		ID: "m1", AuthorID: testUserID, Kind: MarkClear,
	})

	if err == nil {
		t.Error("AddMark() accepted a duplicate id, want it rejected")
	}
}
