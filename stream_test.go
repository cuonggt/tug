package tug

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestAStreamGoesAsItsWritten(t *testing.T) {
	rest := make(chan struct{})
	app := New(Config{})
	app.Get("/export", func(c *Ctx) error {
		return c.StreamDownload("posts.csv", "text/csv; charset=utf-8", func(w io.Writer) error {
			io.WriteString(w, "id,title\n")
			<-rest // until the client has the first line
			io.WriteString(w, "1,Hello\n")
			return nil
		})
	})
	srv := httptest.NewServer(app)
	defer srv.Close()
	// Unflushed, the response wouldn't start, its headers and all, until the
	// handler went on, which it waits to: the client gives up first.
	client := &http.Client{Timeout: 5 * time.Second}
	res, err := client.Get(srv.URL + "/export")
	if err != nil {
		close(rest)
		t.Fatalf("the response didn't start until the rest was written: %v", err)
	}
	defer res.Body.Close()
	// Before the body's closed, and the server, which wait for the handler,
	// which waits for it, however the test ends.
	defer close(rest)
	if kind, name := disposition(t, res.Header); kind != "attachment" || name != "posts.csv" || res.Header.Get("Content-Type") != "text/csv; charset=utf-8" {
		t.Errorf("headers %v", res.Header)
	}
	body := bufio.NewReader(res.Body)
	first := make(chan string, 1)
	go func() {
		line, _ := body.ReadString('\n')
		first <- line
	}()
	select {
	case line := <-first:
		if line != "id,title\n" {
			t.Errorf("the first line: %q", line)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the first line didn't come until the rest was written")
	}
}

func TestAStreamThatFailsOnceStartedIsCutShort(t *testing.T) {
	logs := captureLog(t)
	app := New(Config{})
	app.Get("/export", func(c *Ctx) error {
		return c.Stream("text/csv", func(w io.Writer) error {
			io.WriteString(w, "id,title\n")
			return errors.New("the database went away")
		})
	})
	srv := httptest.NewServer(app)
	defer srv.Close()
	res, err := http.Get(srv.URL + "/export")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if body, err := io.ReadAll(res.Body); err == nil {
		t.Errorf("the stream ended as though it were whole: %q", body)
	}
	srv.Close() // for the handler to have returned
	if !strings.Contains(logs.String(), "a stream failed after it started") || !strings.Contains(logs.String(), "the database went away") {
		t.Errorf("the log:\n%s", logs)
	}
}

// event is one event of a stream, as EventSource reads it.
type event struct{ name, id, data string }

// events reads a stream as EventSource does, and the comments apart.
func events(stream string) (read []event, comments int) {
	for block := range strings.SplitSeq(strings.TrimSuffix(stream, "\n\n"), "\n\n") {
		var e event
		for line := range strings.SplitSeq(block, "\n") {
			field, value, _ := strings.Cut(line, ": ")
			switch field {
			case "event":
				e.name = value
			case "id":
				e.id = value
			case "data":
				e.data = value
			case "", ":":
				comments++
			}
		}
		if e != (event{}) {
			read = append(read, e)
		}
	}
	return read, comments
}

func TestEventsAreSentAsEventSourceReadsThem(t *testing.T) {
	app := New(Config{})
	app.Get("/progress", func(c *Ctx) error {
		return c.Events(func(ctx context.Context, send func(Event) error) error {
			for _, e := range []Event{
				{Name: "progress", ID: "1", Data: map[string]int{"done": 1, "of": 2}},
				{Data: "a message, with\nlines"},
				{Name: "done", ID: "2"},
			} {
				if err := send(e); err != nil {
					return err
				}
			}
			return nil
		})
	})
	w := serve(app, "GET", "/progress", "")
	h := w.Header()
	if h.Get("Content-Type") != "text/event-stream" || h.Get("Cache-Control") != "no-cache" || h.Get("X-Accel-Buffering") != "no" {
		t.Errorf("headers %v", h)
	}
	got, _ := events(w.Body.String())
	want := []event{
		{"progress", "1", `{"done":1,"of":2}`},
		{"", "", `"a message, with\nlines"`},
		{"done", "2", "null"},
	}
	if len(got) != len(want) {
		t.Fatalf("events %+v, from %q", got, w.Body)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event %d: %+v, want %+v", i+1, got[i], want[i])
		}
	}
}

