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
// go when the program does. It has every extra a Store may have: it keeps
// schedules, as a queue.ScheduleStore, the keys of unique jobs, as a
// queue.UniqueStore, which it moves to a push's time and keeps from
// running at once, as a queue.LatestStore and a queue.OneAtATimeStore, and
// lists the jobs that failed, as a queue.FailedStore. The zero value is an
// empty Store.
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
	alone     string // a OneAtATime job's key, which it keeps all its life
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
	if !m.waiting(j) {
		m.push(j)
	}
	return true, nil
}

func (m *Memory) PushUnique(_ context.Context, j *queue.Job) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.waiting(j) {
		return false, nil
	}
	m.push(j)
	return true, nil
}

// waiting says whether a job of j's kind and key waits, which only such a
// job has, as a claim lets its key go; m.mu is held.
func (m *Memory) waiting(j *queue.Job) bool {
	return j.Key != "" && slices.ContainsFunc(m.jobs, func(mj *memoryJob) bool {
		return mj.Kind == j.Kind && mj.Key == j.Key
	})
}

func (m *Memory) PushLatest(_ context.Context, j *queue.Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, mj := range m.jobs {
		if j.Key != "" && mj.Kind == j.Kind && mj.Key == j.Key {
			mj.RunAt, j.ID = j.RunAt, mj.ID
			return nil
		}
	}
	m.push(j)
	return nil
}

func (m *Memory) KeepsOneAtATime() {}

// push keeps j; m.mu is held.
func (m *Memory) push(j *queue.Job) {
	m.last++
	j.ID = strconv.Itoa(m.last)
	kept := *j
	kept.Payload = bytes.Clone(j.Payload)
	mj := &memoryJob{Job: kept}
	if j.OneAtATime {
		mj.alone = j.Key
	}
	m.jobs = append(m.jobs, mj)
}

// running says whether a claim holds another job of mj's kind and
// OneAtATime key at now; m.mu is held.
func (m *Memory) running(mj *memoryJob, now time.Time) bool {
	return mj.alone != "" && slices.ContainsFunc(m.jobs, func(other *memoryJob) bool {
		return other != mj && other.Kind == mj.Kind && other.alone == mj.alone && other.heldUntil.After(now)
	})
}

