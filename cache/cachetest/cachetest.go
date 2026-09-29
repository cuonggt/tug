// Package cachetest is for testing with package cache: TestStore checks
// that a cache.Store keeps the promises a cache.Cache depends on.
package cachetest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/cuonggt/tug/cache"
)

// TestStore checks that the stores open returns keep the promises
// cache.Cache depends on, with a new, empty one for each test. A store's
// own tests call it, as the auth starter's do for its table in its
// database:
//
//	func TestTheCacheTableKeepsItsPromises(t *testing.T) {
//		cachetest.TestStore(t, func(t *testing.T) cache.Store {
//			return &cacheTable{db: testDB(t)}
//		})
//	}
func TestStore(t *testing.T, open func(t *testing.T) cache.Store) {
	// Whole milliseconds, which any store can keep.
	t0 := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()
	key := func(s string) []byte {
		sum := sha256.Sum256([]byte(s))
		return sum[:]
	}
	set := func(t *testing.T, s cache.Store, k, v string, expires time.Time) {
		t.Helper()
		if err := s.Set(ctx, key(k), []byte(v), expires); err != nil {
			t.Fatalf("Set: %v", err)
		}
	}
	add := func(t *testing.T, s cache.Store, k, v string, expires, now time.Time) bool {
		t.Helper()
		ok, err := s.Add(ctx, key(k), []byte(v), expires, now)
		if err != nil {
			t.Fatalf("Add: %v", err)
		}
		return ok
	}
	deleteIf := func(t *testing.T, s cache.Store, k, v string) bool {
		t.Helper()
		ok, err := s.DeleteIf(ctx, key(k), []byte(v))
		if err != nil {
			t.Fatalf("DeleteIf: %v", err)
		}
		return ok
	}
	// has checks the value k has at now, where "" is none.
	has := func(t *testing.T, s cache.Store, k string, now time.Time, want string) {
		t.Helper()
		v, ok, err := s.Get(ctx, key(k), now)
		switch {
		case err != nil:
			t.Fatalf("Get: %v", err)
		case want == "" && ok:
			t.Errorf("%s at %v: %q, want none", k, now, v)
		case want != "" && (!ok || string(v) != want):
			t.Errorf("%s at %v: %q, %v; want %q", k, now, v, ok, want)
		}
	}

	t.Run("a value is kept until it expires", func(t *testing.T) {
		s := open(t)
		set(t, s, "k", "v", t0.Add(time.Minute))
		has(t, s, "k", t0, "v")
		has(t, s, "k", t0.Add(time.Minute-time.Millisecond), "v")
		// The instant it expires, it's gone.
		has(t, s, "k", t0.Add(time.Minute), "")
		has(t, s, "k", t0.Add(time.Hour), "")
	})

	t.Run("a value set again replaces the one there, and its time", func(t *testing.T) {
		s := open(t)
		set(t, s, "k", "old", t0.Add(time.Hour))
		set(t, s, "k", "new", t0.Add(time.Minute))
		has(t, s, "k", t0, "new")
		has(t, s, "k", t0.Add(2*time.Minute), "")
	})

	t.Run("keys are kept apart", func(t *testing.T) {
		s := open(t)
		set(t, s, "a", "for a", t0.Add(time.Minute))
		set(t, s, "b", "for b", t0.Add(time.Minute))
		has(t, s, "a", t0, "for a")
		has(t, s, "b", t0, "for b")
		has(t, s, "c", t0, "")
	})

	t.Run("a value is kept as it was given, every byte of it", func(t *testing.T) {
		s := open(t)
		// A megabyte, of every byte there is: more than a column of 64 KB,
		// as MySQL's BLOB is, holds.
		var all []byte
		for b := range 256 {
			all = append(all, byte(b))
		}
		want := bytes.Repeat(all, 4096)
		if err := s.Set(ctx, key("k"), want, t0.Add(time.Minute)); err != nil {
			t.Fatalf("Set: %v", err)
		}
		got, ok, err := s.Get(ctx, key("k"), t0)
		if err != nil || !ok || !bytes.Equal(got, want) {
			t.Errorf("Get: %d bytes, %v, %v; want the %d set, as they were", len(got), ok, err, len(want))
		}
	})

	t.Run("a key deleted has no value", func(t *testing.T) {
		s := open(t)
		set(t, s, "a", "for a", t0.Add(time.Minute))
		set(t, s, "b", "for b", t0.Add(time.Minute))
		if err := s.Delete(ctx, key("a")); err != nil {
			t.Fatal(err)
		}
		has(t, s, "a", t0, "")
		has(t, s, "b", t0, "for b")
		if err := s.Delete(ctx, key("none")); err != nil {
			t.Errorf("deleting a key with no value: %v", err)
		}
	})

	t.Run("an add keeps its value only where there's none", func(t *testing.T) {
		s := open(t)
		if !add(t, s, "k", "first", t0.Add(time.Minute), t0) {
			t.Fatal("an add to a key with no value didn't keep it")
		}
		has(t, s, "k", t0, "first")
		if add(t, s, "k", "second", t0.Add(2*time.Minute), t0.Add(30*time.Second)) {
			t.Error("an add replaced a value that hadn't expired")
		}
		has(t, s, "k", t0.Add(30*time.Second), "first")
		// The instant the value there expires, it's gone, for an add too.
		if !add(t, s, "k", "third", t0.Add(2*time.Minute), t0.Add(time.Minute)) {
			t.Error("an add didn't replace a value that had expired")
		}
		has(t, s, "k", t0.Add(time.Minute), "third")
		set(t, s, "j", "set", t0.Add(time.Hour))
		if add(t, s, "j", "added", t0.Add(time.Hour), t0) {
			t.Error("an add replaced a value that was set")
		}
	})

	t.Run("of adds at once, one keeps its value", func(t *testing.T) {
		s := open(t)
		const at = 20
		var mu sync.Mutex
		var kept []string
		var wg sync.WaitGroup
		for i := range at {
			wg.Go(func() {
				v := strconv.Itoa(i)
				ok, err := s.Add(ctx, key("k"), []byte(v), t0.Add(time.Minute), t0)
				if err != nil {
					t.Errorf("Add: %v", err)
					return
				}
				if ok {
					mu.Lock()
					kept = append(kept, v)
					mu.Unlock()
				}
			})
		}
		wg.Wait()
		if len(kept) != 1 {
			t.Fatalf("of %d adds at once, %d kept their values: %v", at, len(kept), kept)
		}
		has(t, s, "k", t0, kept[0])
	})

	t.Run("a delete if it's the value deletes only that value", func(t *testing.T) {
		s := open(t)
		add(t, s, "k", "mine", t0.Add(time.Minute), t0)
		if deleteIf(t, s, "k", "theirs") {
			t.Error("deleted another's value")
		}
		has(t, s, "k", t0, "mine")
		if !deleteIf(t, s, "k", "mine") {
			t.Error("didn't delete the value it was")
		}
		has(t, s, "k", t0, "")
		if deleteIf(t, s, "k", "mine") {
			t.Error("deleted a value that wasn't there")
		}
		if !add(t, s, "k", "next", t0.Add(time.Minute), t0) {
			t.Error("an add after the delete didn't keep its value")
		}
	})
}
