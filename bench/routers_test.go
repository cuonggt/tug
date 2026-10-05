package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cuonggt/tug"
)

// The router's benchmark puts one route and its response, GET /posts/{id}
// answering the ID as text, through ServeMux alone and through tug's App,
// as New makes it, with no middleware: what tug adds to a request, its
// Ctx, the error a handler returns, and the recovery of a handler's panic.
//
//	go test -run '^$' -bench Router -benchmem

type router struct {
	name    string
	handler http.Handler
}

func routers() []router {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /posts/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, r.PathValue("id"))
	})

	app := tug.New(tug.Config{})
	app.Get("/posts/{id}", func(c *tug.Ctx) error { return c.String(http.StatusOK, c.Param("id")) })

	return []router{{"ServeMux", mux}, {"tug", app}}
}

func BenchmarkRouter(b *testing.B) {
	for _, rt := range routers() {
		b.Run(rt.name, func(b *testing.B) {
			serve(b, rt.handler, httptest.NewRequest("GET", "/posts/42", nil))
		})
	}
}

func TestRoutersAnswerAlike(t *testing.T) {
	for _, rt := range routers() {
		w := httptest.NewRecorder()
		rt.handler.ServeHTTP(w, httptest.NewRequest("GET", "/posts/42", nil))
		if w.Code != http.StatusOK || w.Body.String() != "42" || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain") {
			t.Errorf("%s answered %d, %q, %q", rt.name, w.Code, w.Header().Get("Content-Type"), w.Body)
		}
	}
}

// serve has h answer req over and over, the first time before the clock
// starts, as tug's App puts its routes together then.
func serve(b *testing.B, h http.Handler, req *http.Request) {
	w := &discard{h: http.Header{}}
	h.ServeHTTP(w, req)
	b.ReportAllocs()
	for b.Loop() {
		clear(w.h)
		h.ServeHTTP(w, req)
	}
}

// discard is a ResponseWriter that keeps nothing, so the benchmarks measure
// the router and not a recorder.
type discard struct{ h http.Header }

func (d *discard) Header() http.Header               { return d.h }
func (d *discard) Write(b []byte) (int, error)       { return len(b), nil }
func (d *discard) WriteString(s string) (int, error) { return len(s), nil }
func (d *discard) WriteHeader(int)                   {}
