package tug

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cuonggt/tug/auth"
	"github.com/cuonggt/tug/inertia"
)

// byHeader is a key made from the X-Client header, as an app's would be
// from the address a request came from.
func byHeader(c *Ctx) string {
	return c.Request().Header.Get("X-Client")
}

func TestARequestOverItsLimitGetsA429AndDoesntReachTheHandler(t *testing.T) {
	searches := &auth.Throttle{Name: "searches", Max: 2, Window: time.Minute}
	ran := 0
	app := New(Config{})
	app.Get("/search", Limit(searches, byHeader, func(c *Ctx) error {
		ran++
		return c.String(http.StatusOK, "found")
	}))
	for i := range 2 {
		if rec := serve(app, "GET", "/search", "", "X-Client", "ann"); rec.Code != http.StatusOK {
			t.Fatalf("request %d: %d %s", i+1, rec.Code, rec.Body)
		}
	}
	rec := serve(app, "GET", "/search", "", "X-Client", "ann")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "60" || ran != 2 {
		t.Errorf("the third: %d, Retry-After %q, and the handler ran %d times", rec.Code, rec.Header().Get("Retry-After"), ran)
	}
	if !strings.Contains(rec.Body.String(), "too many requests: wait 60 seconds, and try again") {
		t.Errorf("the third says %q", rec.Body)
	}
	if rec := serve(app, "GET", "/search", "", "X-Client", "bob"); rec.Code != http.StatusOK {
		t.Errorf("another key: %d", rec.Code)
	}
}

func TestAnInertiaVisitOverItsLimitGetsTheErrorPage(t *testing.T) {
	pages, err := inertia.New(inertia.Config{Template: `<body>{{ .Inertia }}</body>`, Version: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	app := New(Config{Inertia: pages, ErrorPage: "Error"})
	app.Get("/search", Limit(&auth.Throttle{Max: 1}, byHeader, text("found")))
	serve(app, "GET", "/search", "", "X-Inertia", "true", "X-Inertia-Version", "v1")

	rec := serve(app, "GET", "/search", "", "X-Inertia", "true", "X-Inertia-Version", "v1")
	var p inertia.Page
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("%d %s: %v", rec.Code, rec.Body, err)
	}
	if rec.Code != http.StatusTooManyRequests || p.Component != "Error" || p.Props["status"] != 429.0 || rec.Header().Get("Retry-After") != "60" {
		t.Errorf("got %d, Retry-After %q, with %+v", rec.Code, rec.Header().Get("Retry-After"), p)
	}

	// An API's client gets JSON.
	rec = serve(app, "GET", "/search", "", "Accept", "application/json")
	if rec.Code != http.StatusTooManyRequests || rec.Body.String() != `{"message":"too many requests: wait 60 seconds, and try again"}` {
		t.Errorf("an API client got %d %s", rec.Code, rec.Body)
	}
}

// waits is a Limiter whose every key waits, or fails.
type waits struct {
	wait time.Duration
	err  error
}

func (w waits) Try(context.Context, string) (time.Duration, error) { return w.wait, w.err }

func TestTheWaitIsSaidInWholeSecondsRoundedUp(t *testing.T) {
	for wait, want := range map[time.Duration]string{
		time.Millisecond:                     "1",
		time.Second:                          "1",
		59*time.Second + 10*time.Millisecond: "60",
	} {
		app := New(Config{})
		app.Get("/", Limit(waits{wait: wait}, byHeader, text("home")))
		rec := serve(app, "GET", "/", "")
		if got := rec.Header().Get("Retry-After"); got != want {
			t.Errorf("a wait of %v: Retry-After %q, want %q", wait, got, want)
		}
		if wait == time.Second && !strings.Contains(rec.Body.String(), "wait 1 second,") {
			t.Errorf("a wait of a second says %q", rec.Body)
		}
	}
}

func TestALimitersErrorIsTheHandlers(t *testing.T) {
	captureLog(t)
	var got error
	down := errors.New("the throttles table is gone")
	app := New(Config{ErrorHandler: func(c *Ctx, err error) {
		got = err
		DefaultErrorHandler(c, err)
	}})
	app.Get("/", Limit(waits{err: down}, byHeader, text("home")))
	if rec := serve(app, "GET", "/", ""); rec.Code != http.StatusInternalServerError || !errors.Is(got, down) {
		t.Errorf("got %d, and the ErrorHandler %v", rec.Code, got)
	}
}
