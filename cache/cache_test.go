package cache

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

var ctx = context.Background()

type Stats struct {
	Users int `json:"users"`
	Posts int `json:"posts"`
}

func newCache(now *time.Time) *Cache {
	return &Cache{now: func() time.Time { return *now }}
}

// captureLog has the log written to a buffer for the rest of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &buf
}

func TestAValueIsKeptUntilItExpires(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	c := newCache(&now)
	if err := c.Set(ctx, "stats", Stats{Users: 3, Posts: 12}, time.Minute); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute - time.Second)
	if stats, ok, err := Get[Stats](ctx, c, "stats"); !ok || err != nil || stats != (Stats{3, 12}) {
		t.Errorf("a second before it expires: %+v, %v, %v", stats, ok, err)
	}
	now = now.Add(time.Second)
	if stats, ok, err := Get[Stats](ctx, c, "stats"); ok || err != nil {
		t.Errorf("once it has expired: %+v, %v, %v", stats, ok, err)
	}
}

func TestAValueComesBackAsTheTypeItsReadInto(t *testing.T) {
	c := &Cache{}
	c.Set(ctx, "stats", Stats{Users: 3, Posts: 12}, time.Minute)
	if m, ok, _ := Get[map[string]int](ctx, c, "stats"); !ok || m["users"] != 3 || m["posts"] != 12 {
		t.Errorf("as a map: %v, %v", m, ok)
	}
	// A value an older build kept, of a type since changed, is none.
	c.Set(ctx, "count", "twelve", time.Minute)
	if n, ok, err := Get[int](ctx, c, "count"); ok || n != 0 || err != nil {
		t.Errorf("a string read as an int: %v, %v, %v", n, ok, err)
	}
}

func TestAValueJSONCantHoldIsntKept(t *testing.T) {
	c := &Cache{}
	err := c.Set(ctx, "feed", make(chan int), time.Minute)
	if err == nil || !strings.Contains(err.Error(), `the value for "feed" can't be kept`) {
		t.Errorf("got %v", err)
	}
	if _, ok, _ := Get[any](ctx, c, "feed"); ok {
		t.Error("it was kept")
	}
}

func TestAValueDeletedIsGone(t *testing.T) {
	c := &Cache{}
	c.Set(ctx, "stats", Stats{Users: 3}, time.Minute)
	c.Set(ctx, "other", Stats{Users: 4}, time.Minute)
	if err := c.Delete(ctx, "stats"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := Get[Stats](ctx, c, "stats"); ok {
		t.Error("stats is still there")
	}
	if _, ok, _ := Get[Stats](ctx, c, "other"); !ok {
		t.Error("the other value went too")
	}
}

func TestRememberWorksAValueOutWhenThereIsNone(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	c := newCache(&now)
	runs := 0
	stats := func(context.Context) (Stats, error) {
		runs++
		return Stats{Users: runs}, nil
	}
	for i, want := range []Stats{{Users: 1}, {Users: 1}} {
		got, err := Remember(ctx, c, "stats", time.Hour, stats)
		if err != nil || got != want {
			t.Errorf("call %d: %+v, %v; want %+v", i+1, got, err, want)
		}
	}
	if runs != 1 {
		t.Errorf("worked out %d times, want once", runs)
	}
	now = now.Add(time.Hour)
	if got, _ := Remember(ctx, c, "stats", time.Hour, stats); got != (Stats{Users: 2}) || runs != 2 {
		t.Errorf("once it expired: %+v, worked out %d times", got, runs)
	}
}

func TestAValueWorkedOutIsKeptForItsTimeFromThen(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	c := newCache(&now)
	Remember(ctx, c, "stats", time.Minute, func(context.Context) (Stats, error) {
		now = now.Add(30 * time.Second) // a slow query
		return Stats{Users: 1}, nil
	})
	now = now.Add(time.Minute - time.Second)
	if _, ok, _ := Get[Stats](ctx, c, "stats"); !ok {
		t.Error("its minute was counted from before it was worked out")
	}
}

func TestRememberWorksOutAValueOfAnotherTypeAgain(t *testing.T) {
	c := &Cache{}
	c.Set(ctx, "count", "twelve", time.Minute)
	n, err := Remember(ctx, c, "count", time.Minute, func(context.Context) (int, error) { return 12, nil })
	if n != 12 || err != nil {
		t.Errorf("got %v, %v", n, err)
	}
	if n, ok, _ := Get[int](ctx, c, "count"); !ok || n != 12 {
		t.Errorf("kept %v, %v", n, ok)
	}
}

func TestAFailureToWorkItOutIsRemembersAndKeepsNothing(t *testing.T) {
	c := &Cache{}
	down := errors.New("the feed is down")
	if _, err := Remember(ctx, c, "feed", time.Minute, func(context.Context) (string, error) { return "", down }); !errors.Is(err, down) {
		t.Errorf("got %v", err)
	}
	if _, ok, _ := Get[string](ctx, c, "feed"); ok {
		t.Error("something was kept")
	}
	if _, err := Remember(ctx, c, "feed", time.Minute, func(context.Context) (chan int, error) { return make(chan int), nil }); err == nil {
		t.Error("a value JSON can't hold was returned, with no error")
	}
}

func TestCallersAtOnceWaitForTheOneWorkingItOut(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// A store that keeps nothing, so each caller that didn't wait would
		// work it out.
		c := &Cache{Store: &failing{err: errors.New("down")}}
		captureLog(t)
		release := make(chan struct{})
		var runs atomic.Int32
		stats := func(context.Context) (Stats, error) {
			runs.Add(1)
			<-release
			return Stats{Users: 3}, nil
		}
		got := make([]Stats, 20)
		var wg sync.WaitGroup
		for i := range got {
			wg.Go(func() {
				s, err := Remember(ctx, c, "stats", time.Hour, stats)
				if err != nil {
					t.Error(err)
				}
				got[i] = s
			})
		}
		synctest.Wait() // every caller is working it out, or waiting
		close(release)
		wg.Wait()
		if runs.Load() != 1 {
			t.Errorf("worked out %d times for 20 callers at once", runs.Load())
		}
		for i, s := range got {
			if s != (Stats{Users: 3}) {
				t.Errorf("caller %d got %+v", i, s)
			}
		}
	})
}

