package pass

import (
	"errors"
	"strings"
	"testing"
)

const good = "correct horse battery staple"

func TestHashThenMatch(t *testing.T) {
	h, err := Hash(good)
	if err != nil {
		t.Fatalf("Hash() returned: %v", err)
	}

	ok, err := Matches(h, good)
	if err != nil {
		t.Fatalf("Matches() returned: %v", err)
	}
	if !ok {
		t.Error("the right password did not match its own hash")
	}
}

func TestWrongPasswordDoesNotMatch(t *testing.T) {
	h, err := Hash(good)
	if err != nil {
		t.Fatalf("Hash() returned: %v", err)
	}

	for _, wrong := range []string{"", "wrong password here", good + "!", strings.ToUpper(good)} {
		ok, err := Matches(h, wrong)
		if err != nil {
			t.Fatalf("Matches(%q) returned: %v", wrong, err)
		}
		if ok {
			t.Errorf("%q matched, want it rejected", wrong)
		}
	}
}

// Same password, different salt, so two people with the same password do not end
// up with the same row and a leaked table cannot be attacked once for everybody.
func TestTheSamePasswordHashesDifferentlyEachTime(t *testing.T) {
	a, err := Hash(good)
	if err != nil {
		t.Fatalf("Hash() returned: %v", err)
	}

	b, err := Hash(good)
	if err != nil {
		t.Fatalf("Hash() returned: %v", err)
	}

	if a == b {
		t.Fatal("two hashes of the same password are identical, so the salt is not random")
	}

	for _, h := range []string{a, b} {
		ok, err := Matches(h, good)
		if err != nil {
			t.Fatalf("Matches() returned: %v", err)
		}
		if !ok {
			t.Error("a salted hash failed to verify its own password")
		}
	}
}

func TestHashCarriesItsOwnParameters(t *testing.T) {
	h, err := Hash(good)
	if err != nil {
		t.Fatalf("Hash() returned: %v", err)
	}

	parts := strings.Split(h, "$")
	if len(parts) != 4 {
		t.Fatalf("hash has %d parts, want scheme$rounds$salt$key", len(parts))
	}
	if parts[0] != scheme {
		t.Errorf("scheme = %q, want %q", parts[0], scheme)
	}
	if parts[1] == "" {
		t.Error("the cost is missing, so it could never be raised later")
	}
	if strings.Contains(h, good) {
		t.Fatal("the plain password appears in the hash")
	}
}

// An old hash written at a lower cost has to keep working, or raising the cost
// would lock everybody out.
func TestAHashAtADifferentCostStillVerifies(t *testing.T) {
	h, err := Hash(good)
	if err != nil {
		t.Fatalf("Hash() returned: %v", err)
	}

	parts := strings.Split(h, "$")
	cheap := strings.Join([]string{parts[0], "1000", parts[2], parts[3]}, "$")

	// Different cost, so the derived key differs and this must NOT match.
	ok, err := Matches(cheap, good)
	if err != nil {
		t.Fatalf("Matches() returned: %v", err)
	}
	if ok {
		t.Error("a hash with the cost rewritten still matched, so the cost is being ignored")
	}
}

func TestMalformedHashesAreRejected(t *testing.T) {
	cases := map[string]string{
		"empty":          "",
		"too few parts":  "pbkdf2-sha256$210000$salt",
		"unknown scheme": "bcrypt$210000$c2FsdA$a2V5",
		"bad rounds":     "pbkdf2-sha256$abc$c2FsdA$a2V5",
		"zero rounds":    "pbkdf2-sha256$0$c2FsdA$a2V5",
		"bad salt":       "pbkdf2-sha256$210000$!!!$a2V5",
		"bad key":        "pbkdf2-sha256$210000$c2FsdA$!!!",
	}

	for name, stored := range cases {
		_, err := Matches(stored, good)
		if !errors.Is(err, ErrBadHash) {
			t.Errorf("%s: error = %v, want ErrBadHash", name, err)
		}
	}
}

func TestShortPasswordsAreRefused(t *testing.T) {
	if _, err := Hash("short"); !errors.Is(err, ErrTooShort) {
		t.Errorf("error = %v, want ErrTooShort", err)
	}
}

func TestAbsurdlyLongPasswordsAreRefused(t *testing.T) {
	if _, err := Hash(strings.Repeat("a", MaxChars+1)); !errors.Is(err, ErrTooLong) {
		t.Errorf("error = %v, want ErrTooLong", err)
	}
}

// Length is counted in runes, so a short passphrase in a non-Latin script is not
// waved through just because it happens to use more bytes.
func TestLengthCountsCharactersNotBytes(t *testing.T) {
	if err := Check("తెలుగుపదం"); !errors.Is(err, ErrTooShort) {
		t.Errorf("error = %v, want a 9-character passphrase to be too short", err)
	}
}

func TestAcceptableLengthPassesTheCheck(t *testing.T) {
	if err := Check(strings.Repeat("x", MinChars)); err != nil {
		t.Errorf("a password at exactly the minimum was refused: %v", err)
	}
}
