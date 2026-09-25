package tug

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The benchmarks put the same route and response through ServeMux alone and
// through tug, to show what tug adds to a request: its Ctx, the error
// return, and the panic recovery.
//
//	go test -run '^$' -bench . -benchmem .

func BenchmarkServeMux(b *testing.B) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /posts/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, r.PathValue("id"))
	})
	benchServe(b, mux)
}

func BenchmarkTug(b *testing.B) {
	app := New(Config{})
	app.Get("/posts/{id}", func(c *Ctx) error { return c.String(http.StatusOK, c.Param("id")) })
	benchServe(b, app)
}

func benchServe(b *testing.B, h http.Handler) {
	req := httptest.NewRequest("GET", "/posts/42", nil)
	w := &discard{h: http.Header{}}
	b.ReportAllocs()
	for b.Loop() {
		clear(w.h)
		h.ServeHTTP(w, req)
	}
}

// discard is a ResponseWriter that keeps nothing, so the benchmarks measure
// the routing and not a recorder.
type discard struct{ h http.Header }

func (d *discard) Header() http.Header               { return d.h }
func (d *discard) Write(b []byte) (int, error)       { return len(b), nil }
func (d *discard) WriteString(s string) (int, error) { return len(s), nil }
func (d *discard) WriteHeader(int)                   {}
