package auth

import (
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newThrottle(now *time.Time) *Throttle {
	return &Throttle{now: func() time.Time { return *now }}
}

func TestAKeyWaitsAfterItsTries(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	th := newThrottle(&now)
	for i := range 5 {
		if wait := th.Try("ann@example.com|1.2.3.4"); wait != 0 {
			t.Fatalf("waits %v on try %d", wait, i+1)
		}
		now = now.Add(10 * time.Second)
	}
	if wait := th.Try("ann@example.com|1.2.3.4"); wait != 10*time.Second {
		t.Errorf("waits %v, want the rest of the minute since the first try", wait)
	}
	if wait := th.Wait("ann@example.com|1.2.3.4"); wait != 10*time.Second {
		t.Errorf("Wait says %v", wait)
	}
	if wait := th.Try("bob@example.com|1.2.3.4"); wait != 0 {
		t.Errorf("another key waits %v", wait)
	}
}

func TestTheWaitEndsWithTheWindow(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	th := &Throttle{Max: 2, Window: time.Hour, now: func() time.Time { return now }}
	th.Try("k")
	th.Try("k")
	if wait := th.Try("k"); wait != time.Hour {
		t.Fatalf("waits %v", wait)
	}
	now = now.Add(time.Hour)
	if wait := th.Wait("k"); wait != 0 {
		t.Errorf("still waits %v after the window", wait)
	}
	th.Try("k")
	if wait := th.Try("k"); wait != 0 {
		t.Errorf("a try after the window counts with the ones before: waits %v", wait)
	}
}

func TestAWaitingTryDoesntCount(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	th := &Throttle{Max: 1, now: func() time.Time { return now }}
	th.Try("k")
	for range 10 {
		th.Try("k")
	}
	if th.keys["k"].count != 1 {
		t.Errorf("counted %d tries, when the ones that waited weren't made", th.keys["k"].count)
	}
}

func TestTriesSentAtOnceDontAllGetIn(t *testing.T) {
	th := &Throttle{Max: 5}
	var in atomic.Int32
	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			if th.Try("k") == 0 {
				in.Add(1)
			}
		})
	}
	wg.Wait()
	if in.Load() != 5 {
		t.Errorf("%d tries got in, want 5", in.Load())
	}
}

func TestSucceedingClearsTheTries(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	th := newThrottle(&now)
	for range 5 {
		th.Try("k")
	}
	th.Clear("k")
	if wait := th.Try("k"); wait != 0 {
		t.Errorf("waits %v after clearing", wait)
	}
}

func TestOldTriesAreSweptAway(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	th := newThrottle(&now)
	for i := range 1500 {
		th.Try(strconv.Itoa(i))
	}
	now = now.Add(2 * time.Minute)
	for i := range 1500 {
		th.Try("new " + strconv.Itoa(i))
	}
	if len(th.keys) > 2000 {
		t.Errorf("%d keys kept, most of them long over", len(th.keys))
	}
}
