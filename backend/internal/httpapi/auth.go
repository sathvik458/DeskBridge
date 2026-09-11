package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/limit"
	"github.com/sathvik458/deskbridge/backend/internal/pass"
	"github.com/sathvik458/deskbridge/backend/internal/store"
)

const (
	cookieName  = "deskbridge_login"
	guardCookie = "deskbridge_guard"
	guardHeader = "X-Deskbridge-Guard"
	loginLife   = 14 * 24 * time.Hour
	touchAfter  = time.Hour
	maxUsername = 40
	maxLabel    = 60
)

// A private key type: nothing outside this package can read or overwrite the
// user on a request, because nothing outside this package can name the key.
type ctxKey struct{}

var whoKey ctxKey

func withUser(ctx context.Context, u store.User) context.Context {
	return context.WithValue(ctx, whoKey, u)
}

// userOf never returns an error. Every route it serves sits behind the gate, so
// a missing user is a wiring bug, not a runtime case to branch on.
func userOf(r *http.Request) store.User {
	u, _ := r.Context().Value(whoKey).(store.User)
	return u
}

func newToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand is unavailable: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Label    string `json:"label"`
}

func (c credentials) clean() (credentials, error) {
	c.Username = strings.ToLower(strings.TrimSpace(c.Username))
	c.Label = strings.TrimSpace(c.Label)

	if c.Username == "" || len(c.Username) > maxUsername {
		return c, errors.New("username is required")
	}
	if len(c.Label) > maxLabel {
		c.Label = c.Label[:maxLabel]
	}

	return c, nil
}

type meResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Name     string `json:"display_name"`
	Role     string `json:"role"`
}

func meOf(u store.User) meResponse {
	return meResponse{ID: u.ID, Username: u.Username, Name: u.DisplayName, Role: u.Role}
}

// handleClaim sets the first password on an account that has never had one. It is
// how a fresh install gets its accounts without shipping a default credential.
func (s *Server) handleClaim(w http.ResponseWriter, r *http.Request) {
	var c credentials

	if err := decodeBody(w, r, &c); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	c, err := c.clean()
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if !s.letThrough(w, r, c.Username) {
		return
	}

	if err := pass.Check(c.Password); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	u, err := s.users.UserByUsername(r.Context(), c.Username)
	if errors.Is(err, store.ErrNotFound) {
		s.writeError(w, http.StatusNotFound, "no such account")
		return
	}
	if err != nil {
		s.log.Error("looking up an account", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not set that up")
		return
	}

	hash, err := pass.Hash(c.Password)
	if err != nil {
		s.log.Error("hashing a password", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not set that up")
		return
	}

	taken, err := s.users.ClaimPassword(r.Context(), u.ID, hash)
	if err != nil {
		s.log.Error("claiming an account", "user_id", u.ID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not set that up")
		return
	}
	if !taken {
		s.writeError(w, http.StatusConflict, "that account already has a password")
		return
	}

	s.log.Info("account claimed", "user_id", u.ID, "username", u.Username)

	s.begin(w, r, u, c.Label)
}

func (s *Server) handleSignIn(w http.ResponseWriter, r *http.Request) {
	var c credentials

	if err := decodeBody(w, r, &c); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	c, err := c.clean()
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if !s.letThrough(w, r, c.Username) {
		return
	}

	u, err := s.users.UserByUsername(r.Context(), c.Username)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.log.Error("looking up an account", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not sign you in")
		return
	}

	var hash string
	if err == nil {
		if hash, err = s.users.PassHash(r.Context(), u.ID); err != nil {
			s.log.Error("reading a password", "user_id", u.ID, "error", err)
			s.writeError(w, http.StatusInternalServerError, "could not sign you in")
			return
		}
	}

	// A missing user and an unclaimed account both fall through to the same
	// rejection, and both still pay for a hash comparison so the response time
	// does not say which one it was.
	ok := false
	if hash != "" {
		if ok, err = pass.Matches(hash, c.Password); err != nil {
			s.log.Error("checking a password", "user_id", u.ID, "error", err)
			s.writeError(w, http.StatusInternalServerError, "could not sign you in")
			return
		}
	} else {
		burnTime(c.Password)
	}

	if !ok {
		s.log.Warn("sign in refused", "username", c.Username, "remote", r.RemoteAddr)
		s.writeError(w, http.StatusUnauthorized, "that username and password do not match")
		return
	}

	// Getting in clears the count, so somebody who mistyped twice and then got it
	// right is not still carrying those attempts around.
	s.byName.Forget(c.Username)
	s.byHost.Forget(callerHost(r))

	s.begin(w, r, u, c.Label)
}

// letThrough spends one attempt against both the username and the caller's address.
// The username limit stops a wordlist against one account; the host limit stops one
// machine spraying many usernames.
func (s *Server) letThrough(w http.ResponseWriter, r *http.Request, name string) bool {
	host := callerHost(r)

	for _, check := range []struct {
		l   *limit.Limiter
		key string
	}{{s.byName, name}, {s.byHost, host}} {
		if ok, wait := check.l.Allow(check.key); !ok {
			s.log.Warn("too many attempts", "username", name, "remote", host,
				"retry_after", wait.String())
			w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())))
			s.writeError(w, http.StatusTooManyRequests, "too many attempts, wait a moment")
			return false
		}
	}

	return true
}

