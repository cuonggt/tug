package storage

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// source is a KeySource's fetch for a test: keys that last an hour from
// the clock, each set its own, counting the fetches, failing while down.
type source struct {
	now     *time.Time
	fetches atomic.Int32
	down    atomic.Bool
	lasts   time.Duration
}

func (s *source) fetch(ctx context.Context) (Keys, error) {
	n := s.fetches.Add(1)
	if s.down.Load() {
		return Keys{}, errors.New("the source is down")
	}
	lasts := s.lasts
	if lasts == 0 {
		lasts = time.Hour
	}
	return Keys{AccessKeyID: "ASIA" + string(rune('A'+n)), SecretAccessKey: "secret", SessionToken: "token", Expires: s.now.Add(lasts)}, nil
}

// logged captures what the package logs while a test runs.
func logged(t *testing.T) *bytes.Buffer {
	var out bytes.Buffer
	was := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&out, nil)))
	t.Cleanup(func() { slog.SetDefault(was) })
	return &out
}

func TestKeysAreKeptUntilFiveMinutesBeforeTheyExpire(t *testing.T) {
	now := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	src := &source{now: &now}
	var f fetched
	clock := func() time.Time { return now }
	first, err := f.get(ctx, clock, src.fetch)
	if err != nil || first.AccessKeyID == "" || src.fetches.Load() != 1 {
		t.Fatalf("the first keys: %+v, %v, %d fetches", first, err, src.fetches.Load())
	}
	now = now.Add(54 * time.Minute)
	if k, _ := f.get(ctx, clock, src.fetch); k != first || src.fetches.Load() != 1 {
		t.Errorf("54 minutes on: %+v, %d fetches", k, src.fetches.Load())
	}
	now = now.Add(time.Minute)
	if k, _ := f.get(ctx, clock, src.fetch); k == first || src.fetches.Load() != 2 {
		t.Errorf("five minutes before they expire: %+v, %d fetches", k, src.fetches.Load())
	}
}

func TestManySigningsAtOnceWaitForOneFetch(t *testing.T) {
	now := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	release := make(chan struct{})
	src := &source{now: &now}
	slow := func(ctx context.Context) (Keys, error) {
		<-release
		return src.fetch(ctx)
	}
	var f fetched
	var wg sync.WaitGroup
	got := make(chan Keys, 10)
	for range 10 {
		wg.Go(func() {
			k, err := f.get(ctx, func() time.Time { return now }, slow)
			if err != nil {
				t.Error(err)
			}
			got <- k
		})
	}
	time.Sleep(20 * time.Millisecond) // the ten are waiting
	close(release)
	wg.Wait()
	close(got)
	for k := range got {
		if k.AccessKeyID != "ASIAB" {
			t.Errorf("a signing got %+v", k)
		}
	}
	if n := src.fetches.Load(); n != 1 {
		t.Errorf("%d fetches for ten signings at once", n)
	}
}

func TestKeysThatStillWorkSignWhileTheNextAreFetched(t *testing.T) {
	now := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	src := &source{now: &now}
	var f fetched
	clock := func() time.Time { return now }
	first, _ := f.get(ctx, clock, src.fetch)
	now = now.Add(57 * time.Minute)
	fetching, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		f.get(ctx, clock, func(ctx context.Context) (Keys, error) {
			close(fetching)
			<-release
			return src.fetch(ctx)
		})
	}()
	<-fetching
	if k, err := f.get(ctx, clock, src.fetch); err != nil || k != first {
		t.Errorf("a signing as the next keys are fetched: %+v, %v", k, err)
	}
	close(release)
	<-done
}

func TestAFetchThatFailsLeavesTheKeysThatStillWork(t *testing.T) {
	out := logged(t)
	now := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	src := &source{now: &now}
	var f fetched
	clock := func() time.Time { return now }
	first, _ := f.get(ctx, clock, src.fetch)
	src.down.Store(true)
	now = now.Add(56 * time.Minute)
	if k, err := f.get(ctx, clock, src.fetch); err != nil || k != first {
		t.Fatalf("with the source down: %+v, %v", k, err)
	}
	if !strings.Contains(out.String(), "the next keys for the S3 couldn't be fetched") || !strings.Contains(out.String(), "the source is down") {
		t.Errorf("logged %q", out)
	}
	// Not asked again by every signing, but a few seconds on.
	f.get(ctx, clock, src.fetch)
	if n := src.fetches.Load(); n != 2 {
		t.Errorf("%d fetches, with the source down a moment", n)
	}
	now = now.Add(fetchAgainAfter)
	f.get(ctx, clock, src.fetch)
	if n := src.fetches.Load(); n != 3 {
		t.Errorf("%d fetches, a few seconds on", n)
	}
	// Once the keys have expired, a signing has none.
	now = first.Expires
	if _, err := f.get(ctx, clock, src.fetch); err == nil || err.Error() != "the source is down" {
		t.Errorf("with the keys expired and the source down: %v", err)
	}
	src.down.Store(false)
	if _, err := f.get(ctx, clock, src.fetch); err == nil {
		t.Error("a fetch came before a few seconds were up")
	}
	now = now.Add(fetchAgainAfter)
	if k, err := f.get(ctx, clock, src.fetch); err != nil || k.AccessKeyID == "" {
		t.Errorf("with the source back: %+v, %v", k, err)
	}
}

func TestASigningThatGivesUpIsntTheSourceFailing(t *testing.T) {
	now := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	src := &source{now: &now}
	var f fetched
	clock := func() time.Time { return now }
	gone, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := f.get(gone, clock, func(ctx context.Context) (Keys, error) { return Keys{}, ctx.Err() }); !errors.Is(err, context.Canceled) {
		t.Fatalf("a signing that gave up: %v", err)
	}
	if k, err := f.get(ctx, clock, src.fetch); err != nil || k.AccessKeyID == "" {
		t.Errorf("the next signing, at once: %+v, %v", k, err)
	}
}

func TestKeysThatExpiredBeforeTheyCameArentKeys(t *testing.T) {
	now := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	src := &source{now: &now, lasts: -time.Minute}
	var f fetched
	if _, err := f.get(ctx, func() time.Time { return now }, src.fetch); err == nil || !strings.Contains(err.Error(), "before they came") {
		t.Errorf("keys that expired a minute ago: %v", err)
	}
}

// A source that hasn't the next keys yet gives the ones that are due,
// which sign until it has, and aren't fetched again by every signing.
func TestKeysThatAreDueAlreadyArentFetchedAgainAtOnce(t *testing.T) {
	now := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	src := &source{now: &now, lasts: 2 * time.Minute}
	var f fetched
	clock := func() time.Time { return now }
	first, err := f.get(ctx, clock, src.fetch)
	if err != nil {
		t.Fatal(err)
	}
	if k, _ := f.get(ctx, clock, src.fetch); k != first || src.fetches.Load() != 1 {
		t.Errorf("asked again at once: %+v, %d fetches", k, src.fetches.Load())
	}
	now = now.Add(fetchAgainAfter)
	f.get(ctx, clock, src.fetch)
	if n := src.fetches.Load(); n != 2 {
		t.Errorf("%d fetches, a few seconds on", n)
	}
}
