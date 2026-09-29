package tug

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/cuonggt/tug/internal/rw"
	"github.com/cuonggt/tug/lang"
)

// Ctx is a request and its response, as a handler sees them. It belongs to
// its request: don't keep it after the handler returns.
type Ctx struct {
	app   *App
	r     *http.Request
	rw    rw.Writer
	query url.Values

	// body is the JSON body, once Bind has read it.
	body []byte

	// locale is the request's language, once Locale has picked it.
	locale string

	// flash and clearHistory are what this request has for the next page
	// shown; see Flash.
	flash            map[string]any
	clearHistory     bool
	preserveFragment bool
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

// AbsoluteURL builds the whole link to a named route, from the app's
// address, as App.AbsoluteURL does.
func (c *Ctx) AbsoluteURL(name string, params ...any) (string, error) {
	return c.app.AbsoluteURL(name, params...)
}

// SignedURL builds a signed link to a named route, as App.SignedURL does.
func (c *Ctx) SignedURL(name string, expires time.Time, params ...any) (string, error) {
	return c.app.SignedURL(name, expires, params...)
}

// Locale returns the request's language, as App.Locale picks it: the one
// the app chose for it with Config.Locale, or else the browser's, of the
// app's languages, or else the app's default.
func (c *Ctx) Locale() string {
	if c.locale == "" {
		c.locale = c.app.Locale(c.r)
	}
	return c.locale
}

// T says text in the request's language, as lang.Words.T does, from
// Config.Lang: c.T("Welcome back, :name", "name", user.Name), for a flash
// message. A text the language has no words for says itself, in English.
func (c *Ctx) T(text string, args ...any) string {
	return c.words().T(text, args...)
}

// Choice says text for a count in the request's language, as
// lang.Words.Choice does: c.Choice(":count post deleted|:count posts
// deleted", n).
func (c *Ctx) Choice(text string, n int, args ...any) string {
	return c.words().Choice(text, n, args...)
}

func (c *Ctx) words() lang.Words {
	return c.app.config.Lang.In(c.Locale())
}

// IP returns the address the request came from, without its port, as
// "203.0.113.9": a key for Limit that counts by address. Behind a proxy
// it's the proxy's, unless middleware.TrustProxies names the proxy, and
// reads the client's past it.
func (c *Ctx) IP() string {
	host, _, err := net.SplitHostPort(c.r.RemoteAddr)
	if err != nil {
		return c.r.RemoteAddr
	}
	return host
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
	s := c.Session()
	if (len(c.flash) > 0 || c.clearHistory || c.preserveFragment) && s == nil {
		return errors.New("tug: flash data needs Config.Session to reach the page after a redirect")
	}
	if s != nil {
		carryFlash(s, c.flash)
	}
	code := http.StatusSeeOther
	if c.r.Method == http.MethodGet || c.r.Method == http.MethodHead {
		code = http.StatusFound
	}
	http.Redirect(&c.rw, c.r, to, code)
	return nil
}

// RedirectBack redirects to the page the request came from, by its
// Referer, when that's a page of this app, and to "/" otherwise: a
// Referer can name any site. It's for a form that more than one page
// has, which goes back to whichever it was sent from.
func (c *Ctx) RedirectBack() error {
	return c.Redirect(c.back())
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
