// Package auth has the parts of logging users in where a slip is a
// security hole, for an app that keeps its users where it likes:
//
//   - HashPassword and CheckPassword store and check passwords, with
//     argon2id.
//   - Login, Logout and UserID keep who a session is logged in as, in the
//     session's cookie. A login is tied to the password it was made with,
//     so a new password logs out every session that knew the old one.
//   - Resets makes and checks the tokens that password reset links carry.
//   - Throttle limits tries, such as at guessing a password.
//   - SetIntended and Intended send someone who was asked to log in back
//     to the page they were on their way to.
//
// Finding users, by ID or by email, is the app's own business: the auth
// starter, tug new -auth, has a whole app made of these, with its users in
// SQLite. A request's user is found like this:
//
//	s := session.From(r.Context())
//	id, ok := auth.UserID(s)
//	if !ok {
//		return nil, nil // a guest
//	}
//	user, err := users.ByID(r.Context(), id)
//	if errors.Is(err, ErrNoUser) || err == nil && !auth.Current(s, user.PasswordHash) {
//		auth.Logout(s) // the user has gone, or has a new password
//		return nil, nil
//	}
//	return user, err
package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/cuonggt/tug/session"
)

// The session keys a login is kept under.
const (
	idKey       = "tug.auth.id"
	checkKey    = "tug.auth.check"
	intendedKey = "tug.auth.intended"
)

// Login logs the user with this ID in to the session s. passwordHash is
// the user's password hash, as stored: the login lasts only while the
// user has it (see Current), so setting a new password, as a reset does,
// logs out every session that logged in with the old one.
func Login(s *session.Session, id, passwordHash string) {
	if s == nil {
		panic("auth: Login needs a session; set tug's Config.Session")
	}
	s.Set(idKey, id)
	s.Set(checkKey, fingerprint(passwordHash))
}

// Logout logs s out. It empties the session, since whoever uses the
// browser next shouldn't find anything that was kept for the last person.
func Logout(s *session.Session) {
	if s != nil {
		s.Clear()
	}
}

// UserID returns the ID of the user logged in to s, and whether one is.
// Check the login with Current once the user is found.
func UserID(s *session.Session) (string, bool) {
	if s == nil {
		return "", false
	}
	id, ok := s.Get(idKey).(string)
	return id, ok && id != ""
}

// Current reports whether the login in s was made with passwordHash, the
// user's password hash as it's stored now. When it wasn't, the password
// has changed since the login, and s should be logged out.
func Current(s *session.Session, passwordHash string) bool {
	if s == nil {
		return false
	}
	check, _ := s.Get(checkKey).(string)
	return subtle.ConstantTimeCompare([]byte(check), []byte(fingerprint(passwordHash))) == 1
}

// fingerprint stands for a password hash in the session: it tells a
// changed hash from the same one, and gives nothing away should a cookie
// ever be read.
func fingerprint(passwordHash string) string {
	sum := sha256.Sum256([]byte("tug login\x00" + passwordHash))
	return base64.RawURLEncoding.EncodeToString(sum[:12])
}

// SetIntended keeps r's URL in s as the page to go to once the visitor has
// logged in, for middleware that sends a guest to the login page. Only a
// GET is kept: a form sent while logged out can't be sent again for them.
func SetIntended(s *session.Session, r *http.Request) {
	if s != nil && r.Method == http.MethodGet {
		s.Set(intendedKey, r.URL.RequestURI())
	}
}

// Intended returns the page SetIntended kept, and forgets it, or fallback
// when there's none. It's a path on this site, never another site's URL.
func Intended(s *session.Session, fallback string) string {
	if s == nil {
		return fallback
	}
	to, _ := s.Get(intendedKey).(string)
	s.Delete(intendedKey)
	// "//evil.example" and "/\evil.example" are other sites to a browser.
	if !strings.HasPrefix(to, "/") || strings.HasPrefix(to, "//") || strings.HasPrefix(to, "/\\") {
		return fallback
	}
	return to
}
