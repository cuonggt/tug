package broadcast

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"time"
)

// Member is who's on a channel: ID says who they are to the app, as a
// user's ID or a login's, and Data whatever the pages should know of them,
// as their name, which goes as JSON. A member may be on a channel from
// more than one page at once, and is there once; Hub.Here gives them back
// with Data as the JSON it was kept as.
type Member struct {
	ID   string
	Data any
}

// PresenceStore keeps who's on a channel between the instances of an app,
// as a Store carries its events: a table of the app's database, say, where
// a member stays while its lease lasts and no longer, so an instance that
// stops without a word leaves nobody behind for long. Its methods are
// called from several goroutines at once. Package broadcasttest checks a
// store keeps these promises.
type PresenceStore interface {
	// Enter keeps m on the channel under key, one page's connection to it,
	// until Leave or until, and says whether the member was new to the
	// channel, so a hub tells those on it of an arrival alone. The same key
	// entered again is the same connection, with its lease moved on.
	Enter(ctx context.Context, channel string, m Member, key string, until time.Time) (bool, error)

	// Keep moves the lease of each key still on a channel to until: the
	// pages of this instance saying they're still open. A key that has left
	// is left out, not an error.
	Keep(ctx context.Context, keys []string, until time.Time) error

	// Leave takes key off the channel, and says whether its member has no
	// connection left there, so a hub tells those on it of a departure
	// alone. A key that isn't there is no error, and nobody left.
	Leave(ctx context.Context, channel, key string) (bool, error)

	// Here is who's on the channel, each member once, by their ID, leaving
	// out those whose lease has ended.
	Here(ctx context.Context, channel string) ([]Member, error)
}

// lease is how long a member stays on a channel without saying it's still
// there, without a Hub.Lease of the app's own.
const lease = time.Minute

// leaving is how long a hub has to take a member off a channel once its
// page has gone: its request's context is done by then, so the store gets
// one of its own.
const leaving = 5 * time.Second

// Enter follows channel as Subscribe does, and keeps m on it while that
// subscription lives: those on the channel hear "presence.joined", with
// the member as the event's data, as one arrives that wasn't there, and
// "presence.left" as the last page of one goes, so a page that shows who's
// here reloads it then. Hub.Here is who's on a channel.
//
// A member's second page is a second connection, not a second arrival.
// Entering needs the lease said again while the page is open, which Run
// does for this instance's members: an app with presence runs it.
func (h *Hub) Enter(ctx context.Context, m Member, channel string) (<-chan Event, error) {
	if channel == "" {
		return nil, errors.New("broadcast: a member is on a channel, such as users.42")
	}
	key := rand.Text()
	store := h.presence()
	arrived, err := store.Enter(ctx, channel, m, key, time.Now().Add(h.lease()))
	if err != nil {
		return nil, fmt.Errorf("broadcast: keeping a member on %s: %w", channel, err)
	}
	h.mu.Lock()
	if h.here == nil {
		h.here = map[string]string{}
	}
	h.here[key] = channel
	h.mu.Unlock()
	events := h.Subscribe(ctx, channel)
	if arrived {
		if err := h.Publish(ctx, channel, "presence.joined", m); err != nil {
			slog.Error("broadcast: a member arrived, and the channel wasn't told", "channel", channel, "member", m.ID, "err", err)
		}
	}
	context.AfterFunc(ctx, func() { h.leave(channel, key, m) })
	return events, nil
}

// Here is who's on the channel, each member once: for the page that shows
// them, which reloads as the hub says one came or went.
func (h *Hub) Here(ctx context.Context, channel string) ([]Member, error) {
	members, err := h.presence().Here(ctx, channel)
	if err != nil {
		return nil, fmt.Errorf("broadcast: who's on %s: %w", channel, err)
	}
	return members, nil
}

