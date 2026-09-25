package rw

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// server stands in for net/http's own writer, which sends a 1xx and then
// the response. httptest's recorder takes the first status as final, 1xx
// or not, so it can't show the difference.
type server struct {
	h     http.Header
	codes []int
}

func (s *server) Header() http.Header         { return s.h }
func (s *server) Write(b []byte) (int, error) { return len(b), nil }
func (s *server) WriteHeader(code int)        { s.codes = append(s.codes, code) }

func TestTheFirstFinalStatusIsTheOneRecorded(t *testing.T) {
	s := &server{h: http.Header{}}
	w := New(s)
	if w.Written() {
		t.Fatal("a new writer says it has written")
	}
	w.WriteHeader(http.StatusEarlyHints)
	if w.Written() {
		t.Fatal("a 103 Early Hints counted as the response starting")
	}
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte("hello"))
	if w.Status() != 201 || w.Size() != 5 {
		t.Fatalf("status %d size %d, want 201 and 5", w.Status(), w.Size())
	}
	if len(s.codes) != 2 || s.codes[0] != 103 || s.codes[1] != 201 {
		t.Errorf("the writer underneath got %v, want [103 201]", s.codes)
	}
}

func TestAWriteWithoutAStatusIsA200(t *testing.T) {
	w := New(httptest.NewRecorder())
	io.WriteString(w, "hi")
	if w.Status() != 200 || w.Size() != 2 {
		t.Fatalf("status %d size %d, want 200 and 2", w.Status(), w.Size())
	}
}

func TestCopyingIntoTheWriterCountsToo(t *testing.T) {
	rec := httptest.NewRecorder()
	w := New(rec)
	if _, err := io.Copy(w, strings.NewReader("a file's worth")); err != nil {
		t.Fatal(err)
	}
	if w.Status() != 200 || w.Size() != 14 || rec.Body.String() != "a file's worth" {
		t.Fatalf("status %d size %d body %q", w.Status(), w.Size(), rec.Body)
	}
}

func TestFlushReachesTheWriterUnderneath(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := http.NewResponseController(New(rec)).Flush(); err != nil {
		t.Fatal(err)
	}
	if !rec.Flushed {
		t.Fatal("the recorder underneath wasn't flushed")
	}
}
