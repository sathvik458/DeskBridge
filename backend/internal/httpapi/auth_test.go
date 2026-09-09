package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/pass"
	"github.com/sathvik458/deskbridge/backend/internal/store"
)

const (
	studentToken = "test-token-student"
	helperToken  = "test-token-supporter"
	studentID    = "default-student"
	helperID     = "default-supporter"
)

type fakeUserStore struct {
	byID   map[string]store.User
	hashes map[string]string
	err    error
}

func newFakeUserStore() *fakeUserStore {
	return &fakeUserStore{
		byID: map[string]store.User{
			studentID: {ID: studentID, Username: "student", DisplayName: "Student", Role: "student"},
			helperID:  {ID: helperID, Username: "supporter", DisplayName: "Supporter", Role: "supporter"},
		},
		hashes: map[string]string{},
	}
}

func (f *fakeUserStore) User(_ context.Context, id string) (store.User, error) {
	if f.err != nil {
		return store.User{}, f.err
	}
	if u, ok := f.byID[id]; ok {
		return u, nil
	}
	return store.User{}, store.ErrNotFound
}

func (f *fakeUserStore) UserByUsername(_ context.Context, name string) (store.User, error) {
	if f.err != nil {
		return store.User{}, f.err
	}
	for _, u := range f.byID {
		if u.Username == name {
			return u, nil
		}
	}
	return store.User{}, store.ErrNotFound
}

func (f *fakeUserStore) OneStudent(_ context.Context) (store.User, error) {
	if f.err != nil {
		return store.User{}, f.err
	}
	return f.byID[studentID], nil
}

func (f *fakeUserStore) PassHash(_ context.Context, id string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	if _, ok := f.byID[id]; !ok {
		return "", store.ErrNotFound
	}
	return f.hashes[id], nil
}

func (f *fakeUserStore) ClaimPassword(_ context.Context, id, hash string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	if f.hashes[id] != "" {
		return false, nil
	}
	f.hashes[id] = hash
	return true, nil
}

type fakeLoginStore struct {
	live    map[string]string
	touched int
	swept   int
	err     error
}

func newFakeLoginStore() *fakeLoginStore {
	return &fakeLoginStore{live: map[string]string{
		studentToken: studentID,
		helperToken:  helperID,
	}}
}

func (f *fakeLoginStore) StartLogin(_ context.Context, l store.Login) (store.Login, error) {
	if f.err != nil {
		return store.Login{}, f.err
	}
	f.live[l.Token] = l.UserID
	return l, nil
}

func (f *fakeLoginStore) WhoIs(_ context.Context, token string) (store.User, store.Login, error) {
	if f.err != nil {
		return store.User{}, store.Login{}, f.err
	}

	id, ok := f.live[token]
	if !ok {
		return store.User{}, store.Login{}, store.ErrNotFound
	}

	people := newFakeUserStore()
	return people.byID[id], store.Login{Token: token, UserID: id}, nil
}

func (f *fakeLoginStore) TouchLogin(_ context.Context, _ string, _ time.Duration) error {
	f.touched++
	return nil
}

func (f *fakeLoginStore) EndLogin(_ context.Context, token string) error {
	delete(f.live, token)
	return nil
}

func (f *fakeLoginStore) SweepLogins(_ context.Context) (int, error) {
	f.swept++
	return 0, nil
}

// signedIn puts the cookie on a request. Every gated test goes through here, so a
// route that accidentally stops being gated shows up as a test that still passes
// when the cookie is removed.
func signedIn(r *http.Request) *http.Request {
	return signedInAs(r, studentToken)
}

func signedInAs(r *http.Request, token string) *http.Request {
	r.AddCookie(&http.Cookie{Name: cookieName, Value: token})
	return r
}

func fire(t *testing.T, s *Server, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, r)

	return rec
}

func TestGatedRoutesRefuseAnAnonymousCaller(t *testing.T) {
	s := newTestServer(t, time.Now())

	paths := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/devices"},
		{http.MethodGet, "/api/sessions"},
		{http.MethodGet, "/api/sessions/current"},
		{http.MethodGet, "/api/goals"},
		{http.MethodGet, "/api/messages"},
		{http.MethodGet, "/api/messages/unread"},
		{http.MethodGet, "/api/files"},
		{http.MethodGet, "/api/board/marks"},
		{http.MethodGet, "/api/live"},
		{http.MethodGet, "/api/auth/me"},
		{http.MethodPost, "/api/sessions"},
		{http.MethodPost, "/api/goals"},
		{http.MethodPost, "/api/messages"},
		{http.MethodPost, "/api/board/clear"},
		{http.MethodDelete, "/api/files/anything"},
	}

	for _, p := range paths {
		rec := fire(t, s, httptest.NewRequest(p.method, p.path, strings.NewReader("{}")))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want %d", p.method, p.path, rec.Code, http.StatusUnauthorized)
		}
	}
}

