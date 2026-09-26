// Package queue runs jobs in the background: work a request starts and
// doesn't wait for, such as sending a mail. A job is kept by a Store, such
// as a table in the app's database, from the moment it's pushed until it
// has run, so a mail server that's down, or an app that restarts, doesn't
// lose it. A job that fails runs again, after a wait that grows each
// time, until it works or runs out of attempts, and one that runs out is
// kept, with its error, for someone to look at.
//
//	q := queue.New(queue.Config{Store: store})
//	welcome := queue.Handle(q, "welcome-mail", func(ctx context.Context, m WelcomeMail) error {
//		...
//	})
//	app.Go(q.Run) // tug's App: the workers start and stop with the app
//	...
//	err := welcome.Push(ctx, WelcomeMail{User: u.ID})
//
// A job runs at least once. Its worker holds it for a while, and if the
// app is killed before the job is done, it runs again once the hold is up:
// a handler should be safe to run twice, as a mail sent twice is.
//
// Where the jobs are kept is the Store's business: the auth starter, tug
// new -auth, keeps them in SQLite. Package queuetest checks that a Store
// keeps its promises, and has one in memory for tests.
package queue

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"
)

// Config is how a Queue runs its jobs. Every field but Store has a
// default.
type Config struct {
	// Store keeps the jobs.
	Store Store

	// Workers is how many jobs Run runs at once. Default 4.
	Workers int

	// Poll is how often Run, with nothing to do, asks the Store for jobs
	// that have come due, or that another instance of the app pushed. A
	// job pushed through this Queue wakes it at once. Default a second.
	Poll time.Duration

	// Grace is how long the jobs running when Run is told to stop have to
	// finish. Those still running after it have their contexts canceled,
	// and are put back to run at the next start. Default 10 seconds.
	Grace time.Duration
}

// Queue pushes jobs to its Store, and runs them.
type Queue struct {
	store   Store
	workers int
	poll    time.Duration
	grace   time.Duration
	now     func() time.Time

	mu        sync.Mutex
	handlers  map[string]*handler
	scheduled []scheduled
	started   bool // Run has begun, and the handlers and schedules are fixed

	// wake tells Run a job is due, without waiting for its next poll.
	wake chan struct{}
}

const (
	defaultAttempts = 10
	defaultTimeout  = time.Minute

	// holdMargin is how much longer a claim holds a job than the job may
	// run, for the worker to tell the Store how it went.
	holdMargin = time.Minute

	// storeTimeout bounds telling the Store how a job went, which happens
	// even as the app stops.
	storeTimeout = 10 * time.Second

	// letGo is how long the jobs canceled at the end of the Grace have to
	// return and be put back before Run returns without them.
	letGo = 5 * time.Second
)

// New returns a Queue on cfg.Store. It panics without one.
func New(cfg Config) *Queue {
	if cfg.Store == nil {
		panic("queue: Config.Store is nil: the jobs need somewhere to wait")
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}
	if cfg.Poll <= 0 {
		cfg.Poll = time.Second
	}
	if cfg.Grace <= 0 {
		cfg.Grace = 10 * time.Second
	}
	return &Queue{
		store:    cfg.Store,
		workers:  cfg.Workers,
		poll:     cfg.Poll,
		grace:    cfg.Grace,
		now:      time.Now,
		handlers: make(map[string]*handler),
		wake:     make(chan struct{}, 1),
	}
}

// Run runs jobs as they come due, Workers at a time, until ctx is done.
// Then it takes no more, gives the ones running the Grace to finish, and
// returns nil once they have, or have been put back. An error from the
// Store is logged, and Run goes on after a Poll: a database that's down
// for a moment shouldn't stop the jobs for good.
//
// Run is meant for tug's App.Go, which starts it with the app and stops it
// as the app shuts down.
func (q *Queue) Run(ctx context.Context) error {
	q.mu.Lock()
	q.started = true
	hold := q.hold()
	scheduled := q.scheduled
	q.mu.Unlock()

	// The jobs run with a context of their own, which outlives ctx by the
	// Grace: an app told to stop shouldn't cut a mail off halfway.
	jobs, stopJobs := context.WithCancel(context.WithoutCancel(ctx))
	defer stopJobs()
	slots := make(chan struct{}, q.workers)
	var running sync.WaitGroup
	pushed := make([]time.Time, len(scheduled)) // the run of each schedule pushed last
	for {
		q.pushScheduled(ctx, scheduled, pushed)
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
		j, err := q.claim(ctx, hold)
		if err != nil && ctx.Err() == nil {
			slog.Error("the job queue can't claim a job", "err", err)
		}
		if j == nil {
			<-slots
			q.idle(ctx)
			continue
		}
		running.Go(func() {
			defer func() { <-slots }()
			if err := q.run(jobs, j); err != nil {
				slog.Error("the job queue can't record how a job went", "kind", j.Kind, "job", j.ID, "err", err)
			}
		})
	}

	finished := make(chan struct{})
	go func() {
		running.Wait()
		close(finished)
	}()
	grace := time.NewTimer(q.grace)
	defer grace.Stop()
	select {
	case <-finished:
		return nil
	case <-grace.C:
	}
	stopJobs()
	select {
	case <-finished:
	case <-time.After(letGo):
		// Their handlers ignore their contexts. Once their holds run out,
		// they run again, here or elsewhere.
		slog.Warn("jobs are still running as the job queue stops")
	}
	return nil
}

