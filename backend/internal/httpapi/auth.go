package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/pass"
	"github.com/sathvik458/deskbridge/backend/internal/store"
)

const (
	cookieName  = "deskbridge_login"
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

	s.begin(w, r, u, c.Label)
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
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, meOf(userOf(r)))
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
