package queue_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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

func TestAUniqueJobPushedAgainWhileItWaitsRunsOnceAtItsOwnTime(t *testing.T) {
	q, _, c := newQueue(queue.Config{})
	var ran []string
	reindex := queue.Handle(q, "reindex", func(_ context.Context, g greeting) error {
		ran = append(ran, g.Name)
		return nil
	}, queue.Unique())
	ctx := context.Background()
	for _, push := range []func() error{
		func() error { return reindex.PushAt(ctx, c.now().Add(time.Minute), greeting{Name: "Ann"}) },
		func() error { return reindex.Push(ctx, greeting{Name: "Ann"}) }, // Ann's waits, for a minute's time
		func() error { return reindex.Push(ctx, greeting{Name: "Bob"}) },
		func() error { return reindex.Push(ctx, greeting{Name: "Bob"}) },
	} {
		if err := push(); err != nil {
			t.Fatal(err)
		}
	}
	if err := q.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	c.add(time.Minute)
	if err := q.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ran, []string{"Bob", "Ann"}) {
		t.Errorf("ran for %v, want Bob's job, then Ann's a minute on", ran)
	}
}

func TestAUniqueJobPushedWhileOneRunsRunsToo(t *testing.T) {
	q, _, _ := newQueue(queue.Config{})
	runs := 0
	var reindex *queue.Kind[greeting]
	reindex = queue.Handle(q, "reindex", func(ctx context.Context, g greeting) error {
		runs++
		if runs == 1 {
			// What the job reads changes as it runs, and the change pushes it.
			return reindex.Push(ctx, g)
		}
		return nil
	}, queue.Unique())
	ctx := context.Background()
	reindex.Push(ctx, greeting{Name: "Ann"})
	if err := q.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if runs != 2 {
		t.Errorf("ran %d times, want twice: the push as it ran was lost", runs)
	}
}

func TestAUniqueKindNeedsAStoreThatKeepsKeys(t *testing.T) {
	q := queue.New(queue.Config{Store: plain{&queuetest.Memory{}}})
	mustPanic(t, "a unique kind with a Store that doesn't keep keys", func() {
		queue.Handle(q, "reindex", func(context.Context, struct{}) error { return nil }, queue.Unique())
	})
}

