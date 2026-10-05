package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/cuonggt/tug"
	"github.com/cuonggt/tug/bench/page"
	"github.com/cuonggt/tug/inertia"
	gonertia "github.com/romsar/gonertia/v3"
)

// The Inertia benchmark renders one page, page.json's post and comments as
// Posts/Show, through tug and through gonertia, the other Go adapter of
// Inertia's protocol, with no session and no middleware but Inertia's: a
// visit from Inertia's client, answered with the page object as JSON, and
// a first visit, answered with the root template's HTML. tug's inertia
// package, which needs no tug App, is on ServeMux beside gonertia's.
//
//	go test -run '^$' -bench Inertia -benchmem

var show = tug.Page[page.Props](page.Component)

// root is the root template, the same HTML as every app's in apps/; gonertia
// names what it fills in with lower case.
const root = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Blog</title>
    {{ .InertiaHead }}
  </head>
  <body>
    {{ .Inertia }}
  </body>
</html>
`

type adapter struct {
	name    string
	handler http.Handler
}

func adapters(tb testing.TB) []adapter {
	pages, err := inertia.New(inertia.Config{Template: root, Version: "1"})
	if err != nil {
		tb.Fatal(err)
	}
	app := tug.New(tug.Config{Inertia: pages})
	app.Get("/posts/{id}", func(c *tug.Ctx) error {
		id, _ := strconv.Atoi(c.Param("id"))
		return show.Render(c, page.For(id))
	})

	onMux := http.NewServeMux()
	onMux.Handle("GET /posts/{id}", pages.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(r.PathValue("id"))
		if err := pages.Render(w, r, page.Component, page.For(id)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})))

	g, err := gonertia.New(lowerCase.Replace(root), gonertia.WithVersion("1"))
	if err != nil {
		tb.Fatal(err)
	}
	gMux := http.NewServeMux()
	gMux.Handle("GET /posts/{id}", g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(r.PathValue("id"))
		p := page.For(id)
		if err := g.Render(w, r, page.Component, gonertia.Props{"post": p.Post, "comments": p.Comments}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})))

	return []adapter{{"tug, App", app}, {"tug's inertia, ServeMux", onMux}, {"gonertia, ServeMux", gMux}}
}

var lowerCase = strings.NewReplacer("{{ .InertiaHead }}", "{{ .inertiaHead }}", "{{ .Inertia }}", "{{ .inertia }}")

func BenchmarkInertia(b *testing.B) {
	for _, a := range adapters(b) {
		version := pageOf(b, a.handler, firstVisit()).Version
		b.Run("visit/"+a.name, func(b *testing.B) { serve(b, a.handler, visit(version)) })
		b.Run("first visit/"+a.name, func(b *testing.B) { serve(b, a.handler, firstVisit()) })
	}
}

func TestAdaptersRenderTheSamePage(t *testing.T) {
	for _, a := range adapters(t) {
		first := pageOf(t, a.handler, firstVisit())
		later := pageOf(t, a.handler, visit(first.Version))
		for _, p := range []sentPage{first, later} {
			if p.Component != page.Component || p.URL != "/posts/42" || p.Props.Post.ID != 42 || len(p.Props.Comments) != 10 {
				t.Errorf("%s rendered %+v", a.name, p)
			}
		}
	}
}

// firstVisit is a browser's request for the page, as a link from another
// site makes one.
func firstVisit() *http.Request {
	r := httptest.NewRequest("GET", "/posts/42", nil)
	r.Header.Set("Accept", "text/html, application/xhtml+xml")
	return r
}

// visit is the request Inertia's client makes for the page, from another
// page of the app's build.
func visit(version string) *http.Request {
	r := httptest.NewRequest("GET", "/posts/42", nil)
	r.Header.Set("Accept", "text/html, application/xhtml+xml")
	r.Header.Set("X-Requested-With", "XMLHttpRequest")
	r.Header.Set("X-Inertia", "true")
	r.Header.Set("X-Inertia-Version", version)
	return r
}

// pageOf is the page object h answers r with: the JSON of a visit, or what
// a first visit's HTML has in it.
func pageOf(tb testing.TB, h http.Handler, r *http.Request) sentPage {
	tb.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		tb.Fatalf("%s %s: %d %s", r.Method, r.URL, w.Code, w.Body)
	}
	body := w.Body.Bytes()
	if r.Header.Get("X-Inertia") == "" {
		m := pageScript.FindSubmatch(body)
		if m == nil {
			tb.Fatalf("no page in the first visit's HTML:\n%s", body)
		}
		body = m[1]
	} else if w.Header().Get("X-Inertia") != "true" {
		tb.Fatalf("a visit's answer without X-Inertia: %v", w.Header())
	}
	var p sentPage
	if err := json.Unmarshal(body, &p); err != nil {
		tb.Fatalf("the page: %v\n%s", err, body)
	}
	return p
}
