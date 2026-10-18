package session

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"maps"
	"strings"
	"sync"
	"testing"
	"time"
)

// memoryKeeper is a Keeper in memory, for the tests here and for the
// promises sessiontest checks, through MemoryKeeper in export_test.go.
type memoryKeeper struct {
	mu   sync.Mutex
	rows map[string]keptRow
	sets int   // how many sessions it has been given, for the once a minute
	down error // what it answers while it's down
}

type keptRow struct {
	value   []byte
	expires time.Time
}

func newMemoryKeeper() *memoryKeeper {
	return &memoryKeeper{rows: map[string]keptRow{}}
}

func (k *memoryKeeper) Get(_ context.Context, key []byte, now time.Time) ([]byte, bool, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.down != nil {
		return nil, false, k.down
	}
	row, ok := k.rows[string(key)]
	if !ok || !now.Before(row.expires) {
		return nil, false, nil
	}
	return row.value, true, nil
}

func (k *memoryKeeper) Set(_ context.Context, key, value []byte, expires time.Time) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.down != nil {
		return k.down
	}
	k.rows[string(key)] = keptRow{value: bytes.Clone(value), expires: expires}
	k.sets++
	return nil
}

func (k *memoryKeeper) Delete(_ context.Context, key []byte) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.down != nil {
		return k.down
	}
	delete(k.rows, string(key))
	return nil
}

// kept is how many sessions the keeper holds.
func (k *memoryKeeper) kept() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return len(k.rows)
}

// only is the one session the keeper holds: its key and its value.
func (k *memoryKeeper) only(t *testing.T) (string, []byte) {
	t.Helper()
	k.mu.Lock()
	defer k.mu.Unlock()
	if len(k.rows) != 1 {
		t.Fatalf("the keeper holds %d sessions, want 1", len(k.rows))
	}
	for key, row := range k.rows {
		return key, row.value
	}
	return "", nil
}

// cookiePayload is what the browser's cookie holds, opened.
func (b *browser) cookiePayload(t *testing.T) payload {
	t.Helper()
	c, ok := b.cookies["tug_session"]
	if !ok {
		t.Fatal("the browser has no session cookie")
	}
	sealed, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := b.store.open(sealed, []byte(b.store.cookie))
	if !ok {
		t.Fatal("the cookie doesn't open")
	}
	return p
}

func TestWithAKeeperTheCookieCarriesTheSessionsIDAlone(t *testing.T) {
	k := newMemoryKeeper()
	b := newBrowser(t, newStore(t, Config{Keeper: k}))
	b.do(func(s *Session) { s.Set("user_id", 42) })

	p := b.cookiePayload(t)
	if p.ID == "" || p.Values != nil || p.Flash != nil {
		t.Errorf("the cookie holds %+v; want the ID alone", p)
	}
	// The key is the ID's hash, so the keeper holds no ID to send as a
	// session's own, and the value is sealed, not the values themselves.
	key, value := k.only(t)
	if key == p.ID || !bytes.Equal([]byte(key), keyOf(p.ID)) {
		t.Errorf("the session is kept under %q, want the SHA-256 of %q", key, p.ID)
	}
	if bytes.Contains(value, []byte("user_id")) {
		t.Error("the keeper holds the session's keys in the clear")
	}
	b.do(func(s *Session) {
		if s.Get("user_id") != 42.0 {
			t.Errorf("the next request got user_id = %v", s.Get("user_id"))
		}
	})
}

func TestASessionLargerThanACookieIsKept(t *testing.T) {
	var logs bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })

	// What a cookie can't hold, which is why the Keeper is there.
	big := strings.Repeat("x", 20_000)
	b := newBrowser(t, newStore(t, Config{Keeper: newMemoryKeeper()}))
	if rec := b.do(func(s *Session) { s.Set("big", big) }); rec.Header().Get("Set-Cookie") == "" {
		t.Fatal("no cookie was set")
	}
	b.do(func(s *Session) {
		if s.Get("big") != big {
			t.Errorf("the next request got %d bytes of it", len(s.Get("big").(string)))
		}
	})
	if strings.Contains(logs.String(), "too large") {
		t.Errorf("the log says %q", logs.String())
	}
}

