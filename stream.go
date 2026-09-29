package tug

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Stream sends a body as write makes it, of the type contentType, such as
// "text/csv", for a body too long to hold, made as it's read: every post
// as CSV, a row at a time, from rows as a query returns them. Each write
// goes to the client as it's made, flushed, so a client that reads as it
// comes, as a fetch of lines does, gets each one; a writer of many small
// writes wraps w in a bufio.Writer, as csv.Writer does itself.
//
// write's error before its first write is Stream's, for the ErrorHandler,
// as any handler's is. Once the body has started, there's no error page to
// show: the error goes to the log, and the connection is cut, with
// http.ErrAbortHandler, so the client sees the body end before its time,
// where a browser would save half a file as the whole.
func (c *Ctx) Stream(contentType string, write func(w io.Writer) error) error {
	h := c.rw.Header()
	h.Set("Content-Type", contentType)
	h.Set("X-Content-Type-Options", "nosniff")
	err := write(streamWriter{c})
	switch {
	case err == nil:
		if !c.Written() {
			c.rw.WriteHeader(http.StatusOK) // a body of nothing
		}
		return nil
	case !c.Written():
		// The error page isn't a file to save.
		h.Del("Content-Disposition")
		return err
	case c.Context().Err() != nil:
		return nil // the client went away, and there's no one left to tell
	default:
		slog.ErrorContext(c.Context(), "a stream failed after it started, so its connection was cut",
			"method", c.r.Method, "path", c.r.URL.Path, "err", err)
		panic(http.ErrAbortHandler)
	}
}

// StreamDownload sends a body as write makes it, as Stream does, as a
// file to save, as name, which Download writes as it writes its own:
//
//	return c.StreamDownload("posts.csv", "text/csv; charset=utf-8", func(w io.Writer) error {
//		out := csv.NewWriter(w)
//		...
//		out.Flush()
//		return out.Error()
//	})
func (c *Ctx) StreamDownload(name, contentType string, write func(w io.Writer) error) error {
	c.rw.Header().Set("Content-Disposition", attachment(baseName(name)))
	return c.Stream(contentType, write)
}

// streamWriter is the body of a Stream, which flushes each write.
type streamWriter struct{ c *Ctx }

func (w streamWriter) Write(b []byte) (int, error) {
	n, err := w.c.rw.Write(b)
	if err == nil {
		w.c.rw.Flush()
	}
	return n, err
}

// Event is one of the server-sent events Ctx.Events sends.
type Event struct {
	// Name is the event's type, which a page listens for with
	// addEventListener, as "progress". Without one, it's a message, which
	// onmessage gets.
	Name string

	// ID is the event's, which the browser sends back when it connects
	// again, in Last-Event-ID, for the app to go on from after it. Optional.
	ID string

	// Data is what the event carries, as JSON, which the page reads with
	// JSON.parse: a value encoding/json can encode, or nil for none.
	Data any
}

// keepAlive is how often a quiet event stream sends a comment, which the
// browser passes over: a proxy in front of the app closes a connection
// that says nothing for a minute, as nginx's and AWS's load balancers do
// unless told otherwise.
var keepAlive = 20 * time.Second

// Events sends a stream of server-sent events, which the browser's
// EventSource reads, as fn sends them: an import's progress, say, or a
// change a page should show, which it reloads its props for with
// Inertia's router.reload. The stream lasts as long as fn does: fn's
// context is done when the client goes away, and when the app shuts
// down, as Serve's Shutdown waits for the requests in flight, which a
// stream would hold on to. fn returns then; send fails once it's done.
//
//	app.Get("/imports/{id}/progress", func(c *tug.Ctx) error {
//		return c.Events(func(ctx context.Context, send func(tug.Event) error) error {
//			for p := range imports.progress(ctx, c.Param("id")) {
//				if err := send(tug.Event{Name: "progress", Data: p}); err != nil {
//					return err
//				}
//			}
//			return nil
//		})
//	})
//
// An EventSource connects again when a stream ends, the app's shutdown's
// among them, after a few seconds, with the last event's ID in
// Last-Event-ID, for the app to go on from: a page that has what it
// waited for closes it. A quiet stream sends a comment every 20 seconds,
// which keeps the proxies in front of the app from closing it.
//
// fn's error is Events', for the ErrorHandler, which logs it, as the
// response has started, unless the client went away or the app is
// shutting down, which is no failure. An event whose Name or ID has a line
// break, or whose Data JSON can't hold, is send's error, and isn't sent.
func (c *Ctx) Events(fn func(ctx context.Context, send func(Event) error) error) error {
	h := c.rw.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	// nginx holds a response back until its buffer fills, which a stream
	// of events never does.
	h.Set("X-Accel-Buffering", "no")
	c.rw.WriteHeader(http.StatusOK)
	c.rw.Flush()

	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()
	defer context.AfterFunc(c.app.stopping, cancel)()

	// What fn sends and the comments go in turn.
	var mu sync.Mutex
	write := func(b []byte) error {
		mu.Lock()
		defer mu.Unlock()
		if _, err := c.rw.Write(b); err != nil {
			return err
		}
		c.rw.Flush()
		return nil
	}
	var quiet sync.WaitGroup
	quiet.Go(func() {
		tick := time.NewTicker(keepAlive)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				if write([]byte(":\n\n")) != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	})
	err := fn(ctx, func(e Event) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		b, err := e.encode()
		if err != nil {
			return err
		}
		return write(b)
	})
	// A client that went away, or the app's shutdown, is how the stream
	// ends, and fn's error, if any, is only that.
	ended := ctx.Err() != nil
	cancel()
	quiet.Wait()
	if ended {
		return nil
	}
	return err
}

// encode writes e as an event stream has it: its name and ID on lines of
// their own, and its data as JSON, which encoding/json writes on one line.
func (e Event) encode() ([]byte, error) {
	// The browser drops an ID with a NUL in it.
	if strings.ContainsAny(e.Name, "\r\n") || strings.ContainsAny(e.ID, "\r\n\x00") {
		return nil, fmt.Errorf("tug: an event's name and ID are text on one line, not %q and %q", e.Name, e.ID)
	}
	data, err := json.Marshal(e.Data)
	if err != nil {
		return nil, fmt.Errorf("tug: an event's data can't be sent as JSON: %w", err)
	}
	var b bytes.Buffer
	if e.Name != "" {
		b.WriteString("event: " + e.Name + "\n")
	}
	if e.ID != "" {
		b.WriteString("id: " + e.ID + "\n")
	}
	// Always a data line, null for none, as an event with no data is one
	// the browser drops.
	b.WriteString("data: ")
	b.Write(data)
	b.WriteString("\n\n")
	return b.Bytes(), nil
}
