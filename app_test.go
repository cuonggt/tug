package tug

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewWithoutAConfigReadsTheEnvironment(t *testing.T) {
	t.Setenv("ADDR", "")
	t.Setenv("PORT", "9999")
	t.Setenv("APP_DEBUG", "true")
	if app := New(); app.config.Addr != ":9999" || !app.config.Debug {
		t.Fatalf("read %+v from PORT=9999 APP_DEBUG=true", app.config)
	}

	t.Setenv("ADDR", "127.0.0.1:7000")
	if app := New(); app.config.Addr != "127.0.0.1:7000" {
		t.Errorf("Addr = %q, want ADDR over PORT", app.config.Addr)
	}
}

func TestAConfigPassedInIsUsedAsItIs(t *testing.T) {
	t.Setenv("PORT", "9999")
	if app := New(Config{}); app.config.Addr != ":8080" {
		t.Fatalf("Addr = %q, want the default rather than PORT", app.config.Addr)
	}
}

// start serves app on a free port until the test stops it, and returns the
// base URL, a stop function, and where Serve's result arrives.
func start(t *testing.T, app *App) (string, context.CancelFunc, <-chan error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	t.Cleanup(stop)
	served := make(chan error, 1)
	go func() { served <- app.Serve(ctx, ln) }()
	return "http://" + ln.Addr().String(), stop, served
}

func TestServeLetsRequestsInFlightFinishBeforeItReturns(t *testing.T) {
	captureLog(t)
	started, release := make(chan struct{}), make(chan struct{})
	app := New(Config{})
	app.Get("/slow", func(c *Ctx) error {
		close(started)
		<-release
		return c.String(200, "done")
	})
	base, stop, served := start(t, app)

	body := make(chan string, 1)
	go func() {
		resp, err := http.Get(base + "/slow")
		if err != nil {
			body <- err.Error()
			return
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		body <- string(b)
	}()

	<-started
	stop()
	// Once the listener has closed, shutdown is under way with the request
	// still in its handler.
	addr := strings.TrimPrefix(base, "http://")
	for deadline := time.Now().Add(5 * time.Second); ; {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			break
		}
		conn.Close()
		if time.Now().After(deadline) {
			t.Fatal("the listener is still open after the context was canceled")
		}
	}
	close(release)

	if got := <-body; got != "done" {
		t.Fatalf("the request in flight got %q, want it to finish", got)
	}
	if err := <-served; err != nil {
		t.Fatalf("Serve returned %v", err)
	}
}

func TestServeGivesUpOnRequestsThatOutlastTheShutdownTimeout(t *testing.T) {
	captureLog(t)
	started := make(chan struct{})
	app := New(Config{ShutdownTimeout: 50 * time.Millisecond})
	app.Get("/stuck", func(c *Ctx) error {
		close(started)
		<-c.Context().Done() // until the connection is closed under it
		return nil
	})
	base, stop, served := start(t, app)

	go http.Get(base + "/stuck")
	<-started
	stop()

	if err := <-served; err == nil || !strings.Contains(err.Error(), "still running") {
		t.Fatalf("Serve returned %v, want it to say requests were still running", err)
	}
}

func TestWorkBesideTheServerStopsAsItShutsDownAndServeWaitsForIt(t *testing.T) {
	captureLog(t)
	app := New(Config{})
	started := make(chan struct{})
	var finished atomic.Bool
	app.Go(func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		time.Sleep(50 * time.Millisecond) // finishing what it was doing
		finished.Store(true)
		return ctx.Err()
	})
	_, stop, served := start(t, app)
	<-started
	stop()
	if err := <-served; err != nil {
		t.Fatalf("Serve returned %v, want nil for work that stopped when told to", err)
	}
	if !finished.Load() {
		t.Error("Serve returned before the work beside the server had")
	}
}

func TestWorkBesideTheServerThatFailsShutsTheAppDown(t *testing.T) {
	captureLog(t)
	app := New(Config{})
	app.Go(func(ctx context.Context) error { return errors.New("the jobs table is gone") })
	_, _, served := start(t, app)
	select {
	case err := <-served:
		if err == nil || !strings.Contains(err.Error(), "the jobs table is gone") {
			t.Errorf("Serve returned %v, want the work's error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the app went on serving with the work beside it failed")
	}
}

func TestServeHTTPAndTugGenStartNothingGoAdded(t *testing.T) {
	app := New(Config{})
	app.Get("/", func(c *Ctx) error { return c.String(200, "home") })
	app.Go(func(context.Context) error {
		t.Error("the work beside the server started")
		return nil
	})
	app.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))

	path := filepath.Join(t.TempDir(), "gen.json")
	t.Setenv("TUG_GEN", path)
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("tug gen wrote nothing: %v", err)
	}
}

func TestGoOnceTheAppIsServingPanics(t *testing.T) {
	app := New(Config{})
	app.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	defer func() {
		if recover() == nil {
			t.Error("Go didn't panic")
		}
	}()
	app.Go(func(context.Context) error { return nil })
}
