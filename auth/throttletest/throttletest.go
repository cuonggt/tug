// Package throttletest is for testing with auth.Throttle: TestStore checks
// that an auth.ThrottleStore keeps the promises a Throttle depends on.
package throttletest

import (
	"context"
	"crypto/sha256"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/cuonggt/tug/auth"
)

// TestStore checks that the stores open returns keep the promises
// auth.Throttle depends on, with a new, empty one for each test. A store's
// own tests call it, as the auth starter's do for its table in its
// database:
//
//	func TestTheThrottlesTableKeepsItsPromises(t *testing.T) {
//		throttletest.TestStore(t, func(t *testing.T) auth.ThrottleStore {
//			return &throttles{db: testDB(t)}
//		})
//	}
func TestStore(t *testing.T, open func(t *testing.T) auth.ThrottleStore) {
	// Whole milliseconds, which any store can keep.
	t0 := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	window := time.Minute
	ctx := context.Background()
	key := func(s string) []byte {
		sum := sha256.Sum256([]byte(s))
		return sum[:]
	}
	hit := func(t *testing.T, s auth.ThrottleStore, k string, now time.Time) (int, time.Time) {
		t.Helper()
		tries, ends, err := s.Hit(ctx, key(k), now, window)
		if err != nil {
			t.Fatalf("Hit: %v", err)
		}
		return tries, ends
	}
	read := func(t *testing.T, s auth.ThrottleStore, k string, now time.Time) (int, time.Time) {
		t.Helper()
		tries, ends, err := s.Tries(ctx, key(k), now)
		if err != nil {
			t.Fatalf("Tries: %v", err)
		}
		return tries, ends
	}

	t.Run("a try is counted in a window that starts with the first", func(t *testing.T) {
		s := open(t)
		for i, at := range []time.Duration{0, time.Second, window - time.Millisecond} {
			tries, ends := hit(t, s, "k", t0.Add(at))
			if tries != i+1 || !ends.Equal(t0.Add(window)) {
				t.Errorf("a try %v in: %d tries, ending %v; want %d, ending a window after the first, %v", at, tries, ends, i+1, t0.Add(window))
			}
		}
	})

	t.Run("a key whose window has ended starts another", func(t *testing.T) {
		s := open(t)
		hit(t, s, "k", t0)
		hit(t, s, "k", t0.Add(time.Second))
		// The instant it ends is the next window's.
		if tries, ends := hit(t, s, "k", t0.Add(window)); tries != 1 || !ends.Equal(t0.Add(2*window)) {
			t.Errorf("a try as the window ends: %d tries, ending %v; want 1, ending %v", tries, ends, t0.Add(2*window))
		}
		later := t0.Add(time.Hour)
		if tries, ends := hit(t, s, "k", later); tries != 1 || !ends.Equal(later.Add(window)) {
			t.Errorf("a try an hour on: %d tries, ending %v; want 1, ending %v", tries, ends, later.Add(window))
		}
	})

	t.Run("tries are read without being counted", func(t *testing.T) {
		s := open(t)
		if tries, ends := read(t, s, "k", t0); tries != 0 || !ends.IsZero() {
			t.Errorf("a key with no tries has %d, ending %v", tries, ends)
		}
		hit(t, s, "k", t0)
		hit(t, s, "k", t0)
		for range 2 {
			if tries, ends := read(t, s, "k", t0.Add(time.Second)); tries != 2 || !ends.Equal(t0.Add(window)) {
				t.Errorf("read %d tries, ending %v; want 2, ending %v", tries, ends, t0.Add(window))
			}
		}
		if tries, ends := read(t, s, "k", t0.Add(window)); tries != 0 || !ends.IsZero() {
			t.Errorf("once its window ended, a key has %d tries, ending %v", tries, ends)
		}
	})

	t.Run("keys are counted apart", func(t *testing.T) {
		s := open(t)
		hit(t, s, "a", t0)
		hit(t, s, "a", t0)
		if tries, _ := hit(t, s, "b", t0); tries != 1 {
			t.Errorf("b's first try counts as %d, with a's", tries)
		}
		if tries, _ := read(t, s, "a", t0); tries != 2 {
			t.Errorf("a has %d tries, want 2", tries)
		}
	})

	t.Run("a key cleared starts again", func(t *testing.T) {
		s := open(t)
		hit(t, s, "a", t0)
		hit(t, s, "a", t0)
		hit(t, s, "b", t0)
		if err := s.Clear(ctx, key("a")); err != nil {
			t.Fatal(err)
		}
		if tries, ends := hit(t, s, "a", t0.Add(time.Second)); tries != 1 || !ends.Equal(t0.Add(time.Second+window)) {
			t.Errorf("a try after clearing: %d tries, ending %v; want 1, in a window that starts with it", tries, ends)
		}
		if tries, _ := read(t, s, "b", t0); tries != 1 {
			t.Errorf("clearing a cleared b too: it has %d tries", tries)
		}
		if err := s.Clear(ctx, key("none")); err != nil {
			t.Errorf("clearing a key with no tries: %v", err)
		}
	})

	t.Run("tries at once each get a count of their own", func(t *testing.T) {
		s := open(t)
		const at = 20
		var mu sync.Mutex
		var counts []int
		var wg sync.WaitGroup
		for range at {
			wg.Go(func() {
				tries, _, err := s.Hit(ctx, key("k"), t0, window)
				if err != nil {
					t.Errorf("Hit: %v", err)
					return
				}
				mu.Lock()
				counts = append(counts, tries)
				mu.Unlock()
			})
		}
		wg.Wait()
		slices.Sort(counts)
		for i, n := range counts {
			if n != i+1 {
				t.Fatalf("%d tries at once got the counts %v, want each of 1 to %d once", at, counts, at)
			}
		}
		if tries, _ := read(t, s, "k", t0); tries != at {
			t.Errorf("%d tries, want %d", tries, at)
		}
	})
}
