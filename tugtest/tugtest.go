// Package tugtest tests an app's pages as Inertia's client uses them,
// without a browser or a frontend build. A Client stands in for a browser
// with the app open: its visits are the ones Inertia's client makes, it
// keeps the cookies the app sets, and each Response has the page it shows,
// whose props read into the app's own types.
//
//	func TestANewPostIsShown(t *testing.T) {
//		c := tugtest.New(t, newTestApp(t))
//		c.Get("/posts/create")
//		r := c.Post("/posts", map[string]any{"title": "Hello, tug", "body": "The first post."})
//		if r.Location() != "/posts/1" {
//			t.Fatalf("creating a post: %v", r)
//		}
//		r = r.Follow()
//		if post := tugtest.Props(r, PostsShow).Post; post.Title != "Hello, tug" || r.Page.Flash["success"] != "Post created" {
//			t.Errorf("post %+v, flash %v", post, r.Page.Flash)
//		}
//	}
//
// The client fails the test, with t.Fatalf, when it's asked for something
// that isn't there: a prop the page doesn't have, a redirect to follow
// from a response that isn't one. What the app answers is the test's to
// check.
package tugtest

import (
	"bytes"
	"cmp"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cuonggt/tug/session"
)

// Client is a browser with the app open, for one test. Its visits get the
// page as JSON, as Inertia's client's do. It keeps the cookies the app
// sets, the session's among them, which carry a form's errors and flash
// data to the page after it. And it's on the page it was shown last,
// which its visits send as their Referer, as a browser does, so a form
// that fails validation goes back to it, and which a partial reload is of.
//
// A Client is one browser, for one goroutine at a time.
type Client struct {
	// Version is the build of the frontend the client runs, which each
	// visit sends: the version of the last page it was shown. A client
	// that hasn't been shown one takes the app's at its first visit, as if
	// it had the app open already. Set it to another to be a browser still
	// running an old build, which the app answers with a 409 that reloads
	// the page.
	Version string

	t       testing.TB
	handler http.Handler
	cookies map[string]*http.Cookie
	page    string // the component of the page the client is on, "" before the first
	url     string // where that page is, its path and query
	host    string
}

// New returns a client with the app open: handler is a *tug.App, or any
// http.Handler. The client fails t when a test asks it for something that
// isn't there.
func New(t testing.TB, handler http.Handler) *Client {
	return &Client{t: t, handler: handler, cookies: map[string]*http.Cookie{}}
}

// An Option changes a visit: Only, Except and Reset for a partial reload,
// ErrorBag, Validate, and Header.
type Option func(*visit)

// visit is a visit being made.
type visit struct {
	header  http.Header
	partial bool // it names props, so it's a partial reload
}

// Only makes the visit a partial reload of the page the client is on,
// which asks for these props alone, by name or by path: "stats",
// "auth.user". The props that always go out come too, as the errors do.
func Only(props ...string) Option {
	return func(v *visit) {
		v.partial = true
		v.header.Set("X-Inertia-Partial-Data", strings.Join(props, ","))
	}
}

// Except makes the visit a partial reload of the page the client is on,
// which asks for every prop but these.
func Except(props ...string) Option {
	return func(v *visit) {
		v.partial = true
		v.header.Set("X-Inertia-Partial-Except", strings.Join(props, ","))
	}
}

// Reset asks for props afresh, to be replaced rather than merged into
// what the client has, as the client does when a list it pages through
// starts again.
func Reset(props ...string) Option {
	return func(v *visit) { v.header.Set("X-Inertia-Reset", strings.Join(props, ",")) }
}

// ErrorBag names the error bag of the form the visit sends, as a page with
// more than one form does: the page after it has the form's errors under
// that name. Following the visit's redirects sends it again, as the
// client does.
func ErrorBag(name string) Option {
	return func(v *visit) { v.header.Set("X-Inertia-Error-Bag", name) }
}

// Validate makes the visit a Precognition request, as a form sends to
// have its fields checked as they're filled in: the app answers with a
// 204 when they're valid, or a 422 with their errors, and changes
// nothing. With no fields named, it checks them all.
func Validate(fields ...string) Option {
	return func(v *visit) {
		// It isn't a visit: the app answers it with JSON, not a page.
		v.header.Del("X-Inertia")
		v.header.Del("X-Inertia-Version")
		v.header.Set("Accept", "application/json")
		v.header.Set("Precognition", "true")
		if len(fields) > 0 {
			v.header.Set("Precognition-Validate-Only", strings.Join(fields, ","))
		}
	}
}

