package tug

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/cuonggt/tug/inertia"
)

func errorPageApp(t *testing.T, debug bool) *App {
	t.Helper()
	pages, err := inertia.New(inertia.Config{Template: `<body>{{ .Inertia }}</body>`, Version: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	pages.Share("appName", "tug")
	app := New(Config{Inertia: pages, ErrorPage: "Error", Debug: debug})
	app.Get("/posts/{id}", func(c *Ctx) error { return NewHTTPError(http.StatusNotFound, "post not found") })
	app.Get("/broken", func(c *Ctx) error { return errors.New("database password rejected") })
	return app
}

func TestAnErrorIsShownAsTheErrorPage(t *testing.T) {
	app := errorPageApp(t, false)

	rec := serve(app, "GET", "/posts/9", "", "X-Inertia", "true", "X-Inertia-Version", "v1")
	var p inertia.Page
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("%d %s: %v", rec.Code, rec.Body, err)
	}
	if rec.Code != 404 || p.Component != "Error" || p.Props["status"] != 404.0 || p.Props["message"] != "post not found" || p.Props["appName"] != "tug" {
		t.Errorf("got %d with %+v", rec.Code, p)
	}

	// A browser's first visit gets the page too, with the status.
	rec = serve(app, "GET", "/nowhere", "")
	if rec.Code != 404 || !strings.Contains(rec.Body.String(), `{"component":"Error"`) {
		t.Errorf("a first visit got %d %s", rec.Code, rec.Body)
	}
}

func TestAnErrorPageHidesA500sDetailsUnlessDebugShowsThem(t *testing.T) {
	captureLog(t)
	rec := serve(errorPageApp(t, false), "GET", "/broken", "", "X-Inertia", "true", "X-Inertia-Version", "v1")
	if rec.Code != 500 || !strings.Contains(rec.Body.String(), `"message":"Internal Server Error"`) || strings.Contains(rec.Body.String(), "password") {
		t.Errorf("production: %d %s", rec.Code, rec.Body)
	}
	rec = serve(errorPageApp(t, true), "GET", "/broken", "", "X-Inertia", "true", "X-Inertia-Version", "v1")
	if rec.Code != 500 || !strings.Contains(rec.Body.String(), "database password rejected") || strings.Contains(rec.Body.String(), `"component"`) {
		t.Errorf("debug: %d %s", rec.Code, rec.Body)
	}
}

func TestAnAPIClientGetsJSONRatherThanTheErrorPage(t *testing.T) {
	rec := serve(errorPageApp(t, false), "GET", "/posts/9", "", "Accept", "application/json")
	if rec.Code != 404 || rec.Body.String() != `{"message":"post not found"}` {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
}