func (m *Memory) Claim(_ context.Context, now, until time.Time) (*queue.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var next *memoryJob
	for _, mj := range m.jobs {
		if mj.failed || mj.RunAt.After(now) || mj.heldUntil.After(now) || m.running(mj, now) {
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
	next.Key = ""
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
	at := j.FailedAt
	if at.IsZero() {
		at = time.Now()
	}
	for _, mj := range m.jobs {
		if mj.claimedBy(j) {
			mj.Error, mj.FailedAt, mj.failed, mj.heldUntil = j.Error, at, true, time.Time{}
		}
	}
	return nil
}

func (m *Memory) Failed(_ context.Context) ([]*queue.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var failed []*queue.Job
	for _, mj := range slices.Backward(m.jobs) {
		if mj.failed {
			j := mj.Job
			j.Payload = bytes.Clone(mj.Payload)
			failed = append(failed, &j)
		}
	}
	// The latest first, and of two at once, the one pushed last.
	slices.SortStableFunc(failed, func(a, b *queue.Job) int { return b.FailedAt.Compare(a.FailedAt) })
	return failed, nil
}

func (m *Memory) RunAgain(_ context.Context, id string, at time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, mj := range m.jobs {
		if mj.ID == id && mj.failed {
			mj.failed, mj.Attempts, mj.RunAt, mj.heldUntil, mj.FailedAt = false, 0, at, time.Time{}, time.Time{}
			return true, nil
		}
	}
	return false, nil
}

// claimedBy says whether j is mj as its latest claim returned it.
func (mj *memoryJob) claimedBy(j *queue.Job) bool {
	return mj.ID == j.ID && mj.Attempts == j.Attempts && !mj.failed
}

// TestStore checks that the Stores open returns keep the promises package
// queue depends on, with a new, empty one for each test, and those of each
// extra a Store may have when they have it: a ScheduleStore's, a
// UniqueStore's, a LatestStore's, a OneAtATimeStore's and a FailedStore's.
// A Store's own tests call it, as the auth starter's do for its tables in
// SQLite:
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

	// The rest are a UniqueStore's.
	uniques := func(t *testing.T) queue.UniqueStore {
		t.Helper()
		s, ok := open(t).(queue.UniqueStore)
		if !ok {
			t.Skip("not a UniqueStore")
		}
		return s
	}
	pushKey := func(t *testing.T, s queue.UniqueStore, kind, key string, n int) bool {
		t.Helper()
		j := &queue.Job{Kind: kind, Key: key, Payload: []byte(`{"n":` + strconv.Itoa(n) + `}`), RunAt: t0}
		pushed, err := s.PushUnique(ctx, j)
		if err != nil {
			t.Fatalf("PushUnique: %v", err)
		}
		if pushed && j.ID == "" {
			t.Fatal("PushUnique pushed a job and set no ID")
		}
		return pushed
	}

	t.Run("a job isn't pushed while one of its kind and key waits", func(t *testing.T) {
		s := uniques(t)
		if !pushKey(t, s, "count", "a", 1) || pushKey(t, s, "count", "a", 2) || !pushKey(t, s, "count", "b", 3) || !pushKey(t, s, "other", "a", 4) {
			t.Fatal("want key a pushed, then not again, and key b, and another kind's key a")
		}
		var pushed []int
		for range 4 {
			j := claim(t, s, t0, t0.Add(time.Minute))
			if j == nil {
				break
			}
			pushed = append(pushed, n(t, j))
		}
		slices.Sort(pushed)
		if !slices.Equal(pushed, []int{1, 3, 4}) {
			t.Errorf("claimed the jobs pushed with %v, want 1, 3 and 4", pushed)
		}
	})

	t.Run("a claim lets its job's key go, and putting the job back doesn't take it back", func(t *testing.T) {
		s := uniques(t)
		pushKey(t, s, "count", "a", 1)
		first := claimed(t, s, t0, t0.Add(time.Minute))
		if first.Key != "" {
			t.Errorf("the claimed job has its key, %q", first.Key)
		}
		if !pushKey(t, s, "count", "a", 2) {
			t.Fatal("a job with the key of one that's running wasn't pushed")
		}
		first.RunAt, first.Error = t0, "try again"
		if err := s.Retry(ctx, first); err != nil {
			t.Fatal(err)
		}
		if pushKey(t, s, "count", "a", 3) {
			t.Error("pushed while the job pushed as the first ran waits with the key")
		}
		if n := count(t, s); n != 2 {
			t.Errorf("%d jobs, want the one put back, and the one pushed while it ran", n)
		}
	})

	t.Run("a failed job doesn't keep its key", func(t *testing.T) {
		s := uniques(t)
		pushKey(t, s, "count", "a", 1)
		j := claimed(t, s, t0, t0.Add(time.Minute))
		j.Error = "no such post"
		if err := s.Fail(ctx, j); err != nil {
			t.Fatal(err)
		}
		if !pushKey(t, s, "count", "a", 2) {
			t.Error("a failed job's key kept another from being pushed")
		}
	})

	t.Run("of pushes of a key at once, one wins", func(t *testing.T) {
		s := uniques(t)
		var won atomic.Int32
		var wg sync.WaitGroup
		for i := range 8 {
			wg.Go(func() {
				j := &queue.Job{Kind: "count", Key: "a", Payload: []byte(`{"n":` + strconv.Itoa(i) + `}`), RunAt: t0}
				pushed, err := s.PushUnique(ctx, j)
				if err != nil {
					t.Errorf("PushUnique: %v", err)
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

	t.Run("a scheduled run of a unique kind waits as the job with its key", func(t *testing.T) {
		s := uniques(t)
		ss, ok := s.(queue.ScheduleStore)
		if !ok {
			t.Skip("not a ScheduleStore")
		}
		pushKey(t, s, "report", "a", 1)
		run := &queue.Job{Kind: "report", Key: "a", Payload: []byte(`{"n":2}`), RunAt: t0.Add(time.Hour)}
		if _, err := ss.PushScheduled(ctx, "report", run); err != nil {
			t.Fatalf("PushScheduled: %v", err)
		}
		if pushRun(t, ss, "report", t0.Add(time.Hour)) {
			t.Error("the run was pushed again: it wasn't kept")
		}
		if n := count(t, s); n != 1 {
			t.Errorf("%d jobs, want the one with the key, which runs for the run", n)
		}
	})

	// The rest are a LatestStore's.
	latests := func(t *testing.T) queue.LatestStore {
		t.Helper()
		s, ok := open(t).(queue.LatestStore)
		if !ok {
			t.Skip("not a LatestStore")
		}
		return s
	}
	pushLatest := func(t *testing.T, s queue.LatestStore, key string, n int, at time.Time) *queue.Job {
		t.Helper()
		j := &queue.Job{Kind: "count", Key: key, Payload: []byte(`{"n":` + strconv.Itoa(n) + `}`), RunAt: at}
		if err := s.PushLatest(ctx, j); err != nil {
			t.Fatalf("PushLatest: %v", err)
		}
		if j.ID == "" {
			t.Fatal("PushLatest set no ID")
		}
		return j
	}

	t.Run("a push of a key whose job waits gives the job its time, later or earlier", func(t *testing.T) {
		s := latests(t)
		first := pushLatest(t, s, "a", 1, t0.Add(time.Minute))
		if later := pushLatest(t, s, "a", 1, t0.Add(5*time.Minute)); later.ID != first.ID {
			t.Errorf("the second push set the ID %s, want the waiting job's, %s", later.ID, first.ID)
		}
		if j := claim(t, s, t0.Add(4*time.Minute), t0.Add(time.Hour)); j != nil {
			t.Fatalf("claimed %+v at 4 minutes, where the latest push says 5", j)
		}
		pushLatest(t, s, "a", 1, t0.Add(2*time.Minute))
		j := claimed(t, s, t0.Add(2*time.Minute), t0.Add(time.Hour))
		if j.ID != first.ID || !j.RunAt.Equal(t0.Add(2*time.Minute)) {
			t.Errorf("claimed %+v: want the one job, due at 2 minutes, as the latest push says", j)
		}
		if err := s.Done(ctx, j); err != nil {
			t.Fatal(err)
		}
		if n := count(t, s); n != 0 {
			t.Errorf("%d jobs more, want the one", n)
		}
	})

	t.Run("a push of a key whose job runs is pushed, and leaves the running job alone", func(t *testing.T) {
		s := latests(t)
		first := pushLatest(t, s, "a", 1, t0)
		running := claimed(t, s, t0, t0.Add(time.Minute))
		second := pushLatest(t, s, "a", 1, t0.Add(time.Hour))
		if second.ID == first.ID {
			t.Fatal("the push moved the job that runs, where its key was let go")
		}
		running.RunAt, running.Error = t0.Add(10*time.Minute), "try again"
		if err := s.Retry(ctx, running); err != nil {
			t.Fatal(err)
		}
		j := claimed(t, s, t0.Add(10*time.Minute), t0.Add(11*time.Minute))
		if j.ID != first.ID {
			t.Errorf("claimed %+v at 10 minutes, want the job put back, due then", j)
		}
		if err := s.Done(ctx, j); err != nil {
			t.Fatal(err)
		}
		if j := claimed(t, s, t0.Add(time.Hour), t0.Add(2*time.Hour)); j.ID != second.ID {
			t.Errorf("claimed %+v at an hour, want the one pushed as the first ran", j)
		}
	})

	t.Run("of pushes of a key at once, one job waits", func(t *testing.T) {
		s := latests(t)
		var wg sync.WaitGroup
		for i := range 8 {
			wg.Go(func() {
				j := &queue.Job{Kind: "count", Key: "a", Payload: []byte(`{}`), RunAt: t0.Add(time.Duration(i) * time.Second)}
				if err := s.PushLatest(ctx, j); err != nil {
					t.Errorf("PushLatest: %v", err)
				}
			})
		}
		wg.Wait()
		if n := count(t, s); n != 1 {
			t.Errorf("%d jobs, want 1", n)
		}
	})

	// The rest are a OneAtATimeStore's.
	alones := func(t *testing.T) queue.OneAtATimeStore {
		t.Helper()
		s, ok := open(t).(queue.OneAtATimeStore)
		if !ok {
			t.Skip("not a OneAtATimeStore")
		}
		return s
	}
	pushAlone := func(t *testing.T, s queue.OneAtATimeStore, kind, key string, n int) *queue.Job {
		t.Helper()
		j := &queue.Job{Kind: kind, Key: key, OneAtATime: true, Payload: []byte(`{"n":` + strconv.Itoa(n) + `}`), RunAt: t0}
		if pushed, err := s.PushUnique(ctx, j); err != nil || !pushed {
			t.Fatalf("PushUnique of a key no job waits with: %v, %v", pushed, err)
		}
		return j
	}

	t.Run("a job isn't claimed while another of its kind and key runs, and is once that one is done", func(t *testing.T) {
		s := alones(t)
		pushAlone(t, s, "count", "a", 1)
		first := claimed(t, s, t0, t0.Add(time.Minute))
		pushAlone(t, s, "count", "a", 2) // as the first runs, whose claim let its key go
		if j := claim(t, s, t0, t0.Add(time.Minute)); j != nil {
			t.Fatalf("claimed %+v while the job of its key runs", j)
		}
		if err := s.Done(ctx, first); err != nil {
			t.Fatal(err)
		}
		if j := claimed(t, s, t0, t0.Add(time.Minute)); n(t, j) != 2 {
			t.Errorf("claimed the job pushed with %d once the first was done, want 2", n(t, j))
		}
	})

	// Once the job that ran is put back or has failed, the next of its key
	// runs; once its worker is lost, it runs again itself, due as it is,
	// and pushed first, and the next waits for that.
	for end, finish := range map[string]func(t *testing.T, s queue.OneAtATimeStore, j *queue.Job) (now time.Time, next int){
		"is put back": func(t *testing.T, s queue.OneAtATimeStore, j *queue.Job) (time.Time, int) {
			j.RunAt, j.Error = t0.Add(time.Hour), "try again later"
			if err := s.Retry(ctx, j); err != nil {
				t.Fatal(err)
			}
			return t0, 2
		},
		"fails": func(t *testing.T, s queue.OneAtATimeStore, j *queue.Job) (time.Time, int) {
			j.Error = "no such post"
			if err := s.Fail(ctx, j); err != nil {
				t.Fatal(err)
			}
			return t0, 2
		},
		"loses its worker": func(*testing.T, queue.OneAtATimeStore, *queue.Job) (time.Time, int) {
			return t0.Add(time.Minute), 1 // its hold runs out
		},
	} {
		t.Run("the next job of a key is claimed once the one that ran "+end, func(t *testing.T) {
			s := alones(t)
			pushAlone(t, s, "count", "a", 1)
			first := claimed(t, s, t0, t0.Add(time.Minute))
			pushAlone(t, s, "count", "a", 2)
			now, next := finish(t, s, first)
			if j := claimed(t, s, now, now.Add(time.Minute)); n(t, j) != next {
				t.Errorf("claimed the job pushed with %d, want %d", n(t, j), next)
			}
			if j := claim(t, s, now, now.Add(time.Minute)); j != nil {
				t.Errorf("claimed %+v beside the job of its key", j)
			}
		})
	}

	t.Run("jobs of other keys and kinds run beside one that runs", func(t *testing.T) {
		s := alones(t)
		pushAlone(t, s, "count", "a", 1)
		claimed(t, s, t0, t0.Add(time.Minute))
		pushAlone(t, s, "count", "b", 2)
		pushAlone(t, s, "other", "a", 3)
		push(t, s, 4, t0) // a job with no key
		var got []int
		for range 4 {
			j := claim(t, s, t0, t0.Add(time.Minute))
			if j == nil {
				break
			}
			got = append(got, n(t, j))
		}
		slices.Sort(got)
		if !slices.Equal(got, []int{2, 3, 4}) {
			t.Errorf("claimed the jobs pushed with %v beside the first, want 2, 3 and 4", got)
		}
	})

	t.Run("a job put back keeps its key, so the one pushed as it waits doesn't run beside it", func(t *testing.T) {
		s := alones(t)
		pushAlone(t, s, "count", "a", 1)
		first := claimed(t, s, t0, t0.Add(time.Minute))
		first.RunAt, first.Error = t0, "try again"
		if err := s.Retry(ctx, first); err != nil {
			t.Fatal(err)
		}
		pushAlone(t, s, "count", "a", 2) // the first's claim let its key go
		one := claimed(t, s, t0, t0.Add(time.Minute))
		if other := claim(t, s, t0, t0.Add(time.Minute)); other != nil {
			t.Errorf("claimed %+v beside %+v, of the same key", other, one)
		}
	})

	t.Run("claims at once never get two jobs of one key", func(t *testing.T) {
		s := alones(t)
		// Jobs of one key that all come due at once: each pushed as the one
		// before ran, whose claim let the key go, and each put back for an
		// hour on.
		for i := range 8 {
			pushAlone(t, s, "count", "a", i)
			j := claimed(t, s, t0, t0.Add(time.Minute))
			j.RunAt, j.Error = t0.Add(time.Hour), "try again"
			if err := s.Retry(ctx, j); err != nil {
				t.Fatal(err)
			}
		}
		var got atomic.Int32
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				j, err := s.Claim(ctx, t0.Add(time.Hour), t0.Add(2*time.Hour))
				if err != nil {
					t.Errorf("Claim: %v", err)
				}
				if j != nil {
					got.Add(1)
				}
			})
		}
		wg.Wait()
		if got.Load() != 1 {
			t.Errorf("%d claims at once got a job of the key, want 1", got.Load())
		}
	})

	t.Run("a scheduled run of a OneAtATime kind, and a latest push, keep the key too", func(t *testing.T) {
		s := alones(t)
		pushAlone(t, s, "report", "a", 1)
		running := claimed(t, s, t0, t0.Add(time.Minute))
		if ss, ok := s.(queue.ScheduleStore); ok {
			run := &queue.Job{Kind: "report", Key: "a", OneAtATime: true, Payload: []byte(`{"n":2}`), RunAt: t0}
			if pushed, err := ss.PushScheduled(ctx, "report", run); err != nil || !pushed {
				t.Fatalf("PushScheduled: %v, %v", pushed, err)
			}
		}
		if ls, ok := s.(queue.LatestStore); ok {
			j := &queue.Job{Kind: "report", Key: "b", OneAtATime: true, Payload: []byte(`{"n":3}`), RunAt: t0}
			if err := ls.PushLatest(ctx, j); err != nil {
				t.Fatal(err)
			}
			claimed(t, s, t0, t0.Add(time.Minute)) // b's, which runs beside a's
			j = &queue.Job{Kind: "report", Key: "b", OneAtATime: true, Payload: []byte(`{"n":4}`), RunAt: t0}
			if err := ls.PushLatest(ctx, j); err != nil {
				t.Fatal(err)
			}
		}
		if j := claim(t, s, t0, t0.Add(time.Minute)); j != nil {
			t.Errorf("claimed %+v beside the job of its key that runs", j)
		}
		if err := s.Done(ctx, running); err != nil {
			t.Fatal(err)
		}
	})

	// The rest are a FailedStore's.
	faileds := func(t *testing.T) queue.FailedStore {
		t.Helper()
		s, ok := open(t).(queue.FailedStore)
		if !ok {
			t.Skip("not a FailedStore")
		}
		return s
	}
	fail := func(t *testing.T, s queue.FailedStore, n int, err string, at time.Time) *queue.Job {
		t.Helper()
		push(t, s, n, t0)
		j := claimed(t, s, t0, t0.Add(time.Minute))
		j.Error, j.FailedAt = err, at
		if err := s.Fail(ctx, j); err != nil {
			t.Fatal(err)
		}
		return j
	}

	t.Run("the jobs that failed for good are listed, the latest first, with their errors", func(t *testing.T) {
		s := faileds(t)
		fail(t, s, 1, "550 no such mailbox", t0)
		fail(t, s, 2, "the user has gone", t0.Add(time.Minute))
		push(t, s, 3, t0.Add(time.Hour)) // one that waits
		failed, err := s.Failed(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(failed) != 2 {
			t.Fatalf("listed %d jobs, want the 2 that failed", len(failed))
		}
		for i, want := range []struct {
			n   int
			err string
		}{{2, "the user has gone"}, {1, "550 no such mailbox"}} {
			j := failed[i]
			if n(t, j) != want.n || j.Kind != "count" || j.Error != want.err || j.Attempts != 1 || j.FailedAt.IsZero() {
				t.Errorf("listed %+v as number %d, want the job pushed with %d, which failed with %q, when", j, i+1, want.n, want.err)
			}
		}
	})

	t.Run("a failed job run again is claimed at its time, from its first attempt", func(t *testing.T) {
		s := faileds(t)
		j := fail(t, s, 1, "550 no such mailbox", t0)
		if again, err := s.RunAgain(ctx, j.ID, t0.Add(time.Hour)); err != nil || !again {
			t.Fatalf("RunAgain: %v, %v", again, err)
		}
		if failed, err := s.Failed(ctx); err != nil || len(failed) != 0 {
			t.Errorf("listed %v as failed, %v: want none", failed, err)
		}
		if early := claim(t, s, t0.Add(59*time.Minute), t0.Add(2*time.Hour)); early != nil {
			t.Fatalf("claimed a minute before it's due: %+v", early)
		}
		if again := claimed(t, s, t0.Add(time.Hour), t0.Add(2*time.Hour)); again.ID != j.ID || again.Attempts != 1 {
			t.Errorf("claimed %+v: want the job, from its first attempt", again)
		}
	})

	t.Run("running again a job that hasn't failed, or isn't there, does nothing", func(t *testing.T) {
		s := faileds(t)
		waiting := push(t, s, 1, t0.Add(time.Hour))
		for _, id := range []string{waiting.ID, "999999"} {
			if again, err := s.RunAgain(ctx, id, t0); err != nil || again {
				t.Errorf("RunAgain(%q): %v, %v, want false", id, again, err)
			}
		}
		if j := claim(t, s, t0, t0.Add(time.Minute)); j != nil {
			t.Errorf("claimed %+v at once, where it's due in an hour", j)
		}
	})
}
