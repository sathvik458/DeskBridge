// Package vault keeps uploaded bytes on disk, away from the database.
package vault

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ErrTooBig is returned when a reader hands over more bytes than the caller allowed.
var ErrTooBig = errors.New("file is larger than the limit")

// ErrOutsideVault means a stored path tried to escape the root directory.
var ErrOutsideVault = errors.New("path is outside the vault")

type Vault struct {
	root string
}

// Receipt is what the caller records in the database once the bytes are safely down.
type Receipt struct {
	Path     string
	Size     int64
	Checksum string
}

func Open(root string) (*Vault, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("vault root must not be empty")
	}

	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolving vault root %q: %w", root, err)
	}

	if err := os.MkdirAll(absolute, 0o755); err != nil {
		return nil, fmt.Errorf("creating vault root %q: %w", absolute, err)
	}

	return &Vault{root: absolute}, nil
}

func (v *Vault) Root() string {
	return v.root
}

// Keep streams src to disk under a name derived from key, hashing as it goes. Nothing
// appears under the final name until the whole body has landed.
func (v *Vault) Keep(key string, src io.Reader, limit int64) (Receipt, error) {
	relative, err := v.shelve(key)
	if err != nil {
		return Receipt{}, err
	}

	full := filepath.Join(v.root, relative)

	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return Receipt{}, fmt.Errorf("creating vault directory: %w", err)
	}

	partial, err := os.CreateTemp(filepath.Dir(full), ".incoming-*")
	if err != nil {
		return Receipt{}, fmt.Errorf("opening a temporary file: %w", err)
	}

	tempName := partial.Name()
	settled := false

	defer func() {
		partial.Close()
		if !settled {
			os.Remove(tempName)
		}
	}()

	digest := sha256.New()

	// One extra byte past the limit is enough to know the reader had more to give.
	capped := io.LimitReader(src, limit+1)

	written, err := io.Copy(io.MultiWriter(partial, digest), capped)
	if err != nil {
		return Receipt{}, fmt.Errorf("writing the upload: %w", err)
	}

	if written > limit {
		return Receipt{}, ErrTooBig
	}

	if err := partial.Sync(); err != nil {
		return Receipt{}, fmt.Errorf("flushing the upload: %w", err)
	}

	if err := partial.Close(); err != nil {
		return Receipt{}, fmt.Errorf("closing the upload: %w", err)
	}

	if err := os.Rename(tempName, full); err != nil {
		return Receipt{}, fmt.Errorf("moving the upload into place: %w", err)
	}

	settled = true

	return Receipt{
		Path:     relative,
		Size:     written,
		Checksum: hex.EncodeToString(digest.Sum(nil)),
	}, nil
}

func (v *Vault) Read(relative string) (*os.File, error) {
	full, err := v.resolve(relative)
	if err != nil {
		return nil, err
	}

	handle, err := os.Open(full)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", relative, err)
	}

	return handle, nil
}

// Checksum re-hashes what is actually on disk, which is the only way to notice a
// blob that has rotted or been swapped since it was uploaded.
func (v *Vault) Checksum(relative string) (string, error) {
	handle, err := v.Read(relative)
	if err != nil {
		return "", err
	}
	defer handle.Close()

	digest := sha256.New()
	if _, err := io.Copy(digest, handle); err != nil {
		return "", fmt.Errorf("hashing %s: %w", relative, err)
	}

	return hex.EncodeToString(digest.Sum(nil)), nil
}

func (v *Vault) Discard(relative string) error {
	full, err := v.resolve(relative)
	if err != nil {
		return err
	}

	if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing %s: %w", relative, err)
	}

	return nil
}

// shelve spreads blobs over 256 directories by the first two characters of the key,
// so one folder never has to hold every file ever uploaded.
func (v *Vault) shelve(key string) (string, error) {
	clean := strings.ToLower(strings.TrimSpace(key))

	if len(clean) < 4 || strings.ContainsAny(clean, `/\.`) {
		return "", fmt.Errorf("%q is not usable as a vault key", key)
	}

	return filepath.Join(clean[:2], clean), nil
}

func (v *Vault) resolve(relative string) (string, error) {
	if filepath.IsAbs(relative) {
		return "", fmt.Errorf("%q: %w", relative, ErrOutsideVault)
	}

	full := filepath.Join(v.root, filepath.Clean("/"+relative))

	// Clean already strips a leading "..", but this check is what holds the guarantee.
	if full != v.root && !strings.HasPrefix(full, v.root+string(os.PathSeparator)) {
		return "", fmt.Errorf("%q: %w", relative, ErrOutsideVault)
	}

	return full, nil
}