func TestCallersWaitingWorkItOutThemselvesWhenItFails(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := &Cache{}
		release := make(chan struct{})
		var runs atomic.Int32
		stats := func(ctx context.Context) (Stats, error) {
			if runs.Add(1) == 1 {
				<-release
				return Stats{}, context.Canceled // the first caller's client went away
			}
			return Stats{Users: 3}, nil
		}
		first := make(chan error)
		go func() {
			_, err := Remember(ctx, c, "stats", time.Hour, stats)
			first <- err
		}()
		synctest.Wait()
		var wg sync.WaitGroup
		for range 5 {
			wg.Go(func() {
				if s, err := Remember(ctx, c, "stats", time.Hour, stats); err != nil || s != (Stats{Users: 3}) {
					t.Errorf("a caller that waited got %+v, %v", s, err)
				}
			})
		}
		synctest.Wait()
		close(release)
		if err := <-first; !errors.Is(err, context.Canceled) {
			t.Errorf("the first caller got %v", err)
		}
		wg.Wait()
	})
}

func TestACallerWaitingGoesWhenItsContextIsDone(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := &Cache{}
		release := make(chan struct{})
		stats := func(context.Context) (Stats, error) {
			<-release
			return Stats{Users: 3}, nil
		}
		go Remember(ctx, c, "stats", time.Hour, stats)
		synctest.Wait()
		waiting, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		if _, err := Remember(waiting, c, "stats", time.Hour, stats); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("got %v", err)
		}
		close(release)
	})
}

func TestAPanicLetsTheCallersWaitingGo(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := &Cache{}
		release := make(chan struct{})
		var runs atomic.Int32
		stats := func(context.Context) (Stats, error) {
			if runs.Add(1) == 1 {
				<-release
				panic("a bug")
			}
			return Stats{Users: 3}, nil
		}
		go func() {
			defer func() { recover() }()
			Remember(ctx, c, "stats", time.Hour, stats)
		}()
		synctest.Wait()
		done := make(chan Stats)
		go func() {
			s, _ := Remember(ctx, c, "stats", time.Hour, stats)
			done <- s
		}()
		synctest.Wait()
		close(release)
		if s := <-done; s != (Stats{Users: 3}) {
			t.Errorf("the caller waiting got %+v", s)
		}
	})
}

