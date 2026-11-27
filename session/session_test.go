package session

import (
	"bytes"
	"encoding/base64"
	"log/slog"
	"maps"
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

// do sends a request whose handler asks for its session and hands it to
// fn.
func (b *browser) do(fn func(s *Session)) *httptest.ResponseRecorder {
	b.t.Helper()
	return b.send(b.cookies, b.asking(fn))
}

// asking is a handler that asks for the request's session and hands it to
// fn.
func (b *browser) asking(fn func(s *Session)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s := From(r.Context())
		if s == nil {
			b.t.Fatal("no session in the request's context")
		}
		fn(s)
		w.Write([]byte("ok"))
	}
}

// send sends a request with cookies, which are the browser's unless it was
// sent before the answer to another came, through the store's middleware
// to h, and keeps what its answer sets, as it comes.
func (b *browser) send(cookies map[string]*http.Cookie, h http.HandlerFunc) *httptest.ResponseRecorder {
	b.t.Helper()
	req := httptest.NewRequest("GET", "/", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	b.store.Middleware(h).ServeHTTP(rec, req)
	for _, c := range rec.Result().Cookies() {
		if c.MaxAge < 0 {
			delete(b.cookies, c.Name)
		} else {
			b.cookies[c.Name] = c
		}
	}
	return rec
}

// clock is the time of the stores a test hands it, which it moves on.
type clock struct{ now time.Time }

func newClock() *clock { return &clock{now: time.Now()} }

func (c *clock) of(st *Store) *Store {
	st.now = func() time.Time { return c.now }
	return st
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

func TestARequestThatPassesLeavesTheSessionAsItCame(t *testing.T) {
	b := newBrowser(t, newStore(t, Config{}))
	b.do(func(s *Session) {
		s.Set("user_id", 42)
		s.Flash("status", "Saved")
	})
	// Beside the request the flash was left for: no cookie, so the flash
	// is there for that one.
	rec := b.do(func(s *Session) { s.Pass() })
	if c := rec.Result().Cookies(); len(c) != 0 {
		t.Errorf("a request that passed wrote %v", c)
	}
	b.do(func(s *Session) {
		if s.Flashed("status") != "Saved" || s.Get("user_id") != 42.0 {
			t.Errorf("after a pass, the flash is %v and user_id %v", s.Flashed("status"), s.Get("user_id"))
		}
	})
}

func TestARequestThatPassesAndChangesTheSessionKeepsTheFlash(t *testing.T) {
	for name, change := range map[string]func(s *Session){
		"sets":    func(s *Session) { s.Set("theme", "dark") },
		"flashes": func(s *Session) { s.Flash("note", "Hello") },
	} {
		b := newBrowser(t, newStore(t, Config{}))
		b.do(func(s *Session) { s.Flash("status", "Saved") })
		rec := b.do(func(s *Session) {
			s.Pass()
			change(s)
		})
		if len(rec.Result().Cookies()) == 0 {
			t.Errorf("a request that passed and %s wrote no cookie", name)
		}
		b.do(func(s *Session) {
			if s.Flashed("status") != "Saved" {
				t.Errorf("a request that passed and %s lost the flash: %v", name, s.Flashed("status"))
			}
		})
	}
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
	clock := newClock()
	b := newBrowser(t, clock.of(newStore(t, Config{Keys: [][]byte{key(1)}})))
	b.do(func(s *Session) { s.Set("user_id", 7) })

	b.store = clock.of(newStore(t, Config{Keys: [][]byte{key(2), key(1)}}))
	b.do(func(s *Session) {
		if s.Get("user_id") != 7.0 {
			t.Errorf("after the rotation, user_id = %v", s.Get("user_id"))
		}
	})

	// The session is written again once its cookie is a minute old, under
	// the new key, so the old key can go once every session has been, or
	// has expired.
	clock.now = clock.now.Add(time.Minute)
	b.do(func(s *Session) {})
	b.store = clock.of(newStore(t, Config{Keys: [][]byte{key(2)}}))
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

func TestASessionCanOutliveTheStoresLifetimeUntilItsCleared(t *testing.T) {
	st := newStore(t, Config{})
	st.now = func() time.Time { return time.Now().Add(-3 * time.Hour) }
	b := newBrowser(t, st)
	month := 30 * 24 * time.Hour
	if rec := b.do(func(s *Session) {
		s.Set("user_id", 7)
		s.SetLifetime(month)
	}); !strings.Contains(rec.Header().Get("Set-Cookie"), "Max-Age=2592000") {
		t.Fatalf("Set-Cookie %q, want a month", rec.Header().Get("Set-Cookie"))
	}

	// Three hours on, past the Store's two, the session is still there, and
	// the response keeps its lifetime without being told again.
	b.store = newStore(t, Config{})
	rec := b.do(func(s *Session) {
		if s.Get("user_id") != 7.0 {
			t.Errorf("a month-long session ended after three hours: user_id %v", s.Get("user_id"))
		}
		if s.Lifetime() != month {
			t.Errorf("the session's lifetime is %v, want a month", s.Lifetime())
		}
	})
	if !strings.Contains(rec.Header().Get("Set-Cookie"), "Max-Age=2592000") {
		t.Errorf("the next response's Set-Cookie %q, want a month still", rec.Header().Get("Set-Cookie"))
	}

	rec = b.do(func(s *Session) {
		s.Clear()
		s.Set("user_id", 8)
		if s.Lifetime() != 2*time.Hour {
			t.Errorf("after Clear, the session's lifetime is %v, want the Store's two hours", s.Lifetime())
		}
	})
	if !strings.Contains(rec.Header().Get("Set-Cookie"), "Max-Age=7200") {
		t.Errorf("after Clear, Set-Cookie %q, want the Store's two hours", rec.Header().Get("Set-Cookie"))
	}
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

// eachWay runs fn with a Store's config for each way it keeps a session:
// in its cookie, and where a Keeper keeps it.
func eachWay(t *testing.T, fn func(t *testing.T, cfg Config)) {
	t.Run("in its cookie", func(t *testing.T) { fn(t, Config{}) })
	t.Run("kept", func(t *testing.T) { fn(t, Config{Keeper: newMemoryKeeper()}) })
}

func TestARequestThatNeverAsksForTheSessionWritesNone(t *testing.T) {
	eachWay(t, func(t *testing.T, cfg Config) {
		clock := newClock()
		b := newBrowser(t, clock.of(newStore(t, cfg)))
		b.do(func(s *Session) {
			s.Set("user_id", 42)
			s.Flash("status", "Saved")
		})

		// A page's script, an hour on: its handler never asks for the
		// session, so it's read from nowhere, and its answer has no cookie,
		// however old the browser's is.
		k, _ := cfg.Keeper.(*memoryKeeper)
		reads := k.reads()
		clock.now = clock.now.Add(time.Hour)
		rec := b.send(b.cookies, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("script")) })
		if c := rec.Header().Get("Set-Cookie"); c != "" {
			t.Errorf("a request that never asked for its session wrote %s", c)
		}
		if k.reads() != reads {
			t.Errorf("a request that never asked for its session read the keeper %d times", k.reads()-reads)
		}
		b.do(func(s *Session) {
			if s.Get("user_id") != 42.0 || s.Flashed("status") != "Saved" {
				t.Errorf("after it, user_id is %v and the flash %v", s.Get("user_id"), s.Flashed("status"))
			}
		})
	})
}

func TestASessionNothingChangedIsWrittenOnceAMinute(t *testing.T) {
	eachWay(t, func(t *testing.T, cfg Config) {
		clock := newClock()
		b := newBrowser(t, clock.of(newStore(t, cfg)))
		b.do(func(s *Session) { s.Set("user_id", 42) })
		wrote := clock.now

		// The requests of a page that changes nothing write nothing within
		// the minute: the browser's cookie is the session as it is.
		for _, after := range []time.Duration{time.Second, 30 * time.Second, 59 * time.Second} {
			clock.now = wrote.Add(after)
			if rec := b.do(func(s *Session) { s.Get("user_id") }); rec.Header().Get("Set-Cookie") != "" {
				t.Errorf("a request %v after the write, changing nothing, wrote %s", after, rec.Header().Get("Set-Cookie"))
			}
		}
		// Once the cookie is a minute old, a request writes it again, which
		// puts off when it expires.
		clock.now = wrote.Add(time.Minute)
		if rec := b.do(func(s *Session) {}); !strings.Contains(rec.Header().Get("Set-Cookie"), "Max-Age=7200") {
			t.Fatalf("a request a minute on wrote %q, want the session for two hours more", rec.Header().Get("Set-Cookie"))
		}
		if p := b.cookiePayload(t); p.Expires != clock.now.Add(2*time.Hour).Unix() {
			t.Errorf("the session expires at %v, want two hours after the request a minute on", time.Unix(p.Expires, 0))
		}
		b.do(func(s *Session) {
			if s.Get("user_id") != 42.0 {
				t.Errorf("after it was written again, user_id = %v", s.Get("user_id"))
			}
		})
	})
}

func TestEachChangeWritesTheSessionAtOnce(t *testing.T) {
	for name, c := range map[string]struct{ before, change func(s *Session) }{
		"sets a value":        {change: func(s *Session) { s.Set("theme", "dark") }},
		"deletes one":         {change: func(s *Session) { s.Delete("user_id") }},
		"clears it":           {change: func(s *Session) { s.Clear() }},
		"sets its lifetime":   {change: func(s *Session) { s.SetLifetime(time.Hour) }},
		"rotates it":          {change: func(s *Session) { s.Rotate() }},
		"flashes":             {change: func(s *Session) { s.Flash("status", "Saved") }},
		"is shown a flash":    {before: func(s *Session) { s.Flash("status", "Saved") }, change: func(s *Session) { s.Flashed("status") }},
		"keeps a flash again": {before: func(s *Session) { s.Flash("status", "Saved") }, change: func(s *Session) { s.Reflash() }},
	} {
		t.Run(name, func(t *testing.T) {
			eachWay(t, func(t *testing.T, cfg Config) {
				b := newBrowser(t, newStore(t, cfg))
				b.do(func(s *Session) {
					s.Set("user_id", 42)
					if c.before != nil {
						c.before(s)
					}
				})
				if rec := b.do(c.change); rec.Header().Get("Set-Cookie") == "" {
					t.Errorf("a request that %s at once wrote nothing", name)
				}
			})
		})
	}
}

func TestARequestBesideALoginDoesntUndoIt(t *testing.T) {
	eachWay(t, func(t *testing.T, cfg Config) {
		b := newBrowser(t, newStore(t, cfg))
		b.do(func(s *Session) { s.Set("theme", "dark") })

		// A page's request goes with the cookie the browser has as a login
		// is sent, and its answer comes after the login's.
		before := maps.Clone(b.cookies)
		b.do(func(s *Session) {
			s.Rotate()
			s.Set("user_id", 42)
		})
		rec := b.send(before, b.asking(func(s *Session) {
			if s.Get("user_id") != nil {
				t.Errorf("the request beside the login had user_id %v", s.Get("user_id"))
			}
		}))
		if c := rec.Header().Get("Set-Cookie"); c != "" {
			t.Errorf("the request beside the login, which only read the session, wrote %s", c)
		}
		b.do(func(s *Session) {
			if s.Get("user_id") != 42.0 || s.Get("theme") != "dark" {
				t.Errorf("after both, user_id is %v and theme %v", s.Get("user_id"), s.Get("theme"))
			}
		})
	})
}

func TestAShortSessionInUseDoesntRunOut(t *testing.T) {
	eachWay(t, func(t *testing.T, cfg Config) {
		cfg.Lifetime = 30 * time.Second
		clock := newClock()
		b := newBrowser(t, clock.of(newStore(t, cfg)))
		b.do(func(s *Session) { s.Set("user_id", 42) })

		// A lifetime shorter than two minutes is put off at half of it, so
		// requests twenty seconds apart keep it, where a minute's wait
		// would let it expire between them.
		for i := range 6 {
			clock.now = clock.now.Add(20 * time.Second)
			b.do(func(s *Session) {
				if s.Get("user_id") != 42.0 {
					t.Fatalf("a session in use ran out %v in", time.Duration(i+1)*20*time.Second)
				}
			})
		}
	})
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
