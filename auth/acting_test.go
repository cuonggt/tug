package auth

import (
	"net/http"
	"testing"
	"time"

	"github.com/cuonggt/tug/session"
)

// actingBrowser is a browser logged in as 42, an admin, acting as 7 for an
// hour from start.
func actingBrowser(t *testing.T, start time.Time) *browser {
	t.Helper()
	now = func() time.Time { return start }
	b := newBrowser(t)
	b.request("POST", "/login", func(s *session.Session, _ *http.Request) {
		Login(s, "42", "hash-42")
	})
	b.request("POST", "/admin/users/7/act", func(s *session.Session, _ *http.Request) {
		ActAs(s, "7", "hash-7", time.Hour)
	})
	return b
}

func TestActingAsAnotherLogsInAsThemAndKeepsTheActorsLoginForGoingBack(t *testing.T) {
	defer func() { now = time.Now }()
	b := actingBrowser(t, time.Now())
	b.request("GET", "/dashboard", func(s *session.Session, _ *http.Request) {
		if id, ok := UserID(s); !ok || id != "7" || !Current(s, "hash-7") {
			t.Errorf("logged in as %q, %v, current %v", id, ok, Current(s, "hash-7"))
		}
		if actor, ok := Actor(s); !ok || actor != "42" {
			t.Errorf("acted as by %q, %v", actor, ok)
		}
	})
	b.request("POST", "/acting/stop", func(s *session.Session, _ *http.Request) {
		if actor, ok := StopActing(s); !ok || actor != "42" {
			t.Errorf("stopped acting for %q, %v", actor, ok)
		}
	})
	b.request("GET", "/admin/users", func(s *session.Session, _ *http.Request) {
		if id, ok := UserID(s); !ok || id != "42" || !Current(s, "hash-42") {
			t.Errorf("back as %q, %v", id, ok)
		}
		if _, ok := Actor(s); ok {
			t.Error("still acting once back")
		}
		if _, ok := StopActing(s); ok {
			t.Error("stopped acting while not acting")
		}
		if id, _ := UserID(s); id != "42" {
			t.Errorf("stopping acting while not acting logged in %q", id)
		}
	})
}

func TestGoingBackIsCheckedAsTheActorsLoginWas(t *testing.T) {
	defer func() { now = time.Now }()
	b := actingBrowser(t, time.Now())
	b.request("POST", "/acting/stop", func(s *session.Session, _ *http.Request) {
		StopActing(s)
		// The admin's password changed while they acted: a reset, say.
		if Current(s, "hash-42-new") {
			t.Error("the login the actor went back to is current with a password set since")
		}
	})
}

func TestActingStartsAfreshAndGoingBackForgetsWhatWasConfirmed(t *testing.T) {
	defer func() { now = time.Now }()
	start := time.Now()
	now = func() time.Time { return start }
	b := newBrowser(t)
	b.request("POST", "/login", func(s *session.Session, _ *http.Request) {
		Login(s, "42", "hash-42")
		SetPasswordConfirmed(s)
		s.Set(intendedKey, "/admin/failed-jobs")
	})
	b.request("POST", "/admin/users/7/act", func(s *session.Session, _ *http.Request) {
		ActAs(s, "7", "hash-7", time.Hour)
		if PasswordConfirmed(s, time.Hour) {
			t.Error("the actor's confirmed password counts for the user they act as")
		}
		if to := Intended(s, "/dashboard"); to != "/dashboard" {
			t.Errorf("the actor's intended page went with them: %q", to)
		}
		SetPasswordConfirmed(s) // the user's password, which they told the admin
	})
	b.request("POST", "/acting/stop", func(s *session.Session, _ *http.Request) {
		StopActing(s)
		if PasswordConfirmed(s, time.Hour) {
			t.Error("the user's confirmed password counts for the actor gone back")
		}
	})
}

func TestActingEndsWithItsTimeAndLogsTheActorOutToo(t *testing.T) {
	defer func() { now = time.Now }()
	start := time.Now()
	b := actingBrowser(t, start)
	b.request("GET", "/dashboard", func(s *session.Session, _ *http.Request) {
		now = func() time.Time { return start.Add(59 * time.Minute) }
		if id, ok := UserID(s); !ok || id != "7" {
			t.Errorf("acting ended early: %q, %v", id, ok)
		}
	})
	b.request("GET", "/dashboard", func(s *session.Session, _ *http.Request) {
		now = func() time.Time { return start.Add(time.Hour) }
		s.Set("cart", "3 books")
		if _, ok := UserID(s); ok {
			t.Error("acting lasted past its hour")
		}
	})
	b.request("GET", "/admin/users", func(s *session.Session, _ *http.Request) {
		if _, ok := UserID(s); ok {
			t.Error("the actor is still logged in once acting's time was up")
		}
		if _, ok := Actor(s); ok || s.Get("cart") != nil {
			t.Error("the session outlived acting's time")
		}
	})
}

