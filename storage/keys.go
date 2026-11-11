package storage

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Keys are what an S3 signs with: an access key ID and its secret, and a
// session token with temporary keys, which expire. A role's keys are
// temporary, and its platform has the next ones before they do.
type Keys struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string

	// Expires is when temporary keys stop working: zero for keys that
	// don't, as an IAM user's.
	Expires time.Time
}

// A KeySource gives an S3 the keys it signs with, each time it signs: an
// S3 with no AccessKeyID of its own asks its KeySource. One of temporary
// keys fetches the next before they expire, as WebIdentity, ContainerRole
// and InstanceRole do, the places AWS's platforms put a role's keys; an
// app's own, as a vault's, is a KeySource too.
type KeySource interface {
	Keys(ctx context.Context) (Keys, error)
}

// Keys are fetched again five minutes before they expire: EC2 has the next
// ones by then, as it promises, and AWS's SDK for Go asks a container's
// endpoint for them then too.
const fetchBefore = 5 * time.Minute

// A fetch that failed is tried again a few seconds on, not by every
// request meanwhile, which would each wait on a source that's down.
const fetchAgainAfter = 5 * time.Second

// fetched is the keys a source last fetched, and the fetch under way: the
// first signing that finds them due fetches the next, and the others sign
// with the keys there are meanwhile, or, with none that work, wait for it.
type fetched struct {
	mu       sync.Mutex
	keys     Keys
	fetching chan struct{} // closed as the fetch under way ends
	failed   error         // the last fetch's, until one works
	again    time.Time     // when the next fetch may be, after one failed
}

// get returns the keys, fetching them by fetch first when they're due, as
// of now.
func (f *fetched) get(ctx context.Context, now func() time.Time, fetch func(context.Context) (Keys, error)) (Keys, error) {
	for {
		f.mu.Lock()
		t := now()
		keys, usable := f.keys, works(f.keys, t)
		if usable && (keys.Expires.IsZero() || t.Before(keys.Expires.Add(-fetchBefore))) {
			f.mu.Unlock()
			return keys, nil
		}
		if f.fetching != nil {
			done := f.fetching
			f.mu.Unlock()
			if usable {
				return keys, nil
			}
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return Keys{}, ctx.Err()
			}
		}
		if t.Before(f.again) && (usable || f.failed != nil) {
			failed := f.failed
			f.mu.Unlock()
			if usable {
				return keys, nil
			}
			return Keys{}, failed
		}
		done := make(chan struct{})
		f.fetching = done
		f.mu.Unlock()

		next, err := fetch(ctx)
		if err == nil && !works(next, now()) {
			err = fmt.Errorf("storage: the keys fetched for the S3 expired at %s, before they came", next.Expires.Format(time.RFC3339))
		}

		f.mu.Lock()
		f.fetching = nil
		close(done)
		t = now()
		switch {
		case err == nil:
			f.keys, f.failed, f.again = next, nil, time.Time{}
			if !next.Expires.IsZero() && !t.Before(next.Expires.Add(-fetchBefore)) {
				// Keys that are due already, as a source's that hasn't the
				// next ones yet, sign until it has, asked again a few
				// seconds on.
				f.again = t.Add(fetchAgainAfter)
			}
		case ctx.Err() == nil:
			// The source failed, rather than the signing giving up on it.
			f.failed, f.again = err, t.Add(fetchAgainAfter)
		}
		keys = f.keys
		f.mu.Unlock()
		if err == nil {
			return next, nil
		}
		if works(keys, t) {
			slog.WarnContext(ctx, "the next keys for the S3 couldn't be fetched, so it signs with the ones it has until they expire", "expires", keys.Expires, "err", err)
			return keys, nil
		}
		return Keys{}, err
	}
}

// works reports whether k signs at t: they're keys, and haven't expired.
func works(k Keys, t time.Time) bool {
	return k.AccessKeyID != "" && k.SecretAccessKey != "" && (k.Expires.IsZero() || t.Before(k.Expires))
}

// sessions is when this process first had each set of a session's keys
// it has signed a link with, by its access key ID: a link of theirs is
// signed as of then, after the session began.
var sessions = struct {
	sync.Mutex
	since map[string]time.Time
}{since: map[string]time.Time{}}

// firstHad is when the session keys of id were first had, now when it's
// the first time.
func firstHad(id string, now time.Time) time.Time {
	sessions.Lock()
	defer sessions.Unlock()
	if since, ok := sessions.since[id]; ok {
		return since
	}
	// No session lasts a week, so keys first had a week ago have expired,
	// and the links they signed with them.
	for other, since := range sessions.since {
		if now.Sub(since) > longestLink {
			delete(sessions.since, other)
		}
	}
	since := now.Truncate(time.Second)
	sessions.since[id] = since
	return since
}

// clockOf is now, or time.Now unless a test set it.
func clockOf(now func() time.Time) func() time.Time {
	if now != nil {
		return now
	}
	return time.Now
}
