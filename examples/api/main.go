// Command api is a small JSON API on tug: posts kept in memory, behind
// named routes in a group, with the usual middleware, errors that come
// back as JSON to a client that asks for it, and a limit on how fast a
// client writes.
//
//	ADDR=127.0.0.1:8080 go run ./examples/api
//	curl -s localhost:8080/api/posts -H 'Accept: application/json' \
//		-H 'Content-Type: application/json' -d '{"title":"Hello"}'
package main

import (
	"cmp"
	"log"
	"net"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/cuonggt/tug"
	"github.com/cuonggt/tug/auth"
	"github.com/cuonggt/tug/middleware"
)

type Post struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

func main() {
	if err := newApp(tug.ConfigFromEnv()).Run(); err != nil {
		log.Fatal(err)
	}
}

func newApp(cfg tug.Config) *tug.App {
	p := &posts{byID: make(map[int64]Post)}
	app := tug.New(cfg)
	app.Use(middleware.RequestID(), middleware.Logger(), middleware.Recover(), middleware.CSRF())

	// An address writes 60 times a minute at most: past that, a 429, with
	// Retry-After. The counts are in memory, this process's own; give the
	// Throttle a Store for instances that share them.
	writes := &auth.Throttle{Name: "writes", Max: 60, Window: time.Minute}

	api := app.Group("/api")
	api.Get("/posts", p.index).Name("posts.index")
	api.Post("/posts", tug.Limit(writes, byAddress, p.store)).Name("posts.store")
	api.Get("/posts/{id}", p.show).Name("posts.show")
	api.Put("/posts/{id}", tug.Limit(writes, byAddress, p.update)).Name("posts.update")
	api.Delete("/posts/{id}", tug.Limit(writes, byAddress, p.destroy)).Name("posts.destroy")
	return app
}

// byAddress is the address a request came from, which the limit counts by.
// Behind a proxy, it's the proxy's: read the client's from the header the
// proxy sets, once only the proxy can reach the app.
func byAddress(c *tug.Ctx) string {
	host, _, err := net.SplitHostPort(c.Request().RemoteAddr)
	if err != nil {
		return c.Request().RemoteAddr
	}
	return host
}

// posts is the store behind the routes.
type posts struct {
	mu   sync.Mutex
	last int64
	byID map[int64]Post
}

// postInput is what a client sends to create or change a post. The ID comes
// from the URL alone: json:"-" keeps a body from naming one.
type postInput struct {
	ID    int64  `path:"id" json:"-"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

func (in postInput) check() error {
	if in.Title == "" {
		return tug.NewHTTPError(http.StatusUnprocessableEntity, "title is required")
	}
	return nil
}

func postNotFound() error {
	return tug.NewHTTPError(http.StatusNotFound, "post not found")
}

func (p *posts) index(c *tug.Ctx) error {
	p.mu.Lock()
	// Made, not declared: a nil slice would go out as null rather than [].
	list := make([]Post, 0, len(p.byID))
	for _, post := range p.byID {
		list = append(list, post)
	}
	p.mu.Unlock()
	slices.SortFunc(list, func(a, b Post) int { return cmp.Compare(a.ID, b.ID) })
	return c.JSON(http.StatusOK, list)
}

func (p *posts) store(c *tug.Ctx) error {
	var in postInput
	if err := c.Bind(&in); err != nil {
		return err
	}
	if err := in.check(); err != nil {
		return err
	}
	p.mu.Lock()
	p.last++
	post := Post{ID: p.last, Title: in.Title, Body: in.Body}
	p.byID[post.ID] = post
	p.mu.Unlock()

	loc, err := c.URL("posts.show", post.ID)
	if err != nil {
		return err
	}
	c.Response().Header().Set("Location", loc)
	return c.JSON(http.StatusCreated, post)
}

func (p *posts) show(c *tug.Ctx) error {
	var in postInput
	if err := c.Bind(&in); err != nil {
		return err
	}
	p.mu.Lock()
	post, ok := p.byID[in.ID]
	p.mu.Unlock()
	if !ok {
		return postNotFound()
	}
	return c.JSON(http.StatusOK, post)
}

func (p *posts) update(c *tug.Ctx) error {
	var in postInput
	if err := c.Bind(&in); err != nil {
		return err
	}
	if err := in.check(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.byID[in.ID]; !ok {
		return postNotFound()
	}
	post := Post{ID: in.ID, Title: in.Title, Body: in.Body}
	p.byID[post.ID] = post
	return c.JSON(http.StatusOK, post)
}

func (p *posts) destroy(c *tug.Ctx) error {
	var in postInput
	if err := c.Bind(&in); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.byID[in.ID]; !ok {
		return postNotFound()
	}
	delete(p.byID, in.ID)
	return c.NoContent(http.StatusNoContent)
}
