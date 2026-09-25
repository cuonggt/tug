package session

import (
	"bytes"
	"encoding/base64"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func key(b byte) []byte { return bytes.Repeat([]byte{b}, 32) }

func newStore(t *testing.T, cfg Config) *Store {
	t.Helper()
	if cfg.Keys == nil {
		cfg.Keys = [][]byte{key(1)}
	}
	st, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// browser keeps the cookies a store sets, as a browser would, and sends
// requests through the store's middleware to fn.
type browser struct {
	t       *testing.T
	store   *Store
	cookies map[string]*http.Cookie
}

func newBrowser(t *testing.T, st *Store) *browser {
	return &browser{t: t, store: st, cookies: map[string]*http.Cookie{}}
}

func (b *browser) do(fn func(s *Session)) *httptest.ResponseRecorder {
	b.t.Helper()
	req := httptest.NewRequest("GET", "/", nil)
	for _, c := range b.cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	b.store.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := From(r.Context())
		if s == nil {
			b.t.Fatal("no session in the request's context")
		}
		fn(s)
		w.Write([]byte("ok"))
	})).ServeHTTP(rec, req)
	for _, c := range rec.Result().Cookies() {
		if c.MaxAge < 0 {
			delete(b.cookies, c.Name)
		} else {
			b.cookies[c.Name] = c
		}
	}
	return rec
}

func TestAValueSetInOneRequestIsThereInTheNext(t *testing.T) {
	b := newBrowser(t, newStore(t, Config{}))
	b.do(func(s *Session) { s.Set("user_id", 42) })
	b.do(func(s *Session) {
		if s.Get("user_id") != 42.0 {
			t.Errorf("user_id = %v", s.Get("user_id"))
		}
	})
}

func TestFlashDataLastsOneRequest(t *testing.T) {
	b := newBrowser(t, newStore(t, Config{}))
	b.do(func(s *Session) {
		s.Flash("status", "Saved")
		if s.Flashed("status") != nil {
			t.Error("a flash is readable in the request that set it")
		}
	})
	b.do(func(s *Session) {
		if s.Flashed("status") != "Saved" {
			t.Errorf("the next request got %v", s.Flashed("status"))
		}
	})
	b.do(func(s *Session) {
		if s.Flashed("status") != nil {
			t.Errorf("the request after that still got %v", s.Flashed("status"))
		}
	})
}

func TestReflashKeepsFlashDataForAnotherRequest(t *testing.T) {
	b := newBrowser(t, newStore(t, Config{}))
	b.do(func(s *Session) { s.Flash("status", "Saved") })
	b.do(func(s *Session) { s.Reflash() })
	b.do(func(s *Session) {
		if s.Flashed("status") != "Saved" {
			t.Errorf("got %v after a reflash", s.Flashed("status"))
		}
	})
}

func TestUnflashTakesBackAFlash(t *testing.T) {
	b := newBrowser(t, newStore(t, Config{}))
	b.do(func(s *Session) {
		s.Flash("status", "Saved")
		s.Unflash("status")
	})
	b.do(func(s *Session) {
		if s.Flashed("status") != nil {
			t.Errorf("got %v", s.Flashed("status"))
		}
	})
}

func TestTheCookieIsEncryptedHTTPOnlyAndLax(t *testing.T) {
	b := newBrowser(t, newStore(t, Config{}))
	rec := b.do(func(s *Session) { s.Set("secret", "hunter2") })
	header := rec.Header().Get("Set-Cookie")
	for _, want := range []string{"tug_session=", "Path=/", "HttpOnly", "SameSite=Lax", "Max-Age=7200"} {
		if !strings.Contains(header, want) {
			t.Errorf("no %s in %s", want, header)
		}
	}
	if strings.Contains(header, "Secure") {
		t.Errorf("a plain HTTP request got a secure cookie: %s", header)
	}
	raw, _ := base64.RawURLEncoding.DecodeString(b.cookies["tug_session"].Value)
	if bytes.Contains(raw, []byte("hunter2")) {
		t.Error("the value is readable in the cookie")
	}

	b = newBrowser(t, newStore(t, Config{Secure: true}))
	if rec := b.do(func(s *Session) { s.Set("a", 1) }); !strings.Contains(rec.Header().Get("Set-Cookie"), "Secure") {
		t.Errorf("Config.Secure didn't make the cookie secure: %s", rec.Header().Get("Set-Cookie"))
	}
}

func TestACookieThatWasTamperedWithStartsANewSession(t *testing.T) {
	b := newBrowser(t, newStore(t, Config{}))
	b.do(func(s *Session) { s.Set("admin", false) })
	c := b.cookies["tug_session"]
	raw, _ := base64.RawURLEncoding.DecodeString(c.Value)
	raw[len(raw)-1] ^= 1
	c.Value = base64.RawURLEncoding.EncodeToString(raw)

	b.do(func(s *Session) {
		if s.Get("admin") != nil {
			t.Errorf("a tampered cookie read as %v", s.Get("admin"))
		}
	})
}