func TestAScheduledRunOfAUniqueKindIsTheJobLikeItThatWaits(t *testing.T) {
	q, _, c := newQueue(queue.Config{Poll: 10 * time.Millisecond})
	c.add(30 * time.Second) // 12:00:30
	ran := make(chan struct{}, 10)
	report := queue.Handle(q, "report", func(context.Context, struct{}) error {
		ran <- struct{}{}
		return nil
	}, queue.Unique())
	report.Schedule(queue.Every(time.Minute), struct{}{})
	run(t, q)
	time.Sleep(50 * time.Millisecond) // for the run at 12:01 to be pushed
	// Pushed now, it's the run that waits, which runs at its own time.
	if err := report.Push(context.Background(), struct{}{}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ran:
		t.Fatal("ran at once, with the run like it waiting for 12:01")
	case <-time.After(100 * time.Millisecond):
	}
	c.add(30 * time.Second) // 12:01
	within(t, ran, "the run at 12:01")
	select {
	case <-ran:
		t.Error("ran twice at 12:01")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestAJobPushedInAStoreOfItsOwnGoesThereAndDoesntWakeTheQueue(t *testing.T) {
	q, s, _ := newQueue(queue.Config{Poll: time.Hour})
	ran := make(chan string, 10)
	greet := queue.Handle(q, "greet", func(_ context.Context, g greeting) error {
		ran <- g.Name
		return nil
	})

	// A transaction of the app's that's still open: the queue's Store
	// doesn't have the job, and the Store of the transaction does.
	tx := &queuetest.Memory{}
	if err := greet.In(tx).Push(ctx, greeting{Name: "Ann"}); err != nil {
		t.Fatal(err)
	}
	if j, _ := s.Claim(ctx, time.Now().Add(time.Hour), time.Now().Add(2*time.Hour)); j != nil {
		t.Fatalf("the queue's Store has %+v, pushed in another", j)
	}
	if j, _ := tx.Claim(ctx, time.Now().Add(time.Hour), time.Now().Add(2*time.Hour)); j == nil {
		t.Fatal("the Store the job was pushed in doesn't have it")
	}

	// One that has committed: its job is where the queue can see it, and
	// runs once the queue is woken, and not before its next poll without.
	run(t, q)
	time.Sleep(20 * time.Millisecond) // Run is waiting for its next poll, an hour on
	if err := greet.In(s).Push(ctx, greeting{Name: "Bob"}); err != nil {
		t.Fatal(err)
	}
	select {
	case name := <-ran:
		t.Fatalf("ran for %s as it was pushed, before its transaction could commit", name)
	case <-time.After(100 * time.Millisecond):
	}
	q.Wake()
	if name := within(t, ran, "the job, once the queue was woken"); name != "Bob" {
		t.Errorf("ran for %s, want Bob", name)
	}
}

func TestInPanicsForAStoreThatIsntWhatTheKindNeeds(t *testing.T) {
	q, _, _ := newQueue(queue.Config{})
	reindex := queue.Handle(q, "reindex", func(context.Context, struct{}) error { return nil }, queue.Unique())
	mustPanic(t, "a unique kind in a Store that doesn't keep keys", func() { reindex.In(plain{&queuetest.Memory{}}) })
	mustPanic(t, "a kind in no Store", func() { reindex.In(nil) })
}

func TestTheLatestPushOfAUniqueJobSaysWhenItRuns(t *testing.T) {
	q, _, c := newQueue(queue.Config{})
	ran := 0
	reindex := queue.Handle(q, "reindex", func(context.Context, greeting) error {
		ran++
		return nil
	}, queue.Unique(queue.Latest()))
	// An edit, and another two minutes on, each pushing the reindex for a
	// minute after it.
	reindex.PushAt(ctx, c.now().Add(time.Minute), greeting{Name: "Ann"})
	c.add(2 * time.Minute)
	reindex.PushAt(ctx, c.now().Add(time.Minute), greeting{Name: "Ann"})
	q.Drain(ctx)
	if ran != 0 {
		t.Fatal("ran at the time the first push said, which the second moved")
	}
	c.add(time.Minute)
	q.Drain(ctx)
	if ran != 1 {
		t.Errorf("ran %d times a minute after the last push, want once", ran)
	}
}

func TestUniqueJobsOfOneValueRunOneAtATime(t *testing.T) {
	q, _, _ := newQueue(queue.Config{Workers: 4, Poll: 10 * time.Millisecond})
	started, finish := make(chan string, 10), make(chan struct{})
	reindex := queue.Handle(q, "reindex", func(_ context.Context, g greeting) error {
		started <- g.Name
		<-finish
		return nil
	}, queue.Unique(queue.OneAtATime()))
	run(t, q)
	reindex.Push(ctx, greeting{Name: "Ann"})
	within(t, started, "Ann's first job")
	// Pushed as the first runs, whose claim let its value go, it waits for
	// the first, where Bob's, of another value, runs beside it.
	reindex.Push(ctx, greeting{Name: "Ann"})
	reindex.Push(ctx, greeting{Name: "Bob"})
	if name := within(t, started, "Bob's job"); name != "Bob" {
		t.Fatalf("started %s's job beside Ann's first, want Bob's", name)
	}
	select {
	case name := <-started:
		t.Fatalf("started %s's job while Ann's first ran", name)
	case <-time.After(100 * time.Millisecond):
	}
	finish <- struct{}{} // one of the two ends
	finish <- struct{}{} // and the other
	if name := within(t, started, "Ann's second job"); name != "Ann" {
		t.Errorf("started %s's job once Ann's first was done, want Ann's second", name)
	}
	close(finish)
}

// uniqueOnly is a UniqueStore and no more: no LatestStore or
// OneAtATimeStore.
type uniqueOnly struct{ queue.UniqueStore }

func TestTheUniqueOptionsNeedTheirStores(t *testing.T) {
	q := queue.New(queue.Config{Store: uniqueOnly{&queuetest.Memory{}}})
	noop := func(context.Context, struct{}) error { return nil }
	mustPanic(t, "Latest with a Store that doesn't move jobs", func() {
		queue.Handle(q, "reindex", noop, queue.Unique(queue.Latest()))
	})
	mustPanic(t, "OneAtATime with a Store that doesn't keep keys as jobs run", func() {
		queue.Handle(q, "recount", noop, queue.Unique(queue.OneAtATime()))
	})
}

func TestAJobThatFailsForGoodIsKeptWithWhenItFailed(t *testing.T) {
	q, s, c := newQueue(queue.Config{})
	greet := queue.Handle(q, "greet", func(context.Context, greeting) error {
		return queue.Permanent(errors.New("no such person"))
	})
	greet.Push(ctx, greeting{Name: "Ann"})
	q.Drain(ctx)
	failed, err := s.Failed(ctx)
	if err != nil || len(failed) != 1 {
		t.Fatalf("listed %v, %v, want the job that failed", failed, err)
	}
	if j := failed[0]; !j.FailedAt.Equal(c.now()) || j.Error != "no such person" {
		t.Errorf("listed %+v: want it failed at %v, with its error", j, c.now())
	}
}

func TestNoMoreJobsOfAKindRunAtOnceThanItsAtOnce(t *testing.T) {
	// A poll that never comes: the next job starts as one ends, or not at all.
	q, _, _ := newQueue(queue.Config{Workers: 4, Poll: time.Hour})
	var running, most atomic.Int32
	started, release := make(chan struct{}, 3), make(chan struct{})
	resize := queue.Handle(q, "resize", func(context.Context, struct{}) error {
		n := running.Add(1)
		for m := most.Load(); n > m && !most.CompareAndSwap(m, n); m = most.Load() {
		}
		started <- struct{}{}
		<-release
		running.Add(-1)
		return nil
	}, queue.AtOnce(2))
	for range 3 {
		resize.Push(ctx, struct{}{})
	}
	run(t, q)
	within(t, started, "the first job")
	within(t, started, "the second job")
	select {
	case <-started:
		t.Fatal("a third job started beside the two its AtOnce allows, with workers free")
	case <-time.After(100 * time.Millisecond):
	}
	release <- struct{}{}
	within(t, started, "the third job, as one of the two ended")
	close(release)
	if most.Load() != 2 {
		t.Errorf("%d jobs ran at once, want 2", most.Load())
	}
}

func TestAKindAtItsAtOnceLeavesTheWorkersToOtherKinds(t *testing.T) {
	q, _, _ := newQueue(queue.Config{Workers: 4, Poll: 10 * time.Millisecond})
	release := make(chan struct{})
	defer close(release)
	resizing := make(chan struct{}, 2)
	resize := queue.Handle(q, "resize", func(context.Context, struct{}) error {
		resizing <- struct{}{}
		<-release
		return nil
	}, queue.AtOnce(1))
	sent := make(chan string, 1)
	send := queue.Handle(q, "send", func(_ context.Context, g greeting) error {
		sent <- g.Name
		return nil
	})
	resize.Push(ctx, struct{}{})
	resize.Push(ctx, struct{}{})
	run(t, q)
	within(t, resizing, "the first resize")
	send.Push(ctx, greeting{Name: "Ann"})
	within(t, sent, "the mail, beside the resize")
	select {
	case <-resizing:
		t.Error("a second resize ran beside the one its AtOnce allows")
	case <-time.After(100 * time.Millisecond):
	}
}

// limiter lets left more go, then says to wait, as auth.Throttle does,
// and keeps the keys it was tried with.
type limiter struct {
	mu    sync.Mutex
	left  int
	wait  time.Duration
	err   error
	tries []string
}

func (l *limiter) Try(_ context.Context, key string) (time.Duration, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.tries = append(l.tries, key)
	switch {
	case l.err != nil:
		return 0, l.err
	case l.left > 0:
		l.left--
		return 0, nil
	}
	return l.wait, nil
}

func (l *limiter) let(n int, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.left, l.err = n, err
}

func (l *limiter) tried() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.tries)
}

