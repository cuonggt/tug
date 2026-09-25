package tug

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"github.com/cuonggt/tug/internal/rw"
)

// Ctx is a request and its response, as a handler sees them. It belongs to
// its request: don't keep it after the handler returns.
type Ctx struct {
	app   *App
	r     *http.Request
	rw    rw.Writer
	query url.Values
}

// Request returns the request.
func (c *Ctx) Request() *http.Request { return c.r }

// Response returns the response writer.
func (c *Ctx) Response() http.ResponseWriter { return &c.rw }

// Context returns the request's context, which is canceled when the client
// goes away.
func (c *Ctx) Context() context.Context { return c.r.Context() }

// Written reports whether the response has started. After that its status
// and headers are on their way and can't change.
func (c *Ctx) Written() bool { return c.rw.Written() }

// Param returns the value of the path wildcard name: Param("id") is "42"
// for /posts/42 on a "/posts/{id}" route.
func (c *Ctx) Param(name string) string { return c.r.PathValue(name) }

// Query returns the first value of the query parameter name.
func (c *Ctx) Query(name string) string { return c.queryValues().Get(name) }

func (c *Ctx) queryValues() url.Values {
	if c.query == nil {
		c.query = c.r.URL.Query()
	}
	return c.query
}

// URL builds the path of a named route, as App.URL does.
func (c *Ctx) URL(name string, params ...any) (string, error) {
	return c.app.URL(name, params...)
}

// JSON writes v as JSON.
func (c *Ctx) JSON(code int, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.Blob(code, "application/json", b)
}

// String writes s as plain text.
func (c *Ctx) String(code int, s string) error {
	c.writeHeader(code, "text/plain; charset=utf-8")
	io.WriteString(&c.rw, s)
	return nil
}

// HTML writes html as a page, exactly as given: anything from a user in it
// must already be escaped.
func (c *Ctx) HTML(code int, html string) error {
	c.writeHeader(code, "text/html; charset=utf-8")
	io.WriteString(&c.rw, html)
	return nil
}

// Blob writes b as the body, with the given content type.
func (c *Ctx) Blob(code int, contentType string, b []byte) error {
	c.writeHeader(code, contentType)
	c.rw.Write(b)
	return nil
}

// writeHeader starts a response. The writes of the body after it ignore
// their errors: one means the client has gone, and there is no one left
// to tell.
func (c *Ctx) writeHeader(code int, contentType string) {
	c.rw.Header().Set("Content-Type", contentType)
	c.rw.WriteHeader(code)
}

// NoContent writes a status and no body, such as 204.
func (c *Ctx) NoContent(code int) error {
	c.rw.WriteHeader(code)
	return nil
}

// Redirect sends the client to another URL: with 302 Found after a GET or
// HEAD, and 303 See Other after any other method. A 303 makes browsers,
// and Inertia, follow a PUT, PATCH or DELETE with a GET, where a 302 may
// repeat the method.
func (c *Ctx) Redirect(to string) error {
	code := http.StatusSeeOther
	if c.r.Method == http.MethodGet || c.r.Method == http.MethodHead {
		code = http.StatusFound
	}
	http.Redirect(&c.rw, c.r, to, code)
	return nil
}

// RedirectRoute redirects to the named route, filling its wildcards as
// App.URL does.
func (c *Ctx) RedirectRoute(name string, params ...any) error {
	to, err := c.URL(name, params...)
	if err != nil {
		return err
	}
	return c.Redirect(to)
}