// Health and status stay open on purpose: a monitor should be able to see the
// server is alive without holding an account.
func TestHealthAndStatusStayOpen(t *testing.T) {
	s := newTestServer(t, time.Now())

	for _, path := range []string{"/health", "/api/status"} {
		rec := fire(t, s, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s = %d, want %d", path, rec.Code, http.StatusOK)
		}
	}
}

func TestAnUnknownTokenIsRefused(t *testing.T) {
	s := newTestServer(t, time.Now())

	r := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	r.AddCookie(&http.Cookie{Name: cookieName, Value: "not-a-real-token"})

	if rec := fire(t, s, r); rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAnEmptyCookieIsRefused(t *testing.T) {
	s := newTestServer(t, time.Now())

	r := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	r.AddCookie(&http.Cookie{Name: cookieName, Value: ""})

	if rec := fire(t, s, r); rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestMeReportsTheSignedInAccount(t *testing.T) {
	s := newTestServer(t, time.Now())

	rec := fire(t, s, signedIn(httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var me meResponse
	if err := json.NewDecoder(rec.Body).Decode(&me); err != nil {
		t.Fatalf("decoding: %v", err)
	}

	if me.ID != studentID || me.Role != "student" {
		t.Errorf("me = %+v, want the seeded student", me)
	}
}

// The hash must never leave the server, whatever shape the response takes.
func TestMeNeverCarriesAPasswordHash(t *testing.T) {
	people := newFakeUserStore()
	people.hashes[studentID] = "pbkdf2-sha256$210000$c2FsdA$a2V5"
	s := newAuthServer(t, people, newFakeLoginStore())

	rec := fire(t, s, signedIn(httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)))

	for _, leak := range []string{"pbkdf2", "pass_hash", "hash"} {
		if strings.Contains(rec.Body.String(), leak) {
			t.Errorf("the response mentions %q: %s", leak, rec.Body)
		}
	}
}

func TestClaimSetsTheFirstPasswordAndSignsIn(t *testing.T) {
	people := newFakeUserStore()
	s := newAuthServer(t, people, newFakeLoginStore())

	rec := fire(t, s, httptest.NewRequest(http.MethodPost, "/api/auth/claim",
		strings.NewReader(`{"username":"student","password":"a good long password"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body)
	}

	if people.hashes[studentID] == "" {
		t.Fatal("no password was stored")
	}
	if strings.Contains(people.hashes[studentID], "a good long password") {
		t.Fatal("the password was stored in the clear")
	}

	ok, err := pass.Matches(people.hashes[studentID], "a good long password")
	if err != nil || !ok {
		t.Errorf("the stored hash does not verify the password: ok=%v err=%v", ok, err)
	}

	if cookieFrom(rec) == "" {
		t.Error("claiming did not hand back a login cookie")
	}
}

// Claiming twice would be account takeover on any install where somebody has
// already set their password.
func TestClaimingAnAlreadyClaimedAccountIsRefused(t *testing.T) {
	people := newFakeUserStore()
	people.hashes[studentID] = "pbkdf2-sha256$210000$c2FsdA$a2V5"
	s := newAuthServer(t, people, newFakeLoginStore())

	rec := fire(t, s, httptest.NewRequest(http.MethodPost, "/api/auth/claim",
		strings.NewReader(`{"username":"student","password":"a good long password"}`)))

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestClaimRefusesAShortPassword(t *testing.T) {
	s := newAuthServer(t, newFakeUserStore(), newFakeLoginStore())

	rec := fire(t, s, httptest.NewRequest(http.MethodPost, "/api/auth/claim",
		strings.NewReader(`{"username":"student","password":"short"}`)))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestSignInWithTheRightPassword(t *testing.T) {
	people := newFakeUserStore()
	h, err := pass.Hash("a good long password")
	if err != nil {
		t.Fatalf("Hash() returned: %v", err)
	}
	people.hashes[studentID] = h

	s := newAuthServer(t, people, newFakeLoginStore())

	rec := fire(t, s, httptest.NewRequest(http.MethodPost, "/api/auth/signin",
		strings.NewReader(`{"username":"student","password":"a good long password","label":"mac"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body)
	}
	if cookieFrom(rec) == "" {
		t.Error("no login cookie was set")
	}
}

func TestSignInWithTheWrongPassword(t *testing.T) {
	people := newFakeUserStore()
	h, err := pass.Hash("a good long password")
	if err != nil {
		t.Fatalf("Hash() returned: %v", err)
	}
	people.hashes[studentID] = h

	s := newAuthServer(t, people, newFakeLoginStore())

	rec := fire(t, s, httptest.NewRequest(http.MethodPost, "/api/auth/signin",
		strings.NewReader(`{"username":"student","password":"not the password"}`)))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if cookieFrom(rec) != "" {
		t.Error("a failed sign in still set a cookie")
	}
}

// An unknown username and a wrong password have to look identical, or the login
// form becomes a way to find out which accounts exist.
func TestUnknownUserAndWrongPasswordLookTheSame(t *testing.T) {
	people := newFakeUserStore()
	h, err := pass.Hash("a good long password")
	if err != nil {
		t.Fatalf("Hash() returned: %v", err)
	}
	people.hashes[studentID] = h

	s := newAuthServer(t, people, newFakeLoginStore())

	missing := fire(t, s, httptest.NewRequest(http.MethodPost, "/api/auth/signin",
		strings.NewReader(`{"username":"nobody","password":"a good long password"}`)))
	wrong := fire(t, s, httptest.NewRequest(http.MethodPost, "/api/auth/signin",
		strings.NewReader(`{"username":"student","password":"wrong password here"}`)))

	if missing.Code != wrong.Code {
		t.Errorf("statuses differ: unknown user %d, wrong password %d", missing.Code, wrong.Code)
	}
	if missing.Body.String() != wrong.Body.String() {
		t.Errorf("bodies differ:\n unknown: %s wrong:   %s", missing.Body, wrong.Body)
	}
}

// An account nobody has claimed cannot be signed into at all, which is what makes
// shipping with no default password safe.
func TestAnUnclaimedAccountCannotBeSignedInto(t *testing.T) {
	s := newAuthServer(t, newFakeUserStore(), newFakeLoginStore())

	rec := fire(t, s, httptest.NewRequest(http.MethodPost, "/api/auth/signin",
		strings.NewReader(`{"username":"student","password":"anything at all here"}`)))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestUsernameIsCaseAndSpaceInsensitive(t *testing.T) {
	people := newFakeUserStore()
	h, err := pass.Hash("a good long password")
	if err != nil {
		t.Fatalf("Hash() returned: %v", err)
	}
	people.hashes[studentID] = h

	s := newAuthServer(t, people, newFakeLoginStore())

	rec := fire(t, s, httptest.NewRequest(http.MethodPost, "/api/auth/signin",
		strings.NewReader(`{"username":"  StUdEnT  ","password":"a good long password"}`)))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body)
	}
}

func TestTheLoginCookieIsLockedDown(t *testing.T) {
	people := newFakeUserStore()
	h, _ := pass.Hash("a good long password")
	people.hashes[studentID] = h
	s := newAuthServer(t, people, newFakeLoginStore())

	rec := fire(t, s, httptest.NewRequest(http.MethodPost, "/api/auth/signin",
		strings.NewReader(`{"username":"student","password":"a good long password"}`)))

	var c *http.Cookie
	for _, got := range rec.Result().Cookies() {
		if got.Name == cookieName {
			c = got
		}
	}
	if c == nil {
		t.Fatal("no login cookie")
	}

	if !c.HttpOnly {
		t.Error("the cookie is readable from JavaScript")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Error("the cookie has no SameSite protection")
	}
	if c.Path != "/" {
		t.Errorf("cookie path = %q, want /", c.Path)
	}
	if c.Expires.IsZero() {
		t.Error("the cookie never expires")
	}
}

func TestSignOutClearsTheCookieAndTheToken(t *testing.T) {
	logins := newFakeLoginStore()
	s := newAuthServer(t, newFakeUserStore(), logins)

	rec := fire(t, s, signedIn(httptest.NewRequest(http.MethodPost, "/api/auth/signout", nil)))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if _, still := logins.live[studentToken]; still {
		t.Error("the token still resolves after signing out")
	}

	for _, c := range rec.Result().Cookies() {
		if c.Name == cookieName && c.MaxAge >= 0 {
			t.Errorf("the cookie was not expired, MaxAge = %d", c.MaxAge)
		}
	}
}

// Signing out with no cookie should not be an error, or a stale tab clicking
// sign out twice would show the user a failure for something that worked.
func TestSignOutWithoutACookieIsFine(t *testing.T) {
	s := newTestServer(t, time.Now())

	rec := fire(t, s, httptest.NewRequest(http.MethodPost, "/api/auth/signout", nil))

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestASignedInRequestTouchesTheLogin(t *testing.T) {
	logins := newFakeLoginStore()
	s := newAuthServer(t, newFakeUserStore(), logins)

	fire(t, s, signedIn(httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)))

	if logins.touched == 0 {
		t.Error("seen_at was never refreshed, so an active login would expire mid-use")
	}
}

func TestLoginStoreFailureIsAServerErrorNotARejection(t *testing.T) {
	logins := newFakeLoginStore()
	logins.err = errors.New("database is down")
	s := newAuthServer(t, newFakeUserStore(), logins)

	rec := fire(t, s, signedIn(httptest.NewRequest(http.MethodGet, "/api/devices", nil)))

	// 401 would tell the user their login is bad when the real problem is ours.
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func cookieFrom(rec *httptest.ResponseRecorder) string {
	for _, c := range rec.Result().Cookies() {
		if c.Name == cookieName && c.Value != "" {
			return c.Value
		}
	}
	return ""
}
