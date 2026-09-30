package tug

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/cuonggt/tug/inertia"
	"github.com/cuonggt/tug/middleware"
)

func TestEveryResponseSaysWhatABrowserMayDoWithIt(t *testing.T) {
	captureLog(t) // the 500's
	pages, err := inertia.New(inertia.Config{Template: `<script nonce="{{ .Nonce }}">theme()</script>{{ .Inertia }}`})
	if err != nil {
		t.Fatal(err)
	}
	app := New(Config{Inertia: pages})
	app.Use(middleware.Headers(middleware.HeadersConfig{}), middleware.CSP(middleware.CSPConfig{}))
	app.Get("/posts", func(c *Ctx) error { return c.Inertia("Posts/Index", nil) })
	app.Get("/posts.json", func(c *Ctx) error { return c.JSON(200, map[string]int{"posts": 2}) })
	app.Get("/report", func(c *Ctx) error { return c.FileFS(fstest.MapFS{"report.txt": {Data: []byte("hello")}}, "report.txt") })
	app.Get("/fails", func(c *Ctx) error { return errors.New("the database is down") })

	for _, path := range []string{"/posts", "/posts.json", "/report", "/fails", "/nowhere"} {
		w := serve(app, "GET", path, "")
		h := w.Header()
		if h.Get("X-Frame-Options") != "SAMEORIGIN" || h.Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(h.Get("Content-Security-Policy"), "'strict-dynamic'") {
			t.Errorf("%s: %d, headers %v", path, w.Code, h)
		}
	}

	// A page's own script carries the nonce its policy runs scripts by.
	w := serve(app, "GET", "/posts", "")
	_, n, _ := strings.Cut(w.Header().Get("Content-Security-Policy"), "'nonce-")
	n, _, _ = strings.Cut(n, "'")
	if n == "" || !strings.HasPrefix(w.Body.String(), `<script nonce="`+n+`">theme()</script>`) {
		t.Errorf("the nonce %q, and the page\n%s", n, w.Body)
	}
}