// leave takes a page's connection off its channel as the page goes, and
// tells the channel when it was the member's last. Its request's context
// is done, so the store gets one of its own.
func (h *Hub) leave(channel, key string, m Member) {
	ctx, stop := context.WithTimeout(context.Background(), leaving)
	defer stop()
	h.mu.Lock()
	delete(h.here, key)
	h.mu.Unlock()
	gone, err := h.presence().Leave(ctx, channel, key)
	if err != nil {
		slog.Error("broadcast: a member's page went, and it's still on the channel until its lease ends", "channel", channel, "member", m.ID, "err", err)
		return
	}
	if !gone {
		return
	}
	if err := h.Publish(ctx, channel, "presence.left", m); err != nil {
		slog.Error("broadcast: a member left, and the channel wasn't told", "channel", channel, "member", m.ID, "err", err)
	}
}

// keepHere says this instance's members are still on their channels every
// third of the lease, so a lease doesn't run out under a page that's open.
// Run runs it.
func (h *Hub) keepHere(ctx context.Context) {
	tick := time.NewTicker(h.lease() / 3)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		h.mu.Lock()
		keys := slices.Collect(maps.Keys(h.here))
		h.mu.Unlock()
		if len(keys) == 0 {
			continue
		}
		if err := h.presence().Keep(ctx, keys, time.Now().Add(h.lease())); err != nil && ctx.Err() == nil {
			slog.Error("broadcast: this instance's members couldn't say they're still here", "members", len(keys), "err", err)
		}
	}
}

// presence is the store who's where is kept in: the app's, or this
// instance's own, as a hub without a Store delivers in place.
func (h *Hub) presence() PresenceStore {
	if h.Presence != nil {
		return h.Presence
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.local == nil {
		h.local = &localPresence{}
	}
	return h.local
}

// lease is Hub.Lease, or a minute.
func (h *Hub) lease() time.Duration {
	if h.Lease > 0 {
		return h.Lease
	}
	return lease
}

// localPresence keeps who's on a channel in this instance alone, for a hub
// with no PresenceStore: an app of one instance, and tests.
type localPresence struct {
	mu   sync.Mutex
	here map[string]map[string]localMember // by channel, then key
}

type localMember struct {
	member Member
	until  time.Time
}

func (p *localPresence) Enter(_ context.Context, channel string, m Member, key string, until time.Time) (bool, error) {
	data, err := json.Marshal(m.Data)
	if err != nil {
		return false, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.here == nil {
		p.here = map[string]map[string]localMember{}
	}
	if p.here[channel] == nil {
		p.here[channel] = map[string]localMember{}
	}
	arrived := true
	for k, l := range p.here[channel] {
		if l.member.ID == m.ID && k != key && l.until.After(time.Now()) {
			arrived = false
		}
	}
	p.here[channel][key] = localMember{member: Member{ID: m.ID, Data: json.RawMessage(data)}, until: until}
	return arrived, nil
}

func (p *localPresence) Keep(_ context.Context, keys []string, until time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, members := range p.here {
		for _, key := range keys {
			if l, ok := members[key]; ok {
				l.until = until
				members[key] = l
			}
		}
	}
	return nil
}

func (p *localPresence) Leave(_ context.Context, channel, key string) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	members := p.here[channel]
	gone, ok := members[key]
	if !ok {
		return false, nil
	}
	delete(members, key)
	for _, l := range members {
		if l.member.ID == gone.member.ID && l.until.After(time.Now()) {
			return false, nil
		}
	}
	return true, nil
}

func (p *localPresence) Here(_ context.Context, channel string) ([]Member, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	seen := map[string]bool{}
	var members []Member
	for _, l := range p.here[channel] {
		if l.until.Before(now) || seen[l.member.ID] {
			continue
		}
		seen[l.member.ID] = true
		members = append(members, l.member)
	}
	slices.SortFunc(members, func(a, b Member) int { return cmp.Compare(a.ID, b.ID) })
	return members, nil
}
