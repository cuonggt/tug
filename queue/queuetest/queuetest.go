// Package queuetest is for testing with package queue: Memory is a Store
// for tests, and TestStore checks that a Store keeps the promises the
// queue depends on.
package queuetest

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cuonggt/tug/queue"
)

// Memory is a queue.Store that keeps its jobs in memory, for tests: they
// go when the program does. It keeps schedules too, as a
// queue.ScheduleStore. The zero value is an empty Store.
type Memory struct {
	mu        sync.Mutex
	last      int
	jobs      []*memoryJob         // in the order they were pushed
	schedules map[string]time.Time // each schedule's run pushed last
}

type memoryJob struct {
	queue.Job
	heldUntil time.Time
	failed    bool
}

func (m *Memory) Push(_ context.Context, j *queue.Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.push(j)
	return nil
}

func (m *Memory) PushScheduled(_ context.Context, schedule string, j *queue.Job) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if last, ok := m.schedules[schedule]; ok && !last.Before(j.RunAt) {
		return false, nil
	}
	if m.schedules == nil {
		m.schedules = make(map[string]time.Time)
	}
	m.schedules[schedule] = j.RunAt
	m.push(j)
	return true, nil
}

// push keeps j; m.mu is held.
func (m *Memory) push(j *queue.Job) {
	m.last++
	j.ID = strconv.Itoa(m.last)
	kept := *j
	kept.Payload = bytes.Clone(j.Payload)
	m.jobs = append(m.jobs, &memoryJob{Job: kept})
}

func (m *Memory) Claim(_ context.Context, now, until time.Time) (*queue.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var next *memoryJob
	for _, mj := range m.jobs {
		if mj.failed || mj.RunAt.After(now) || mj.heldUntil.After(now) {
			continue
		}
		if next == nil || mj.RunAt.Before(next.RunAt) {
			next = mj
		}
	}
	if next == nil {
		return nil, nil
	}
	next.heldUntil = until
	next.Attempts++
	j := next.Job
	j.Payload = bytes.Clone(next.Payload)
	return &j, nil
}

func (m *Memory) Done(_ context.Context, j *queue.Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobs = slices.DeleteFunc(m.jobs, func(mj *memoryJob) bool { return mj.claimedBy(j) })
	return nil
}

func (m *Memory) Retry(_ context.Context, j *queue.Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, mj := range m.jobs {
		if mj.claimedBy(j) {
			mj.RunAt, mj.Error, mj.heldUntil = j.RunAt, j.Error, time.Time{}
		}
	}
	return nil
}

func (m *Memory) Fail(_ context.Context, j *queue.Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, mj := range m.jobs {
		if mj.claimedBy(j) {
			mj.Error, mj.failed, mj.heldUntil = j.Error, true, time.Time{}
		}
	}
	return nil
}

// claimedBy says whether j is mj as its latest claim returned it.
func (mj *memoryJob) claimedBy(j *queue.Job) bool {
	return mj.ID == j.ID && mj.Attempts == j.Attempts && !mj.failed
}

