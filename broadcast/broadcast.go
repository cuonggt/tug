// Package broadcast carries events between an app's instances, to the
// pages open on any of them: a post one instance saves, the pages on the
// others hear of, through a route of tug's Ctx.Events. An event goes on a
// channel, a name the app picks, as "posts" or "users.42", and every
// subscriber to the channel gets it, on every instance.
//
//	hub := &broadcast.Hub{Store: store} // without a Store, in memory, for one instance
//	app.Go(hub.Run)                     // listens to the Store
//
//	err := hub.Publish(ctx, "posts", "created", map[string]int64{"id": post.ID})
//
//	app.Get("/posts/events", func(c *tug.Ctx) error {
//		return c.Events(func(ctx context.Context, send func(tug.Event) error) error {
//			for e := range hub.Subscribe(ctx, "posts") {
//				if err := send(tug.Event{Name: e.Name, Data: e.Data}); err != nil {
//					return err
//				}
//			}
//			return nil
//		})
//	})
//
// An event reaches the subscribers there are as it's published, at most
// once: one that wasn't subscribed, or fell behind, doesn't get it, and a
// page that connects again reloads what it shows. It has no import of tug.
package broadcast

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Event is an event on a channel, as a subscriber gets it.
type Event struct {
	Channel string
	Name    string
	Data    json.RawMessage // JSON, null for none
}

// Store carries events between the instances of an app, such as its
// database does: what one publishes, every one's Listen hands on, its
// own too. An event is bytes to it, which it carries as they are. Its
// methods are called from several goroutines at once, and Listen once on
// each instance. Package broadcasttest checks a store keeps these
// promises.
type Store interface {
	// Publish sends an event to every instance's Listen.
	Publish(ctx context.Context, event []byte) error

	// Listen hands deliver each event published from the time it listens,
	// in the order they were published, until ctx is done, and returns
	// ctx's error then, or the store's, as when its connection drops.
	Listen(ctx context.Context, deliver func(event []byte)) error
}

// Hub publishes events on channels, and hands each to the subscribers to
// its channel on every instance of the app, through its Store: Publish
// sends one, Subscribe follows channels, and Run, which the app runs with
// App.Go, listens to the Store. Share one by pointer.
type Hub struct {
	// Store carries the events between the app's instances, such as its
	// database. Nil hands them to this instance's subscribers alone, for an
	// app of one instance, and for tests.
	Store Store

	mu   sync.Mutex
	subs map[*subscriber]bool
}

// subscriber is one Subscribe's channels, and where its events go.
type subscriber struct {
	channels map[string]bool
	events   chan Event
}

// MaxEvent is the most an event's JSON may be, its channel, name and data
// together, in bytes: a Postgres NOTIFY carries 8000 at most, and an event
// says what changed, for a page to reload what it shows.
const MaxEvent = 7000

// wire is an event as the Store carries it.
type wire struct {
	Channel string          `json:"channel"`
	Name    string          `json:"name"`
	Data    json.RawMessage `json:"data"`
}

// Publish sends the event name, with data as JSON, on channel, to every
// subscriber to the channel on every instance: data is what changed, as
// an ID, for the page to reload what it shows, or nil. An event with no
// channel, over MaxEvent, or with data encoding/json can't encode, is an
// error, as is the Store's.
func (h *Hub) Publish(ctx context.Context, channel, name string, data any) error {
	return h.publish(ctx, h.Store, channel, name, data)
}

// In returns a Publisher that sends through s: a Store the app made of a
// transaction of its own, so the event goes as the transaction commits,
// with what it writes, or not at all.
func (h *Hub) In(s Store) Publisher {
	return &publisher{hub: h, store: s}
}

// Publisher publishes events, as Hub.Publish does, through a Store of the
// app's: see Hub.In.
type Publisher interface {
	Publish(ctx context.Context, channel, name string, data any) error
}

type publisher struct {
	hub   *Hub
	store Store
}

func (p *publisher) Publish(ctx context.Context, channel, name string, data any) error {
	return p.hub.publish(ctx, p.store, channel, name, data)
}

func (h *Hub) publish(ctx context.Context, store Store, channel, name string, data any) error {
	if channel == "" {
		return errors.New("broadcast: an event goes on a channel, such as posts")
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("broadcast: the event's data can't be sent as JSON: %w", err)
	}
	event, err := json.Marshal(wire{channel, name, raw})
	if err != nil {
		return err
	}
	if len(event) > MaxEvent {
		return fmt.Errorf("broadcast: an event is %d bytes at most, and this one is %d: send what changed, for the page to reload the rest", MaxEvent, len(event))
	}
	if store == nil {
		h.deliver(event)
		return nil
	}
	return store.Publish(ctx, event)
}

// Subscribe returns the events published on channels from now on, on any
// instance, until ctx is done, when the Go channel it returns closes. A
// subscriber that falls behind, with events waiting that it hasn't taken,
// is closed early, as it's missing some: its page, which connects again,
// reloads what it shows.
func (h *Hub) Subscribe(ctx context.Context, channels ...string) <-chan Event {
	s := &subscriber{channels: map[string]bool{}, events: make(chan Event, 16)}
	for _, c := range channels {
		s.channels[c] = true
	}
	h.mu.Lock()
	if h.subs == nil {
		h.subs = map[*subscriber]bool{}
	}
	h.subs[s] = true
	h.mu.Unlock()
	context.AfterFunc(ctx, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.drop(s)
	})
	return s.events
}

// drop closes s, unless it's closed already; h.mu is held.
func (h *Hub) drop(s *subscriber) {
	if h.subs[s] {
		delete(h.subs, s)
		close(s.events)
	}
}

// deliver hands an event, as the Store carries it, to this instance's
// subscribers to its channel.
func (h *Hub) deliver(event []byte) {
	var w wire
	if err := json.Unmarshal(event, &w); err != nil || w.Channel == "" {
		slog.Error("broadcast: an event the store carried isn't one, so it's left out", "event", string(event), "err", err)
		return
	}
	e := Event{Channel: w.Channel, Name: w.Name, Data: w.Data}
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs {
		if !s.channels[e.Channel] {
			continue
		}
		select {
		case s.events <- e:
		default:
			// It has fallen behind, and missed this: its stream ends, and
			// its page, connecting again, reloads what it missed.
			h.drop(s)
		}
	}
}

// listenAgain is how long Run waits to listen to the Store again after it
// stops, at first; it doubles each time, to a minute.
var listenAgain = time.Second

// Run listens to the Store, and hands each event any instance publishes
// to this instance's subscribers, until ctx is done: run it beside the
// server, with App.Go. When the Store stops listening, as when its
// connection drops, Run logs why, closes every subscriber, as each may
// have missed an event, and listens again, after a second, then two, up
// to a minute. Without a Store, it waits for ctx.
func (h *Hub) Run(ctx context.Context) error {
	if h.Store == nil {
		<-ctx.Done()
		return nil
	}
	wait := listenAgain
	for {
		err := h.Store.Listen(ctx, h.deliver)
		if ctx.Err() != nil {
			return nil
		}
		slog.Error("broadcast: the store stopped listening, and is listened to again", "err", err, "in", wait)
		h.mu.Lock()
		for s := range h.subs {
			h.drop(s)
		}
		h.mu.Unlock()
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil
		}
		wait = min(2*wait, time.Minute)
	}
}
