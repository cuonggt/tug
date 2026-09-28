package auth

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var ctx = context.Background()

func newThrottle(now *time.Time) *Throttle {
	return &Throttle{now: func() time.Time { return *now }}
}

// try is th.Try, failing the test on an error.
func try(t *testing.T, th *Throttle, key string) time.Duration {
	t.Helper()
	wait, err := th.Try(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	return wait
}

func TestAKeyWaitsAfterItsTries(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	th := newThrottle(&now)
	for i := range 5 {
		if wait := try(t, th, "ann@example.com|1.2.3.4"); wait != 0 {
			t.Fatalf("waits %v on try %d", wait, i+1)
		}
		now = now.Add(10 * time.Second)
	}
	if wait := try(t, th, "ann@example.com|1.2.3.4"); wait != 10*time.Second {
		t.Errorf("waits %v, want the rest of the minute since the first try", wait)
	}
	if wait, err := th.Wait(ctx, "ann@example.com|1.2.3.4"); wait != 10*time.Second || err != nil {
		t.Errorf("Wait says %v, %v", wait, err)
	}
	if wait := try(t, th, "bob@example.com|1.2.3.4"); wait != 0 {
		t.Errorf("another key waits %v", wait)
	}
}

func TestTheWaitEndsWithTheWindow(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	th := &Throttle{Max: 2, Window: time.Hour, now: func() time.Time { return now }}
	try(t, th, "k")
	try(t, th, "k")
	if wait := try(t, th, "k"); wait != time.Hour {
		t.Fatalf("waits %v", wait)
	}
	now = now.Add(time.Hour)
	if wait, _ := th.Wait(ctx, "k"); wait != 0 {
		t.Errorf("still waits %v after the window", wait)
	}
	try(t, th, "k")
	if wait := try(t, th, "k"); wait != 0 {
		t.Errorf("a try after the window counts with the ones before: waits %v", wait)
	}
}

func TestATryThatWaitsDoesntMoveTheWindow(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	th := &Throttle{Max: 1, now: func() time.Time { return now }}
	try(t, th, "k")
	for i := range 5 {
		now = now.Add(10 * time.Second)
		if wait := try(t, th, "k"); wait != time.Duration(5-i)*10*time.Second {
			t.Fatalf("waits %v after %v, want the rest of the minute since the first try", wait, time.Duration(i+1)*10*time.Second)
		}
	}
	now = now.Add(10 * time.Second)
	if wait := try(t, th, "k"); wait != 0 {
		t.Errorf("waits %v once the minute is over, for the tries that waited", wait)
	}
}

func TestTriesSentAtOnceDontAllGetIn(t *testing.T) {
	th := &Throttle{Max: 5}
	var in atomic.Int32
	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			if wait, err := th.Try(ctx, "k"); err == nil && wait == 0 {
				in.Add(1)
			}
		})
	}
	wg.Wait()
	if in.Load() != 5 {
		t.Errorf("%d tries got in, want 5", in.Load())
	}
}

func TestSucceedingClearsTheTries(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	th := newThrottle(&now)
	for range 5 {
		try(t, th, "k")
	}
	if err := th.Clear(ctx, "k"); err != nil {
		t.Fatal(err)
	}
	if wait := try(t, th, "k"); wait != 0 {
		t.Errorf("waits %v after clearing", wait)
	}
}

func TestOldTriesAreSweptAway(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	th := newThrottle(&now)
	for i := range 1500 {
		try(t, th, strconv.Itoa(i))
	}
	now = now.Add(2 * time.Minute)
	for i := range 1500 {
		try(t, th, "new "+strconv.Itoa(i))
	}
	if len(th.memory.keys) > 2000 {
		t.Errorf("%d keys kept, most of them long over", len(th.memory.keys))
	}
}

func TestThrottlesOnOneStoreCountUnderTheirNames(t *testing.T) {
	store := &memoryThrottles{}
	passwords := &Throttle{Name: "passwords", Max: 1, Store: store}
	codes := &Throttle{Name: "codes", Max: 1, Store: store}
	try(t, passwords, "42")
	if wait := try(t, codes, "42"); wait != 0 {
		t.Errorf("a user's code waits %v for their password's try", wait)
	}
	again := &Throttle{Name: "passwords", Max: 1, Store: store} // as another instance's
	if wait := try(t, again, "42"); wait == 0 {
		t.Error("the same name's key on the same store didn't count the try before")
	}
}

// recorder is a store that keeps the keys it's given, and fails when told.
type recorder struct {
	memoryThrottles
	keys [][]byte
	fail error
}

func (r *recorder) Hit(ctx context.Context, key []byte, now time.Time, window time.Duration) (int, time.Time, error) {
	r.keys = append(r.keys, key)
	if r.fail != nil {
		return 0, time.Time{}, r.fail
	}
	return r.memoryThrottles.Hit(ctx, key, now, window)
}

func (r *recorder) Tries(ctx context.Context, key []byte, now time.Time) (int, time.Time, error) {
	if r.fail != nil {
		return 0, time.Time{}, r.fail
	}
	return r.memoryThrottles.Tries(ctx, key, now)
}

func (r *recorder) Clear(ctx context.Context, key []byte) error {
	if r.fail != nil {
		return r.fail
	}
	return r.memoryThrottles.Clear(ctx, key)
}

func TestAStoreIsGivenAHashNotTheKey(t *testing.T) {
	store := &recorder{}
	th := &Throttle{Name: "logins", Store: store}
	try(t, th, "ann@example.com|1.2.3.4")
	try(t, th, "ann@example.com|1.2.3.4")
	if len(store.keys) != 2 || len(store.keys[0]) != 32 || !bytes.Equal(store.keys[0], store.keys[1]) {
		t.Fatalf("the store was given %x", store.keys)
	}
	if bytes.Contains(store.keys[0], []byte("ann")) {
		t.Errorf("the key the store was given has the email in it: %q", store.keys[0])
	}
}

func TestAStoresErrorIsTheCallersToAnswer(t *testing.T) {
	down := errors.New("dial tcp: connection refused")
	th := &Throttle{Name: "logins", Store: &recorder{fail: down}}
	if wait, err := th.Try(ctx, "k"); wait != 0 || !errors.Is(err, down) {
		t.Errorf("Try: %v, %v", wait, err)
	}
	if wait, err := th.Wait(ctx, "k"); wait != 0 || !errors.Is(err, down) {
		t.Errorf("Wait: %v, %v", wait, err)
	}
	if err := th.Clear(ctx, "k"); !errors.Is(err, down) {
		t.Errorf("Clear: %v", err)
	}
}