func TestARotatedKeyStillReadsTheSessionsItMade(t *testing.T) {
	old := newStore(t, Config{Keys: [][]byte{key(1)}})
	b := newBrowser(t, old)
	b.do(func(s *Session) { s.Set("user_id", 7) })

	b.store = newStore(t, Config{Keys: [][]byte{key(2), key(1)}})
	b.do(func(s *Session) {
		if s.Get("user_id") != 7.0 {
			t.Errorf("after the rotation, user_id = %v", s.Get("user_id"))
		}
	})

	// The response re-encrypted it with the new key, so the old key can go.
	b.store = newStore(t, Config{Keys: [][]byte{key(2)}})
	b.do(func(s *Session) {
		if s.Get("user_id") != 7.0 {
			t.Errorf("with the old key gone, user_id = %v", s.Get("user_id"))
		}
	})
}

func TestASessionPastItsLifetimeStartsAfresh(t *testing.T) {
	st := newStore(t, Config{})
	st.now = func() time.Time { return time.Now().Add(-3 * time.Hour) } // saved three hours ago
	b := newBrowser(t, st)
	b.do(func(s *Session) { s.Set("user_id", 7) })

	// The browser still has the cookie, as one that ignored its Max-Age
	// would, or an attacker who kept a copy.
	b.store = newStore(t, Config{})
	b.do(func(s *Session) {
		if s.Get("user_id") != nil {
			t.Errorf("an expired session still had user_id %v", s.Get("user_id"))
		}
	})
}

func TestNewTakesOnlyWellFormedKeysAndLifetimes(t *testing.T) {
	for name, cfg := range map[string]Config{
		"no keys":           {},
		"a short key":       {Keys: [][]byte{[]byte("short")}},
		"a negative length": {Keys: [][]byte{key(1)}, Lifetime: -time.Second},
	} {
		if _, err := New(cfg); err == nil {
			t.Errorf("New took %s", name)
		}
	}
}

func TestAnEmptiedSessionDropsItsCookie(t *testing.T) {
	b := newBrowser(t, newStore(t, Config{}))
	b.do(func(s *Session) { s.Set("user_id", 7) })
	rec := b.do(func(s *Session) { s.Clear() })
	if !strings.Contains(rec.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("Set-Cookie %q, want the cookie dropped", rec.Header().Get("Set-Cookie"))
	}
	if rec := b.do(func(s *Session) {}); rec.Header().Get("Set-Cookie") != "" {
		t.Errorf("a request without a session got a cookie: %s", rec.Header().Get("Set-Cookie"))
	}
}

func TestASessionTooLargeForACookieIsntSavedAndSaysSo(t *testing.T) {
	var logs bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })

	b := newBrowser(t, newStore(t, Config{}))
	rec := b.do(func(s *Session) { s.Set("big", strings.Repeat("x", 5000)) })
	if rec.Header().Get("Set-Cookie") != "" {
		t.Error("a cookie too large for a browser was set")
	}
	if !strings.Contains(logs.String(), "too large for a cookie") {
		t.Errorf("the log says %q", logs.String())
	}
}

func TestTheCookieIsSetBeforeTheResponseStarts(t *testing.T) {
	st := newStore(t, Config{})
	rec := httptest.NewRecorder()
	st.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		From(r.Context()).Set("a", 1)
		http.Redirect(w, r, "/next", http.StatusSeeOther)
		From(r.Context()).Set("b", 2) // too late to be saved, as the headers are out
	})).ServeHTTP(rec, httptest.NewRequest("POST", "/", nil))
	if rec.Code != 303 || rec.Header().Get("Set-Cookie") == "" {
		t.Fatalf("got %d with %v", rec.Code, rec.Header())
	}
}

func TestKeysFromEnvReadsLaravelsFormat(t *testing.T) {
	k := base64.StdEncoding.EncodeToString(key(3))
	t.Setenv("APP_KEY", "base64:"+k)
	t.Setenv("APP_PREVIOUS_KEYS", " base64:"+base64.StdEncoding.EncodeToString(key(4))+",")
	keys, err := KeysFromEnv()
	if err != nil || len(keys) != 2 || !bytes.Equal(keys[0], key(3)) || !bytes.Equal(keys[1], key(4)) {
		t.Fatalf("got %d keys, %v", len(keys), err)
	}

	t.Setenv("APP_KEY", "")
	if _, err := KeysFromEnv(); err != ErrNoKey {
		t.Errorf("no APP_KEY: %v", err)
	}
	t.Setenv("APP_KEY", "base64:dG9vIHNob3J0")
	if _, err := KeysFromEnv(); err == nil {
		t.Error("a short key was taken")
	}
}