// TestStore checks that the Stores open returns keep the promises package
// queue depends on, with a new, empty one for each test, and those of a
// ScheduleStore too when they are one. A Store's own tests call it, as the
// auth starter's do for its tables in SQLite:
//
//	func TestTheJobsTableKeepsTheQueuesPromises(t *testing.T) {
//		queuetest.TestStore(t, func(t *testing.T) queue.Store {
//			return &jobs{db: testDB(t)}
//		})
//	}
func TestStore(t *testing.T, open func(t *testing.T) queue.Store) {
	// Whole seconds, which any Store can keep.
	t0 := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()
	push := func(t *testing.T, s queue.Store, n int, at time.Time) *queue.Job {
		t.Helper()
		j := &queue.Job{Kind: "count", Payload: []byte(`{"n":` + strconv.Itoa(n) + `}`), RunAt: at}
		if err := s.Push(ctx, j); err != nil {
			t.Fatalf("Push: %v", err)
		}
		if j.ID == "" {
			t.Fatal("Push set no ID")
		}
		return j
	}
	claim := func(t *testing.T, s queue.Store, now, until time.Time) *queue.Job {
		t.Helper()
		j, err := s.Claim(ctx, now, until)
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}
		return j
	}
	// claimed is claim, when a job is due.
	claimed := func(t *testing.T, s queue.Store, now, until time.Time) *queue.Job {
		t.Helper()
		j := claim(t, s, now, until)
		if j == nil {
			t.Fatal("no job was claimed, with one due")
		}
		return j
	}
	// n is the number the job was pushed with.
	n := func(t *testing.T, j *queue.Job) int {
		t.Helper()
		var v struct{ N int }
		if err := json.Unmarshal(j.Payload, &v); err != nil {
			t.Fatalf("the payload %q isn't the JSON pushed: %v", j.Payload, err)
		}
		return v.N
	}

	t.Run("a pushed job waits until it's due, and a claim holds it", func(t *testing.T) {
		s := open(t)
		pushed := push(t, s, 1, t0.Add(10*time.Second))
		if j := claim(t, s, t0, t0.Add(time.Minute)); j != nil {
			t.Fatalf("claimed %+v 10 seconds before it's due", j)
		}
		j := claimed(t, s, t0.Add(10*time.Second), t0.Add(time.Minute))
		var payload any
		json.Unmarshal(j.Payload, &payload)
		if j.ID != pushed.ID || j.Kind != "count" || !reflect.DeepEqual(payload, map[string]any{"n": 1.0}) || !j.RunAt.Equal(pushed.RunAt) || j.Attempts != 1 || j.Error != "" {
			t.Errorf("claimed %+v (payload %s), want what was pushed, %+v, with 1 attempt", j, j.Payload, pushed)
		}
		if again := claim(t, s, t0.Add(59*time.Second), t0.Add(2*time.Minute)); again != nil {
			t.Errorf("claimed again while held: %+v", again)
		}
	})

	t.Run("a job whose hold ran out is claimed again, and the claim before can't change it", func(t *testing.T) {
		s := open(t)
		push(t, s, 1, t0)
		first := claimed(t, s, t0, t0.Add(time.Minute))
		second := claimed(t, s, t0.Add(time.Minute), t0.Add(2*time.Minute))
		if second.ID != first.ID || second.Attempts != 2 {
			t.Fatalf("claimed %+v, then once its hold ran out, %+v: want the job again, with 2 attempts", first, second)
		}
		// The first claim's worker, back after its hold ran out.
		first.RunAt, first.Error = t0.Add(time.Hour), "late"
		for _, change := range []func(context.Context, *queue.Job) error{s.Retry, s.Fail, s.Done} {
			if err := change(ctx, first); err != nil {
				t.Fatalf("a late claim: %v", err)
			}
		}
		if j := claim(t, s, t0.Add(90*time.Second), t0.Add(3*time.Minute)); j != nil {
			t.Fatalf("claimed while the second claim holds it: %+v", j)
		}
		third := claimed(t, s, t0.Add(2*time.Minute), t0.Add(3*time.Minute))
		if third.Attempts != 3 || !third.RunAt.Equal(t0) || third.Error != "" {
			t.Errorf("once the second hold ran out, claimed %+v: want it as the late claim found it, with 3 attempts", third)
		}
	})

	t.Run("a job that's done is gone", func(t *testing.T) {
		s := open(t)
		push(t, s, 1, t0)
		j := claimed(t, s, t0, t0.Add(time.Minute))
		if err := s.Done(ctx, j); err != nil {
			t.Fatal(err)
		}
		if j := claim(t, s, t0.Add(24*time.Hour), t0.Add(25*time.Hour)); j != nil {
			t.Errorf("claimed a job that's done: %+v", j)
		}
	})

	t.Run("a job put back runs again when it's due, with its error", func(t *testing.T) {
		s := open(t)
		push(t, s, 1, t0)
		j := claimed(t, s, t0, t0.Add(time.Minute))
		j.RunAt, j.Error = t0.Add(10*time.Minute), "the mail server said no"
		if err := s.Retry(ctx, j); err != nil {
			t.Fatal(err)
		}
		if early := claim(t, s, t0.Add(9*time.Minute), t0.Add(10*time.Minute)); early != nil {
			t.Fatalf("claimed a minute before it's due again: %+v", early)
		}
		again := claimed(t, s, t0.Add(10*time.Minute), t0.Add(11*time.Minute))
		if again.Attempts != 2 || !again.RunAt.Equal(j.RunAt) || again.Error != "the mail server said no" {
			t.Errorf("claimed %+v once due again: want it with 2 attempts, and the error", again)
		}
	})

	t.Run("a failed job is never claimed again", func(t *testing.T) {
		s := open(t)
		push(t, s, 1, t0)
		j := claimed(t, s, t0, t0.Add(time.Minute))
		j.Error = "no such user"
		if err := s.Fail(ctx, j); err != nil {
			t.Fatal(err)
		}
		if j := claim(t, s, t0.Add(24*time.Hour), t0.Add(25*time.Hour)); j != nil {
			t.Errorf("claimed a failed job: %+v", j)
		}
	})

	t.Run("the job due soonest is claimed first", func(t *testing.T) {
		s := open(t)
		for _, n := range []int{3, 1, 2} {
			push(t, s, n, t0.Add(time.Duration(n)*time.Second))
		}
		var order []int
		for range 4 { // a Store that hands a job out twice ends too
			j := claim(t, s, t0.Add(time.Hour), t0.Add(2*time.Hour))
			if j == nil {
				break
			}
			order = append(order, n(t, j))
		}
		if !slices.Equal(order, []int{1, 2, 3}) {
			t.Errorf("claimed in the order %v, want 1, 2, 3, then none", order)
		}
	})

	t.Run("claims at once never get the same job", func(t *testing.T) {
		s := open(t)
		const jobs, claimers = 40, 8
		for i := range jobs {
			push(t, s, i, t0)
		}
		var mu sync.Mutex
		claimed := map[string]int{}
		var wg sync.WaitGroup
		for range claimers {
			wg.Go(func() {
				for range jobs {
					j, err := s.Claim(ctx, t0, t0.Add(time.Hour))
					if err != nil {
						t.Errorf("Claim: %v", err)
						return
					}
					if j == nil {
						return
					}
					mu.Lock()
					claimed[j.ID]++
					mu.Unlock()
				}
			})
		}
		wg.Wait()
		for id, times := range claimed {
			if times > 1 {
				t.Errorf("job %s was claimed %d times at once", id, times)
			}
		}
		if len(claimed) != jobs {
			t.Errorf("%d of the %d jobs were claimed", len(claimed), jobs)
		}
	})

	// The rest are a ScheduleStore's.
	schedules := func(t *testing.T) queue.ScheduleStore {
		t.Helper()
		s, ok := open(t).(queue.ScheduleStore)
		if !ok {
			t.Skip("not a ScheduleStore")
		}
		return s
	}
	pushRun := func(t *testing.T, s queue.ScheduleStore, schedule string, at time.Time) bool {
		t.Helper()
		j := &queue.Job{Kind: schedule, Payload: []byte(`{}`), RunAt: at}
		pushed, err := s.PushScheduled(ctx, schedule, j)
		if err != nil {
			t.Fatalf("PushScheduled: %v", err)
		}
		if pushed && j.ID == "" {
			t.Fatal("PushScheduled pushed a job and set no ID")
		}
		return pushed
	}
	// count claims the jobs due by a day after t0, and counts them.
	count := func(t *testing.T, s queue.Store) int {
		t.Helper()
		n := 0
		for range 10 {
			if claim(t, s, t0.Add(24*time.Hour), t0.Add(25*time.Hour)) == nil {
				break
			}
			n++
		}
		return n
	}

	t.Run("a run of a schedule is pushed once", func(t *testing.T) {
		s := schedules(t)
		if !pushRun(t, s, "report", t0) {
			t.Fatal("the first push of a run didn't push it")
		}
		if pushRun(t, s, "report", t0) {
			t.Error("the run was pushed a second time")
		}
		if n := count(t, s); n != 1 {
			t.Errorf("%d jobs, want the run's one", n)
		}
	})

	t.Run("a schedule's runs are pushed in order, and an earlier one isn't", func(t *testing.T) {
		s := schedules(t)
		if !pushRun(t, s, "report", t0.Add(time.Hour)) || pushRun(t, s, "report", t0) || !pushRun(t, s, "report", t0.Add(2*time.Hour)) {
			t.Error("want the runs at 1 and 2 hours pushed, and the one at 0 not, after the one at 1")
		}
	})

	t.Run("schedules are kept apart by name", func(t *testing.T) {
		s := schedules(t)
		if !pushRun(t, s, "report", t0) || !pushRun(t, s, "digest", t0) {
			t.Error("a run of one schedule kept another's run at the same time from being pushed")
		}
	})

	t.Run("of pushes of a run at once, one wins", func(t *testing.T) {
		s := schedules(t)
		var won atomic.Int32
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				j := &queue.Job{Kind: "report", Payload: []byte(`{}`), RunAt: t0}
				pushed, err := s.PushScheduled(ctx, "report", j)
				if err != nil {
					t.Errorf("PushScheduled: %v", err)
				}
				if pushed {
					won.Add(1)
				}
			})
		}
		wg.Wait()
		if n := count(t, s); won.Load() != 1 || n != 1 {
			t.Errorf("%d pushes won, and %d jobs were pushed: want 1 of each", won.Load(), n)
		}
	})
}