func TestAnEventThatCantBeSentIsntSent(t *testing.T) {
	app := New(Config{})
	app.Get("/progress", func(c *Ctx) error {
		return c.Events(func(ctx context.Context, send func(Event) error) error {
			for _, e := range []Event{
				{Name: "progress\ndata: forged"},
				{ID: "1\r2"},
				{ID: "1\x00"},
				{Data: make(chan int)},
			} {
				if err := send(e); err == nil {
					t.Errorf("%+v was sent", e)
				}
			}
			return nil
		})
	})
	if w := serve(app, "GET", "/progress", ""); w.Body.Len() != 0 {
		t.Errorf("sent %q", w.Body)
	}
}

func TestAQuietStreamSaysSomethingEvery20Seconds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		logs := captureLog(t)
		app := New(Config{})
		app.Get("/progress", func(c *Ctx) error {
			return c.Events(func(ctx context.Context, send func(Event) error) error {
				<-ctx.Done()
				return ctx.Err()
			})
		})
		ctx, leave := context.WithCancel(context.Background())
		w := httptest.NewRecorder()
		done := make(chan struct{})
		go func() {
			app.ServeHTTP(w, httptest.NewRequest("GET", "/progress", nil).WithContext(ctx))
			close(done)
		}()
		time.Sleep(45 * time.Second)
		leave()
		<-done
		if got, comments := events(w.Body.String()); len(got) != 0 || comments != 2 {
			t.Errorf("in 45 seconds, %d events and %d comments: %q", len(got), comments, w.Body)
		}
		if logs.Len() > 0 {
			t.Errorf("a client that went away was logged:\n%s", logs)
		}
	})
}

func TestAClientThatGoesEndsItsStream(t *testing.T) {
	logs := captureLog(t)
	ended := make(chan error, 1)
	app := New(Config{})
	app.Get("/progress", func(c *Ctx) error {
		return c.Events(func(ctx context.Context, send func(Event) error) error {
			send(Event{Name: "started"})
			<-ctx.Done()
			ended <- send(Event{Name: "too late"})
			return ctx.Err()
		})
	})
	srv := httptest.NewServer(app)
	defer srv.Close()
	res, err := http.Get(srv.URL + "/progress")
	if err != nil {
		t.Fatal(err)
	}
	if line, _ := bufio.NewReader(res.Body).ReadString('\n'); line != "event: started\n" {
		t.Errorf("the first line: %q", line)
	}
	res.Body.Close()
	select {
	case err := <-ended:
		if err == nil {
			t.Error("an event was sent once the stream had ended")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the stream went on after its client went away")
	}
	srv.Close() // for the handler to have returned
	if strings.Contains(logs.String(), "request failed") {
		t.Errorf("a client that went away was logged:\n%s", logs)
	}
}

func TestTheAppsShutdownEndsItsStreams(t *testing.T) {
	captureLog(t)
	app := New(Config{ShutdownTimeout: 10 * time.Second})
	app.Get("/progress", func(c *Ctx) error {
		return c.Events(func(ctx context.Context, send func(Event) error) error {
			send(Event{Name: "started"})
			<-ctx.Done()
			return ctx.Err()
		})
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, shutDown := context.WithCancel(context.Background())
	defer shutDown()
	served := make(chan error, 1)
	go func() { served <- app.Serve(ctx, ln) }()
	res, err := http.Get("http://" + ln.Addr().String() + "/progress")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body := bufio.NewReader(res.Body)
	if line, _ := body.ReadString('\n'); line != "event: started\n" {
		t.Fatalf("the first line: %q", line)
	}
	shutDown()
	select {
	case err := <-served:
		if err != nil {
			t.Errorf("Serve: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the stream held the shutdown up")
	}
	// It ended, as a stream that's done does, for the browser to connect
	// again, to an instance that's up.
	if _, err := io.ReadAll(body); err != nil {
		t.Errorf("the stream was cut: %v", err)
	}
}

func TestAnEventStreamsFailureIsLogged(t *testing.T) {
	logs := captureLog(t)
	app := New(Config{})
	app.Get("/progress", func(c *Ctx) error {
		return c.Events(func(ctx context.Context, send func(Event) error) error {
			send(Event{Name: "started"})
			return errors.New("the import's table is gone")
		})
	})
	w := serve(app, "GET", "/progress", "")
	if got, _ := events(w.Body.String()); len(got) != 1 || w.Code != http.StatusOK {
		t.Errorf("%d %q", w.Code, w.Body)
	}
	if !strings.Contains(logs.String(), "the import's table is gone") {
		t.Errorf("the log:\n%s", logs)
	}
}