// failing is a store that fails, with err, at everything.
type failing struct{ err error }

func (f *failing) Get(context.Context, []byte, time.Time) ([]byte, bool, error) {
	return nil, false, f.err
}
func (f *failing) Set(context.Context, []byte, []byte, time.Time) error { return f.err }
func (f *failing) Add(context.Context, []byte, []byte, time.Time, time.Time) (bool, error) {
	return false, f.err
}
func (f *failing) Delete(context.Context, []byte) error { return f.err }
func (f *failing) DeleteIf(context.Context, []byte, []byte) (bool, error) {
	return false, f.err
}

func TestAStoreThatFailsIsNoValueToRemember(t *testing.T) {
	logs := captureLog(t)
	down := errors.New("dial tcp: connection refused")
	c := &Cache{Store: &failing{err: down}}
	got, err := Remember(ctx, c, "stats", time.Hour, func(context.Context) (Stats, error) { return Stats{Users: 3}, nil })
	if err != nil || got != (Stats{Users: 3}) {
		t.Errorf("got %+v, %v", got, err)
	}
	for _, msg := range []string{"a value couldn't be read, so it was worked out again", "a value couldn't be kept", "connection refused"} {
		if !strings.Contains(logs.String(), msg) {
			t.Errorf("the log doesn't say %q:\n%s", msg, logs)
		}
	}
}

func TestAStoreThatFailsForAContextThatsDoneLogsNothing(t *testing.T) {
	logs := captureLog(t)
	done, cancel := context.WithCancel(ctx)
	cancel()
	c := &Cache{Store: &failing{err: context.Canceled}}
	Remember(done, c, "stats", time.Hour, func(context.Context) (Stats, error) { return Stats{Users: 3}, nil })
	if logs.Len() > 0 {
		t.Errorf("logged a client that went away:\n%s", logs)
	}
}

func TestAStoresErrorIsTheCallersOtherwise(t *testing.T) {
	down := errors.New("dial tcp: connection refused")
	c := &Cache{Store: &failing{err: down}}
	if err := c.Set(ctx, "k", 1, time.Minute); !errors.Is(err, down) {
		t.Errorf("Set: %v", err)
	}
	if _, ok, err := Get[int](ctx, c, "k"); ok || !errors.Is(err, down) {
		t.Errorf("Get: %v, %v", ok, err)
	}
	if err := c.Delete(ctx, "k"); !errors.Is(err, down) {
		t.Errorf("Delete: %v", err)
	}
	lock := c.Lock("import", time.Minute)
	if ok, err := lock.Try(ctx); ok || !errors.Is(err, down) {
		t.Errorf("Try: %v, %v", ok, err)
	}
	if err := lock.Wait(ctx); !errors.Is(err, down) {
		t.Errorf("Wait: %v", err)
	}
	if err := lock.Release(ctx); !errors.Is(err, down) {
		t.Errorf("Release: %v", err)
	}
}

// recorder is a store that keeps the keys it's given.
type recorder struct {
	memoryStore
	keys [][]byte
}

func (r *recorder) Set(ctx context.Context, key, value []byte, expires time.Time) error {
	r.keys = append(r.keys, key)
	return r.memoryStore.Set(ctx, key, value, expires)
}

func (r *recorder) Add(ctx context.Context, key, value []byte, expires, now time.Time) (bool, error) {
	r.keys = append(r.keys, key)
	return r.memoryStore.Add(ctx, key, value, expires, now)
}

func TestAStoreIsGivenAHashNotTheKey(t *testing.T) {
	store := &recorder{}
	c := &Cache{Store: store}
	c.Set(ctx, "posts:ann@example.com", 1, time.Minute)
	c.Set(ctx, "posts:ann@example.com", 2, time.Minute)
	if len(store.keys) != 2 || len(store.keys[0]) != 32 || !bytes.Equal(store.keys[0], store.keys[1]) {
		t.Fatalf("the store was given %x", store.keys)
	}
	if bytes.Contains(store.keys[0], []byte("ann")) {
		t.Errorf("the key the store was given has the email in it: %q", store.keys[0])
	}
	c.Lock("posts:ann@example.com", time.Minute).Try(ctx)
	if bytes.Equal(store.keys[2], store.keys[0]) {
		t.Error("a lock and a value of one name are under one key")
	}
}

