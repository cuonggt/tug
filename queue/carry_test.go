package queue_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cuonggt/tug/queue"
	"github.com/cuonggt/tug/queue/queuetest"
)

type tenantKey struct{}

// tenant carries a tenant's name from the context a job is pushed from to
// the one it runs in, as a Carrier of an app's own would.
type tenant struct{}

func (tenant) Carry(ctx context.Context, into map[string]string) {
	if name := tenantOf(ctx); name != "" {
		into["tenant"] = name
	}
}

func (tenant) Restore(ctx context.Context, from map[string]string) context.Context {
	if name := from["tenant"]; name != "" {
		return context.WithValue(ctx, tenantKey{}, name)
	}
	return ctx
}

func tenantOf(ctx context.Context) string {
	name, _ := ctx.Value(tenantKey{}).(string)
	return name
}

func TestAJobRunsWithWhatTheContextItWasPushedFromCarried(t *testing.T) {
	q, s, c := newQueue(queue.Config{Carry: []queue.Carrier{tenant{}}})
	var got []string
	ran := func(kind string) func(context.Context, int) error {
		return func(ctx context.Context, n int) error {
			got = append(got, fmt.Sprintf("%s %d %q", kind, n, tenantOf(ctx)))
			return nil
		}
	}
	plain := queue.Handle(q, "plain", ran("plain"))
	unique := queue.Handle(q, "unique", ran("unique"), queue.Unique())
	latest := queue.Handle(q, "latest", ran("latest"), queue.Unique(queue.Latest()))

	acme := context.WithValue(ctx, tenantKey{}, "acme")
	for _, push := range []func() error{
		func() error { return plain.Push(acme, 1) },
		func() error { return plain.PushAt(acme, c.now(), 2) },
		func() error { return plain.In(s).Push(acme, 3) }, // in a transaction of the app's
		func() error { return unique.Push(acme, 4) },
		func() error { return latest.Push(acme, 5) },
		func() error { return plain.Push(ctx, 6) }, // from a context that carries nothing
	} {
		if err := push(); err != nil {
			t.Fatal(err)
		}
	}
	if err := q.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	want := []string{`latest 5 "acme"`, `plain 1 "acme"`, `plain 2 "acme"`, `plain 3 "acme"`, `plain 6 ""`, `unique 4 "acme"`}
	if !slices.Equal(got, want) {
		t.Errorf("ran %q, want %q", got, want)
	}
}

func TestAJobThatFailedForGoodSaysWhatItCarriedInTheLogAndToOnFail(t *testing.T) {
	logs := captureLog(t)
	q, _, _ := newQueue(queue.Config{Carry: []queue.Carrier{tenant{}}})
	onFail := "unrun"
	mail := queue.Handle(q, "mail", func(context.Context, int) error {
		return fmt.Errorf("550 no such mailbox")
	}, queue.Attempts(1), queue.OnFail(func(ctx context.Context, _ int, _ error) error {
		onFail = tenantOf(ctx)
		return nil
	}))
	if err := mail.Push(context.WithValue(ctx, tenantKey{}, "acme"), 1); err != nil {
		t.Fatal(err)
	}
	q.Drain(ctx)
	if line := logs.String(); !strings.Contains(line, `a job failed, and won't run again" kind=mail job=1 attempts=1 err="550 no such mailbox" tenant=acme`) {
		t.Errorf("the log says\n%s", line)
	}
	if onFail != "acme" {
		t.Errorf("OnFail ran with the tenant %q, want acme", onFail)
	}
}

type stampKey struct{}

// stamp carries a mark on every job pushed, whatever its context.
type stamp struct{}

func (stamp) Carry(_ context.Context, into map[string]string) { into["stamp"] = "pushed" }
func (stamp) Restore(ctx context.Context, from map[string]string) context.Context {
	return context.WithValue(ctx, stampKey{}, from["stamp"])
}

func TestAScheduledRunCarriesNothing(t *testing.T) {
	q, _, c := newQueue(queue.Config{Poll: 10 * time.Millisecond, Carry: []queue.Carrier{stamp{}}})
	c.add(30 * time.Second) // 12:00:30
	ran := make(chan string, 10)
	report := queue.Handle(q, "report", func(ctx context.Context, _ struct{}) error {
		stamped, _ := ctx.Value(stampKey{}).(string)
		ran <- stamped
		return nil
	})
	report.Schedule(queue.Every(time.Minute), struct{}{})
	run(t, q)
	if err := report.Push(ctx, struct{}{}); err != nil {
		t.Fatal(err)
	}
	if stamped := within(t, ran, "the job pushed"); stamped != "pushed" {
		t.Errorf("the job pushed carried %q, want its stamp", stamped)
	}
	time.Sleep(50 * time.Millisecond) // for the run at 12:01 to be pushed
	c.add(30 * time.Second)           // 12:01
	if stamped := within(t, ran, "the run at 12:01"); stamped != "" {
		t.Errorf("the schedule's run carried %q, which no push gave it", stamped)
	}
}

// forgetful is a Store in memory that doesn't keep what a job carried, as
// a Store from before Carry doesn't: it isn't a CarryStore.
type forgetful struct{ m queuetest.Memory }

func (f *forgetful) Push(ctx context.Context, j *queue.Job) error { return f.m.Push(ctx, j) }
func (f *forgetful) Claim(ctx context.Context, now, until time.Time) (*queue.Job, error) {
	return f.m.Claim(ctx, now, until)
}
func (f *forgetful) Done(ctx context.Context, j *queue.Job) error  { return f.m.Done(ctx, j) }
func (f *forgetful) Retry(ctx context.Context, j *queue.Job) error { return f.m.Retry(ctx, j) }
func (f *forgetful) Fail(ctx context.Context, j *queue.Job) error  { return f.m.Fail(ctx, j) }

func TestAStoreThatKeepsNothingCarriedRunsTheJobAsBefore(t *testing.T) {
	q := queue.New(queue.Config{Store: &forgetful{}, Carry: []queue.Carrier{tenant{}}})
	got := "unrun"
	plain := queue.Handle(q, "plain", func(ctx context.Context, _ int) error {
		got = tenantOf(ctx)
		return nil
	})
	if err := plain.Push(context.WithValue(ctx, tenantKey{}, "acme"), 1); err != nil {
		t.Fatal(err)
	}
	if err := q.Drain(ctx); err != nil || got != "" {
		t.Errorf("the job ran with the tenant %q, %v: want it run, carrying nothing", got, err)
	}
}

// lavish carries more than a job may.
type lavish struct{}

func (lavish) Carry(_ context.Context, into map[string]string) {
	into["everything"] = strings.Repeat("x", 5000)
}
func (lavish) Restore(ctx context.Context, _ map[string]string) context.Context { return ctx }

func TestAPushThatWouldCarryTooMuchFails(t *testing.T) {
	q, _, _ := newQueue(queue.Config{Carry: []queue.Carrier{lavish{}}})
	plain := queue.Handle(q, "plain", func(context.Context, int) error { return nil })
	if err := plain.Push(ctx, 1); err == nil || !strings.Contains(err.Error(), "past the 4096 a job may") {
		t.Errorf("pushing it got %v", err)
	}
}
