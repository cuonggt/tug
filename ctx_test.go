package tug

import (
	"net/http"
	"testing"
)

func TestResponsesSayWhatTheyAre(t *testing.T) {
	for _, tc := range []struct {
		h           HandlerFunc
		code        int
		contentType string
		body        string
	}{
		{func(c *Ctx) error { return c.JSON(201, map[string]int{"id": 1}) }, 201, "application/json", `{"id":1}`},
		{func(c *Ctx) error { return c.String(200, "hi") }, 200, "text/plain; charset=utf-8", "hi"},
		{func(c *Ctx) error { return c.HTML(200, "<p>hi</p>") }, 200, "text/html; charset=utf-8", "<p>hi</p>"},
		{func(c *Ctx) error { return c.Blob(200, "text/csv", []byte("a,b")) }, 200, "text/csv", "a,b"},
		{func(c *Ctx) error { return c.NoContent(204) }, 204, "", ""},
	} {
		app := New(Config{})
		app.Get("/", tc.h)
		rec := serve(app, "GET", "/", "")
		if rec.Code != tc.code || rec.Header().Get("Content-Type") != tc.contentType || rec.Body.String() != tc.body {
			t.Errorf("got %d %q %q, want %d %q %q",
				rec.Code, rec.Header().Get("Content-Type"), rec.Body, tc.code, tc.contentType, tc.body)
		}
	}
}

func TestJSONThatCannotBeEncodedIsAnError(t *testing.T) {
	app := New(Config{})
	app.Get("/", func(c *Ctx) error { return c.JSON(200, func() {}) })
	if rec := serve(app, "GET", "/", ""); rec.Code != 500 {
		t.Fatalf("got %d %q, want a 500 rather than half a response", rec.Code, rec.Body)
	}
}

func TestParamAndQueryReadTheURL(t *testing.T) {
	app := New(Config{})
	app.Get("/posts/{id}", func(c *Ctx) error {
		return c.String(200, c.Param("id")+" "+c.Query("tab"))
	})
	if rec := serve(app, "GET", "/posts/42?tab=comments", ""); rec.Body.String() != "42 comments" {
		t.Fatalf("got %q", rec.Body)
	}
}

func TestARedirectAfterAGetIsFoundAndAfterAnythingElseSeeOther(t *testing.T) {
	app := New(Config{})
	app.Get("/posts", text("posts")).Name("posts.index")
	for _, m := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
		app.Handle(m, "/go", func(c *Ctx) error { return c.RedirectRoute("posts.index") })
	}

	for method, want := range map[string]int{
		"GET":    http.StatusFound,
		"POST":   http.StatusSeeOther,
		"PUT":    http.StatusSeeOther,
		"PATCH":  http.StatusSeeOther,
		"DELETE": http.StatusSeeOther,
	} {
		rec := serve(app, method, "/go", "")
		if rec.Code != want || rec.Header().Get("Location") != "/posts" {
			t.Errorf("%s /go = %d to %q, want %d to /posts", method, rec.Code, rec.Header().Get("Location"), want)
		}
	}
}

func TestARedirectToARouteThatIsNotThereIsAnError(t *testing.T) {
	app := New(Config{})
	app.Get("/", func(c *Ctx) error { return c.RedirectRoute("nowhere") })
	if rec := serve(app, "GET", "/", ""); rec.Code != 500 {
		t.Fatalf("got %d, want a 500", rec.Code)
	}
}

func TestWrapHandlerPutsAPlainHandlerOnARoute(t *testing.T) {
	app := New(Config{})
	app.Get("/plain", WrapHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("plain"))
	})))
	if rec := serve(app, "GET", "/plain", ""); rec.Body.String() != "plain" {
		t.Fatalf("got %q", rec.Body)
	}
}
