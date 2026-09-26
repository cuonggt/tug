package queue_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cuonggt/tug/queue"
	"github.com/cuonggt/tug/queue/queuetest"
)

// clock is the time a test's queue goes by, which the test moves.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// store is a Store in memory that keeps a copy of each job it's told has
// failed.
type store struct {
	queuetest.Memory
	mu     sync.Mutex
	failed []queue.Job
}

func (s *store) Fail(ctx context.Context, j *queue.Job) error {
	s.mu.Lock()
	s.failed = append(s.failed, *j)
	s.mu.Unlock()
	return s.Memory.Fail(ctx, j)
}

func (s *store) failures() []queue.Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.failed)
}

// newQueue returns a Queue on a store in memory, going by a clock the test
// moves.
func newQueue(cfg queue.Config) (*queue.Queue, *store, *clock) {
	s := &store{}
	cfg.Store = s
	q := queue.New(cfg)
	c := &clock{t: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)}
	queue.SetNow(q, c.now)
	return q, s, c
}

// run runs q until the test ends, and returns a function that stops it
// and waits for Run to return.
func run(t *testing.T, q *queue.Queue) (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		if err := q.Run(ctx); err != nil {
			t.Errorf("Run returned %v", err)
		}
		close(done)
	}()
	stop = func() {
		cancel()
		<-done
	}
	t.Cleanup(stop)
	return stop
}

// logs is what the queue logged, which it may write from several
// goroutines.
type logs struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *logs) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *logs) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

func captureLog(t *testing.T) *logs {
	t.Helper()
	l := &logs{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(l, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	return l
}

// within waits for ch, and fails the test if it takes too long.
func within[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		t.Fatalf("waited 5 seconds for %s", what)
	}
	var zero T
	return zero
}

type greeting struct {
	Name string `json:"name"`
}

var ctx = context.Background()

func TestAPushedJobRunsWithTheValueItWasPushedWith(t *testing.T) {
	q, _, c := newQueue(queue.Config{})
	var got []string
	greet := queue.Handle(q, "greet", func(_ context.Context, g greeting) error {
		got = append(got, g.Name)
		return nil
	})
	if err := greet.Push(ctx, greeting{Name: "Ann"}); err != nil {
		t.Fatal(err)
	}
	if err := q.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"Ann"}) {
		t.Fatalf("ran with %v, want Ann", got)
	}
	c.add(24 * time.Hour)
	q.Drain(ctx)
	if len(got) != 1 {
		t.Errorf("ran %d times, want the job done after once", len(got))
	}
}

func TestAJobPushedForLaterRunsOnceItsDue(t *testing.T) {
	q, _, c := newQueue(queue.Config{})
	ran := 0
	remind := queue.Handle(q, "remind", func(context.Context, greeting) error {
		ran++
		return nil
	})
	remind.PushAt(ctx, c.now().Add(time.Hour), greeting{Name: "Ann"})
	q.Drain(ctx)
	if ran != 0 {
		t.Fatal("ran an hour before it was due")
	}
	c.add(time.Hour)
	q.Drain(ctx)
	if ran != 1 {
		t.Errorf("ran %d times once due, want 1", ran)
	}
}

func TestAFailedJobRunsAgainAfterAWaitAndIsKeptAfterItsLastAttempt(t *testing.T) {
	logs := captureLog(t)
	q, s, c := newQueue(queue.Config{})
	ran := 0
	send := queue.Handle(q, "send", func(context.Context, greeting) error {
		ran++
		return errors.New("the mail server said no")
	}, queue.Attempts(3), queue.Backoff(func(attempt int) time.Duration { return time.Duration(attempt) * time.Minute }))
	send.Push(ctx, greeting{Name: "Ann"})

	q.Drain(ctx) // the first attempt, and another in a minute
	c.add(59 * time.Second)
	q.Drain(ctx)
	if ran != 1 {
		t.Fatalf("ran %d times before the first wait was over, want 1", ran)
	}
	c.add(time.Second)
	q.Drain(ctx) // the second, and another in two minutes
	c.add(2 * time.Minute)
	q.Drain(ctx) // the third, and last
	if ran != 3 {
		t.Fatalf("ran %d times, want 3", ran)
	}
	if failed := s.failures(); len(failed) != 1 || failed[0].Attempts != 3 || failed[0].Error != "the mail server said no" {
		t.Errorf("kept as failed: %+v, want the job after 3 attempts, with its error", failed)
	}
	c.add(24 * time.Hour)
	q.Drain(ctx)
	if ran != 3 {
		t.Errorf("a failed job ran again")
	}
	if out := logs.String(); !strings.Contains(out, "a job failed, and will run again") || !strings.Contains(out, "a job failed, and won't run again") {
		t.Errorf("logged:\n%s", out)
	}
}

