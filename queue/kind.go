package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"runtime/debug"
	"time"
)

// Kind is a kind of job, as Handle made it: the name its jobs are kept
// by, and the handler that runs them. Its Push puts one on the queue.
type Kind[T any] struct {
	q    *Queue
	name string
	h    *handler

	// store is where its jobs are pushed: the queue's Store, or one In
	// made from a transaction of the app's, which Push doesn't wake the
	// queue for, as its workers can't see the job until the commit.
	store Store
	in    bool
}

// handler runs one kind of job.
type handler struct {
	run        func(ctx context.Context, payload []byte) error
	attempts   int
	timeout    time.Duration
	backoff    func(attempt int) time.Duration
	unique     bool
	latest     bool
	oneAtATime bool
	atOnce     int
	rate       Limiter

	// onFail is OnFail's function, given the payload, and onFailType the
	// value it takes, which Handle checks is the kind's.
	onFail     func(ctx context.Context, payload []byte, err error) error
	onFailType reflect.Type
}

// needs panics unless s is the Store the kind named name needs to push
// its jobs to: one that keeps keys for a Unique kind, and moves its jobs
// or keeps them from running at once for its options, and one that keeps
// an AtOnce kind's limit.
func (h *handler) needs(name string, s Store) {
	if _, ok := s.(UniqueStore); h.unique && !ok {
		panic(fmt.Sprintf("queue: the unique jobs of kind %q need a Store that keeps their keys, a UniqueStore, and %T isn't one", name, s))
	}
	if _, ok := s.(LatestStore); h.latest && !ok {
		panic(fmt.Sprintf("queue: the jobs of kind %q move to the latest push's time, which needs a Store that moves them, a LatestStore, and %T isn't one", name, s))
	}
	if _, ok := s.(OneAtATimeStore); h.oneAtATime && !ok {
		panic(fmt.Sprintf("queue: the jobs of kind %q run one at a time, which needs a Store that keeps them from running at once, a OneAtATimeStore, and %T isn't one", name, s))
	}
	if _, ok := s.(AtOnceStore); h.atOnce > 0 && !ok {
		panic(fmt.Sprintf("queue: the jobs of kind %q run %d at once at most, which needs a Store that counts the ones running, an AtOnceStore, and %T isn't one", name, h.atOnce, s))
	}
}

// Handle gives q the handler for the jobs of kind name, which runs fn with
// the value each was pushed with, and returns the Kind to push them with.
// The value is kept as JSON, so T is a type encoding/json turns into JSON
// and back: IDs, say, rather than the user they name, who may have
// changed by the time the job runs. The name is what the Store keeps to
// find the handler by, so it stays the same from one version of the app
// to the next: jobs one pushes, the next may run.
//
// Handle panics when q has a handler for name already, or once q runs, or
// when q's Store isn't what the kind's options need: a UniqueStore for a
// Unique kind, and the extra its UniqueOptions need, an AtOnceStore for
// AtOnce, and a HoldBackStore for Rate. It panics for an OnFail that
// doesn't take T.
func Handle[T any](q *Queue, name string, fn func(ctx context.Context, v T) error, opts ...Option) *Kind[T] {
	if name == "" || fn == nil {
		panic("queue: Handle takes a name and a function")
	}
	h := &handler{attempts: defaultAttempts, timeout: defaultTimeout, backoff: backoff}
	for _, opt := range opts {
		opt(h)
	}
	h.needs(name, q.store)
	if _, ok := q.store.(HoldBackStore); h.rate != nil && !ok {
		panic(fmt.Sprintf("queue: the jobs of kind %q start at a rate, which needs a Store that holds them back, a HoldBackStore, and %T isn't one", name, q.store))
	}
	if t := reflect.TypeFor[T](); h.onFailType != nil && h.onFailType != t {
		panic(fmt.Sprintf("queue: OnFail of the jobs of kind %q takes their value, a %v, not a %v", name, t, h.onFailType))
	}
	h.run = func(ctx context.Context, payload []byte) error {
		var v T
		if err := json.Unmarshal(payload, &v); err != nil {
			return Permanent(fmt.Errorf("its payload doesn't fit a %T: %w", v, err))
		}
		return fn(ctx, v)
	}

	q.mu.Lock()
	defer q.mu.Unlock()
	if q.started {
		panic(fmt.Sprintf("queue: Handle(%q) after Run has started; give the queue its handlers first", name))
	}
	if _, ok := q.handlers[name]; ok {
		panic(fmt.Sprintf("queue: the jobs of kind %q have a handler already", name))
	}
	q.handlers[name] = h
	return &Kind[T]{q: q, name: name, h: h, store: q.store}
}