// Header sets a header of the visit, over the one the client would send:
// Header("Referer", "") sends none.
func Header(name, value string) Option {
	return func(v *visit) { v.header.Set(name, value) }
}

// Get visits target, as following a link does. target is a path, such as
// "/posts?page=2", or a URL.
func (c *Client) Get(target string, opts ...Option) *Response {
	c.t.Helper()
	return c.Visit(http.MethodGet, target, nil, opts...)
}

// Post sends body to target, as a form does: see Visit.
func (c *Client) Post(target string, body any, opts ...Option) *Response {
	c.t.Helper()
	return c.Visit(http.MethodPost, target, body, opts...)
}

// Put sends body to target with a PUT, as a form does: see Visit.
func (c *Client) Put(target string, body any, opts ...Option) *Response {
	c.t.Helper()
	return c.Visit(http.MethodPut, target, body, opts...)
}

// Patch sends body to target with a PATCH, as a form does: see Visit.
func (c *Client) Patch(target string, body any, opts ...Option) *Response {
	c.t.Helper()
	return c.Visit(http.MethodPatch, target, body, opts...)
}

// Delete sends body to target with a DELETE, as a form does: see Visit.
// body can be nil.
func (c *Client) Delete(target string, body any, opts ...Option) *Response {
	c.t.Helper()
	return c.Visit(http.MethodDelete, target, body, opts...)
}

// Visit makes a visit as Inertia's client does: with X-Inertia, the
// version of the build the client runs, the cookies the app has set, and
// the page the client is on as the Referer. body, unless it's nil, goes as
// JSON: encoded with encoding/json, or as it is when it's a string or a
// []byte. Inertia's <Form> sends each value as a string, "on" for a ticked
// box and "42" from a number input, which a body can do too.
func (c *Client) Visit(method, target string, body any, opts ...Option) *Response {
	c.t.Helper()
	req, ok := c.newRequest(method, target, body)
	if !ok {
		return c.none()
	}
	req.header.Set("X-Inertia", "true")
	req.header.Set("X-Inertia-Version", c.Version)
	req.header.Set("Accept", "text/html, application/xhtml+xml")
	if c.page != "" {
		req.header.Set("Referer", c.referer())
	}
	v := &visit{header: req.header}
	for _, opt := range opts {
		opt(v)
	}
	if v.partial && req.header.Get("X-Inertia-Partial-Component") == "" {
		if c.page == "" {
			c.t.Fatalf("%s %s: a partial reload is of the page the client is on, and it isn't on one yet: visit it first", method, target)
			return c.none()
		}
		req.header.Set("X-Inertia-Partial-Component", c.page)
	}
	return c.send(req)
}

// Reload visits the page the client is on again, as the client's
// router.reload does: with Only or Except, a partial reload, such as the
// one that fetches a page's deferred props.
func (c *Client) Reload(opts ...Option) *Response {
	c.t.Helper()
	if c.page == "" {
		c.t.Fatalf("reloading: the client isn't on a page yet: visit one first")
		return c.none()
	}
	return c.Visit(http.MethodGet, c.referer(), nil, opts...)
}

// FirstVisit loads target whole, as a browser does an address typed in, a
// link from another site, or a reload: a GET without X-Inertia, which an
// Inertia page answers with HTML, the page object in it. The Response
// reads the page out of the HTML, and the client is on it after, running
// the build it names.
func (c *Client) FirstVisit(target string) *Response {
	c.t.Helper()
	req, ok := c.newRequest(http.MethodGet, target, nil)
	if !ok {
		return c.none()
	}
	req.header.Set("Accept", "text/html, application/xhtml+xml")
	return c.send(req)
}

// Do makes req as it is, its method, URL, headers and body, with the
// client's cookies: for a request that isn't a visit, such as a health
// check's, an upload's or an API client's. It keeps the cookies the
// response sets, as a visit does.
func (c *Client) Do(req *http.Request) *Response {
	c.t.Helper()
	var body []byte
	if req.Body != nil {
		var err error
		if body, err = io.ReadAll(req.Body); err != nil {
			c.t.Fatalf("%s %s: reading the request's body: %v", req.Method, req.URL, err)
			return c.none()
		}
	}
	host := cmp.Or(req.Host, req.URL.Host, "example.com")
	return c.send(&request{method: req.Method, path: req.URL.RequestURI(), host: host, header: req.Header.Clone(), body: body})
}