func TestAJobThatWorksOnAnotherAttemptIsDone(t *testing.T) {
	captureLog(t)
	q, s, c := newQueue(queue.Config{})
	ran := 0
	send := queue.Handle(q, "send", func(context.Context, greeting) error {
		ran++
		if ran == 1 {
			return errors.New("the mail server said no")
		}
		return nil
	})
	send.Push(ctx, greeting{Name: "Ann"})
	q.Drain(ctx)
	c.add(time.Hour)
	q.Drain(ctx)
	c.add(24 * time.Hour)
	q.Drain(ctx)
	if ran != 2 || len(s.failures()) != 0 {
		t.Errorf("ran %d times, and failed %v: want it done at the second attempt", ran, s.failures())
	}
}

func TestAPermanentErrorFailsAJobAtOnce(t *testing.T) {
	captureLog(t)
	q, s, c := newQueue(queue.Config{})
	ran := 0
	send := queue.Handle(q, "send", func(context.Context, greeting) error {
		ran++
		return queue.Permanent(errors.New("no such user"))
	})
	send.Push(ctx, greeting{Name: "Ann"})
	q.Drain(ctx)
	c.add(24 * time.Hour)
	q.Drain(ctx)
	if failed := s.failures(); ran != 1 || len(failed) != 1 || failed[0].Error != "no such user" {
		t.Errorf("ran %d times, kept as failed: %+v", ran, failed)
	}
}

func TestAJobThatPanicsFailsAnAttempt(t *testing.T) {
	logs := captureLog(t)
	q, _, _ := newQueue(queue.Config{})
	ran := 0
	send := queue.Handle(q, "send", func(context.Context, greeting) error {
		ran++
		if ran == 1 {
			panic("assignment to entry in nil map")
		}
		return nil
	}, queue.Backoff(func(int) time.Duration { return 0 }))
	send.Push(ctx, greeting{Name: "Ann"})
	if err := q.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if ran != 2 {
		t.Errorf("ran %d times, want again after the panic", ran)
	}
	if out := logs.String(); !strings.Contains(out, "a job panicked") || !strings.Contains(out, "panic: assignment to entry in nil map") {
		t.Errorf("logged:\n%s", out)
	}
}

func TestAPayloadThatDoesntFitTheHandlerFailsAtOnce(t *testing.T) {
	captureLog(t)
	q, s, c := newQueue(queue.Config{})
	ran := false
	queue.Handle(q, "greet", func(context.Context, greeting) error {
		ran = true
		return nil
	})
	s.Push(ctx, &queue.Job{Kind: "greet", Payload: []byte(`{"name": 42}`), RunAt: c.now()})
	q.Drain(ctx)
	if failed := s.failures(); ran || len(failed) != 1 || !strings.Contains(failed[0].Error, "payload doesn't fit") {
		t.Errorf("ran: %v; kept as failed: %+v", ran, failed)
	}
}

