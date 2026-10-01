package tug

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type takesInput struct {
	Title string `json:"title" validate:"required"`
}

type otherInput struct {
	Body string `json:"body"`
}

// takesFilters are a list's, from the query alone, which a handler binds
// beside a form.
type takesFilters struct {
	Page int `query:"page"`
}

func TestTugGenWritesWhatARouteTakesByItsName(t *testing.T) {
	app := New(Config{})
	app.Post("/posts", text("ok")).Name("posts.store").Takes(takesInput{})
	app.Post("/drafts", text("ok")).Name("drafts.store").Takes(&takesInput{}) // a pointer to one is the struct
	app.Get("/posts", text("ok")).Name("posts.index")

	path := filepath.Join(t.TempDir(), "gen.json")
	if err := app.gen(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out struct{ Routes string }
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"export interface takesInput {\n  title: string\n}",
		"export interface Inputs {\n  'drafts.store': takesInput\n  'posts.store': takesInput\n}",
	} {
		if !strings.Contains(out.Routes, want) {
			t.Errorf("routes.ts hasn't\n%s\nin\n%s", want, out.Routes)
		}
	}
}

func TestARouteTakesAStructOnceItHasAName(t *testing.T) {
	app := New(Config{})
	if !panics(func() { app.Post("/posts", text("ok")).Takes(takesInput{}) }) {
		t.Error("a route with no name took an input")
	}
	if !panics(func() { app.Post("/posts/{id}", text("ok")).Name("posts.update").Takes("a title") }) {
		t.Error("a route took a string")
	}
}

func TestBindFailsAHandlerThatBindsOtherThanItsRouteTakes(t *testing.T) {
	logs := captureLog(t)
	app := New(Config{})
	app.Post("/posts", func(c *Ctx) error {
		var in otherInput
		return c.Bind(&in)
	}).Name("posts.store").Takes(takesInput{})
	app.Post("/drafts", func(c *Ctx) error {
		var filters takesFilters
		if err := c.Bind(&filters); err != nil {
			return err
		}
		var in takesInput
		if err := c.BindValid(&in); err != nil {
			return err
		}
		return c.String(http.StatusOK, in.Title)
	}).Name("drafts.store").Takes(takesInput{})
	app.Post("/notes", func(c *Ctx) error {
		var in otherInput
		if err := c.Bind(&in); err != nil {
			return err
		}
		return c.String(http.StatusOK, in.Body)
	})

	rec := serve(app, "POST", "/posts", `{"body":"Hello"}`, "Content-Type", "application/json")
	if rec.Code != http.StatusInternalServerError || !strings.Contains(logs.String(), "takes tug.takesInput, by Takes, but its handler binds tug.otherInput") {
		t.Errorf("a handler binding another struct got %d, and the log says\n%s", rec.Code, logs)
	}
	// Query fields alone are let through, beside what the route takes.
	if rec := serve(app, "POST", "/drafts?page=2", `{"title":"Hello"}`, "Content-Type", "application/json"); rec.Code != http.StatusOK || rec.Body.String() != "Hello" {
		t.Errorf("binding filters and what the route takes got %d %q", rec.Code, rec.Body)
	}
	// A route that takes nothing is checked for nothing.
	if rec := serve(app, "POST", "/notes", `{"body":"Hello"}`, "Content-Type", "application/json"); rec.Code != http.StatusOK || rec.Body.String() != "Hello" {
		t.Errorf("a route that takes nothing got %d %q", rec.Code, rec.Body)
	}
}