// Drain runs the jobs that are due, one at a time in the calling
// goroutine, until none is, and returns the first error from the Store. A
// job that fails is put back or kept as failed, as Run does, and runs
// again in the same Drain if it's due again before the others are done.
// It's for tests, with a Store such as queuetest.Memory, and for programs
// that run what's due and exit.
func (q *Queue) Drain(ctx context.Context) error {
	q.mu.Lock()
	hold := q.hold()
	q.mu.Unlock()
	for {
		j, err := q.claim(ctx, hold)
		if err != nil || j == nil {
			return err
		}
		if err := q.run(ctx, j); err != nil {
			return err
		}
	}
}

// pushScheduled pushes the next run of each schedule whose run pushed last
// has come round, or, as Run starts, has none. Every instance does, and the
// Store sees that one push of each run wins, so an instance that loses has
// its answer all the same: the run is pushed.
func (q *Queue) pushScheduled(ctx context.Context, scheduled []scheduled, pushed []time.Time) {
	now := q.now()
	for i, sc := range scheduled {
		if ctx.Err() != nil {
			return
		}
		if now.Before(pushed[i]) {
			continue
		}
		at := sc.schedule.Next(now)
		if at.IsZero() {
			continue // it never comes round again
		}
		_, err := q.store.(ScheduleStore).PushScheduled(ctx, sc.kind, &Job{Kind: sc.kind, Payload: sc.payload, RunAt: at})
		if err != nil {
			if ctx.Err() == nil {
				slog.Error("the job queue can't push a scheduled job", "kind", sc.kind, "at", at, "err", err)
			}
			continue // it tries again next time round
		}
		pushed[i] = at
	}
}

// hold is how long a claim holds a job: as long as the longest Timeout of
// any kind, and the holdMargin. q.mu must be held.
func (q *Queue) hold() time.Duration {
	longest := defaultTimeout
	for _, h := range q.handlers {
		longest = max(longest, h.timeout)
	}
	return longest + holdMargin
}

func (q *Queue) claim(ctx context.Context, hold time.Duration) (*Job, error) {
	now := q.now()
	return q.store.Claim(ctx, now, now.Add(hold))
}

// run runs j, and tells the Store how it went. ctx is done when Run gives
// up waiting for j as it stops: then j goes back, to run again at the next
// start.
func (q *Queue) run(ctx context.Context, j *Job) error {
	q.mu.Lock()
	h, ok := q.handlers[j.Kind]
	q.mu.Unlock()
	if !ok {
		// Pushed by an instance of the app from after a deploy, maybe,
		// where the kind is new: one that knows it may claim it next time.
		h = &handler{attempts: defaultAttempts, timeout: defaultTimeout, backoff: backoff}
	}
	err := h.call(ctx, j)

	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), storeTimeout)
	defer cancel()
	switch {
	case err == nil:
		return q.store.Done(sctx, j)
	case ctx.Err() != nil:
		j.RunAt, j.Error = q.now(), err.Error()
		slog.Info("a job was stopped with the job queue, and will run again", "kind", j.Kind, "job", j.ID)
		return q.store.Retry(sctx, j)
	case errors.As(err, new(permanentError)) || j.Attempts >= h.attempts:
		j.Error = err.Error()
		slog.Error("a job failed, and won't run again", "kind", j.Kind, "job", j.ID, "attempts", j.Attempts, "err", err)
		return q.store.Fail(sctx, j)
	default:
		j.RunAt, j.Error = q.now().Add(h.backoff(j.Attempts)), err.Error()
		slog.Warn("a job failed, and will run again", "kind", j.Kind, "job", j.ID, "attempt", j.Attempts, "at", j.RunAt, "err", err)
		if err := q.store.Retry(sctx, j); err != nil {
			return err
		}
		if !j.RunAt.After(q.now()) {
			q.poke()
		}
		return nil
	}
}

// poke wakes Run, if it's waiting, to claim a job that's due.
func (q *Queue) poke() {
	select {
	case q.wake <- struct{}{}:
	default: // it's awake, or will be
	}
}

// idle waits for a push, the next poll, or ctx to be done.
func (q *Queue) idle(ctx context.Context) {
	t := time.NewTimer(q.poll)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-q.wake:
	case <-t.C:
	}
}

// Permanent marks err as one that trying again won't mend: a handler that
// returns it fails its job at once, however many attempts it has left.
// A payload that doesn't fit the handler's type fails its job this way.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanentError{err}
}

type permanentError struct{ error }

func (p permanentError) Unwrap() error { return p.error }

// backoff is how long a job waits to run again after its attempt-th
// attempt failed: attempt⁴ seconds, so 1s, 16s, 81s and on to 1.8 hours
// after the ninth, and a tenth more at most, at random, so that jobs that
// failed together don't all come back together. It's a day at most.
func backoff(attempt int) time.Duration {
	if attempt > 19 { // 20⁴ seconds is over a day already
		attempt = 19
	}
	d := time.Duration(attempt*attempt*attempt*attempt) * time.Second
	d += rand.N(d/10 + 1)
	return min(d, 24*time.Hour)
}

// errNoHandler is what a job of a kind this Queue has no handler for
// fails with.
func errNoHandler(kind string) error {
	return fmt.Errorf("no handler for jobs of kind %q here", kind)
}