func TestAJobItsRateRefusesWaitsAndThenRunsAsItWould(t *testing.T) {
	// A Drain that finds the job it held back due again, and again, stops.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	q, s, c := newQueue(queue.Config{})
	l := &limiter{left: 1, wait: time.Minute}
	var sent []string
	send := queue.Handle(q, "send", func(_ context.Context, g greeting) error {
		sent = append(sent, g.Name)
		return queue.Permanent(errors.New("no such address")) // kept, with its attempts
	}, queue.Rate(l))
	send.Push(ctx, greeting{Name: "Ann"})
	send.Push(ctx, greeting{Name: "Bob"})
	q.Drain(ctx)
	if !slices.Equal(sent, []string{"Ann"}) {
		t.Fatalf("sent %v, want Ann's alone, as the rate let one go", sent)
	}
	// The kind is held back for the wait: the limiter isn't asked again.
	c.add(59 * time.Second)
	q.Drain(ctx)
	if tried := l.tried(); len(sent) != 1 || !slices.Equal(tried, []string{"send", "send"}) {
		t.Fatalf("sent %v, and tried the limiter %d times, before the wait was over: want once for each job, by its kind", sent, len(tried))
	}
	c.add(time.Second)
	l.let(1, nil)
	q.Drain(ctx)
	if !slices.Equal(sent, []string{"Ann", "Bob"}) {
		t.Fatalf("sent %v once the wait was over, want Bob's too", sent)
	}
	for _, j := range s.failures() {
		if j.Attempts != 1 {
			t.Errorf("a job ran on its attempt %d, want 1: being held back isn't an attempt", j.Attempts)
		}
	}
}

