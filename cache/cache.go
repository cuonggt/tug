// Package cache keeps what's slow to work out, such as a dashboard's
// counts or what another service answered, for a while, in a store every
// instance of the app shares, such as a table in its database, and has
// locks for what mustn't run twice at once, on any of them.
//
//	c := &cache.Cache{Store: table} // without a Store, in memory
//
//	stats, err := cache.Remember(ctx, c, "stats", time.Hour, func(ctx context.Context) (Stats, error) {
//		return db.stats(ctx) // only when there's no value kept, or it has expired
//	})
//
//	lock := c.Lock("import", 10*time.Minute)
//	ok, err := lock.Try(ctx)
//	if err != nil {
//		return err
//	}
//	if !ok {
//		// someone, on this instance or another, is importing
//	}
//	defer lock.Release(context.WithoutCancel(ctx))
//
// Values go through encoding/json on their way, as a job's payload does,
// and come back as the type they're read into.
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Cache keeps values under keys, each until it expires, in its Store:
// Set keeps one, Get reads one, Delete drops one, and Remember reads one,
// or works it out and keeps it. A key is any string the app likes, such as
// "stats" or "posts:42". Lock is a lock that every instance of the app on
// the Store sees. The zero value keeps its values in memory; share one by
// pointer.
type Cache struct {
	// Store keeps the values, where every instance of the app finds them,
	// such as a table in its database. Nil keeps them in memory, this
	// process's own, which a restart empties.
	Store Store

	mu      sync.Mutex
	memory  *memoryStore
	flights map[string]*flight // what Remember is working out, by key
	now     func() time.Time
}

// Store keeps a Cache's values where every instance of the app finds
// them, such as a table in its database. A key is 32 bytes, a SHA-256 the
// Cache makes of the key it was given, so the store keeps no email, say,
// that a key has in it. A value is bytes, which the store keeps as they
// are. The times are the app's clock, which a Cache passes in. Its methods
// are called from several goroutines at once, and with a database, from
// several instances. Package cachetest checks that a store keeps these
// promises.
type Store interface {
	// Get returns the value under key, and true, or false for a key with
	// none, or whose value has expired by now.
	Get(ctx context.Context, key []byte, now time.Time) (value []byte, ok bool, err error)

	// Set keeps value under key until expires, in place of any value there.
	Set(ctx context.Context, key, value []byte, expires time.Time) error

	// Add keeps value under key until expires, as Set does, but only when
	// the key has no value at now, or one that has expired, and reports
	// whether it did. Looking and keeping are one step: of adds at once,
	// one keeps its value. It's how a lock is taken.
	Add(ctx context.Context, key, value []byte, expires, now time.Time) (bool, error)

	// Delete removes the value under key. A key with none isn't an error.
	Delete(ctx context.Context, key []byte) error

	// DeleteIf removes the value under key, expired or not, only while
	// it's value, and reports whether it did. Looking and deleting are one
	// step. It's how a lock's holder lets go of it, and not of the lock
	// another has taken since the holder's time ran out.
	DeleteIf(ctx context.Context, key, value []byte) (bool, error)
}

// Set keeps value under key for ttl, as JSON, in place of any value there.
// A value encoding/json can't encode is an error, as is the store's. A ttl
// of 0 or less panics: a value kept for good is one nothing drops.
func (c *Cache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	mustLive(ttl)
	b, err := encode(key, value)
	if err != nil {
		return err
	}
	return c.store().Set(ctx, hash("value", key), b, c.clock().Add(ttl))
}

// Get reads the value kept under key into a T, and returns true, or false
// when there's none, it has expired, or it isn't a T's JSON, as a value an
// older build of the app kept, of a type since changed, may not be: it's
// one to work out again. The error is the store's.
//
//	stats, ok, err := cache.Get[Stats](ctx, c, "stats")
func Get[T any](ctx context.Context, c *Cache, key string) (T, bool, error) {
	b, ok, err := c.store().Get(ctx, hash("value", key), c.clock())
	var value T
	if err != nil || !ok || json.Unmarshal(b, &value) != nil {
		var zero T
		return zero, false, err
	}
	return value, true, nil
}

// Delete drops the value kept under key, as when what it was worked out
// from changes. A key with none isn't an error.
func (c *Cache) Delete(ctx context.Context, key string) error {
	return c.store().Delete(ctx, hash("value", key))
}

