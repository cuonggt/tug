package auth

import (
	"sync"
	"time"
)

// Throttle limits attempts at something by a key, such as logins by the
// email tried and the address they came from: after Max tries within
// Window, the key waits for the Window to end.
//
//	if wait := logins.Try(key); wait > 0 {
//		// too many; try again in wait
//	}
//	if !auth.CheckPassword(hash, password) {
//		// wrong: the try counts
//	}
//	logins.Clear(key)
//
// It counts in memory, so the counts are this process's own and start
// again when it restarts. The zero value is ready to use.
type Throttle struct {
	// Max is how many tries a key has within the Window. Default 5.
	Max int

	// Window is how long a key's tries count for, from the first of them.
	// Default 1 minute.
	Window time.Duration

	mu      sync.Mutex
	keys    map[string]*tries
	sweepAt int
	now     func() time.Time
}

type tries struct {
	count int
	until time.Time // when the count starts again
}

// Try counts a try for key, such as a login about to check a password, and
// returns 0. When key has had its Max tries within the Window, it returns
// how long until the Window ends instead, and the try, which shouldn't be
// made, doesn't count. Checking and counting are one step, so tries sent
// at the same moment can't all get in under Max. Clear the key after a try
// that succeeds.
func (t *Throttle) Try(key string) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.clock()
	if t.keys == nil {
		t.keys = map[string]*tries{}
	}
	k := t.keys[key]
	if k == nil || !now.Before(k.until) {
		k = &tries{until: now.Add(t.window())}
		t.keys[key] = k
		t.sweep(now)
	}
	if k.count >= t.limit() {
		return k.until.Sub(now)
	}
	k.count++
	return 0
}

// Wait returns how long key has to wait before its next try, or 0 when it
// can try now, without counting a try.
func (t *Throttle) Wait(key string) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	k, now := t.keys[key], t.clock()
	if k == nil || !now.Before(k.until) || k.count < t.limit() {
		return 0
	}
	return k.until.Sub(now)
}

// Clear forgets key's tries, as after a login that succeeds.
func (t *Throttle) Clear(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.keys, key)
}

// sweep drops the keys whose window is over, once there are twice as many
// as last time, so the counts from long ago don't pile up.
func (t *Throttle) sweep(now time.Time) {
	if len(t.keys) < max(t.sweepAt, 1024) {
		return
	}
	for key, k := range t.keys {
		if !now.Before(k.until) {
			delete(t.keys, key)
		}
	}
	t.sweepAt = 2 * len(t.keys)
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
