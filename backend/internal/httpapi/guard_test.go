package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/pass"
)

// The whole point of the guard: a forged request from another origin arrives with
// the cookies attached, because browsers do that, but without the header, because
// the attacker's page cannot read the cookie to copy it.
func TestAWriteWithoutTheGuardHeaderIsRefused(t *testing.T) {
	s := newTestServer(t, time.Now())

	r := httptest.NewRequest(http.MethodPost, "/api/messages", strings.NewReader(`{"body":"hi"}`))
	r.AddCookie(&http.Cookie{Name: cookieName, Value: studentToken})
	r.AddCookie(&http.Cookie{Name: guardCookie, Value: testGuard})
	r.Header.Set("Content-Type", "application/json")

	if rec := fire(t, s, r); rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestAWriteWithAMismatchedGuardIsRefused(t *testing.T) {
	s := newTestServer(t, time.Now())

	r := httptest.NewRequest(http.MethodPost, "/api/messages", strings.NewReader(`{"body":"hi"}`))
	r.AddCookie(&http.Cookie{Name: cookieName, Value: studentToken})
	r.AddCookie(&http.Cookie{Name: guardCookie, Value: testGuard})
	r.Header.Set(guardHeader, "a-different-value")
	r.Header.Set("Content-Type", "application/json")

	if rec := fire(t, s, r); rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

// Only the header is under attacker control in the interesting case, but a missing
// cookie must fail too rather than falling through as "nothing to compare".
func TestAWriteWithNoGuardCookieIsRefused(t *testing.T) {
	s := newTestServer(t, time.Now())

	r := httptest.NewRequest(http.MethodPost, "/api/messages", strings.NewReader(`{"body":"hi"}`))
	r.AddCookie(&http.Cookie{Name: cookieName, Value: studentToken})
	r.Header.Set(guardHeader, testGuard)
	r.Header.Set("Content-Type", "application/json")

	if rec := fire(t, s, r); rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

// Two empty strings match each other, so a naive comparison would wave through a
// request carrying neither.
func TestTwoEmptyGuardsDoNotCountAsAMatch(t *testing.T) {
	s := newTestServer(t, time.Now())

	r := httptest.NewRequest(http.MethodPost, "/api/messages", strings.NewReader(`{"body":"hi"}`))
	r.AddCookie(&http.Cookie{Name: cookieName, Value: studentToken})
	r.AddCookie(&http.Cookie{Name: guardCookie, Value: ""})
	r.Header.Set(guardHeader, "")
	r.Header.Set("Content-Type", "application/json")

	if rec := fire(t, s, r); rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

// Reads change nothing, so requiring a header on them would break the dashboard
// on first load before it has a guard to send.
func TestReadsDoNotNeedTheGuard(t *testing.T) {
	s := newTestServer(t, time.Now())

	r := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	r.AddCookie(&http.Cookie{Name: cookieName, Value: studentToken})

	if rec := fire(t, s, r); rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestSigningInHandsOutAReadableGuardCookie(t *testing.T) {
	people := newFakeUserStore()
	h, err := pass.Hash("a good long password")
	if err != nil {
		t.Fatalf("Hash() returned: %v", err)
	}
	people.hashes[studentID] = h

	s := newAuthServer(t, people, newFakeLoginStore())

	rec := fire(t, s, httptest.NewRequest(http.MethodPost, "/api/auth/signin",
		strings.NewReader(`{"username":"student","password":"a good long password"}`)))

	var guard, login *http.Cookie
	for _, c := range rec.Result().Cookies() {
		switch c.Name {
		case guardCookie:
			guard = c
		case cookieName:
			login = c
		}
	}

	if guard == nil {
		t.Fatal("no guard cookie was issued")
	}
	if guard.HttpOnly {
		t.Error("the guard cookie is HttpOnly, so the client could never read it to send the header")
	}
	if login == nil || !login.HttpOnly {
		t.Error("the login cookie should still be HttpOnly")
	}
	if guard.Value == login.Value {
		t.Error("the guard and the session token are the same value, so the guard leaks the session")
	}
}

func TestSignOutClearsBothCookies(t *testing.T) {
	s := newAuthServer(t, newFakeUserStore(), newFakeLoginStore())

	rec := fire(t, s, signedIn(httptest.NewRequest(http.MethodPost, "/api/auth/signout", nil)))

	cleared := map[string]bool{}
	for _, c := range rec.Result().Cookies() {
		if c.MaxAge < 0 {
			cleared[c.Name] = true
		}
	}

	for _, name := range []string{cookieName, guardCookie} {
		if !cleared[name] {
			t.Errorf("%s was not cleared on sign out", name)
		}
	}
}

func TestRepeatedBadSignInsAreThrottled(t *testing.T) {
	people := newFakeUserStore()
	h, err := pass.Hash("a good long password")
	if err != nil {
		t.Fatalf("Hash() returned: %v", err)
	}
	people.hashes[studentID] = h

	s := newAuthServer(t, people, newFakeLoginStore())

	wrong := func() int {
		r := httptest.NewRequest(http.MethodPost, "/api/auth/signin",
			strings.NewReader(`{"username":"student","password":"definitely wrong"}`))
		r.RemoteAddr = "10.0.0.9:5555"
		return fire(t, s, r).Code
	}

	seen429 := false
	for i := 0; i < 12; i++ {
		if wrong() == http.StatusTooManyRequests {
			seen429 = true
			break
		}
	}

	if !seen429 {
		t.Fatal("twelve wrong passwords in a row were all accepted for checking, want a 429")
	}
}

func TestAThrottledReplySaysHowLongToWait(t *testing.T) {
	s := newAuthServer(t, newFakeUserStore(), newFakeLoginStore())

	var rec *httptest.ResponseRecorder
	for i := 0; i < 12; i++ {
		r := httptest.NewRequest(http.MethodPost, "/api/auth/signin",
			strings.NewReader(`{"username":"student","password":"definitely wrong"}`))
		r.RemoteAddr = "10.0.0.9:5555"
		rec = fire(t, s, r)
		if rec.Code == http.StatusTooManyRequests {
			break
		}
	}

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("never got throttled, last status %d", rec.Code)
	}

	after := rec.Header().Get("Retry-After")
	if after == "" {
		t.Fatal("a 429 came back with no Retry-After")
	}
	if n, err := strconv.Atoi(after); err != nil || n <= 0 {
		t.Errorf("Retry-After = %q, want a positive number of seconds", after)
	}
}

// Someone else guessing at my account must not lock me out of my own.
func TestThrottlingOneAccountLeavesAnotherAlone(t *testing.T) {
	people := newFakeUserStore()
	h, err := pass.Hash("a good long password")
	if err != nil {
		t.Fatalf("Hash() returned: %v", err)
	}
	people.hashes[helperID] = h

	s := newAuthServer(t, people, newFakeLoginStore())

	// Hammer the student from one address.
	for i := 0; i < 12; i++ {
		r := httptest.NewRequest(http.MethodPost, "/api/auth/signin",
			strings.NewReader(`{"username":"student","password":"wrong wrong wrong"}`))
		r.RemoteAddr = "10.0.0.9:5555"
		fire(t, s, r)
	}

	// The supporter, from a different address, is unaffected.
	r := httptest.NewRequest(http.MethodPost, "/api/auth/signin",
		strings.NewReader(`{"username":"supporter","password":"a good long password"}`))
	r.RemoteAddr = "10.0.0.44:5555"

	if rec := fire(t, s, r); rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d - the other account got caught in the throttle", rec.Code, http.StatusOK)
	}
}

func TestChangingThePassword(t *testing.T) {
	people := newFakeUserStore()
	h, err := pass.Hash("the old long password")
	if err != nil {
		t.Fatalf("Hash() returned: %v", err)
	}
	people.hashes[studentID] = h

	logins := newFakeLoginStore()
	logins.live["another-browser"] = studentID

	s := newAuthServer(t, people, logins)

	rec := fire(t, s, signedIn(httptest.NewRequest(http.MethodPost, "/api/auth/password",
		strings.NewReader(`{"current":"the old long password","next":"a brand new password"}`))))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body)
	}

	fresh, err := pass.Matches(people.hashes[studentID], "a brand new password")
	if err != nil || !fresh {
		t.Errorf("the new password does not verify: ok=%v err=%v", fresh, err)
	}

	// Every other browser has to be signed out, or changing it achieves nothing.
	if _, still := logins.live["another-browser"]; still {
		t.Error("another browser is still signed in after the password changed")
	}

	// ...but this one gets a replacement immediately, or the person who just
	// changed it would be the first one locked out.
	if cookieFrom(rec) == "" {
		t.Error("no fresh login cookie was issued to the browser that made the change")
	}
}

func TestChangingWithTheWrongCurrentPassword(t *testing.T) {
	people := newFakeUserStore()
	h, err := pass.Hash("the old long password")
	if err != nil {
		t.Fatalf("Hash() returned: %v", err)
	}
	people.hashes[studentID] = h

	logins := newFakeLoginStore()
	s := newAuthServer(t, people, logins)

	rec := fire(t, s, signedIn(httptest.NewRequest(http.MethodPost, "/api/auth/password",
		strings.NewReader(`{"current":"not my password","next":"a brand new password"}`))))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	// Nothing may change on a failed attempt.
	same, err := pass.Matches(people.hashes[studentID], "the old long password")
	if err != nil || !same {
		t.Error("the stored password changed despite the attempt failing")
	}
	if _, gone := logins.live[studentToken]; !gone {
		t.Error("a failed password change signed the user out")
	}
}

func TestChangingToAShortPassword(t *testing.T) {
	people := newFakeUserStore()
	h, _ := pass.Hash("the old long password")
	people.hashes[studentID] = h
	s := newAuthServer(t, people, newFakeLoginStore())

	rec := fire(t, s, signedIn(httptest.NewRequest(http.MethodPost, "/api/auth/password",
		strings.NewReader(`{"current":"the old long password","next":"short"}`))))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestChangingToTheSamePassword(t *testing.T) {
	people := newFakeUserStore()
	h, _ := pass.Hash("the old long password")
	people.hashes[studentID] = h
	s := newAuthServer(t, people, newFakeLoginStore())

	rec := fire(t, s, signedIn(httptest.NewRequest(http.MethodPost, "/api/auth/password",
		strings.NewReader(`{"current":"the old long password","next":"the old long password"}`))))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestChangingPasswordNeedsTheGuard(t *testing.T) {
	s := newAuthServer(t, newFakeUserStore(), newFakeLoginStore())

	r := httptest.NewRequest(http.MethodPost, "/api/auth/password",
		strings.NewReader(`{"current":"a","next":"b"}`))
	r.AddCookie(&http.Cookie{Name: cookieName, Value: studentToken})
	r.Header.Set("Content-Type", "application/json")

	if rec := fire(t, s, r); rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestSweepLimitersReportsWhatItDropped(t *testing.T) {
	s := newTestServer(t, time.Now())

	// Nothing has been used, so nothing is tracked and nothing can be swept.
	if n := s.SweepLimiters(); n != 0 {
		t.Errorf("swept %d from an untouched server, want 0", n)
	}
}
