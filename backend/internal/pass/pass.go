// Package pass turns a plain password into something safe to keep in the database.
package pass

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	rounds   = 210_000
	saltLen  = 16
	keyLen   = 32
	scheme   = "pbkdf2-sha256"
	MinChars = 10
	MaxChars = 256
)

var (
	ErrTooShort = fmt.Errorf("a password needs at least %d characters", MinChars)
	ErrTooLong  = errors.New("that password is unreasonably long")
	ErrBadHash  = errors.New("stored password hash is malformed")
)

var enc = base64.RawStdEncoding

// Hash returns "scheme$rounds$salt$key". Everything needed to check the password
// later travels with it, so the cost can be raised without a migration.
func Hash(plain string) (string, error) {
	if err := Check(plain); err != nil {
		return "", err
	}

	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("reading random salt: %w", err)
	}

	key, err := pbkdf2.Key(sha256.New, plain, salt, rounds, keyLen)
	if err != nil {
		return "", fmt.Errorf("deriving key: %w", err)
	}

	return strings.Join([]string{
		scheme,
		strconv.Itoa(rounds),
		enc.EncodeToString(salt),
		enc.EncodeToString(key),
	}, "$"), nil
}

// Matches re-derives with the salt and cost recorded in stored, then compares in
// constant time so a wrong password never leaks how much of it was right.
func Matches(stored, plain string) (bool, error) {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != scheme {
		return false, ErrBadHash
	}

	n, err := strconv.Atoi(parts[1])
	if err != nil || n < 1 {
		return false, ErrBadHash
	}

	salt, err := enc.DecodeString(parts[2])
	if err != nil {
		return false, ErrBadHash
	}

	want, err := enc.DecodeString(parts[3])
	if err != nil {
		return false, ErrBadHash
	}

	got, err := pbkdf2.Key(sha256.New, plain, salt, n, len(want))
	if err != nil {
		return false, fmt.Errorf("deriving key: %w", err)
	}

	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

func Check(plain string) error {
	switch {
	case len([]rune(plain)) < MinChars:
		return ErrTooShort
	case len(plain) > MaxChars:
		return ErrTooLong
	}
	return nil
}
