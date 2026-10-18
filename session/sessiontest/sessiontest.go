// Package sessiontest is for testing with package session: TestStore
// checks that a session.Keeper keeps the promises a session.Store depends
// on.
package sessiontest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/cuonggt/tug/session"
)

// TestStore checks that the keepers open returns keep the promises
// session.Store depends on, with a new, empty one for each test. A
// keeper's own tests call it, as the auth starter's do for its table in
// its database:
//
//	func TestTheSessionsTableKeepsItsPromises(t *testing.T) {
//		sessiontest.TestStore(t, func(t *testing.T) session.Keeper {
//			return &sessionTable{db: testDB(t)}
//		})
//	}
func TestStore(t *testing.T, open func(t *testing.T) session.Keeper) {
	// Whole milliseconds, which any keeper can keep.
	t0 := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()
	key := func(s string) []byte {
		sum := sha256.Sum256([]byte(s))
		return sum[:]
	}
	set := func(t *testing.T, k session.Keeper, id, value string, expires time.Time) {
		t.Helper()
		if err := k.Set(ctx, key(id), []byte(value), expires); err != nil {
			t.Fatalf("Set: %v", err)
		}
	}
	// has checks the session under id at now, where "" is none.
	has := func(t *testing.T, k session.Keeper, id string, now time.Time, want string) {
		t.Helper()
		v, ok, err := k.Get(ctx, key(id), now)
		switch {
		case err != nil:
			t.Fatalf("Get: %v", err)
		case want == "" && ok:
			t.Errorf("%s at %v: %q, want none", id, now, v)
		case want != "" && (!ok || string(v) != want):
			t.Errorf("%s at %v: %q, %v; want %q", id, now, v, ok, want)
		}
	}

	t.Run("a session is kept until it expires", func(t *testing.T) {
		k := open(t)
		set(t, k, "id", "the session", t0.Add(time.Hour))
		has(t, k, "id", t0, "the session")
		has(t, k, "id", t0.Add(time.Hour-time.Millisecond), "the session")
		// The instant it expires, it's gone, so a session the app has
		// stopped keeping alive logs out at the time it says.
		has(t, k, "id", t0.Add(time.Hour), "")
		has(t, k, "id", t0.Add(24*time.Hour), "")
	})

	t.Run("a session set again replaces the one there, and its time", func(t *testing.T) {
		k := open(t)
		set(t, k, "id", "as it was", t0.Add(time.Hour))
		set(t, k, "id", "as it is", t0.Add(time.Minute))
		has(t, k, "id", t0, "as it is")
		has(t, k, "id", t0.Add(2*time.Minute), "")
	})

	t.Run("sessions are kept apart", func(t *testing.T) {
		k := open(t)
		set(t, k, "mine", "for mine", t0.Add(time.Hour))
		set(t, k, "theirs", "for theirs", t0.Add(time.Hour))
		has(t, k, "mine", t0, "for mine")
		has(t, k, "theirs", t0, "for theirs")
		has(t, k, "another", t0, "")
	})

	t.Run("a session is kept as it was given, every byte of it", func(t *testing.T) {
		k := open(t)
		// A session is sealed before it's kept, so it's bytes, not text,
		// and 64 KB of it, more than a cookie would take by far.
		var all []byte
		for b := range 256 {
			all = append(all, byte(b))
		}
		want := bytes.Repeat(all, 256)
		if err := k.Set(ctx, key("id"), want, t0.Add(time.Hour)); err != nil {
			t.Fatalf("Set: %v", err)
		}
		got, ok, err := k.Get(ctx, key("id"), t0)
		if err != nil || !ok || !bytes.Equal(got, want) {
			t.Errorf("Get: %d bytes, %v, %v; want the %d set, as they were", len(got), ok, err, len(want))
		}
	})

	t.Run("a session deleted is gone, and logs its browser out", func(t *testing.T) {
		k := open(t)
		set(t, k, "mine", "for mine", t0.Add(time.Hour))
		set(t, k, "theirs", "for theirs", t0.Add(time.Hour))
		if err := k.Delete(ctx, key("mine")); err != nil {
			t.Fatal(err)
		}
		has(t, k, "mine", t0, "")
		has(t, k, "theirs", t0, "for theirs")
		if err := k.Delete(ctx, key("none")); err != nil {
			t.Errorf("deleting a session that isn't there: %v", err)
		}
	})

	t.Run("sessions are kept from several goroutines at once", func(t *testing.T) {
		k := open(t)
		// A keeper is one instance's, and every request of it writes.
		const at = 20
		var wg sync.WaitGroup
		for i := range at {
			wg.Go(func() {
				id := strconv.Itoa(i)
				if err := k.Set(ctx, key(id), []byte("for "+id), t0.Add(time.Hour)); err != nil {
					t.Errorf("Set: %v", err)
					return
				}
				if _, _, err := k.Get(ctx, key(id), t0); err != nil {
					t.Errorf("Get: %v", err)
				}
			})
		}
		wg.Wait()
		for i := range at {
			id := strconv.Itoa(i)
			has(t, k, id, t0, "for "+id)
		}
	})
}

// Memory is a session.Keeper that keeps the sessions in memory, for
// tests: they go when the program does. The zero value is an empty
// keeper.
type Memory struct {
	mu   sync.Mutex
	rows map[string]memoryRow
}

type memoryRow struct {
	value   []byte
	expires time.Time
}

func (m *Memory) Get(_ context.Context, key []byte, now time.Time) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.rows[string(key)]
	if !ok || !now.Before(row.expires) {
		return nil, false, nil
	}
	return row.value, true, nil
}

func (m *Memory) Set(_ context.Context, key, value []byte, expires time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rows == nil {
		m.rows = map[string]memoryRow{}
	}
	m.rows[string(key)] = memoryRow{value: bytes.Clone(value), expires: expires}
	return nil
}

func (m *Memory) Delete(_ context.Context, key []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rows, string(key))
	return nil
}

// Keys returns the keys of the sessions it holds, in no order, for a test
// that watches a session move from one ID to another, as a login's does.
func (m *Memory) Keys() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([][]byte, 0, len(m.rows))
	for key := range m.rows {
		keys = append(keys, []byte(key))
	}
	return keys
}