// Push puts a job on the queue, to run as soon as a worker is free.
func (k *Kind[T]) Push(ctx context.Context, v T) error {
	return k.PushAt(ctx, k.q.now(), v)
}

// PushAt puts a job on the queue, to run at at, or as soon after as a
// worker is free. For a Unique kind, it does nothing while a job with the
// same value waits, which keeps its own time, or with Latest, gives that
// job its own time.
func (k *Kind[T]) PushAt(ctx context.Context, at time.Time, v T) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("queue: a %s job: %w", k.name, err)
	}
	j := &Job{Kind: k.name, Payload: payload, RunAt: at, OneAtATime: k.h.oneAtATime, AtOnce: k.h.atOnce}
	if j.Carried, err = k.q.carried(ctx, k.store); err != nil {
		return fmt.Errorf("queue: pushing a %s job: %w", k.name, err)
	}
	switch {
	case k.h.latest:
		j.Key = keyOf(payload)
		err = k.store.(LatestStore).PushLatest(ctx, j)
	case k.h.unique:
		j.Key = keyOf(payload)
		_, err = k.store.(UniqueStore).PushUnique(ctx, j)
	default:
		err = k.store.Push(ctx, j)
	}
	if err != nil {
		return fmt.Errorf("queue: pushing a %s job: %w", k.name, err)
	}
	if !k.in && !at.After(k.q.now()) {
		k.q.poke()
	}
	return nil
}

// In is the kind, pushing its jobs to s rather than to the queue's Store:
// one the app makes from a transaction of its own, as the auth starter's
// jobs.in does, so that a job is kept with what else the transaction
// writes, as it commits, or not at all, as it rolls back. The queue's
// workers can't see such a job until the commit, so its Push doesn't wake
// them: the queue's Wake, after the commit, does, or else the next Poll
// finds the job.
//
// In panics when s isn't what the kind needs of a Store, as Handle does
// of the queue's: a UniqueStore for a Unique kind, say.
func (k *Kind[T]) In(s Store) *Kind[T] {
	if s == nil {
		panic(fmt.Sprintf("queue: In(nil) for the jobs of kind %q: they need a Store to go in", k.name))
	}
	k.h.needs(k.name, s)
	in := *k
	in.store, in.in = s, true
	return &in
}

// Schedule runs a job of this kind on its own, with v, at each time s
// names: queue.Cron("0 2 * * *") at 2:00 each day, say. Each run is pushed
// ahead, as a job due at its time, once the run before it has come round,
// so a run that comes due while the app is down runs as it starts again:
// once, however many runs it missed. A new schedule's first run is its
// next time, not one that has passed. Every instance of the app that runs
// the queue pushes the runs, and the Store sees that each is pushed once:
// Schedule needs a ScheduleStore.
//
// Schedule panics when q's Store isn't a ScheduleStore, when the kind has
// a schedule already, or once q runs.
func (k *Kind[T]) Schedule(s Schedule, v T) {
	payload, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("queue: the value of the %s schedule: %v", k.name, err))
	}
	if _, ok := k.q.store.(ScheduleStore); !ok {
		panic(fmt.Sprintf("queue: scheduling %s needs a Store that keeps schedules, a ScheduleStore, and the queue's %T isn't one", k.name, k.q.store))
	}
	k.q.mu.Lock()
	defer k.q.mu.Unlock()
	if k.q.started {
		panic(fmt.Sprintf("queue: scheduling %s after Run has started; give the queue its schedules first", k.name))
	}
	for _, sc := range k.q.scheduled {
		if sc.kind == k.name {
			panic(fmt.Sprintf("queue: the jobs of kind %q have a schedule already", k.name))
		}
	}
	sc := scheduled{kind: k.name, schedule: s, payload: payload, oneAtATime: k.h.oneAtATime, atOnce: k.h.atOnce}
	if k.h.unique {
		sc.key = keyOf(payload)
	}
	k.q.scheduled = append(k.q.scheduled, sc)
}

