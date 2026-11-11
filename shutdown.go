package tug

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"
)

// shutDown stops s taking requests without dropping one. Go's Shutdown
// reads the request a connection sends once it has begun, and hangs up on
// it unanswered, which a client can't tell from a request that failed and
// doesn't send again, as it does one on a connection closed while it was
// idle: a connection the app took as it was told to stop would lose its
// request, as one systemd held across a restart for this process could.
//
// So s's listener is closed first, which refuses the connections that come
// after it, or leaves them in the kernel's queue for the next process when
// systemd holds the socket; keep-alives are turned off, which closes the
// idle connections, and has each response close its own, and each HTTP/2
// connection go away once its streams are done; and once no connection is
// in the middle of a request, or new and still in time to send one, those
// that have said nothing for newConnGrace are closed, and Shutdown has only
// idle ones left to close, and ctx's time to wait for any it hasn't.
func (s listening) shutDown(ctx context.Context) error {
	s.ln.Close()
	s.server.SetKeepAlivesEnabled(false)
	for wait := time.Millisecond; ctx.Err() == nil && s.conns.busy(time.Now()); wait = min(2*wait, 100*time.Millisecond) {
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
		case <-t.C:
		}
		t.Stop()
	}
	s.conns.closeSilent(time.Now())
	return s.server.Shutdown(ctx)
}

// newConnGrace is how long a connection that has said nothing is waited
// for, as the app shuts down, to send its request: as long as Go's own
// Shutdown waits before it takes one for idle, as a browser's spare
// connection is. shutDown closes one itself then, as Shutdown counts the
// time in whole seconds of the clock, and would wait up to one more.
var newConnGrace = 5 * time.Second

// conns is a server's connections, as its ConnState says them, for
// shutDown to know when none is in the middle of a request. An HTTP/2
// connection says its own, Active as a stream starts and Idle as the last
// one ends.
type conns struct {
	mu   sync.Mutex
	open map[net.Conn]connAt
}

// connAt is a connection's state, and when it came to it.
type connAt struct {
	state http.ConnState
	since time.Time
}

func (cs *conns) track(c net.Conn, state http.ConnState) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	switch state {
	case http.StateClosed, http.StateHijacked:
		delete(cs.open, c)
	default:
		if cs.open == nil {
			cs.open = map[net.Conn]connAt{}
		}
		cs.open[c] = connAt{state: state, since: time.Now()}
	}
}

// busy reports whether a connection is in the middle of a request at now,
// or new, and still in time to send one.
func (cs *conns) busy(now time.Time) bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for _, c := range cs.open {
		if c.state == http.StateActive || c.state == http.StateNew && now.Sub(c.since) < newConnGrace {
			return true
		}
	}
	return false
}

// closeSilent closes the connections that are new at now, and have said
// nothing for newConnGrace. They're closed once the lock is let go, as a
// TLS connection's close writes to its peer, which could take a while.
func (cs *conns) closeSilent(now time.Time) {
	cs.mu.Lock()
	var silent []net.Conn
	for c, at := range cs.open {
		if at.state == http.StateNew && now.Sub(at.since) >= newConnGrace {
			silent = append(silent, c)
		}
	}
	cs.mu.Unlock()
	for _, c := range silent {
		c.Close()
	}
}

// closeOnce is a listener closed once, however often it's closed:
// shutDown closes it before Shutdown, whose own close would otherwise be
// Shutdown's error.
type closeOnce struct {
	net.Listener
	once sync.Once
	err  error
}

func (l *closeOnce) Close() error {
	l.once.Do(func() { l.err = l.Listener.Close() })
	return l.err
}
