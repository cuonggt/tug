package auth

import (
	"context"
	"crypto/sha256"
	"sync"
	"time"
)

// Throttle limits attempts at something by a key, such as logins by the
// email tried and the address they came from: after Max tries within
// Window, the key waits for the Window to end.
//
//	wait, err := logins.Try(ctx, key)
//	if err != nil {
//		return err
//	}
//	if wait > 0 {
//		// too many; try again in wait
//	}
//	if !auth.CheckPassword(hash, password) {
//		// wrong: the try counts
//	}
//	err = logins.Clear(ctx, key)
//
// Without a Store, it counts in memory, so the counts are this process's
// own and start again when it restarts. With one, such as a table in the
// app's database, every instance of the app counts in it. The zero value
// is ready to use; share one by pointer.
type Throttle struct {
	// Name is what the throttle's keys are counted under in a Store it
	// shares with others, such as "logins": two that count by user ID
	// would otherwise count as one. In memory, each Throttle has its own
	// counts, and needs none.
	Name string

	// Max is how many tries a key has within the Window. Default 5.
	Max int

	// Window is how long a key's tries count for, from the first of them.
	// Default 1 minute.
	Window time.Duration

	// Store keeps the counts, where every instance of the app counts. Nil
	// keeps them in memory.
	Store ThrottleStore

	mu     sync.Mutex
	memory *memoryThrottles
	now    func() time.Time
}

// ThrottleStore keeps Throttles' counts where every instance of the app
// counts, such as a table in its database. A key is 32 bytes: the SHA-256
// of a throttle's name and a key it counts by, so the store keeps no
// email or address that a key was made of. The times are the app's clock,
// which a Throttle passes in. Its methods are called from several
// goroutines at once, and with a database, from several instances.
// Package throttletest checks that a store keeps these promises.
type ThrottleStore interface {
	// Hit counts a try for key at now, and returns the tries counted in
	// its window, this one included, and when the window ends. A key
	// whose window has ended by now, or that has none, starts one at now,
	// which ends window later. Counting and reading are one step: of tries
	// at once, each gets a count of its own.
	Hit(ctx context.Context, key []byte, now time.Time, window time.Duration) (tries int, ends time.Time, err error)

	// Tries returns key's tries at now, without counting one, and when
	// their window ends: 0 and the zero time for a key that has none, or
	// whose window has ended.
	Tries(ctx context.Context, key []byte, now time.Time) (tries int, ends time.Time, err error)

	// Clear forgets key's tries. A key with none isn't an error.
	Clear(ctx context.Context, key []byte) error
}

// Try counts a try for key, such as a login about to check a password, and
// returns 0. When key has had its Max tries within the Window, it returns
// how long until the Window ends instead: the try shouldn't be made.
// Checking and counting are one step, so tries sent at the same moment
// can't all get in under Max. Clear the key after a try that succeeds. The
// error is the Store's, when it can't count: a try that isn't counted is
// the caller's to answer, rather than let in or turned away.
func (t *Throttle) Try(ctx context.Context, key string) (time.Duration, error) {
	now := t.clock()
	tries, ends, err := t.store().Hit(ctx, t.hash(key), now, t.window())
	if err != nil || tries <= t.limit() {
		return 0, err
	}
	return ends.Sub(now), nil
}

// Wait returns how long key has to wait before its next try, or 0 when it
// can try now, without counting a try.
func (t *Throttle) Wait(ctx context.Context, key string) (time.Duration, error) {
	now := t.clock()
	tries, ends, err := t.store().Tries(ctx, t.hash(key), now)
	if err != nil || tries < t.limit() || !now.Before(ends) {
		return 0, err
	}
	return ends.Sub(now), nil
}

// Clear forgets key's tries, as after a login that succeeds.
func (t *Throttle) Clear(ctx context.Context, key string) error {
	return t.store().Clear(ctx, t.hash(key))
}

// hash is the key a store counts key under: the SHA-256 of the throttle's
// name and key, a NUL between them, which neither has.
func (t *Throttle) hash(key string) []byte {
	sum := sha256.Sum256([]byte(t.Name + "\x00" + key))
	return sum[:]
}

// store is the Store, or else the throttle's own memory.
func (t *Throttle) store() ThrottleStore {
	if t.Store != nil {
		return t.Store
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.memory == nil {
		t.memory = &memoryThrottles{}
	}
	return t.memory
}

func (t *Throttle) limit() int {
	if t.Max <= 0 {
		return 5
	}
	return t.Max
}

func (t *Throttle) window() time.Duration {
	if t.Window <= 0 {
		return time.Minute
	}
	return t.Window
}

func (t *Throttle) clock() time.Time {
	if t.now != nil {
		return t.now()
	}
	return time.Now()
}

// memoryThrottles is the ThrottleStore of a Throttle without one: its
// counts in a map.
type memoryThrottles struct {
	mu      sync.Mutex
	keys    map[string]*tries
	sweepAt int
}

type tries struct {
	count int
	until time.Time // when the count starts again
}

func (m *memoryThrottles) Hit(_ context.Context, key []byte, now time.Time, window time.Duration) (int, time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.keys == nil {
		m.keys = map[string]*tries{}
	}
	k := m.keys[string(key)]
	if k == nil || !now.Before(k.until) {
		k = &tries{until: now.Add(window)}
		m.keys[string(key)] = k
		m.sweep(now)
	}
	k.count++
	return k.count, k.until, nil
}

func (m *memoryThrottles) Tries(_ context.Context, key []byte, now time.Time) (int, time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := m.keys[string(key)]
	if k == nil || !now.Before(k.until) {
		return 0, time.Time{}, nil
	}
	return k.count, k.until, nil
}

func (m *memoryThrottles) Clear(_ context.Context, key []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.keys, string(key))
	return nil
}

// sweep drops the keys whose window is over, once there are twice as many
// as last time, so the counts from long ago don't pile up.
func (m *memoryThrottles) sweep(now time.Time) {
	if len(m.keys) < max(m.sweepAt, 1024) {
		return
	}
	for key, k := range m.keys {
		if !now.Before(k.until) {
			delete(m.keys, key)
		}
	}
	m.sweepAt = 2 * len(m.keys)
}
