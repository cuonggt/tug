package cache

import (
	"context"
	"crypto/rand"
	"time"
)

// Lock is a lock under a name, which every instance of the app on its
// cache's Store sees: one holder has it at a time, until it lets go, or
// its time is up, as it is for an instance that died holding it. Each
// holder asks the cache for its own Lock.
//
//	lock := c.Lock("import", 10*time.Minute)
//	if err := lock.Wait(ctx); err != nil {
//		return err
//	}
//	defer lock.Release(context.WithoutCancel(ctx))
type Lock struct {
	c     *Cache
	key   []byte
	ttl   time.Duration
	owner []byte // who holds it, as the store keeps it: random, this Lock's
}

// Lock returns the lock called name, such as "import" or "report:42",
// held for ttl once it's taken: long enough for the work it guards, as a
// holder whose work runs past its time may find another has taken the
// lock. A ttl of 0 or less panics, as Set's does.
func (c *Cache) Lock(name string, ttl time.Duration) *Lock {
	mustLive(ttl)
	owner := make([]byte, 16)
	rand.Read(owner)
	return &Lock{c: c, key: hash("lock", name), ttl: ttl, owner: owner}
}

// Try takes the lock, and returns true, when no one holds it: it's free,
// or the time of whoever held it is up. It returns false at once when
// someone has it, this Lock too. The error is the store's: a lock that
// can't be taken for one is neither taken nor free.
func (l *Lock) Try(ctx context.Context) (bool, error) {
	now := l.c.clock()
	return l.c.store().Add(ctx, l.key, l.owner, now.Add(l.ttl), now)
}

// retry is how often Wait asks again for a lock someone has, as Laravel's
// block does.
var retry = 250 * time.Millisecond

// Wait takes the lock, asking again every 250 milliseconds while someone
// has it, until it's taken or ctx is done, whose error it returns then:
// context.WithTimeout sets how long to wait. Another error is the store's.
func (l *Lock) Wait(ctx context.Context) error {
	for {
		ok, err := l.Try(ctx)
		if err != nil || ok {
			return err
		}
		select {
		case <-time.After(retry):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Release lets go of the lock, if this Lock has it. A lock whose time ran
// out, and that another holder took since, is theirs, and stays. The error
// is the store's. Give it a context that isn't done, as
// context.WithoutCancel(ctx) isn't once a command's is canceled: with one
// that's done, the store can't be asked, and the lock stays until its time
// is up.
func (l *Lock) Release(ctx context.Context) error {
	_, err := l.c.store().DeleteIf(ctx, l.key, l.owner)
	return err
}
