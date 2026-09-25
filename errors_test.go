package tug

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"
)

// captureLog sends slog.Default() to a buffer for the rest of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &buf
}

func TestAnHTTPErrorChoosesTheStatusAndTheMessage(t *testing.T) {
	app := New(Config{})
	app.Get("/draft", func(c *Ctx) error {
		return NewHTTPError(http.StatusForbidden, "drafts are for their authors")
	})
	app.Get("/gone", func(c *Ctx) error { return NewHTTPError(http.StatusNotFound) })

	if rec := serve(app, "GET", "/draft", ""); rec.Code != 403 || rec.Body.String() != "drafts are for their authors" {
		t.Errorf("GET /draft = %d %q", rec.Code, rec.Body)
	}
	if rec := serve(app, "GET", "/gone", ""); rec.Code != 404 || rec.Body.String() != "Not Found" {
		t.Errorf("GET /gone = %d %q, want the status's own text", rec.Code, rec.Body)
	}
}

func TestAnyOtherErrorIsA500ThatKeepsItsDetailsInTheLog(t *testing.T) {
	logs := captureLog(t)
	app := New(Config{})
	app.Get("/", func(c *Ctx) error { return errors.New("database password rejected") })

	rec := serve(app, "GET", "/", "")
	if rec.Code != 500 || rec.Body.String() != "Internal Server Error" {
		t.Fatalf("got %d %q, want a 500 that says nothing more", rec.Code, rec.Body)
	}
	if !strings.Contains(logs.String(), "database password rejected") {
		t.Errorf("the log doesn't have the error: %s", logs)
	}
}

func TestDebugPutsTheErrorIntoThe500(t *testing.T) {
	captureLog(t)
	app := New(Config{Debug: true})
	app.Get("/", func(c *Ctx) error { return errors.New("database password rejected") })

	if rec := serve(app, "GET", "/", ""); !strings.Contains(rec.Body.String(), "database password rejected") {
		t.Fatalf("got %q", rec.Body)
	}
}

func TestAPanicIsA500WithItsStackInTheLog(t *testing.T) {
	logs := captureLog(t)
	app := New(Config{})
	app.Get("/", func(c *Ctx) error { panic("nil map") })

	if rec := serve(app, "GET", "/", ""); rec.Code != 500 {
		t.Fatalf("got %d, want 500", rec.Code)
	}
	if !strings.Contains(logs.String(), "panic: nil map") || !strings.Contains(logs.String(), "goroutine") {
		t.Errorf("the log doesn't have the panic and its stack: %s", logs)
	}
}

func TestErrAbortHandlerStillAbortsTheResponse(t *testing.T) {
	app := New(Config{})
	app.Get("/", func(c *Ctx) error { panic(http.ErrAbortHandler) })

	var got any
	func() {
		defer func() { got = recover() }()
		serve(app, "GET", "/", "")
	}()
	if got != http.ErrAbortHandler {
		t.Fatalf("recovered %v, want http.ErrAbortHandler to reach net/http", got)
	}
}

func TestErrorsAreJSONForClientsThatAskForIt(t *testing.T) {
	app := New(Config{})
	app.Get("/", func(c *Ctx) error { return NewHTTPError(http.StatusNotFound, "post not found") })

	rec := serve(app, "GET", "/", "", "Accept", "application/json")
	if rec.Header().Get("Content-Type") != "application/json" || rec.Body.String() != `{"message":"post not found"}` {
		t.Errorf("an API client got %q %s", rec.Header().Get("Content-Type"), rec.Body)
	}
	rec = serve(app, "GET", "/", "", "Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") {
		t.Errorf("a browser got %q", rec.Header().Get("Content-Type"))
	}
}

func TestAnErrorAfterTheResponseStartedIsOnlyLogged(t *testing.T) {
	logs := captureLog(t)
	app := New(Config{})
	app.Get("/", func(c *Ctx) error {
		c.String(200, "partial")
		return NewHTTPError(http.StatusBadRequest, "too late")
	})

	if rec := serve(app, "GET", "/", ""); rec.Code != 200 || rec.Body.String() != "partial" {
		t.Fatalf("got %d %q, want the response as it was started", rec.Code, rec.Body)
	}
	if !strings.Contains(logs.String(), "too late") {
		t.Errorf("the log doesn't have the error: %s", logs)
	}
}

func TestACustomErrorHandlerAnswersMissesToo(t *testing.T) {
	var seen []int
	app := New(Config{ErrorHandler: func(c *Ctx, err error) {
		var he *HTTPError
		errors.As(err, &he)
		seen = append(seen, he.Code)
		c.String(he.Code, "custom")
	}})
	app.Get("/posts", text("posts"))

	serve(app, "GET", "/missing", "")
	serve(app, "POST", "/posts", "")
	if !slices.Equal(seen, []int{404, 405}) {
		t.Fatalf("the error handler saw %v, want [404 405]", seen)
	}
}