// scheduled is a kind's schedule, and the value its runs are pushed with.
type scheduled struct {
	kind       string
	schedule   Schedule
	payload    []byte
	key        string // a unique kind's
	oneAtATime bool
	atOnce     int
}

// keyOf is the key of a unique kind's job with payload: its SHA-256, the
// same for the same value, as encoding/json writes a struct's fields and
// a map's keys in one order, and short however big the value is.
func keyOf(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// call runs the handler on j, with a context done after its Timeout. A
// panic is an error, as the job's worker has others to run.
func (h *handler) call(ctx context.Context, j *Job) (err error) {
	if h.run == nil {
		return errNoHandler(j.Kind)
	}
	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()
	defer func() {
		if v := recover(); v != nil {
			slog.Error("a job panicked", about(j, "panic", v, "stack", string(debug.Stack()))...)
			err = fmt.Errorf("panic: %v", v)
		}
	}()
	return h.run(ctx, j.Payload)
}

// failed runs the kind's OnFail for j, which failed for good with err,
// with a context of its own, done after the kind's Timeout, as the job's
// may be done: the queue may be stopping. Its error, and a panic, go to
// the log, as there's no job left to fail.
func (h *handler) failed(ctx context.Context, j *Job, err error) {
	if h.onFail == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), h.timeout)
	defer cancel()
	defer func() {
		if v := recover(); v != nil {
			slog.Error("OnFail panicked for a job that failed", about(j, "panic", v, "stack", string(debug.Stack()))...)
		}
	}()
	if err := h.onFail(ctx, j.Payload, err); err != nil {
		slog.Error("OnFail failed for a job that failed", about(j, "err", err)...)
	}
}

// Option changes how a kind of job runs, for Handle.
type Option func(*handler)

// Attempts is how many times a job may run before it fails for good, the
// first time included. Default 10: with the default Backoff, the last
// runs about four hours after the first.
func Attempts(n int) Option {
	if n < 1 {
		panic("queue: Attempts takes 1 or more")
	}
	return func(h *handler) { h.attempts = n }
}

// Timeout is how long a job may run: its context is done after it, so a
// handler that heeds its context fails the attempt. Default a minute.
func Timeout(d time.Duration) Option {
	if d <= 0 {
		panic("queue: Timeout takes a duration over 0")
	}
	return func(h *handler) { h.timeout = d }
}

// Unique makes the kind's jobs unique by their value: while a job of the
// kind waits, pushing another with the same value does nothing, and the one
// that waits runs, at its own time. Once a worker has it, a push is pushed,
// and runs too, maybe beside it on another worker, unless OneAtATime says
// otherwise: the running job may have read what the push is about. What
// varies from one push of the same work to the next, such as when it was
// asked for, stays out of the value. Unique needs a Store that keeps the
// keys, a UniqueStore, and its options their own extras.
func Unique(opts ...UniqueOption) Option {
	return func(h *handler) {
		h.unique = true
		for _, opt := range opts {
			opt(h)
		}
	}
}