func TestAKindItsRateHoldsBackWaitsOnEveryInstance(t *testing.T) {
	s := &store{}
	c := &clock{t: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)}
	l := &limiter{left: 1, wait: time.Minute} // counting where both count, as a throttle's table does
	var sent atomic.Int32
	var queues []*queue.Queue
	var send *queue.Kind[greeting]
	for range 2 {
		q := queue.New(queue.Config{Store: s})
		queue.SetNow(q, c.now)
		send = queue.Handle(q, "send", func(context.Context, greeting) error {
			sent.Add(1)
			return nil
		}, queue.Rate(l))
		queues = append(queues, q)
	}
	for range 3 {
		send.Push(ctx, greeting{Name: "Ann"})
	}
	queues[0].Drain(ctx) // one sent, and the next held back
	queues[1].Drain(ctx)
	if sent.Load() != 1 || len(l.tried()) != 2 {
		t.Errorf("sent %d, trying the limiter %d times, want 1 and 2: the other instance claims no job of the kind held back", sent.Load(), len(l.tried()))
	}
}

func TestAJobWhoseLimiterFailsIsHeldBackAWhile(t *testing.T) {
	logs := captureLog(t)
	q, s, c := newQueue(queue.Config{})
	l := &limiter{err: errors.New("the database is down")}
	sent := 0
	send := queue.Handle(q, "send", func(context.Context, greeting) error {
		sent++
		return queue.Permanent(errors.New("no such address"))
	}, queue.Rate(l))
	send.Push(ctx, greeting{Name: "Ann"})
	q.Drain(ctx)
	if sent != 0 {
		t.Fatal("the job ran past a limiter that couldn't say")
	}
	if !strings.Contains(logs.String(), "the database is down") {
		t.Errorf("logged:\n%s", logs)
	}
	c.add(10 * time.Second)
	l.let(1, nil)
	q.Drain(ctx)
	if failed := s.failures(); sent != 1 || len(failed) != 1 || failed[0].Attempts != 1 {
		t.Errorf("ran %d times, and kept %+v, want once, on its first attempt", sent, failed)
	}
}