// Session changes the client's session as fn does, as a request to the app
// would: to log in as a user without the pages it takes, say, or as one who
// logged in so long ago that the app asks for their password again. store
// is the app's session.Store, or one made with the same session.Config.
// What the request before flashed is kept for the next visit, as if this
// one hadn't been made.
func (c *Client) Session(store *session.Store, fn func(*session.Session)) {
	c.t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c.addCookies(req)
	rec := httptest.NewRecorder()
	store.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := session.From(r.Context())
		s.Reflash()
		fn(s)
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)
	c.keep(rec.Result().Cookies())
}

// request is a request as the client makes it, kept with its response so
// that Follow can make it again.
type request struct {
	method string
	path   string // with the query
	host   string
	header http.Header // without the cookies, which are the client's when it's made
	body   []byte
}

// newRequest is a request to target, with body as JSON.
func (c *Client) newRequest(method, target string, body any) (*request, bool) {
	c.t.Helper()
	target, _, _ = strings.Cut(target, "#") // a browser keeps the fragment to itself
	u, err := url.ParseRequestURI(target)
	if err != nil || (u.Host == "" && !strings.HasPrefix(target, "/")) {
		c.t.Fatalf("%s %s: a target is a path, such as /posts, or a URL", method, target)
		return nil, false
	}
	req := &request{method: method, path: u.RequestURI(), host: cmp.Or(u.Host, "example.com"), header: http.Header{}}
	switch b := body.(type) {
	case nil:
	case string:
		req.body = []byte(b)
	case []byte:
		req.body = b
	default:
		if req.body, err = json.Marshal(body); err != nil {
			c.t.Fatalf("%s %s: the body isn't JSON: %v", method, target, err)
			return nil, false
		}
	}
	if req.body != nil {
		req.header.Set("Content-Type", "application/json")
	}
	return req, true
}

// send makes req. A client that hasn't been shown a page yet, and so knows
// no version, takes the app's from the 409 that asks it to reload, and
// makes the request again, as if it had the app open already.
func (c *Client) send(req *request) *Response {
	c.t.Helper()
	r := c.serve(req)
	if r.Code == http.StatusConflict && c.page == "" && c.Version == "" && req.header.Get("X-Inertia") == "true" {
		if v := r.Header.Get("X-Inertia-Version"); v != "" && r.Header.Get("X-Inertia-Location") != "" {
			c.Version = v
			req.header.Set("X-Inertia-Version", v)
			r = c.serve(req)
		}
	}
	return r
}

// serve makes req with the client's cookies, keeps the cookies the
// response sets, and notes the page it shows, which the client is on
// after.
func (c *Client) serve(req *request) *Response {
	c.t.Helper()
	var body io.Reader
	if req.body != nil {
		body = bytes.NewReader(req.body)
	}
	hr := httptest.NewRequest(req.method, req.path, body)
	hr.Host = req.host
	hr.Header = req.header.Clone()
	c.addCookies(hr)
	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, hr)
	c.keep(rec.Result().Cookies())

	r := &Response{Code: rec.Code, Header: rec.Header(), Body: rec.Body.String(), t: c.t, client: c, req: req}
	if r.readPage() && r.Page.Component != "" {
		c.page, c.url, c.host, c.Version = r.Page.Component, r.Page.URL, req.host, r.Page.Version
	}
	return r
}

func (c *Client) addCookies(req *http.Request) {
	for _, name := range slices.Sorted(maps.Keys(c.cookies)) {
		req.AddCookie(c.cookies[name])
	}
}

// keep keeps the cookies a response sets, as a browser does: one set to
// expire is dropped.
func (c *Client) keep(cookies []*http.Cookie) {
	for _, ck := range cookies {
		if ck.MaxAge < 0 || (!ck.Expires.IsZero() && !ck.Expires.After(time.Now())) {
			delete(c.cookies, ck.Name)
		} else {
			c.cookies[ck.Name] = ck
		}
	}
}

// referer is the address of the page the client is on, as a browser sends
// it.
func (c *Client) referer() string {
	return "http://" + c.host + c.url
}

// none is the response to a request the client couldn't make, having
// failed the test.
func (c *Client) none() *Response {
	return &Response{t: c.t, client: c}
}