func TestEachWayOfAskingEndsActingWhoseTimeIsUp(t *testing.T) {
	defer func() { now = time.Now }()
	start := time.Now()
	for name, ask := range map[string]func(*session.Session) bool{
		"UserID":     func(s *session.Session) bool { _, ok := UserID(s); return ok },
		"Current":    func(s *session.Session) bool { return Current(s, "hash-7") },
		"StopActing": func(s *session.Session) bool { _, ok := StopActing(s); return ok },
	} {
		b := actingBrowser(t, start)
		b.request("GET", "/dashboard", func(s *session.Session, _ *http.Request) {
			now = func() time.Time { return start.Add(2 * time.Hour) }
			if ask(s) {
				t.Errorf("%s said yes once acting's time was up", name)
			}
			if id, ok := s.Get(idKey).(string); ok || id != "" {
				t.Errorf("%s left %q logged in", name, id)
			}
		})
	}
}

// A request that found its user acted as decides what it lets them do by
// it, however long it takes: a refusal while acting can't lapse partway.
func TestARequestThatFoundItsUserActedAsStaysActedAsAsTheTimeRunsOut(t *testing.T) {
	defer func() { now = time.Now }()
	start := time.Now()
	b := actingBrowser(t, start)
	b.request("PUT", "/settings/password", func(s *session.Session, _ *http.Request) {
		now = func() time.Time { return start.Add(time.Hour - time.Second) }
		if _, ok := UserID(s); !ok || !Current(s, "hash-7") {
			t.Fatal("acting ended early")
		}
		now = func() time.Time { return start.Add(time.Hour) }
		if _, ok := Actor(s); !ok {
			t.Error("the request's user stopped being acted as partway through it")
		}
	})
}

func TestLoggingOutOrInWhileActingEndsItAndTheActorsLogin(t *testing.T) {
	defer func() { now = time.Now }()
	b := actingBrowser(t, time.Now())
	b.request("POST", "/logout", func(s *session.Session, _ *http.Request) {
		Logout(s)
	})
	b.request("GET", "/", func(s *session.Session, _ *http.Request) {
		if _, ok := UserID(s); ok {
			t.Error("logged in after logging out")
		}
		if _, ok := StopActing(s); ok {
			t.Error("went back to the actor after logging out")
		}
	})

	b = actingBrowser(t, time.Now())
	b.request("POST", "/login", func(s *session.Session, _ *http.Request) {
		Login(s, "9", "hash-9")
	})
	b.request("GET", "/", func(s *session.Session, _ *http.Request) {
		if _, ok := Actor(s); ok {
			t.Error("a new login is still acted as")
		}
		if id, _ := UserID(s); id != "9" {
			t.Errorf("logged in as %q", id)
		}
	})

	b = actingBrowser(t, time.Now())
	b.request("POST", "/login", func(s *session.Session, _ *http.Request) {
		StartTwoFactor(s, "9")
		if _, ok := Actor(s); ok {
			t.Error("a login waiting for its second factor is acted as")
		}
	})
}

func TestActingWhileActingKeepsTheActorAndTheSoonerEnd(t *testing.T) {
	defer func() { now = time.Now }()
	start := time.Now()
	b := actingBrowser(t, start)
	b.request("POST", "/admin/users/9/act", func(s *session.Session, _ *http.Request) {
		ActAs(s, "9", "hash-9", 2*time.Hour)
	})
	b.request("GET", "/dashboard", func(s *session.Session, _ *http.Request) {
		if id, _ := UserID(s); id != "9" {
			t.Errorf("acting as %q", id)
		}
		if actor, _ := Actor(s); actor != "42" {
			t.Errorf("acted as by %q", actor)
		}
		now = func() time.Time { return start.Add(time.Hour) }
		if _, ok := UserID(s); ok {
			t.Error("acting again lasted past the first's end")
		}
	})
}

func TestActingNeedsSomeoneLoggedIn(t *testing.T) {
	b := newBrowser(t)
	b.request("POST", "/admin/users/7/act", func(s *session.Session, _ *http.Request) {
		defer func() {
			if recover() == nil {
				t.Error("ActAs acted for no one")
			}
		}()
		ActAs(s, "7", "hash-7", time.Hour)
	})
	if _, ok := Actor(nil); ok {
		t.Error("Actor found an actor with no session")
	}
	if _, ok := StopActing(nil); ok {
		t.Error("StopActing went back with no session")
	}
}

func TestWhatTheSessionKeepsOfTheActorThatDoesntReadEndsActing(t *testing.T) {
	for _, kept := range []any{map[string]any{"id": "42", "check": "x"}, map[string]any{"until": float64(time.Now().Add(time.Hour).Unix())}} {
		b := newBrowser(t)
		b.request("GET", "/dashboard", func(s *session.Session, _ *http.Request) {
			Login(s, "7", "hash-7")
			s.Set(actorKey, kept)
			if _, ok := UserID(s); ok {
				t.Errorf("acting with %v is still logged in", kept)
			}
		})
	}
}
