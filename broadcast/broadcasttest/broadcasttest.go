// Package broadcasttest is for testing with package broadcast: Memory is
// a Store in memory, and TestStore checks that a Store keeps the promises
// a Hub depends on.
package broadcasttest

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cuonggt/tug/broadcast"
)

// Memory is a broadcast.Store in memory, for tests: what's published on
// it, every Listen on it gets, as every instance of an app does on a
// database. The zero value is ready to use.
type Memory struct {
	mu        sync.Mutex
	listeners map[*func([]byte)]bool
}

func (m *Memory) Publish(_ context.Context, event []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for deliver := range m.listeners {
		(*deliver)(bytes.Clone(event))
	}
	return nil
}

func (m *Memory) Listen(ctx context.Context, deliver func([]byte)) error {
	m.mu.Lock()
	if m.listeners == nil {
		m.listeners = map[*func([]byte)]bool{}
	}
	m.listeners[&deliver] = true
	m.mu.Unlock()
	<-ctx.Done()
	m.mu.Lock()
	delete(m.listeners, &deliver)
	m.mu.Unlock()
	return ctx.Err()
}

// TestStore checks that the stores open returns keep the promises a
// broadcast.Hub depends on, with a new one for each test, which two
// Listens on it share, as two instances of the app share a database. A
// store's own tests call it, as the auth starter's do for its database:
//
//	func TestTheBroadcastsKeepTheirPromises(t *testing.T) {
//		broadcasttest.TestStore(t, func(t *testing.T) broadcast.Store {
//			return &broadcasts{db: testDB(t)}
//		})
//	}
func TestStore(t *testing.T, open func(t *testing.T) broadcast.Store) {
	ctx := context.Background()
	publish := func(t *testing.T, s broadcast.Store, event string) {
		t.Helper()
		if err := s.Publish(ctx, []byte(event)); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	t.Run("an event reaches every instance that listens, its own too", func(t *testing.T) {
		s := open(t)
		one, other := listen(t, s), listen(t, s)
		publish(t, s, `{"n":1}`)
		for _, l := range []*listener{one, other} {
			if got := l.next(t); got != `{"n":1}` {
				t.Errorf("got %s", got)
			}
		}
	})

	t.Run("events come in the order they were published", func(t *testing.T) {
		s := open(t)
		l := listen(t, s)
		for i := range 10 {
			publish(t, s, fmt.Sprintf(`{"n":%d}`, i))
		}
		for i := range 10 {
			if got, want := l.next(t), fmt.Sprintf(`{"n":%d}`, i); got != want {
				t.Fatalf("event %d: %s, want %s", i, got, want)
			}
		}
	})

	t.Run("what was published before listening isn't handed on", func(t *testing.T) {
		s := open(t)
		publish(t, s, `{"when":"before"}`)
		l := listen(t, s)
		publish(t, s, `{"when":"after"}`)
		if got := l.next(t); got != `{"when":"after"}` {
			t.Errorf("got %s, want only what came after", got)
		}
	})

	t.Run("events published at once all come, each once", func(t *testing.T) {
		s := open(t)
		l := listen(t, s)
		const at = 20
		var wg sync.WaitGroup
		for i := range at {
			wg.Go(func() {
				if err := s.Publish(ctx, fmt.Appendf(nil, `{"n":%d}`, i)); err != nil {
					t.Errorf("Publish: %v", err)
				}
			})
		}
		wg.Wait()
		seen := map[string]bool{}
		for range at {
			got := l.next(t)
			if seen[got] {
				t.Fatalf("%s came twice", got)
			}
			seen[got] = true
		}
	})

	t.Run("an event as big as a hub sends comes whole", func(t *testing.T) {
		s := open(t)
		l := listen(t, s)
		event := `{"d":"` + strings.Repeat("é", (broadcast.MaxEvent-len(`{"d":""}`))/2) + `"}`
		publish(t, s, event)
		if got := l.next(t); got != event {
			t.Errorf("an event of %d bytes came as %d", len(event), len(got))
		}
	})

	t.Run("Listen returns as its context is done", func(t *testing.T) {
		s := open(t)
		ctx, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- s.Listen(ctx, func([]byte) {}) }()
		time.Sleep(100 * time.Millisecond)
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("Listen returned %v, want the context's error", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("Listen went on after its context was done")
		}
	})
}

// listener is one Listen on a store, as an instance of the app has, with
// what it has been handed.
type listener struct {
	events chan string
}

// probe starts an event that a listener sends itself until one comes
// back, to know it listens: a store listens from a moment after Listen is
// called, which nothing else tells.
const probe = `{"probe":`

// listen starts a Listen on s, for the rest of the test, and returns once
// it hears what's published.
func listen(t *testing.T, s broadcast.Store) *listener {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	l := &listener{events: make(chan string, 1000)}
	done := make(chan error, 1)
	go func() { done <- s.Listen(ctx, func(e []byte) { l.events <- string(e) }) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	mine := probe + `"` + rand.Text() + `"}`
	deadline := time.After(10 * time.Second)
	for {
		if err := s.Publish(ctx, []byte(mine)); err != nil {
			t.Fatalf("Publish: %v", err)
		}
		for waiting := time.After(100 * time.Millisecond); ; {
			select {
			case e := <-l.events:
				if e == mine {
					return l
				}
				continue
			case err := <-done:
				t.Fatalf("Listen returned %v before it heard anything", err)
			case <-waiting:
			case <-deadline:
				t.Fatal("Listen heard nothing published in 10 seconds")
			}
			break
		}
	}
}

// next returns the next event l was handed, but for the probes the
// listeners sent themselves, waiting up to 10 seconds for it.
func (l *listener) next(t *testing.T) string {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case e := <-l.events:
			if !strings.HasPrefix(e, probe) {
				return e
			}
		case <-deadline:
			t.Fatal("no event came in 10 seconds")
			return ""
		}
	}
}
