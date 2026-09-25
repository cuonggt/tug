package auth

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cuonggt/tug/session"
)

// browser makes requests that carry a session, as a browser keeps its
// cookie from one request to the next.
type browser struct {
	t      *testing.T
	store  *session.Store
	cookie *http.Cookie
}

func newBrowser(t *testing.T) *browser {
	store, err := session.New(session.Config{Keys: [][]byte{bytes.Repeat([]byte{7}, 32)}})
	if err != nil {
		t.Fatal(err)
	}
	return &browser{t: t, store: store}
}

// request runs fn inside a request to target, with the browser's session.
func (b *browser) request(method, target string, fn func(s *session.Session, r *http.Request)) {
	b.t.Helper()
	req := httptest.NewRequest(method, target, nil)
	if b.cookie != nil {
		req.AddCookie(b.cookie)
	}
	rec := httptest.NewRecorder()
	b.store.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fn(session.From(r.Context()), r)
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)
	for _, c := range rec.Result().Cookies() {
		b.cookie = c
	}
}

func TestALoginLastsFromOneRequestToTheNext(t *testing.T) {
	b := newBrowser(t)
	b.request("POST", "/login", func(s *session.Session, _ *http.Request) {
		Login(s, "42", "hash-1")
	})
	b.request("GET", "/dashboard", func(s *session.Session, _ *http.Request) {
		id, ok := UserID(s)
		if !ok || id != "42" {
			t.Errorf("logged in as %q, %v", id, ok)
		}
		if !Current(s, "hash-1") {
			t.Error("the login isn't current with the password it was made with")
		}
	})
}

func TestANewPasswordEndsTheLoginsMadeWithTheOldOne(t *testing.T) {
	b := newBrowser(t)
	b.request("POST", "/login", func(s *session.Session, _ *http.Request) {
		Login(s, "42", "hash-1")
	})
	b.request("GET", "/dashboard", func(s *session.Session, _ *http.Request) {
		if Current(s, "hash-2") {
			t.Error("a login made with the old password is current with the new one")
		}
	})
}

func TestTheSessionKeepsNoPasswordHash(t *testing.T) {
	b := newBrowser(t)
	b.request("POST", "/login", func(s *session.Session, _ *http.Request) {
		Login(s, "42", "$argon2id$v=19$m=19456,t=2,p=1$c2FsdHNhbHRzYWx0c2FsdA$aGFzaA")
		if check := s.Get(checkKey).(string); bytes.Contains([]byte(check), []byte("argon2id")) || len(check) > 20 {
			t.Errorf("the session keeps %q", check)
		}
	})
}

func TestLoggingOutEmptiesTheSession(t *testing.T) {
	b := newBrowser(t)
	b.request("POST", "/login", func(s *session.Session, _ *http.Request) {
		Login(s, "42", "hash-1")
		s.Set("cart", "3 books")
	})
	b.request("POST", "/logout", func(s *session.Session, _ *http.Request) {
		Logout(s)
	})
	b.request("GET", "/", func(s *session.Session, _ *http.Request) {
		if _, ok := UserID(s); ok || s.Get("cart") != nil {
			t.Error("the session outlived logging out")
		}
	})
}

func TestWithoutASessionNoOneIsLoggedIn(t *testing.T) {
	if _, ok := UserID(nil); ok {
		t.Error("UserID found someone")
	}
	if Current(nil, "") {
		t.Error("Current is true")
	}
	if to := Intended(nil, "/dashboard"); to != "/dashboard" {
		t.Errorf("Intended is %q", to)
	}
	Logout(nil)
	SetIntended(nil, httptest.NewRequest("GET", "/", nil))
}

func TestAGuestSentToLogInGoesBackToThePageAfter(t *testing.T) {
	b := newBrowser(t)
	b.request("GET", "/posts/7/edit?tab=body", func(s *session.Session, r *http.Request) {
		SetIntended(s, r)
	})
	b.request("POST", "/login", func(s *session.Session, _ *http.Request) {
		if to := Intended(s, "/dashboard"); to != "/posts/7/edit?tab=body" {
			t.Errorf("intended %q", to)
		}
		if to := Intended(s, "/dashboard"); to != "/dashboard" {
			t.Errorf("the page was kept after it was used: %q", to)
		}
	})
}

func TestAFormSentWhileLoggedOutGoesBackToItsPageAfter(t *testing.T) {
	for referer, want := range map[string]string{
		"http://example.com/posts/new?draft=1": "/posts/new?draft=1",
		"https://evil.example/posts/new":       "/dashboard", // another site's page
		"":                                     "/dashboard",
	} {
		b := newBrowser(t)
		b.request("POST", "/posts", func(s *session.Session, r *http.Request) {
			r.Header.Set("Referer", referer)
			SetIntended(s, r)
			if to := Intended(s, "/dashboard"); to != want {
				t.Errorf("from %q: intended %q, want %q", referer, to, want)
			}
		})
	}
}

func TestTheIntendedPageIsNeverAnotherSite(t *testing.T) {
	for _, to := range []string{"//evil.example/x", "/\\evil.example", "https://evil.example/", "javascript:alert(1)", ""} {
		b := newBrowser(t)
		b.request("GET", "/", func(s *session.Session, _ *http.Request) {
			s.Set(intendedKey, to)
			if got := Intended(s, "/dashboard"); got != "/dashboard" {
				t.Errorf("%q: went to %q", to, got)
			}
		})
	}
}
