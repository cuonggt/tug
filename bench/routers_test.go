package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cuonggt/tug"
	"github.com/gin-gonic/gin"
	"github.com/go-chi/chi/v5"
	"github.com/labstack/echo/v5"
)

// The routers' benchmark puts one route and its response, GET /posts/{id}
// answering the ID as text, through ServeMux alone and through each
// framework, each as its New makes it, with no middleware: what each adds
// to a request. tug's App recovers a handler's panic without being asked;
// the others need a middleware for it, which this leaves out.
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

	gin.SetMode(gin.ReleaseMode)
	g := gin.New()
	g.GET("/posts/:id", func(c *gin.Context) { c.String(http.StatusOK, c.Param("id")) })

	e := echo.New()
	e.GET("/posts/:id", func(c *echo.Context) error { return c.String(http.StatusOK, c.Param("id")) })

	ch := chi.NewRouter()
	ch.Get("/posts/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, chi.URLParam(r, "id"))
	})

	return []router{{"ServeMux", mux}, {"tug", app}, {"Gin", g}, {"Echo", e}, {"Chi", ch}}
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
// the framework and not a recorder.
type discard struct{ h http.Header }

func (d *discard) Header() http.Header               { return d.h }
func (d *discard) Write(b []byte) (int, error)       { return len(b), nil }
func (d *discard) WriteString(s string) (int, error) { return len(s), nil }
func (d *discard) WriteHeader(int)                   {}
