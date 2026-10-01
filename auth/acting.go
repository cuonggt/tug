package auth

import (
	"time"

	"github.com/cuonggt/tug/session"
)

// actorKey is the session key of the login an actor, as an admin, had
// before acting as another user: their ID, their login's check, and when
// acting ends, in Unix seconds.
const actorKey = "tug.auth.actor"

// ActAs logs s in as another user, the one with this ID and password hash,
// for the user logged in to s now, the actor, as an admin who helps them
// sees the app as they do. The actor's own login is kept beside it, as it
// was, which StopActing logs back in. Who may act as whom is the app's to
// say, as an ability.
//
// Acting starts afresh, as a login does: a password the actor confirmed
// is forgotten, so what asks for the password again asks for the other
// user's. It lasts d at most: after that, UserID logs s out, the actor's
// login too, rather than hand a form sent from the other user's page to
// the actor's account. Login and Logout end it, and the actor's login with
// it. Acting as someone else while acting keeps the actor who started it,
// and its end, when that's sooner.
//
// ActAs panics without a login in s to keep.
func ActAs(s *session.Session, id, passwordHash string, d time.Duration) {
	if s == nil {
		panic("auth: ActAs needs a session; set tug's Config.Session")
	}
	actorID, _ := s.Get(idKey).(string)
	check, _ := s.Get(checkKey).(string)
	if actorID == "" {
		panic("auth: ActAs needs someone logged in to act as another")
	}
	until := now().Add(d)
	if kept, ok := keptActor(s); ok {
		actorID, check = kept.id, kept.check
		if kept.until.Before(until) {
			until = kept.until
		}
	}
	s.Delete(confirmedKey)
	s.Delete(passkeyKey)
	s.Delete(intendedKey)
	s.Set(actorKey, map[string]any{"id": actorID, "check": check, "until": until.Unix()})
	s.Set(idKey, id)
	s.Set(checkKey, fingerprint(passwordHash))
}

// Actor returns the ID of the user acting as the one logged in to s, by
// ActAs, and whether one is. Ask it once UserID has found the login, as
// finding a request's user does: UserID ends acting whose time is up, and
// Actor never says no to a login that's still acting, so a request that
// found its user acted as goes on as acted as, however long it takes.
func Actor(s *session.Session) (string, bool) {
	kept, ok := keptActor(s)
	return kept.id, ok
}

// StopActing logs the actor back in to s, the user acting as the one
// logged in, as they were before ActAs, and returns their ID, and whether
// one was acting. Check their login with Current, as any: their password
// may have changed meanwhile. A password confirmed while acting was the
// other user's, and is forgotten. Acting whose time is up is over, and s
// logged out, as UserID has it.
func StopActing(s *session.Session) (string, bool) {
	if actingEnded(s) {
		return "", false
	}
	kept, ok := keptActor(s)
	if !ok {
		return "", false
	}
	s.Delete(actorKey)
	s.Delete(confirmedKey)
	s.Delete(passkeyKey)
	s.Delete(intendedKey)
	s.Set(idKey, kept.id)
	s.Set(checkKey, kept.check)
	return kept.id, true
}

// actor is the login an actor had before acting, as the session keeps it.
type actor struct {
	id, check string
	until     time.Time // the zero time for none, which is over
}

// keptActor reads the actor s keeps, and whether there's one, whatever
// the time.
func keptActor(s *session.Session) (actor, bool) {
	if s == nil {
		return actor{}, false
	}
	kept, ok := s.Get(actorKey).(map[string]any)
	if !ok {
		return actor{}, false
	}
	var a actor
	a.id, _ = kept["id"].(string)
	a.check, _ = kept["check"].(string)
	a.until, _ = unixTime(kept["until"])
	return a, true
}

// actingEnded logs s out when it's acting and its time is up, or what it
// keeps doesn't read, the actor's login and all, and reports whether it
// did.
func actingEnded(s *session.Session) bool {
	kept, ok := keptActor(s)
	if !ok || kept.id != "" && now().Before(kept.until) {
		return false
	}
	Logout(s)
	return true
}