func TestAJobOfAKindWithNoHandlerHereRunsAgainLater(t *testing.T) {
	captureLog(t)
	q, s, c := newQueue(queue.Config{})
	s.Push(ctx, &queue.Job{Kind: "welcome-mail", Payload: []byte(`{}`), RunAt: c.now()})
	q.Drain(ctx)
	if failed := s.failures(); len(failed) != 0 {
		t.Fatalf("failed at once: %+v", failed)
	}

	// An instance of the app from after a deploy, which knows the kind.
	next := queue.New(queue.Config{Store: s})
	queue.SetNow(next, c.now)
	ran := false
	queue.Handle(next, "welcome-mail", func(context.Context, struct{}) error {
		ran = true
		return nil
	})
	c.add(time.Hour)
	next.Drain(ctx)
	if !ran {
		t.Error("the instance that knows the kind didn't run it")
	}
}

func TestAJobsContextEndsAfterItsKindsTimeout(t *testing.T) {
	q, _, _ := newQueue(queue.Config{})
	var left time.Duration
	slow := queue.Handle(q, "slow", func(ctx context.Context, _ struct{}) error {
		deadline, ok := ctx.Deadline()
		if !ok {
			return errors.New("no deadline")
		}
		left = time.Until(deadline)
		return nil
	}, queue.Timeout(30*time.Second))
	slow.Push(ctx, struct{}{})
	q.Drain(ctx)
	if left < 29*time.Second || left > 30*time.Second {
		t.Errorf("the job had %v left, want 30 seconds", left)
	}
}

func TestTheWaitBetweenAttemptsGrows(t *testing.T) {
	for attempt, want := range map[int]time.Duration{1: time.Second, 2: 16 * time.Second, 3: 81 * time.Second, 9: 6561 * time.Second} {
		if got := queue.DefaultBackoff(attempt); got < want || got > want+want/10 {
			t.Errorf("after attempt %d: %v, want %v and a tenth more at most", attempt, got, want)
		}
	}
	if got := queue.DefaultBackoff(1000); got != 24*time.Hour {
		t.Errorf("after attempt 1000: %v, want a day", got)
	}
}

func TestRunRunsAJobAsSoonAsItsPushed(t *testing.T) {
	q, _, _ := newQueue(queue.Config{Poll: time.Hour})
	ran := make(chan string, 1)
	greet := queue.Handle(q, "greet", func(_ context.Context, g greeting) error {
		ran <- g.Name
		return nil
	})
	run(t, q)
	time.Sleep(50 * time.Millisecond) // for Run to find nothing, and wait
	greet.Push(ctx, greeting{Name: "Ann"})
	if name := within(t, ran, "the job, with the next poll an hour away"); name != "Ann" {
		t.Errorf("ran with %q", name)
	}
}

func TestRunFindsTheJobsAnotherInstancePushed(t *testing.T) {
	s := &store{}
	web := queue.New(queue.Config{Store: s})
	worker := queue.New(queue.Config{Store: s, Poll: 10 * time.Millisecond})
	greet := queue.Handle(web, "greet", func(context.Context, greeting) error { return nil })
	ran := make(chan string, 1)
	queue.Handle(worker, "greet", func(_ context.Context, g greeting) error {
		ran <- g.Name
		return nil
	})
	run(t, worker)
	greet.Push(ctx, greeting{Name: "Ann"})
	if name := within(t, ran, "the job another instance pushed"); name != "Ann" {
		t.Errorf("ran with %q", name)
	}
}