// Remember returns the value kept under key, or else runs fn, keeps what
// it returns for ttl, and returns that:
//
//	stats, err := cache.Remember(ctx, c, "stats", time.Hour, a.stats)
//
// fn runs once at a time for a key in this process: the callers that want
// the key while it runs wait for its value, rather than each run it, as a
// page that many ask for at once, the moment its value has expired, would.
// Other instances may run it too, once each; where that costs more than
// waiting, fn takes a Lock.
//
// A store that fails is no value: fn runs, and its value is returned, with
// the store's error in the log, as the app works without its cache, only
// slower. fn's error is Remember's, and nothing is kept, as for a value
// encoding/json can't encode. A ttl of 0 or less panics, as Set's does.
func Remember[T any](ctx context.Context, c *Cache, key string, ttl time.Duration, fn func(ctx context.Context) (T, error)) (T, error) {
	mustLive(ttl)
	c.mu.Lock()
	if f, ok := c.flights[key]; ok {
		c.mu.Unlock()
		select {
		case <-f.done:
		case <-ctx.Done():
			var zero T
			return zero, ctx.Err()
		}
		var value T
		if f.value != nil && json.Unmarshal(f.value, &value) == nil {
			return value, nil
		}
		// It failed, or it's another type's: this caller works it out on
		// its own, as the one that ran it did.
		return remember(ctx, c, key, ttl, fn, nil)
	}
	f := &flight{done: make(chan struct{})}
	if c.flights == nil {
		c.flights = map[string]*flight{}
	}
	c.flights[key] = f
	c.mu.Unlock()
	// Deferred, so that the callers waiting are let go when fn panics too.
	defer func() {
		c.mu.Lock()
		delete(c.flights, key)
		c.mu.Unlock()
		close(f.done)
	}()
	return remember(ctx, c, key, ttl, fn, f)
}

// flight is a value Remember is working out, which the callers that want
// its key meanwhile wait for: value is its JSON, or nil when there's none.
type flight struct {
	done  chan struct{}
	value []byte
}

// remember reads key's value from the store, or else works it out with fn
// and keeps it, and gives f, when there is one, its JSON, for the callers
// that wait on it.
func remember[T any](ctx context.Context, c *Cache, key string, ttl time.Duration, fn func(context.Context) (T, error), f *flight) (T, error) {
	h := hash("value", key)
	b, ok, err := c.store().Get(ctx, h, c.clock())
	if err != nil {
		logFailure(ctx, "cache: a value couldn't be read, so it was worked out again", err)
	}
	var value T
	if ok && json.Unmarshal(b, &value) == nil {
		f.give(b)
		return value, nil
	}
	value, err = fn(ctx)
	if err == nil {
		b, err = encode(key, value)
	}
	if err != nil {
		var zero T
		return zero, err
	}
	// The value's time starts once it's worked out, however long that took.
	if err := c.store().Set(ctx, h, b, c.clock().Add(ttl)); err != nil {
		logFailure(ctx, "cache: a value couldn't be kept", err)
	}
	f.give(b)
	return value, nil
}

func (f *flight) give(b []byte) {
	if f != nil {
		f.value = b
	}
}

// logFailure logs what the store couldn't do, unless ctx is done, which is
// why: a client that went away, say, is no failure of the store's.
func logFailure(ctx context.Context, msg string, err error) {
	if ctx.Err() == nil {
		slog.ErrorContext(ctx, msg, "err", err)
	}
}

// encode is value as JSON, for keeping under key.
func encode(key string, value any) ([]byte, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("cache: the value for %q can't be kept, as JSON can't hold it: %w", key, err)
	}
	return b, nil
}

// hash is what a store keeps name under: the SHA-256 of kind, "value" or
// "lock", and name, with a NUL between them, which neither kind has, so a
// value and a lock never share a key, whatever they're called.
func hash(kind, name string) []byte {
	sum := sha256.Sum256([]byte(kind + "\x00" + name))
	return sum[:]
}

// mustLive panics for a time to live that isn't one.
func mustLive(ttl time.Duration) {
	if ttl <= 0 {
		panic(fmt.Sprintf("cache: a value or a lock is kept for a time, such as time.Hour, not %v: one kept for good is one nothing drops", ttl))
	}
}

// store is the Store, or else the cache's own memory.
func (c *Cache) store() Store {
	if c.Store != nil {
		return c.Store
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.memory == nil {
		c.memory = &memoryStore{}
	}
	return c.memory
}

func (c *Cache) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}
