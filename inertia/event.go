package inertia

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
)

// LiveEvent is an event's data for the pages with live props that follow
// it, as hub.Publish in package broadcast sends it: the app's own data,
// with the protocol's __inertia beside it, which the client reads. Make
// one with Event.
type LiveEvent struct {
	data     any
	props    Props
	socketID string
}

// An EventOption says what a LiveEvent carries for the client: Carry and
// ToOthers.
type EventOption func(*LiveEvent)

// Event is data, an object's fields or nil for none, as an event's data
// for the pages with live props that hear it, with what opts add:
//
//	a.hub.Publish(ctx, "posts", "updated", inertia.Event(
//		map[string]int64{"id": post.ID},
//		inertia.Carry(inertia.Props{"stats": stats}),
//		inertia.ToOthers(c.Request()),
//	))
//
// An event goes to every page that follows its channel, so what it carries
// is the same for each of them: what's private to one user goes on a
// channel of their own.
func Event(data any, opts ...EventOption) LiveEvent {
	e := LiveEvent{data: data}
	for _, opt := range opts {
		opt(&e)
	}
	return e
}

// Carry has the event carry props' values, which the client writes into
// the live props of the same paths that listen for the event, with no
// request, where it would otherwise reload them: values the same for
// everyone on the channel, within the hub's MaxEvent with the rest. They're
// worked out as a page's are, whatever their type, a deferred prop's too,
// and a path is the prop's whole, as "stats" or "account.balance". A prop
// whose value isn't carried, or holds a BigInt, which the client wouldn't
// make one, is reloaded.
func Carry(props Props) EventOption {
	return func(e *LiveEvent) {
		if e.props == nil {
			e.props = Props{}
		}
		for k, v := range props {
			e.props[k] = v
		}
	}
}

// ToOthers leaves out the page that sent r, by the socket ID its client
// sends with each request, X-Socket-Id, which the client drops an event of
// its own on: the tab that made a change shows it already, as its visit's
// response has it. Without the header, as from a script, every page hears
// it.
func ToOthers(r *http.Request) EventOption {
	id := r.Header.Get("X-Socket-Id")
	return func(e *LiveEvent) {
		if plainSocketID(id) {
			e.socketID = id
		}
	}
}

// plainSocketID reports whether id is a socket ID the client could have
// made, rather than whatever a request says, which goes out to every page
// on the channel.
func plainSocketID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, c := range []byte(id) {
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		case c == '-', c == '_', c == '.':
		default:
			return false
		}
	}
	return true
}

// MarshalJSON writes the event's data, with __inertia's props and socketId
// beside its fields, when it has either.
func (e LiveEvent) MarshalJSON() ([]byte, error) {
	data, err := json.Marshal(e.data)
	if err != nil {
		return nil, err
	}
	meta := map[string]any{}
	if len(e.props) > 0 {
		values, err := carried(e.props)
		if err != nil {
			return nil, err
		}
		if len(values) > 0 {
			meta["props"] = values
		}
	}
	if e.socketID != "" {
		meta["socketId"] = e.socketID
	}
	if len(meta) == 0 {
		return data, nil
	}
	inner, err := json.Marshal(map[string]any{"__inertia": meta})
	if err != nil {
		return nil, err
	}
	fields := bytes.TrimSpace(data)
	switch {
	case bytes.Equal(fields, []byte("null")):
		return inner, nil
	case len(fields) < 2 || fields[0] != '{':
		return nil, errors.New("inertia: an event's data is an object's fields, or nil, for what it carries for the client to go beside them")
	case bytes.Equal(bytes.TrimSpace(fields[1:len(fields)-1]), nil):
		return inner, nil
	}
	out := make([]byte, 0, len(fields)+len(inner))
	out = append(out, fields[:len(fields)-1]...)
	out = append(out, ',')
	return append(out, inner[1:]...), nil
}

// carried works out props as a page's are, each whole, whatever its type,
// with no partial reload's or first load's rules, and leaves out a value
// with a BigInt in it, which the client wouldn't make one: its prop is
// reloaded instead, with the page's own preserveBigIntegers.
func carried(props Props) (map[string]any, error) {
	rs := &resolver{partial: true}
	values, err := rs.level(sortedEntries(props), "", true)
	if err != nil {
		return nil, err
	}
	for k, v := range values {
		data, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		if bytes.Contains(data, bigIntMarker) {
			delete(values, k)
		}
	}
	return values, nil
}