// callerHost drops the port so every request from one machine shares a bucket
// rather than getting a fresh one per connection.
func callerHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) begin(w http.ResponseWriter, r *http.Request, u store.User, label string) {
	if label == "" {
		label = "a browser"
	}

	l, err := s.logins.StartLogin(r.Context(), store.Login{
		Token:     newToken(),
		UserID:    u.ID,
		Label:     label,
		ExpiresAt: s.now().Add(loginLife),
	})
	if err != nil {
		s.log.Error("starting a login", "user_id", u.ID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not sign you in")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    l.Token,
		Path:     "/",
		Expires:  l.ExpiresAt,
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})

	// Deliberately readable by JavaScript: the client has to copy it into a header,
	// and a page on another origin cannot read it to do the same.
	http.SetCookie(w, &http.Cookie{
		Name:     guardCookie,
		Value:    newToken(),
		Path:     "/",
		Expires:  l.ExpiresAt,
		HttpOnly: false,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})

	s.writeJSON(w, http.StatusOK, meOf(u))
}

func (s *Server) handleSignOut(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		if err := s.logins.EndLogin(r.Context(), c.Value); err != nil {
			s.log.Error("ending a login", "error", err)
		}
	}

	// Clear it regardless, so a browser holding a token the server has already
	// forgotten does not keep sending it.
	for _, name := range []string{cookieName, guardCookie} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: name == cookieName,
			Secure:   s.secureCookies,
			SameSite: http.SameSiteLaxMode,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, meOf(userOf(r)))
}

// withGuard is the CSRF check. An attacker's page can make the browser *send* our
// cookies, but the same-origin policy stops it reading them, so it cannot copy the
// guard value into a header. Reads are exempt because they change nothing.
func (s *Server) withGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}

		c, err := r.Cookie(guardCookie)
		sent := r.Header.Get(guardHeader)

		if err != nil || c.Value == "" || sent == "" ||
			subtle.ConstantTimeCompare([]byte(c.Value), []byte(sent)) != 1 {
			s.log.Warn("guard mismatch", "path", r.URL.Path, "remote", r.RemoteAddr)
			s.writeError(w, http.StatusForbidden, "that request did not carry a valid guard token")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// withSignIn is the gate. Everything under /api except the auth routes goes
// through it, so a handler can assume there is a user and never check.
func (s *Server) withSignIn(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		if err != nil || c.Value == "" {
			s.writeError(w, http.StatusUnauthorized, "sign in first")
			return
		}

		u, _, err := s.logins.WhoIs(r.Context(), c.Value)
		if errors.Is(err, store.ErrNotFound) {
			s.writeError(w, http.StatusUnauthorized, "sign in first")
			return
		}
		if err != nil {
			s.log.Error("resolving a login", "error", err)
			s.writeError(w, http.StatusInternalServerError, "could not check who you are")
			return
		}

		if err := s.logins.TouchLogin(r.Context(), c.Value, touchAfter); err != nil {
			s.log.Error("touching a login", "error", err)
		}

		next.ServeHTTP(w, r.WithContext(withUser(r.Context(), u)))
	})
}

// burnTime keeps the no-such-user path roughly as slow as a real check, so the
// clock cannot be used to tell which usernames exist.
func burnTime(plain string) {
	const decoy = "pbkdf2-sha256$210000$ZGVjb3lzYWx0MTIzNA$ZGVjb3lrZXl0aGF0bmV2ZXJtYXRjaGVzYW55dGg"
	_, _ = pass.Matches(decoy, plain)
}

// studying answers "whose study data is this request about". A student sees their
// own; a supporter sees the student they are watching. Today there is one student,
// so this is where a real pairing table will slot in later.
func (s *Server) studying(r *http.Request) (string, error) {
	me := userOf(r)
	if me.Role == "student" {
		return me.ID, nil
	}

	kid, err := s.users.OneStudent(r.Context())
	if err != nil {
		return "", err
	}

	return kid.ID, nil
}

type changeRequest struct {
	Current string `json:"current"`
	Next    string `json:"next"`
}

// Changing a password signs out every browser, this one included, so a fresh login
// is started before responding. Otherwise the person who just changed it would be
// the first one locked out.
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var req changeRequest

	if err := decodeBody(w, r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := pass.Check(req.Next); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	me := userOf(r)

	stored, err := s.users.PassHash(r.Context(), me.ID)
	if err != nil {
		s.log.Error("reading a password", "user_id", me.ID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not change your password")
		return
	}

	ok, err := pass.Matches(stored, req.Current)
	if err != nil {
		s.log.Error("checking a password", "user_id", me.ID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not change your password")
		return
	}
	if !ok {
		s.log.Warn("password change refused", "user_id", me.ID, "remote", r.RemoteAddr)
		s.writeError(w, http.StatusUnauthorized, "that is not your current password")
		return
	}

	if req.Current == req.Next {
		s.writeError(w, http.StatusBadRequest, "the new password is the same as the old one")
		return
	}

	fresh, err := pass.Hash(req.Next)
	if err != nil {
		s.log.Error("hashing a password", "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not change your password")
		return
	}

	if err := s.users.SetPassHash(r.Context(), me.ID, fresh); err != nil {
		s.log.Error("setting a password", "user_id", me.ID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "could not change your password")
		return
	}

	// The whole point: anyone who had the old password is now out.
	gone, err := s.logins.EndAllLogins(r.Context(), me.ID)
	if err != nil {
		s.log.Error("ending logins", "user_id", me.ID, "error", err)
		s.writeError(w, http.StatusInternalServerError, "your password changed but other browsers may still be signed in")
		return
	}

	s.log.Info("password changed", "user_id", me.ID, "logins_ended", gone)

	s.begin(w, r, me, "this browser")
}