func TestFlashDataGoesThroughAKeeper(t *testing.T) {
	b := newBrowser(t, newStore(t, Config{Keeper: newMemoryKeeper()}))
	b.do(func(s *Session) { s.Flash("status", "Saved") })
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

func TestAClearedSessionIsTakenOutOfTheKeeper(t *testing.T) {
	k := newMemoryKeeper()
	b := newBrowser(t, newStore(t, Config{Keeper: k}))
	b.do(func(s *Session) { s.Set("user_id", 42) })
	rec := b.do(func(s *Session) { s.Clear() })
	if k.kept() != 0 {
		t.Errorf("the keeper still holds %d sessions", k.kept())
	}
	if !strings.Contains(rec.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Errorf("the cookie wasn't dropped: %s", rec.Header().Get("Set-Cookie"))
	}
}

func TestALoginGivesTheSessionANewID(t *testing.T) {
	k := newMemoryKeeper()
	b := newBrowser(t, newStore(t, Config{Keeper: k}))
	// A session someone else fixed in the browser, as auth.Login finds it.
	b.do(func(s *Session) { s.Set("cart", "theirs") })
	fixed := b.cookiePayload(t).ID

	b.do(func(s *Session) {
		s.Rotate()
		s.Set("user_id", 42)
	})
	id := b.cookiePayload(t).ID
	if id == fixed {
		t.Error("the session kept the ID it was logged in under")
	}
	// The row of the old ID goes, so the ID whoever set it still has is
	// no longer a session at all.
	if k.kept() != 1 {
		t.Errorf("the keeper holds %d sessions, want the one under the new ID", k.kept())
	}
	if _, ok, _ := k.Get(context.Background(), keyOf(fixed), time.Now()); ok {
		t.Error("the row of the old ID is still there")
	}
	b.do(func(s *Session) {
		if s.Get("user_id") != 42.0 || s.Get("cart") != "theirs" {
			t.Errorf("the session lost its values: %v, %v", s.Get("user_id"), s.Get("cart"))
		}
	})
}

func TestARotateWithoutAKeeperDoesNothing(t *testing.T) {
	b := newBrowser(t, newStore(t, Config{}))
	b.do(func(s *Session) {
		s.Set("user_id", 42)
		s.Rotate()
	})
	b.do(func(s *Session) {
		if s.Get("user_id") != 42.0 {
			t.Errorf("the next request got user_id = %v", s.Get("user_id"))
		}
	})
}

func TestASessionTheKeeperNoLongerHasIsANewOne(t *testing.T) {
	k := newMemoryKeeper()
	b := newBrowser(t, newStore(t, Config{Keeper: k}))
	b.do(func(s *Session) { s.Set("user_id", 42) })
	// Logged out from another browser, or pruned long after it expired.
	k.rows = map[string]keptRow{}

	rec := b.do(func(s *Session) {
		if s.Get("user_id") != nil {
			t.Errorf("the request still had user_id = %v", s.Get("user_id"))
		}
	})
	// Nothing to keep: the cookie goes, so the browser stops sending an ID
	// that's no longer a session.
	if !strings.Contains(rec.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Errorf("the cookie wasn't dropped: %s", rec.Header().Get("Set-Cookie"))
	}
	if k.kept() != 0 {
		t.Errorf("the keeper holds %d sessions", k.kept())
	}
}

func TestAKeeperThatFailsLeavesTheSessionItHoldsAlone(t *testing.T) {
	var logs bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })

	k := newMemoryKeeper()
	b := newBrowser(t, newStore(t, Config{Keeper: k}))
	b.do(func(s *Session) { s.Set("user_id", 42) })
	kept := maps.Clone(k.rows)

	k.down = errors.New("the database isn't answering")
	rec := b.do(func(s *Session) {
		if s.Get("user_id") != nil {
			t.Errorf("the request had user_id = %v", s.Get("user_id"))
		}
		s.Set("user_id", 7) // what the request does with it is dropped
	})
	if rec.Header().Get("Set-Cookie") != "" {
		t.Errorf("a request whose keeper was down wrote a cookie: %s", rec.Header().Get("Set-Cookie"))
	}
	if !strings.Contains(logs.String(), "can't be read") {
		t.Errorf("the log says %q", logs.String())
	}

	k.down, k.rows = nil, kept
	b.do(func(s *Session) {
		if s.Get("user_id") != 42.0 {
			t.Errorf("once the keeper answered, the session had user_id = %v", s.Get("user_id"))
		}
	})
}

func TestASessionNothingChangedIsWrittenOnceAMinute(t *testing.T) {
	k := newMemoryKeeper()
	st := newStore(t, Config{Keeper: k})
	now := time.Now()
	st.now = func() time.Time { return now }
	b := newBrowser(t, st)

	b.do(func(s *Session) { s.Set("user_id", 42) })
	if k.sets != 1 {
		t.Fatalf("the first request wrote %d times", k.sets)
	}
	// A page of a logged-in app changes nothing: its cookie goes out, so
	// the browser's own expiry is exact, but the row stays as it is.
	for range 3 {
		if rec := b.do(func(s *Session) {}); rec.Header().Get("Set-Cookie") == "" {
			t.Error("a request that changed nothing wrote no cookie")
		}
	}
	if k.sets != 1 {
		t.Errorf("requests that changed nothing wrote the row %d times", k.sets)
	}
	// A request that changes something writes it whenever it comes.
	b.do(func(s *Session) { s.Set("user_id", 7) })
	if k.sets != 2 {
		t.Errorf("a request that set a value wrote the row %d times in all", k.sets)
	}
	// Past the minute, the row is written again to put off its expiry.
	now = now.Add(2 * time.Minute)
	b.do(func(s *Session) {})
	if k.sets != 3 {
		t.Errorf("a request a minute later wrote the row %d times in all", k.sets)
	}
}

