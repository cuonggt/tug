package devtools

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
	"runtime"
	"strings"
)

// bodyLimit is the most of a body an entry keeps: one longer is left out,
// as too large, as Laravel's are.
const bodyLimit = 256_000

// sent is a response as it's sent: its status and headers, as they went
// out, and its body, kept while it's text and under bodyLimit, and whether
// it was flushed as it went, as a stream is.
type sent struct {
	http.ResponseWriter
	status   int
	header   http.Header // as it went out
	text     bool        // its Content-Type says it's text
	body     bytes.Buffer
	size     int64
	tooLarge bool
	streamed bool
	hijacked bool // the connection taken, whose request's body can't be read

	// by is the function of the app's that answered, as its status was
	// written: the handler, or a wrapper of it that answered for it.
	by *runtime.Frame
}

func (s *sent) WriteHeader(code int) {
	if s.status == 0 && code >= 200 {
		s.status = code
		s.header = s.ResponseWriter.Header().Clone()
		s.text = textual(s.header.Get("Content-Type"))
		if f, ok := Caller(); ok {
			s.by = &f
		}
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *sent) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.WriteHeader(http.StatusOK)
	}
	n, err := s.ResponseWriter.Write(b)
	s.size += int64(n)
	if s.text && !s.tooLarge && s.body.Len()+n > bodyLimit {
		s.tooLarge = true
		s.body.Reset()
	}
	if s.text && !s.tooLarge {
		s.body.Write(b[:n])
	}
	return n, err
}

// Flush flushes, as a stream or events do, which the entry says.
func (s *sent) Flush() {
	s.streamed = true
	if s.status == 0 {
		s.WriteHeader(http.StatusOK)
	}
	http.NewResponseController(s.ResponseWriter).Flush()
}

func (s *sent) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	s.streamed, s.hijacked = true, true
	return http.NewResponseController(s.ResponseWriter).Hijack()
}

func (s *sent) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// sentStatus is the response's status: 200 for one that wrote nothing, as
// net/http sends.
func (s *sent) sentStatus() int {
	if s.status == 0 {
		return http.StatusOK
	}
	return s.status
}

// sentHeader is the response's headers as they went out, or as they are,
// for one that wrote nothing.
func (s *sent) sentHeader() http.Header {
	if s.header == nil {
		return s.ResponseWriter.Header()
	}
	return s.header
}

// textual reports whether a body of contentType is text, which an entry
// keeps.
func textual(contentType string) bool {
	contentType = strings.ToLower(contentType)
	for _, t := range []string{"json", "text/", "xml", "javascript"} {
		if strings.Contains(contentType, t) {
			return true
		}
	}
	return false
}

// read is a request's body as the handler reads it, kept up to bodyLimit:
// what an entry says it was.
type read struct {
	io.ReadCloser
	got      bytes.Buffer
	tooLarge bool
}

func (r *read) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if !r.tooLarge && r.got.Len()+n > bodyLimit {
		r.tooLarge = true
		r.got.Reset()
	}
	if !r.tooLarge {
		r.got.Write(p[:n])
	}
	return n, err
}

// rest reads what the handler left of the body, as net/http does before
// the connection's next request, so the entry of a handler that didn't
// read it, as one that only redirects, has it too.
func (r *read) rest() {
	if !r.tooLarge {
		io.Copy(io.Discard, io.LimitReader(r, int64(bodyLimit-r.got.Len()+1)))
	}
}
