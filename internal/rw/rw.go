// Package rw wraps an http.ResponseWriter to remember what went through it:
// the status, and so whether the response has started, which an error
// handler has to know before it writes an error page, and the size, for
// the log.
package rw

import (
	"bufio"
	"io"
	"net"
	"net/http"
)

// Writer is an http.ResponseWriter that records its status and size. It
// keeps what the writer it wraps can do: Flush, Hijack and ReadFrom, and
// Unwrap for http.ResponseController.
type Writer struct {
	http.ResponseWriter
	status int
	size   int64
}

// New wraps w.
func New(w http.ResponseWriter) *Writer { return &Writer{ResponseWriter: w} }

func (w *Writer) WriteHeader(code int) {
	if w.status == 0 {
		// A 1xx such as 103 Early Hints goes before the response, which
		// hasn't started.
		if code >= 100 && code < 200 && code != http.StatusSwitchingProtocols {
			w.ResponseWriter.WriteHeader(code)
			return
		}
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *Writer) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.size += int64(n)
	return n, err
}

func (w *Writer) WriteString(s string) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := io.WriteString(w.ResponseWriter, s)
	w.size += int64(n)
	return n, err
}

// ReadFrom lets io.Copy hand a file to the connection whole, with sendfile,
// when the writer underneath can.
func (w *Writer) ReadFrom(src io.Reader) (int64, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	var n int64
	var err error
	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		n, err = rf.ReadFrom(src)
	} else {
		n, err = io.Copy(w.ResponseWriter, src)
	}
	w.size += n
	return n, err
}

func (w *Writer) Flush() {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *Writer) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, buf, err := http.NewResponseController(w.ResponseWriter).Hijack()
	if err == nil && w.status == 0 {
		w.status = http.StatusSwitchingProtocols // the connection is someone else's now
	}
	return conn, buf, err
}

// Unwrap returns the writer underneath, for http.ResponseController.
func (w *Writer) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Status is the status written, or 0 while nothing has been.
func (w *Writer) Status() int { return w.status }

// Written reports whether the response has started.
func (w *Writer) Written() bool { return w.status != 0 }

// Size is how many bytes of body have been written.
func (w *Writer) Size() int64 { return w.size }