func TestATimeToLiveOfNoTimePanics(t *testing.T) {
	c := &Cache{}
	for _, ttl := range []time.Duration{0, -time.Minute} {
		for name, keep := range map[string]func(){
			"Set":      func() { c.Set(ctx, "k", 1, ttl) },
			"Remember": func() { Remember(ctx, c, "k", ttl, func(context.Context) (int, error) { return 1, nil }) },
			"Lock":     func() { c.Lock("k", ttl) },
		} {
			func() {
				defer func() {
					if msg, _ := recover().(string); !strings.Contains(msg, "such as time.Hour") {
						t.Errorf("%s for %v: panicked with %q", name, ttl, msg)
					}
				}()
				keep()
			}()
		}
	}
}

func TestALockIsHeldByOneAtATime(t *testing.T) {
	c := &Cache{}
	held := c.Lock("import", time.Minute)
	if ok, err := held.Try(ctx); !ok || err != nil {
		t.Fatalf("a free lock: %v, %v", ok, err)
	}
	other := c.Lock("import", time.Minute)
	if ok, _ := other.Try(ctx); ok {
		t.Error("another took the lock held")
	}
	if ok, _ := held.Try(ctx); ok {
		t.Error("the holder took its lock again")
	}
	if ok, _ := c.Lock("report", time.Minute).Try(ctx); !ok {
		t.Error("a lock of another name was held too")
	}
	if err := held.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if ok, _ := other.Try(ctx); !ok {
		t.Error("the lock released wasn't free")
	}
}

func TestALockWhoseTimeIsUpIsFree(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	c := newCache(&now)
	died := c.Lock("import", time.Minute)
	died.Try(ctx)
	now = now.Add(time.Minute)
	next := c.Lock("import", time.Minute)
	if ok, _ := next.Try(ctx); !ok {
		t.Fatal("a lock whose time was up was still held")
	}
	// Its holder, late, lets go of nothing but its own.
	died.Release(ctx)
	if ok, _ := c.Lock("import", time.Minute).Try(ctx); ok {
		t.Error("the late holder let go of the next one's lock")
	}
}

func TestALockAndAValueOfOneNameAreApart(t *testing.T) {
	c := &Cache{}
	c.Set(ctx, "import", "a value", time.Minute)
	lock := c.Lock("import", time.Minute)
	if ok, _ := lock.Try(ctx); !ok {
		t.Error("the value held the lock")
	}
	lock.Release(ctx)
	if v, ok, _ := Get[string](ctx, c, "import"); !ok || v != "a value" {
		t.Errorf("releasing the lock left the value %q, %v", v, ok)
	}
}

func TestWaitTakesTheLockOnceItsFree(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := &Cache{}
		held := c.Lock("import", time.Minute)
		held.Try(ctx)
		took := make(chan time.Time)
		go func() {
			if err := c.Lock("import", time.Minute).Wait(ctx); err != nil {
				t.Error(err)
			}
			took <- time.Now()
		}()
		time.Sleep(time.Second)
		released := time.Now()
		held.Release(ctx)
		if at := <-took; at.Sub(released) > retry {
			t.Errorf("took the lock %v after it was released", at.Sub(released))
		}
		if ok, _ := c.Lock("import", time.Minute).Try(ctx); ok {
			t.Error("the one that waited doesn't hold the lock")
		}
	})
}

func TestWaitGivesUpWhenItsContextIsDone(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := &Cache{}
		c.Lock("import", time.Hour).Try(ctx)
		waiting, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		start := time.Now()
		if err := c.Lock("import", time.Minute).Wait(waiting); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("got %v", err)
		}
		if waited := time.Since(start); waited != 5*time.Second {
			t.Errorf("waited %v", waited)
		}
	})
}

func TestValuesThatExpiredAreSweptAway(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	c := newCache(&now)
	for i := range 1500 {
		c.Set(ctx, strconv.Itoa(i), i, time.Minute)
	}
	now = now.Add(2 * time.Minute)
	Get[int](ctx, c, "0") // the time, for the sweep
	for i := range 1500 {
		c.Set(ctx, "new "+strconv.Itoa(i), i, time.Minute)
	}
	if n := len(c.memory.values); n > 2000 {
		t.Errorf("%d values kept, most of them long expired", n)
	}
}