func TestOnFailGetsTheValueAndErrorOnceTheJobHasFailedForGood(t *testing.T) {
	q, s, _ := newQueue(queue.Config{})
	var calls []string
	send := queue.Handle(q, "send", func(context.Context, greeting) error {
		return errors.New("the mail server said no")
	}, queue.Attempts(2), queue.Backoff(func(int) time.Duration { return 0 }), queue.OnFail(func(_ context.Context, g greeting, err error) error {
		calls = append(calls, fmt.Sprintf("%s: %v, kept as failed: %v", g.Name, err, len(s.failures()) == 1))
		return nil
	}))
	send.Push(ctx, greeting{Name: "Ann"})
	q.Drain(ctx) // both attempts
	if want := []string{"Ann: the mail server said no, kept as failed: true"}; !slices.Equal(calls, want) {
		t.Errorf("OnFail was called %q, want %q", calls, want)
	}
}

func TestOnFailsErrorOrPanicGoesToTheLog(t *testing.T) {
	logs := captureLog(t)
	q, _, _ := newQueue(queue.Config{})
	fail := func(context.Context, greeting) error { return queue.Permanent(errors.New("no such person")) }
	for name, onFail := range map[string]func(context.Context, greeting, error) error{
		"mark":   func(context.Context, greeting, error) error { return errors.New("the database is down") },
		"notify": func(context.Context, greeting, error) error { panic("no one to tell") },
	} {
		queue.Handle(q, name, fail, queue.OnFail(onFail)).Push(ctx, greeting{Name: "Ann"})
	}
	if err := q.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"OnFail failed for a job that failed", "the database is down", "OnFail panicked for a job that failed", "no one to tell"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("no %q in the log:\n%s", want, logs)
		}
	}
}

func TestOnFailIsntRunWithAPayloadThatDoesntFit(t *testing.T) {
	logs := captureLog(t)
	q, s, _ := newQueue(queue.Config{})
	ran := false
	queue.Handle(q, "send", func(context.Context, greeting) error { return nil }, queue.OnFail(func(context.Context, greeting, error) error {
		ran = true
		return nil
	}))
	s.Push(ctx, &queue.Job{Kind: "send", Payload: []byte(`"Ann"`), RunAt: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)})
	q.Drain(ctx)
	if ran || !strings.Contains(logs.String(), "OnFail wasn't run") {
		t.Errorf("OnFail ran: %v; logged:\n%s", ran, logs)
	}
}

func TestTheOptionsNeedTheirStoresAndOnFailTheKindsValue(t *testing.T) {
	q := queue.New(queue.Config{Store: uniqueOnly{&queuetest.Memory{}}})
	noop := func(context.Context, greeting) error { return nil }
	mustPanic(t, "AtOnce with a Store that doesn't count the jobs running", func() {
		queue.Handle(q, "resize", noop, queue.AtOnce(2))
	})
	mustPanic(t, "Rate with a Store that doesn't hold a kind back", func() {
		queue.Handle(q, "send", noop, queue.Rate(&limiter{}))
	})
	mustPanic(t, "AtOnce(0)", func() { queue.AtOnce(0) })
	mustPanic(t, "OnFail taking another value than the kind's", func() {
		queue.Handle(queue.New(queue.Config{Store: &queuetest.Memory{}}), "send", noop, queue.OnFail(func(context.Context, string, error) error { return nil }))
	})
}