// UniqueOption changes how a Unique kind's jobs of one value go, for Unique.
type UniqueOption func(*handler)

// Latest has the latest push of a value say when its job runs: a push
// while the job waits gives it the push's time, earlier or later, where
// without Latest the first push keeps its own. A reindex pushed for a
// minute on at each edit runs a minute after the last. A scheduled run
// doesn't move a job that waits, as a schedule's time isn't a push's.
// Latest needs a Store that moves the jobs that wait, a LatestStore.
func Latest() UniqueOption {
	return func(h *handler) { h.latest = true }
}

// OneAtATime keeps the kind's jobs of one value from running at once: one
// isn't claimed while another of the value runs, and runs once that one is
// done, or has failed, or has lost its worker and its hold has run out. A
// push while one runs is still pushed, as for any Unique kind, and waits
// for it. OneAtATime needs a Store that keeps the jobs' keys while they
// run, a OneAtATimeStore.
func OneAtATime() UniqueOption {
	return func(h *handler) { h.oneAtATime = true }
}

// Backoff is how long a job waits to run again after its attempt-th
// attempt failed, the first being 1. Default attempt⁴ seconds, with a
// tenth more at most at random: 1s, 16s, 81s, and on, a day at most.
func Backoff(wait func(attempt int) time.Duration) Option {
	if wait == nil {
		panic("queue: Backoff takes a function")
	}
	return func(h *handler) { h.backoff = wait }
}

// AtOnce keeps the kind's jobs from running more than n at once, on all
// the instances of the app together: a hundred photos to resize take n
// workers, and leave the rest to the other kinds. A job waits while n of
// its kind are held, and runs once one is done, has failed or gone back,
// or has lost its worker and its hold has run out. AtOnce needs a Store
// that counts them, an AtOnceStore.
func AtOnce(n int) Option {
	if n < 1 {
		panic("queue: AtOnce takes 1 or more")
	}
	return func(h *handler) { h.atOnce = n }
}

// A Limiter says whether one more may go now, counting it, or how long to
// wait: auth.Throttle's Try is one, which counts in its Store, so every
// instance of the app counts together.
type Limiter interface {
	Try(ctx context.Context, key string) (wait time.Duration, err error)
}

// Rate keeps the kind's jobs from starting faster than l lets them: a
// newsletter's mails go no faster than the mail provider takes them, with
// an auth.Throttle of so many a second. Each job, as it's claimed, tries l
// by its kind's name, and one l refuses goes back as it was, its attempts
// unchanged, as it hasn't run, while no job of the kind is claimed until
// l's wait is over. A limiter that fails, as when its store is down, holds
// the job back too, for a while, rather than let it past the limit. Rate
// needs a Store that holds a kind back, a HoldBackStore.
func Rate(l Limiter) Option {
	if l == nil {
		panic("queue: Rate takes a Limiter")
	}
	return func(h *handler) { h.rate = l }
}

// OnFail has fn run when a job of the kind fails for good, after its last
// attempt or a Permanent error, with the value it was pushed with and the
// error its last attempt returned: to mark what it was about, as a post
// whose photo never came, or to tell someone. It runs once, not after each
// attempt, once the Store has kept the job as failed, on the instance that
// ran it, with a context done after the kind's Timeout. Its error, or its
// panic, goes to the log. fn takes the kind's value: Handle panics for
// another type.
func OnFail[T any](fn func(ctx context.Context, v T, err error) error) Option {
	if fn == nil {
		panic("queue: OnFail takes a function")
	}
	return func(h *handler) {
		h.onFailType = reflect.TypeFor[T]()
		h.onFail = func(ctx context.Context, payload []byte, err error) error {
			var v T
			if jsonErr := json.Unmarshal(payload, &v); jsonErr != nil {
				return fmt.Errorf("its payload doesn't fit a %T, so OnFail wasn't run: %w", v, jsonErr)
			}
			return fn(ctx, v, err)
		}
	}
}
