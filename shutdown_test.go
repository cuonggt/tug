package tug

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// accepting is a listener that says each connection it accepts, for a
// test to know the server has one before it acts on it.
type accepting struct {
	net.Listener
	accepted chan net.Conn
}

func (l accepting) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err == nil {
		l.accepted <- c
	}
	return c, err
}

// serveAccepting serves app on a free port, through accepting, until the
// test stops it.
func serveAccepting(t *testing.T, app *App) (addr string, accepted <-chan net.Conn, stop context.CancelFunc, served <-chan error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	l := accepting{Listener: ln, accepted: make(chan net.Conn, 100)}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { done <- app.Serve(ctx, l) }()
	return ln.Addr().String(), l.accepted, cancel, done
}

// closed waits until nothing listens at addr: the app has stopped taking
// connections, and is shutting down.
func closed(t *testing.T, addr string) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		c, err := net.Dial("tcp", addr)
		if err != nil {
			return
		}
		c.Close()
		if time.Now().After(deadline) {
			t.Fatal("the app still takes connections after it was told to stop")
		}
	}
}

func TestARequestAConnectionSendsAsTheAppShutsDownIsAnswered(t *testing.T) {
	captureLog(t)
	app := New(Config{})
	app.Get("/", text("home"))
	addr, accepted, stop, served := serveAccepting(t, app)

	// A connection the app has taken, which says nothing yet, as a
	// browser's sends its request a moment after it connects, and as one
	// the kernel held for a restart is taken by the new process.
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	<-accepted

	stop()
	closed(t, addr)
	// Its request, sent once the app is shutting down: Go's Shutdown reads
	// one and hangs up, as the client gets nothing it could try again on.
	fmt.Fprint(conn, "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	res, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("the request sent as the app shut down got %v, want it answered", err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || string(body) != "home" || !res.Close {
		t.Errorf("the request sent as the app shut down: %d %q, close %v, want the page, and the connection closed after it", res.StatusCode, body, res.Close)
	}
	if err := <-served; err != nil {
		t.Errorf("Serve returned %v", err)
	}
}

func TestAConnectionThatSaysNothingKeepsTheAppFromStoppingAWhileAlone(t *testing.T) {
	captureLog(t)
	was := newConnGrace
	newConnGrace = 200 * time.Millisecond
	t.Cleanup(func() { newConnGrace = was })
	app := New(Config{})
	app.Get("/", text("home"))
	addr, accepted, stop, served := serveAccepting(t, app)

	// A browser's spare connection, opened for a request it may never make.
	dialed := time.Now()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	<-accepted
	stop()
	select {
	case err := <-served:
		if err != nil {
			t.Errorf("Serve returned %v", err)
		}
	case <-time.After(3 * time.Second): // Go's Shutdown alone waits 5
		t.Fatal("a connection that says nothing held the app from stopping")
	}
	if took := time.Since(dialed); took < newConnGrace {
		t.Errorf("the app stopped %v after the connection came, before its grace of %v was up", took, newConnGrace)
	}
	// The app closed it, as it's idle.
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := conn.Read(make([]byte, 1)); err != io.EOF {
		t.Errorf("reading the connection the app left: %v, want it closed", err)
	}
}

func TestAnHTTP2RequestInFlightIsAnsweredAsTheAppShutsDown(t *testing.T) {
	captureLog(t)
	certFile, keyFile, cert := writeCert(t, t.TempDir(), "example.com")
	started, release := make(chan struct{}), make(chan struct{})
	app := New(Config{TLS: TLS{CertFile: certFile, KeyFile: keyFile}})
	app.Get("/slow", func(c *Ctx) error {
		close(started)
		<-release
		return c.String(http.StatusOK, "done")
	})
	s := serveTLS(t, app, nil)
	client := trusting(cert, "example.com")
	got := make(chan string, 1)
	go func() {
		res, err := client.Get("https://" + s.https + "/slow")
		if err != nil {
			got <- err.Error()
			return
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		got <- res.Proto + " " + string(body)
	}()
	<-started
	s.stop()
	closed(t, s.https)
	close(release)
	if answer := <-got; answer != "HTTP/2.0 done" {
		t.Errorf("the request in flight over HTTP/2 got %q, want it answered", answer)
	}
}
