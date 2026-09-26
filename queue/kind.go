package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"
)

// Kind is a kind of job, as Handle made it: the name its jobs are kept
// by, and the handler that runs them. Its Push puts one on the queue.
type Kind[T any] struct {
	q    *Queue
	name string
}

// handler runs one kind of job.
type handler struct {
	run      func(ctx context.Context, payload []byte) error
	attempts int
	timeout  time.Duration
	backoff  func(attempt int) time.Duration
}

// Handle gives q the handler for the jobs of kind name, which runs fn with
// the value each was pushed with, and returns the Kind to push them with.
// The value is kept as JSON, so T is a type encoding/json turns into JSON
// and back: IDs, say, rather than the user they name, who may have
// changed by the time the job runs. The name is what the Store keeps to
// find the handler by, so it stays the same from one version of the app
// to the next: jobs one pushes, the next may run.
//
// Handle panics when q has a handler for name already, or once q runs.
func Handle[T any](q *Queue, name string, fn func(ctx context.Context, v T) error, opts ...Option) *Kind[T] {
	if name == "" || fn == nil {
		panic("queue: Handle takes a name and a function")
	}
	h := &handler{attempts: defaultAttempts, timeout: defaultTimeout, backoff: backoff}
	for _, opt := range opts {
		opt(h)
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
	return &Kind[T]{q: q, name: name}
}

// Push puts a job on the queue, to run as soon as a worker is free.
func (k *Kind[T]) Push(ctx context.Context, v T) error {
	return k.PushAt(ctx, k.q.now(), v)
}

// PushAt puts a job on the queue, to run at at, or as soon after as a
// worker is free.
func (k *Kind[T]) PushAt(ctx context.Context, at time.Time, v T) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("queue: a %s job: %w", k.name, err)
	}
	if err := k.q.store.Push(ctx, &Job{Kind: k.name, Payload: payload, RunAt: at}); err != nil {
		return fmt.Errorf("queue: pushing a %s job: %w", k.name, err)
	}
	if !at.After(k.q.now()) {
		k.q.poke()
	}
	return nil
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
			slog.Error("a job panicked", "kind", j.Kind, "job", j.ID, "panic", v, "stack", string(debug.Stack()))
			err = fmt.Errorf("panic: %v", v)
		}
	}()
	return h.run(ctx, j.Payload)
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

// Backoff is how long a job waits to run again after its attempt-th
// attempt failed, the first being 1. Default attempt⁴ seconds, with a
// tenth more at most at random: 1s, 16s, 81s, and on, a day at most.
func Backoff(wait func(attempt int) time.Duration) Option {
	if wait == nil {
		panic("queue: Backoff takes a function")
	}
	return func(h *handler) { h.backoff = wait }
}
