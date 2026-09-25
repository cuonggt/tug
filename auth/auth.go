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
//   - Verifications makes and checks the tokens that email verification
//     links carry.
//   - SetPasswordConfirmed and PasswordConfirmed ask for the password again
//     before somewhere sensitive, such as a user's security settings.
//   - TwoFactor has two-factor logins: the codes of authenticator apps,
//     and recovery codes. StartTwoFactor and TwoFactorPending hold a login
//     back until the second factor comes.
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
	"net/url"
	"strings"
	"time"

	"github.com/cuonggt/tug/session"
)

// The session keys a login is kept under.
const (
	idKey        = "tug.auth.id"
	checkKey     = "tug.auth.check"
	intendedKey  = "tug.auth.intended"
	confirmedKey = "tug.auth.confirmed"
	pendingKey   = "tug.auth.pending"
)

// pendingFor is how long a login waits for its second factor.
const pendingFor = 10 * time.Minute

// now is the time, which the tests move on.
var now = time.Now

// Login logs the user with this ID in to the session s. passwordHash is
// the user's password hash, as stored: the login lasts only while the
// user has it (see Current), so setting a new password, as a reset does,
// logs out every session that logged in with the old one.
//
// A login starts afresh: a password confirmed before it (see
// PasswordConfirmed), or a login waiting for its second factor (see
// StartTwoFactor), is forgotten.
func Login(s *session.Session, id, passwordHash string) {
	if s == nil {
		panic("auth: Login needs a session; set tug's Config.Session")
	}
	s.Delete(confirmedKey)
	s.Delete(pendingKey)
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
// logged in, for middleware that sends a guest to the login page, or to a
// page that asks for the password again. A form sent meanwhile can't be
// sent again for them, so for a request other than a GET it keeps the page
// the form was on instead, by its Referer, when that's a page of this site.
func SetIntended(s *session.Session, r *http.Request) {
	if s == nil {
		return
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		s.Set(intendedKey, r.URL.RequestURI())
		return
	}
	// A Referer can name any site.
	if ref, err := url.Parse(r.Referer()); err == nil && ref.Host == r.Host && ref.Path != "" {
		s.Set(intendedKey, ref.RequestURI())
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

// SetPasswordConfirmed records that the user logged in to s has just typed
// their password, as on a page that asks for it again before somewhere
// sensitive, such as their security settings. PasswordConfirmed says so
// for as long as the app allows. Login forgets it, so set it after a login
// that took the password too.
func SetPasswordConfirmed(s *session.Session) {
	if s != nil {
		s.Set(confirmedKey, now().Unix())
	}
}

// PasswordConfirmed reports whether the user logged in to s typed their
// password within the last d, as SetPasswordConfirmed records. Somewhere
// sensitive asks for it again when they haven't: whoever finds the browser
// logged in, or has an old copy of its cookie, doesn't know it.
func PasswordConfirmed(s *session.Session, within time.Duration) bool {
	if s == nil {
		return false
	}
	at, ok := unixTime(s.Get(confirmedKey))
	return ok && now().Sub(at) < within
}

// StartTwoFactor holds back the login of the user with this ID, whose
// password was right, until they give their second factor too, such as a
// code from their authenticator app. Meanwhile the session isn't logged
// in, as anyone: it keeps the ID for TwoFactorPending, for ten minutes,
// and Login ends the wait once the second factor checks out.
func StartTwoFactor(s *session.Session, id string) {
	if s == nil {
		panic("auth: StartTwoFactor needs a session; set tug's Config.Session")
	}
	s.Delete(idKey)
	s.Delete(checkKey)
	s.Delete(confirmedKey)
	s.Set(pendingKey, map[string]any{"id": id, "at": now().Unix()})
}

// TwoFactorPending returns the ID of the user whose login StartTwoFactor
// held back in s, and whether there's one waiting: for ten minutes, until
// Login or Logout.
func TwoFactorPending(s *session.Session) (string, bool) {
	if s == nil {
		return "", false
	}
	pending, _ := s.Get(pendingKey).(map[string]any)
	id, _ := pending["id"].(string)
	at, ok := unixTime(pending["at"])
	if !ok || id == "" || now().Sub(at) >= pendingFor {
		return "", false
	}
	return id, true
}

// unixTime reads back a time kept in the session in Unix seconds, which is
// an int64 in the request that set it and a float64 after the cookie's
// JSON.
func unixTime(v any) (time.Time, bool) {
	switch v := v.(type) {
	case int64:
		return time.Unix(v, 0), true
	case float64:
		return time.Unix(int64(v), 0), true
	}
	return time.Time{}, false
}
