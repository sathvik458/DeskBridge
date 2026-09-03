package vault

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestVault(t *testing.T) *Vault {
	t.Helper()

	v, err := Open(filepath.Join(t.TempDir(), "files"))
	if err != nil {
		t.Fatalf("Open() returned: %v", err)
	}

	return v
}

func TestOpenCreatesTheRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "nested", "files")

	v, err := Open(root)
	if err != nil {
		t.Fatalf("Open() returned: %v", err)
	}

	info, err := os.Stat(v.Root())
	if err != nil {
		t.Fatalf("stat on the root returned: %v", err)
	}
	if !info.IsDir() {
		t.Error("the vault root is not a directory")
	}
}

func TestOpenRejectsAnEmptyRoot(t *testing.T) {
	if _, err := Open("   "); err == nil {
		t.Error("Open() accepted an empty root, want an error")
	}
}

func TestKeepWritesTheBytesAndHashesThem(t *testing.T) {
	v := newTestVault(t)
	body := "the mitochondria is the powerhouse of the cell"

	receipt, err := v.Keep("abcd1234", strings.NewReader(body), 1024)
	if err != nil {
		t.Fatalf("Keep() returned: %v", err)
	}

	if receipt.Size != int64(len(body)) {
		t.Errorf("Size = %d, want %d", receipt.Size, len(body))
	}

	sum := sha256.Sum256([]byte(body))
	if want := hex.EncodeToString(sum[:]); receipt.Checksum != want {
		t.Errorf("Checksum = %q, want %q", receipt.Checksum, want)
	}

	handle, err := v.Read(receipt.Path)
	if err != nil {
		t.Fatalf("Read() returned: %v", err)
	}
	defer handle.Close()

	read, err := io.ReadAll(handle)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if string(read) != body {
		t.Errorf("read back %q, want %q", read, body)
	}
}

func TestKeepShardsByTheFirstTwoCharacters(t *testing.T) {
	v := newTestVault(t)

	receipt, err := v.Keep("beef9999", strings.NewReader("x"), 1024)
	if err != nil {
		t.Fatalf("Keep() returned: %v", err)
	}

	if want := filepath.Join("be", "beef9999"); receipt.Path != want {
		t.Errorf("Path = %q, want %q", receipt.Path, want)
	}
}

func TestKeepRefusesSomethingOverTheLimit(t *testing.T) {
	v := newTestVault(t)

	_, err := v.Keep("abcd1234", strings.NewReader(strings.Repeat("x", 200)), 100)

	if !errors.Is(err, ErrTooBig) {
		t.Fatalf("error = %v, want ErrTooBig", err)
	}
}

// The point of writing to a temp name first: a rejected upload must not leave a
// truncated file sitting under the name a later download would use.
func TestARejectedUploadLeavesNothingBehind(t *testing.T) {
	v := newTestVault(t)

	if _, err := v.Keep("abcd1234", strings.NewReader(strings.Repeat("x", 200)), 100); err == nil {
		t.Fatal("Keep() accepted an oversized body")
	}

	shelf := filepath.Join(v.Root(), "ab")
	entries, err := os.ReadDir(shelf)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("reading %s: %v", shelf, err)
	}

	if len(entries) != 0 {
		t.Errorf("the shelf holds %d entries, want it empty", len(entries))
	}
}

func TestKeepAtExactlyTheLimitIsAllowed(t *testing.T) {
	v := newTestVault(t)

	receipt, err := v.Keep("abcd1234", strings.NewReader(strings.Repeat("x", 100)), 100)
	if err != nil {
		t.Fatalf("Keep() rejected a body at exactly the limit: %v", err)
	}

	if receipt.Size != 100 {
		t.Errorf("Size = %d, want 100", receipt.Size)
	}
}

func TestKeepRejectsAKeyThatCouldEscape(t *testing.T) {
	v := newTestVault(t)

	for _, key := range []string{"../escape", "a/b", "..", "ab", `a\b`} {
		if _, err := v.Keep(key, strings.NewReader("x"), 1024); err == nil {
			t.Errorf("Keep(%q) was accepted, want it rejected", key)
		}
	}
}

func TestReadRefusesToLeaveTheVault(t *testing.T) {
	v := newTestVault(t)

	secret := filepath.Join(filepath.Dir(v.Root()), "secret.txt")
	if err := os.WriteFile(secret, []byte("not yours"), 0o600); err != nil {
		t.Fatalf("writing the decoy: %v", err)
	}

	if _, err := v.Read("../secret.txt"); err == nil {
		t.Error("Read() followed a path out of the vault")
	}

	if _, err := v.Read(secret); !errors.Is(err, ErrOutsideVault) {
		t.Errorf("error = %v, want ErrOutsideVault for an absolute path", err)
	}
}

func TestChecksumMatchesWhatWasStored(t *testing.T) {
	v := newTestVault(t)

	receipt, err := v.Keep("abcd1234", strings.NewReader("chemistry notes"), 1024)
	if err != nil {
		t.Fatalf("Keep() returned: %v", err)
	}

	again, err := v.Checksum(receipt.Path)
	if err != nil {
		t.Fatalf("Checksum() returned: %v", err)
	}

	if again != receipt.Checksum {
		t.Errorf("Checksum() = %q, want %q", again, receipt.Checksum)
	}
}

func TestChecksumNoticesATamperedFile(t *testing.T) {
	v := newTestVault(t)

	receipt, err := v.Keep("abcd1234", strings.NewReader("chemistry notes"), 1024)
	if err != nil {
		t.Fatalf("Keep() returned: %v", err)
	}

	full := filepath.Join(v.Root(), receipt.Path)
	if err := os.WriteFile(full, []byte("physics notes"), 0o600); err != nil {
		t.Fatalf("tampering: %v", err)
	}

	again, err := v.Checksum(receipt.Path)
	if err != nil {
		t.Fatalf("Checksum() returned: %v", err)
	}

	if again == receipt.Checksum {
		t.Error("the checksum did not change after the file was rewritten")
	}
}

func TestDiscardRemovesTheFile(t *testing.T) {
	v := newTestVault(t)

	receipt, err := v.Keep("abcd1234", strings.NewReader("bye"), 1024)
	if err != nil {
		t.Fatalf("Keep() returned: %v", err)
	}

	if err := v.Discard(receipt.Path); err != nil {
		t.Fatalf("Discard() returned: %v", err)
	}

	if _, err := v.Read(receipt.Path); err == nil {
		t.Error("the file is still readable after Discard()")
	}
}

// Delete runs after the database row is gone, so a second attempt on a blob that is
// already missing has to be a no-op rather than an error the caller has to explain.
func TestDiscardIsFineWithAMissingFile(t *testing.T) {
	v := newTestVault(t)

	if err := v.Discard(filepath.Join("ab", "abcd1234")); err != nil {
		t.Errorf("Discard() on a missing file returned: %v", err)
	}
}