func TestAKeptSessionOutlastsItsCookie(t *testing.T) {
	k := newMemoryKeeper()
	st := newStore(t, Config{Keeper: k, Lifetime: time.Hour})
	now := time.Now()
	st.now = func() time.Time { return now }
	b := newBrowser(t, st)
	b.do(func(s *Session) { s.Set("user_id", 42) })

	// The row is written once a minute at most, so it expires refresh
	// after the cookie of the response that wrote it: a session the
	// browser still has is one the keeper still holds.
	c := b.cookies["tug_session"]
	ends := now.Add(time.Duration(c.MaxAge) * time.Second)
	if _, ok, _ := k.Get(context.Background(), keyOf(b.cookiePayload(t).ID), ends); !ok {
		t.Error("the keeper's row expires before the cookie does")
	}
}

func TestAKeptSessionDoesntOpenUnderAnotherID(t *testing.T) {
	k := newMemoryKeeper()
	st := newStore(t, Config{Keeper: k})
	b := newBrowser(t, st)
	b.do(func(s *Session) { s.Set("user_id", 42) })

	id := b.cookiePayload(t).ID
	_, value := k.only(t)
	if _, ok := st.open(value, ad(st.cookie, id)); !ok {
		t.Fatal("the session doesn't open under its own ID")
	}
	// A row moved to another ID, or to an app whose cookie has another
	// name, is bound to neither.
	if _, ok := st.open(value, ad(st.cookie, "another-id")); ok {
		t.Error("the session opens under another ID")
	}
	if _, ok := st.open(value, ad("other_session", id)); ok {
		t.Error("the session opens under another cookie's name")
	}
}

func TestSessionsInCookiesMoveToAKeeperWithNoOneLoggedOut(t *testing.T) {
	keys := [][]byte{key(1)}
	b := newBrowser(t, newStore(t, Config{Keys: keys}))
	b.do(func(s *Session) { s.Set("user_id", 42) })
	if p := b.cookiePayload(t); p.ID != "" {
		t.Fatalf("a cookie without a Keeper carries an ID: %+v", p)
	}

	// The app is deployed again, with a Keeper this time.
	k := newMemoryKeeper()
	b.store = newStore(t, Config{Keys: keys, Keeper: k})
	b.do(func(s *Session) {
		if s.Get("user_id") != 42.0 {
			t.Errorf("the session the cookie held was lost: user_id = %v", s.Get("user_id"))
		}
	})
	if k.kept() != 1 {
		t.Errorf("the keeper holds %d sessions, want the one that was in the cookie", k.kept())
	}
	if b.cookiePayload(t).ID == "" {
		t.Error("the cookie still holds the session itself")
	}
	b.do(func(s *Session) {
		if s.Get("user_id") != 42.0 {
			t.Errorf("the request after that got user_id = %v", s.Get("user_id"))
		}
	})
}

func TestAPassedRequestWritesNoRow(t *testing.T) {
	k := newMemoryKeeper()
	b := newBrowser(t, newStore(t, Config{Keeper: k}))
	b.do(func(s *Session) { s.Flash("status", "Saved") })
	sets := k.sets

	// A page's partial reload beside the visit the flash is for: it reads
	// the session and leaves it, and its flash, as it came.
	if rec := b.do(func(s *Session) { s.Pass() }); rec.Header().Get("Set-Cookie") != "" {
		t.Errorf("a passed request wrote a cookie: %s", rec.Header().Get("Set-Cookie"))
	}
	if k.sets != sets {
		t.Errorf("a passed request wrote the row %d times", k.sets-sets)
	}
	b.do(func(s *Session) {
		if s.Flashed("status") != "Saved" {
			t.Errorf("the visit the flash was left for got %v", s.Flashed("status"))
		}
	})
}

func TestARequestWithNoSessionDoesntTouchTheKeeper(t *testing.T) {
	k := newMemoryKeeper()
	b := newBrowser(t, newStore(t, Config{Keeper: k}))
	if rec := b.do(func(s *Session) {}); rec.Header().Get("Set-Cookie") != "" {
		t.Errorf("a request without a session got a cookie: %s", rec.Header().Get("Set-Cookie"))
	}
	if k.kept() != 0 {
		t.Errorf("the keeper holds %d sessions", k.kept())
	}
}

var _ Keeper = (*memoryKeeper)(nil)

// cacheStore is a cache.Store's methods, spelled out rather than by
// importing package cache, which package session has no need of: the
// assignment below is the documentation's promise that one satisfies a
// Keeper, so an app can keep its sessions in its cache.
type cacheStore interface {
	Get(ctx context.Context, key []byte, now time.Time) (value []byte, ok bool, err error)
	Set(ctx context.Context, key, value []byte, expires time.Time) error
	Add(ctx context.Context, key, value []byte, expires, now time.Time) (bool, error)
	Delete(ctx context.Context, key []byte) error
	DeleteIf(ctx context.Context, key, value []byte) (bool, error)
}

var _ Keeper = cacheStore(nil)