func TestRunRunsJobsAtOnceUpToItsWorkers(t *testing.T) {
	q, _, _ := newQueue(queue.Config{Workers: 2})
	var running, most atomic.Int32
	started, release := make(chan struct{}, 3), make(chan struct{})
	work := queue.Handle(q, "work", func(context.Context, struct{}) error {
		n := running.Add(1)
		for m := most.Load(); n > m && !most.CompareAndSwap(m, n); m = most.Load() {
		}
		started <- struct{}{}
		<-release
		running.Add(-1)
		return nil
	})
	for range 3 {
		work.Push(ctx, struct{}{})
	}
	run(t, q)
	within(t, started, "the first job")
	within(t, started, "the second job")
	select {
	case <-started:
		t.Fatal("a third job started with both workers busy")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	within(t, started, "the third job, once a worker was free")
	if most.Load() != 2 {
		t.Errorf("%d jobs ran at once, want 2", most.Load())
	}
}

func TestRunLetsARunningJobFinishAsItStops(t *testing.T) {
	q, s, c := newQueue(queue.Config{Grace: 5 * time.Second})
	started, release := make(chan struct{}), make(chan struct{})
	work := queue.Handle(q, "work", func(context.Context, struct{}) error {
		close(started)
		<-release
		return nil
	})
	work.Push(ctx, struct{}{})
	stop := run(t, q)
	within(t, started, "the job")
	stopped := make(chan struct{})
	go func() {
		stop()
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("Run returned with the job still running")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	within(t, stopped, "Run to return once the job was done")
	if j, _ := s.Claim(ctx, c.now().Add(24*time.Hour), c.now().Add(25*time.Hour)); j != nil {
		t.Errorf("the job is still in the store: %+v", j)
	}
}

func TestRunPutsBackAJobThatOutlastsTheGrace(t *testing.T) {
	captureLog(t)
	q, s, c := newQueue(queue.Config{Grace: 50 * time.Millisecond})
	started := make(chan struct{})
	work := queue.Handle(q, "work", func(ctx context.Context, _ struct{}) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	work.Push(ctx, struct{}{})
	stop := run(t, q)
	within(t, started, "the job")
	stop()

	// Due now, for the next start, rather than once its hold is up.
	j, err := s.Claim(ctx, c.now(), c.now().Add(time.Minute))
	if err != nil || j == nil || j.Error != "context canceled" {
		t.Errorf("the job after Run stopped: %+v, %v; want it back, due now", j, err)
	}
}

// flaky is a Store whose Claim fails a number of times first.
type flaky struct {
	queuetest.Memory
	fails atomic.Int32
}

func (s *flaky) Claim(ctx context.Context, now, until time.Time) (*queue.Job, error) {
	if s.fails.Add(-1) >= 0 {
		return nil, errors.New("database is locked")
	}
	return s.Memory.Claim(ctx, now, until)
}

func TestRunGoesOnWhenTheStoreFails(t *testing.T) {
	logs := captureLog(t)
	s := &flaky{}
	s.fails.Store(3)
	q := queue.New(queue.Config{Store: s, Poll: 10 * time.Millisecond})
	ran := make(chan struct{}, 1)
	work := queue.Handle(q, "work", func(context.Context, struct{}) error {
		ran <- struct{}{}
		return nil
	})
	work.Push(ctx, struct{}{})
	if err := q.Drain(ctx); err == nil || err.Error() != "database is locked" {
		t.Errorf("Drain returned %v, want the Store's error", err)
	}
	run(t, q)
	within(t, ran, "the job, once the store was back")
	if !strings.Contains(logs.String(), "the job queue can't claim a job") {
		t.Errorf("logged:\n%s", logs)
	}
}

func TestHandlingAKindTwiceOrOnceTheQueueRunsPanics(t *testing.T) {
	q, _, _ := newQueue(queue.Config{})
	noop := func(context.Context, struct{}) error { return nil }
	queue.Handle(q, "work", noop)
	mustPanic(t, "a second handler for a kind", func() { queue.Handle(q, "work", noop) })
	stop := run(t, q)
	stop()
	mustPanic(t, "a handler after Run", func() { queue.Handle(q, "other", noop) })
}

func mustPanic(t *testing.T, what string, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s didn't panic", what)
		}
	}()
	f()
}

func TestAScheduledJobRunsAtEachOfItsTimes(t *testing.T) {
	q, _, c := newQueue(queue.Config{Poll: 10 * time.Millisecond})
	c.add(30 * time.Second) // 12:00:30
	ran := make(chan string, 10)
	report := queue.Handle(q, "report", func(_ context.Context, g greeting) error {
		ran <- g.Name
		return nil
	})
	report.Schedule(queue.Every(time.Minute), greeting{Name: "Ann"})
	run(t, q)
	select {
	case <-ran:
		t.Fatal("ran before 12:01, its first time")
	case <-time.After(100 * time.Millisecond):
	}
	c.add(30 * time.Second) // 12:01
	if name := within(t, ran, "the run at 12:01"); name != "Ann" {
		t.Errorf("ran with %q, want the schedule's value", name)
	}
	time.Sleep(50 * time.Millisecond) // for the run at 12:02 to be pushed
	c.add(time.Minute)
	within(t, ran, "the run at 12:02")
	select {
	case <-ran:
		t.Error("ran a third time, at 12:02")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestARunDueWhileTheAppWasDownRunsOnceAsItStarts(t *testing.T) {
	s := &store{}
	c := &clock{t: time.Date(2026, 9, 1, 12, 0, 30, 0, time.UTC)}
	// instance starts an instance of the app, whose runs go to ran.
	instance := func(ran chan<- struct{}) (stop func()) {
		q := queue.New(queue.Config{Store: s, Poll: 10 * time.Millisecond})
		queue.SetNow(q, c.now)
		queue.Handle(q, "report", func(context.Context, struct{}) error {
			ran <- struct{}{}
			return nil
		}).Schedule(queue.Every(time.Minute), struct{}{})
		stop = run(t, q)
		time.Sleep(50 * time.Millisecond) // for the next run to be pushed
		return stop
	}

	before, after := make(chan struct{}, 10), make(chan struct{}, 10)
	instance(before)()     // it pushes the run at 12:01, and stops before then
	c.add(5 * time.Minute) // 12:05:30
	instance(after)
	within(t, after, "the run at 12:01, as the app started again")
	select {
	case <-after:
		t.Error("ran again: the runs missed after 12:01 ran too")
	case <-time.After(100 * time.Millisecond):
	}
	if len(before) != 0 {
		t.Error("the instance that stopped ran the job")
	}
}

func TestEachRunOfAScheduleRunsOnceWithSeveralInstances(t *testing.T) {
	s := &store{}
	c := &clock{t: time.Date(2026, 9, 1, 12, 0, 30, 0, time.UTC)}
	ran := make(chan struct{}, 20)
	for range 3 {
		q := queue.New(queue.Config{Store: s, Poll: 10 * time.Millisecond})
		queue.SetNow(q, c.now)
		queue.Handle(q, "report", func(context.Context, struct{}) error {
			ran <- struct{}{}
			return nil
		}).Schedule(queue.Every(time.Minute), struct{}{})
		run(t, q)
	}
	for minute := 1; minute <= 3; minute++ {
		time.Sleep(50 * time.Millisecond) // for the next run to be pushed
		c.add(time.Minute)
		within(t, ran, "a run")
	}
	select {
	case <-ran:
		t.Error("a run ran twice")
	case <-time.After(100 * time.Millisecond):
	}
}

// plain is a Store and no more: no ScheduleStore.
type plain struct{ queue.Store }

func TestSchedulingNeedsAStoreThatKeepsSchedules(t *testing.T) {
	q := queue.New(queue.Config{Store: plain{&queuetest.Memory{}}})
	report := queue.Handle(q, "report", func(context.Context, struct{}) error { return nil })
	mustPanic(t, "a schedule with a Store that doesn't keep them", func() {
		report.Schedule(queue.Every(time.Hour), struct{}{})
	})
}

func TestSchedulingAKindTwiceOrOnceTheQueueRunsPanics(t *testing.T) {
	q, _, _ := newQueue(queue.Config{})
	noop := func(context.Context, struct{}) error { return nil }
	report, digest := queue.Handle(q, "report", noop), queue.Handle(q, "digest", noop)
	report.Schedule(queue.Every(time.Hour), struct{}{})
	mustPanic(t, "a second schedule for a kind", func() { report.Schedule(queue.Cron("@daily"), struct{}{}) })
	stop := run(t, q)
	stop()
	mustPanic(t, "a schedule after Run", func() { digest.Schedule(queue.Every(time.Hour), struct{}{}) })
}
