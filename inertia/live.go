package inertia

import (
	"slices"
	"time"
)

// A live prop is one the client refreshes as an event comes, on a channel
// the page follows: the page object's liveProps names the events, by their
// channels, and the client reloads the props an event touches, in one
// partial reload, or writes in the values the event carries (see Event).
// Inertia's client hears them through a transport, which tug gen writes
// for tug's own channels, resources/js/tug/live.ts.
//
// A channel is a route of the app's that sends events, with c.Events in
// package tug, by its path, as "/broadcasts": the route says who may
// follow it, as any page's says who may see it, and there's no protocol
// to authorize a channel by its name.

// A LiveOption says what refreshes a live prop: On, and Throttle.
type LiveOption func(*behavior)

// listener is a route the page follows, and the events on it that refresh
// a prop.
type listener struct {
	url    string
	events []string
}

// On has the prop refreshed as one of events comes on the route at url, a
// route of the app's that sends events, as "/broadcasts", or the path a
// route's name makes: the route of a team's channel, "/teams/42/events",
// is the team's alone. On panics without a url or an event, as a prop
// that nothing refreshes is a mistake.
func On(url string, events ...string) LiveOption {
	if url == "" {
		panic("inertia: On needs the path of the route that sends the events, as \"/broadcasts\"")
	}
	if len(events) == 0 {
		panic("inertia: On needs the names of the events that refresh the prop")
	}
	events = slices.Clone(events)
	return func(b *behavior) { b.live = append(b.live, listener{url: url, events: events}) }
}

// Throttle refreshes the prop once every d at most, however often its
// events come: the client's own default is a second, and 0 refreshes it
// as each comes. Throttle panics on a d under 0.
func Throttle(d time.Duration) LiveOption {
	if d < 0 {
		panic("inertia: Throttle takes a duration of 0 or more")
	}
	ms := d.Milliseconds()
	return func(b *behavior) { b.throttle = &ms }
}

// withLive is b with what opts say refreshes it.
func withLive(b behavior, opts []LiveOption) behavior {
	if b.merge {
		panic(mergedLive)
	}
	b.live = slices.Clone(b.live)
	for _, opt := range opts {
		opt(&b)
	}
	if len(b.live) == 0 {
		panic("inertia: Live needs On, the route of the events that refresh the prop")
	}
	return b
}

// mergedLive is the panic of a prop that's live and merged: the client's
// refresh of it would add all of it again to what it has.
const mergedLive = "inertia: a merged prop can't be live: each refresh would add the whole of it again to what the client has"

// LiveMeta is a live prop's entry in the page object's liveProps: the
// events that refresh it, by the channel each comes on, and how often it's
// refreshed at most, in milliseconds, when its throttle isn't the
// client's.
type LiveMeta struct {
	Listeners []LiveListener `json:"listeners"`
	Throttle  *int64         `json:"throttle,omitempty"`
}

// LiveListener is a channel a live prop is refreshed from, and the events
// on it that refresh it.
type LiveListener struct {
	Channel LiveChannel `json:"channel"`
	Events  []string    `json:"events"`
}

// LiveChannel is a channel as the protocol names it: tug's are the paths of
// the app's routes of events, of the type "public", as the route itself
// says who may follow it.
type LiveChannel struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// liveMeta is b's entry in liveProps: a listener for each route, in the
// order On named them, with each of its events once.
func liveMeta(b behavior) LiveMeta {
	var meta LiveMeta
	for _, l := range b.live {
		at := slices.IndexFunc(meta.Listeners, func(m LiveListener) bool { return m.Channel.Name == l.url })
		if at < 0 {
			meta.Listeners = append(meta.Listeners, LiveListener{Channel: LiveChannel{Name: l.url, Type: "public"}})
			at = len(meta.Listeners) - 1
		}
		for _, e := range l.events {
			if !slices.Contains(meta.Listeners[at].Events, e) {
				meta.Listeners[at].Events = append(meta.Listeners[at].Events, e)
			}
		}
	}
	meta.Throttle = b.throttle
	return meta
}

// collectLive notes a live prop at path, for the client to refresh it. A
// response that isn't a partial reload names every live prop of the
// page's, those it left out too, as a deferred one the client fetches
// after, and a partial reload those it resolved, which the client adds
// to what it has: always props, those inside a prop resolved whole, and
// those it asked for.
func (rs *resolver) collectLive(b behavior, path string, parentResolved bool) {
	if len(b.live) == 0 {
		return
	}
	if rs.partial && !b.always && !parentResolved && !rs.labeled(path) {
		return
	}
	if rs.live == nil {
		rs.live = map[string]LiveMeta{}
	}
	rs.live[path] = liveMeta(b)
}
