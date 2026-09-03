package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func stashTestFile(t *testing.T, s *Store, id, category, name string) File {
	t.Helper()

	file, err := s.RecordUpload(context.Background(), File{
		ID:           id,
		UploaderID:   testUserID,
		Category:     category,
		OriginalName: name,
		StoredPath:   id[:2] + "/" + id,
		SizeBytes:    128,
		Checksum:     "0000000000000000000000000000000000000000000000000000000000000000",
	})
	if err != nil {
		t.Fatalf("RecordUpload(%s) returned: %v", id, err)
	}

	return file
}

func TestRecordAndReadFile(t *testing.T) {
	s := newTestStore(t)

	created := stashTestFile(t, s, "f1aa", ShelfNotes, "thermo.pdf")

	if !created.CreatedAt.Equal(testClock) {
		t.Errorf("CreatedAt = %s, want %s", created.CreatedAt, testClock)
	}

	found, err := s.File(context.Background(), "f1aa")
	if err != nil {
		t.Fatalf("File() returned: %v", err)
	}

	if found.OriginalName != "thermo.pdf" {
		t.Errorf("OriginalName = %q, want %q", found.OriginalName, "thermo.pdf")
	}
	if found.SizeBytes != 128 {
		t.Errorf("SizeBytes = %d, want 128", found.SizeBytes)
	}
}

func TestFileNotFound(t *testing.T) {
	s := newTestStore(t)

	_, err := s.File(context.Background(), "ghost")

	if !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestFilesAreNewestFirst(t *testing.T) {
	s := newTestStore(t)

	stashTestFile(t, s, "f1aa", ShelfNotes, "old.pdf")
	s.setClock(testClock.Add(time.Hour))
	stashTestFile(t, s, "f2bb", ShelfNotes, "new.pdf")

	files, err := s.Files(context.Background(), "")
	if err != nil {
		t.Fatalf("Files() returned: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("got %d files, want 2", len(files))
	}
	if files[0].OriginalName != "new.pdf" {
		t.Errorf("first = %q, want the most recent upload", files[0].OriginalName)
	}
}

func TestFilesCanBeNarrowedToOneShelf(t *testing.T) {
	s := newTestStore(t)

	stashTestFile(t, s, "f1aa", ShelfNotes, "thermo.pdf")
	stashTestFile(t, s, "f2bb", ShelfHomework, "sheet4.docx")
	stashTestFile(t, s, "f3cc", ShelfNotes, "optics.pdf")

	notes, err := s.Files(context.Background(), ShelfNotes)
	if err != nil {
		t.Fatalf("Files() returned: %v", err)
	}

	if len(notes) != 2 {
		t.Fatalf("got %d notes, want 2", len(notes))
	}
	for _, file := range notes {
		if file.Category != ShelfNotes {
			t.Errorf("%s is on the %q shelf, want only notes", file.OriginalName, file.Category)
		}
	}
}

func TestFilesReturnsEmptySliceNotNil(t *testing.T) {
	s := newTestStore(t)

	files, err := s.Files(context.Background(), "")
	if err != nil {
		t.Fatalf("Files() returned: %v", err)
	}
	if files == nil {
		t.Error("Files() returned nil, want an empty slice")
	}
}

func TestForgetFileHandsBackTheRowItDeleted(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	stashTestFile(t, s, "f1aa", ShelfNotes, "thermo.pdf")

	gone, err := s.ForgetFile(ctx, "f1aa")
	if err != nil {
		t.Fatalf("ForgetFile() returned: %v", err)
	}

	// The path is the whole reason this returns the row: without it the caller
	// cannot find the bytes to remove.
	if gone.StoredPath != "f1/f1aa" {
		t.Errorf("StoredPath = %q, want %q", gone.StoredPath, "f1/f1aa")
	}

	if _, err := s.File(ctx, "f1aa"); !errors.Is(err, ErrNotFound) {
		t.Errorf("the row survived the delete, File() returned: %v", err)
	}
}

func TestForgetUnknownFile(t *testing.T) {
	s := newTestStore(t)

	_, err := s.ForgetFile(context.Background(), "ghost")

	if !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestForgettingTwiceOnlySucceedsOnce(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	stashTestFile(t, s, "f1aa", ShelfNotes, "thermo.pdf")

	if _, err := s.ForgetFile(ctx, "f1aa"); err != nil {
		t.Fatalf("first ForgetFile() returned: %v", err)
	}

	if _, err := s.ForgetFile(ctx, "f1aa"); !errors.Is(err, ErrNotFound) {
		t.Errorf("second ForgetFile() error = %v, want ErrNotFound", err)
	}
}

func TestFileTallyCountsEachShelf(t *testing.T) {
	s := newTestStore(t)

	stashTestFile(t, s, "f1aa", ShelfNotes, "one.pdf")
	stashTestFile(t, s, "f2bb", ShelfNotes, "two.pdf")
	stashTestFile(t, s, "f3cc", ShelfImages, "board.png")

	tally, err := s.FileTally(context.Background())
	if err != nil {
		t.Fatalf("FileTally() returned: %v", err)
	}

	if tally[ShelfNotes] != 2 {
		t.Errorf("notes = %d, want 2", tally[ShelfNotes])
	}
	if tally[ShelfImages] != 1 {
		t.Errorf("images = %d, want 1", tally[ShelfImages])
	}
	if _, counted := tally[ShelfHomework]; counted {
		t.Error("an empty shelf appeared in the tally, want it left out")
	}
}

func TestFileRejectsAnUnknownShelf(t *testing.T) {
	s := newTestStore(t)

	_, err := s.RecordUpload(context.Background(), File{
		ID: "f1aa", UploaderID: testUserID, Category: "memes",
		OriginalName: "cat.png", StoredPath: "f1/f1aa", SizeBytes: 1, Checksum: "abc",
	})

	if err == nil {
		t.Error("RecordUpload() accepted an unknown category, want the check constraint to reject it")
	}
}

func TestTwoFilesCannotShareAStoredPath(t *testing.T) {
	s := newTestStore(t)
	stashTestFile(t, s, "f1aa", ShelfNotes, "thermo.pdf")

	_, err := s.RecordUpload(context.Background(), File{
		ID: "f2bb", UploaderID: testUserID, Category: ShelfNotes,
		OriginalName: "copy.pdf", StoredPath: "f1/f1aa", SizeBytes: 1, Checksum: "abc",
	})

	if err == nil {
		t.Error("RecordUpload() accepted a duplicate stored path, want it rejected")
	}
}

func TestIsShelf(t *testing.T) {
	for _, shelf := range Shelves {
		if !IsShelf(shelf) {
			t.Errorf("IsShelf(%q) = false, want true", shelf)
		}
	}

	if IsShelf("memes") {
		t.Error(`IsShelf("memes") = true, want false`)
	}
}
