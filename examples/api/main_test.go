package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cuonggt/tug"
)

// client sends JSON requests to one app, as an API client would.
type client struct {
	app *tug.App
}

func (c client) do(method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Accept", "application/json")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	c.app.ServeHTTP(rec, req)
	return rec
}

func TestAPostCanBeCreatedReadChangedAndDeleted(t *testing.T) {
	c := client{newApp(tug.Config{})}

	rec := c.do("POST", "/api/posts", `{"title":"Hello","body":"First post"}`)
	if rec.Code != http.StatusCreated || rec.Header().Get("Location") != "/api/posts/1" {
		t.Fatalf("create = %d at %q: %s", rec.Code, rec.Header().Get("Location"), rec.Body)
	}
	if rec := c.do("GET", "/api/posts/1", ""); rec.Code != 200 || rec.Body.String() != `{"id":1,"title":"Hello","body":"First post"}` {
		t.Fatalf("show = %d %s", rec.Code, rec.Body)
	}
	if rec := c.do("PUT", "/api/posts/1", `{"id":9,"title":"Hello again"}`); rec.Code != 200 {
		t.Fatalf("update = %d %s", rec.Code, rec.Body)
	}
	if rec := c.do("GET", "/api/posts", ""); rec.Body.String() != `[{"id":1,"title":"Hello again","body":""}]` {
		t.Fatalf("index = %s, want the post changed in place, still 1", rec.Body)
	}
	if rec := c.do("DELETE", "/api/posts/1", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("destroy = %d %s", rec.Code, rec.Body)
	}
	if rec := c.do("GET", "/api/posts/1", ""); rec.Code != 404 || rec.Body.String() != `{"message":"post not found"}` {
		t.Fatalf("show after destroy = %d %s", rec.Code, rec.Body)
	}
}

func TestNoPostsIsAnEmptyList(t *testing.T) {
	c := client{newApp(tug.Config{})}
	if rec := c.do("GET", "/api/posts", ""); rec.Body.String() != "[]" {
		t.Fatalf("index = %s, want []", rec.Body)
	}
}

func TestAPostNeedsATitle(t *testing.T) {
	c := client{newApp(tug.Config{})}
	rec := c.do("POST", "/api/posts", `{"body":"no title"}`)
	if rec.Code != http.StatusUnprocessableEntity || rec.Body.String() != `{"message":"title is required"}` {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}
}

func TestAPostIDThatIsNotANumberIsNotFound(t *testing.T) {
	c := client{newApp(tug.Config{})}
	if rec := c.do("GET", "/api/posts/abc", ""); rec.Code != 404 {
		t.Fatalf("show /api/posts/abc = %d %s", rec.Code, rec.Body)
	}
}
