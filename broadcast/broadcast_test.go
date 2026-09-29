package broadcast_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cuonggt/tug/broadcast"
	"github.com/cuonggt/tug/broadcast/broadcasttest"
)

var ctx = context.Background()

func TestMemoryKeepsAStoresPromises(t *testing.T) {
	broadcasttest.TestStore(t, func(t *testing.T) broadcast.Store { return &broadcasttest.Memory{} })
}

// captureLog has the log written to a buffer for the rest of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &buf
}

// received is what's waiting on events now, without waiting for more.
func received(events <-chan broadcast.Event) []broadcast.Event {
	var got []broadcast.Event
	for {
		select {
		case e, ok := <-events:
			if !ok {
				return got
			}
			got = append(got, e)
		default:
			return got
		}
	}
}

func TestAnEventReachesTheSubscribersToItsChannelAlone(t *testing.T) {
	hub := &broadcast.Hub{}
	posts := hub.Subscribe(ctx, "posts")
	ann := hub.Subscribe(ctx, "users.1")
	both := hub.Subscribe(ctx, "posts", "users.1")
	if err := hub.Publish(ctx, "posts", "created", map[string]int{"id": 26}); err != nil {
		t.Fatal(err)
	}
	for name, events := range map[string]<-chan broadcast.Event{"posts": posts, "posts and users.1": both} {
		got := received(events)
		if len(got) != 1 || got[0].Channel != "posts" || got[0].Name != "created" || string(got[0].Data) != `{"id":26}` {
			t.Errorf("%s got %+v", name, got)
		}
	}
	if got := received(ann); len(got) != 0 {
		t.Errorf("users.1 got %+v", got)
	}
	hub.Publish(ctx, "users.1", "verified", nil)
	if got := received(ann); len(got) != 1 || string(got[0].Data) != "null" {
		t.Errorf("users.1 got %+v", got)
	}
}

func TestAnEventReachesEveryInstanceThroughTheStore(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &broadcasttest.Memory{}
		one, other := &broadcast.Hub{Store: store}, &broadcast.Hub{Store: store}
		ctx, stop := context.WithCancel(ctx)
		defer stop()
		for _, hub := range []*broadcast.Hub{one, other} {
			go hub.Run(ctx)
		}
		synctest.Wait() // both listen
		here, there := one.Subscribe(ctx, "posts"), other.Subscribe(ctx, "posts")
		if err := one.Publish(ctx, "posts", "created", 26); err != nil {
			t.Fatal(err)
		}
		for name, events := range map[string]<-chan broadcast.Event{"the one that published": here, "the other": there} {
			if got := received(events); len(got) != 1 || string(got[0].Data) != "26" {
				t.Errorf("%s got %+v", name, got)
			}
		}
	})
}

func TestASubscriptionEndsWithItsContext(t *testing.T) {
	hub := &broadcast.Hub{}
	ctx, cancel := context.WithCancel(ctx)
	events := hub.Subscribe(ctx, "posts")
	cancel()
	select {
	case _, ok := <-events:
		if ok {
			t.Error("an event came after the subscription ended")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the subscription went on after its context was done")
	}
	if err := hub.Publish(context.Background(), "posts", "created", 1); err != nil {
		t.Errorf("publishing with no one subscribed: %v", err)
	}
}

func TestASubscriberThatFallsBehindIsClosed(t *testing.T) {
	hub := &broadcast.Hub{}
	events := hub.Subscribe(ctx, "posts")
	for i := range 20 {
		hub.Publish(ctx, "posts", "created", i)
	}
	got := received(events)
	if len(got) != 16 {
		t.Errorf("got %d events before the subscription closed, want the 16 it held", len(got))
	}
	if _, ok := <-events; ok {
		t.Error("the subscriber that fell behind wasn't closed")
	}
}

func TestWhatCantBeAnEventIsntPublished(t *testing.T) {
	hub := &broadcast.Hub{}
	for name, publish := range map[string]func() error{
		"no channel":           func() error { return hub.Publish(ctx, "", "created", 1) },
		"data JSON can't hold": func() error { return hub.Publish(ctx, "posts", "created", make(chan int)) },
		"one too big": func() error {
			return hub.Publish(ctx, "posts", "created", strings.Repeat("x", broadcast.MaxEvent))
		},
	} {
		if err := publish(); err == nil {
			t.Errorf("%s was published", name)
		}
	}
	if err := hub.Publish(ctx, "posts", "created", strings.Repeat("x", broadcast.MaxEvent-60)); err != nil {
		t.Errorf("one just under the most: %v", err)
	}
}

// recorder is a store that keeps what's published on it.
type recorder struct {
	broadcasttest.Memory
	published [][]byte
}

func (r *recorder) Publish(ctx context.Context, event []byte) error {
	r.published = append(r.published, event)
	return r.Memory.Publish(ctx, event)
}

func TestInPublishesThroughAnotherStore(t *testing.T) {
	hub := &broadcast.Hub{Store: &broadcasttest.Memory{}}
	tx := &recorder{} // as a store of a transaction's is
	if err := hub.In(tx).Publish(ctx, "users.1", "verified", nil); err != nil {
		t.Fatal(err)
	}
	if len(tx.published) != 1 || string(tx.published[0]) != `{"channel":"users.1","name":"verified","data":null}` {
		t.Errorf("the transaction's store got %q", tx.published)
	}
}

func TestAnEventTheStoreCarriesThatIsntOneIsLeftOut(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		logs := captureLog(t)
		store := &broadcasttest.Memory{}
		hub := &broadcast.Hub{Store: store}
		ctx, stop := context.WithCancel(ctx)
		defer stop()
		go hub.Run(ctx)
		synctest.Wait()
		events := hub.Subscribe(ctx, "posts")
		store.Publish(ctx, []byte("not an event"))
		if got := received(events); len(got) != 0 {
			t.Errorf("got %+v", got)
		}
		if !strings.Contains(logs.String(), "isn't one") {
			t.Errorf("the log:\n%s", logs)
		}
	})
}

// flaky is a store whose Listen fails, the first time, once it has been
// listening, as when a connection drops.
type flaky struct {
	broadcasttest.Memory
	listens atomic.Int32
}

func (f *flaky) Listen(ctx context.Context, deliver func([]byte)) error {
	if f.listens.Add(1) == 1 {
		time.Sleep(time.Second)
		return errors.New("the connection dropped")
	}
	return f.Memory.Listen(ctx, deliver)
}

func TestRunListensAgainWhenTheStoreStops(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		logs := captureLog(t)
		store := &flaky{}
		hub := &broadcast.Hub{Store: store}
		ctx, stop := context.WithCancel(ctx)
		defer stop()
		go hub.Run(ctx)
		events := hub.Subscribe(ctx, "posts")
		time.Sleep(1500 * time.Millisecond) // the first Listen has failed, and Run waits a second
		// The subscriber may have missed events, so it's closed, for its page
		// to connect again and reload.
		if _, ok := <-events; ok {
			t.Error("a subscriber was left open as the store stopped")
		}
		time.Sleep(time.Second)
		synctest.Wait()
		again := hub.Subscribe(ctx, "posts")
		store.Publish(ctx, []byte(`{"channel":"posts","name":"created","data":1}`))
		if got := received(again); len(got) != 1 || store.listens.Load() != 2 {
			t.Errorf("after listening again: %+v, listened %d times", got, store.listens.Load())
		}
		if !strings.Contains(logs.String(), "the connection dropped") {
			t.Errorf("the log:\n%s", logs)
		}
	})
}
