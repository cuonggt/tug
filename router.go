package tug

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"runtime"
	"slices"
	"strings"

	"github.com/cuonggt/tug/internal/devtools"
	"github.com/cuonggt/tug/internal/route"
)

// Router adds routes. The App is the root Router, and Group makes one for
// a path prefix, with middleware of its own.
//
// A path matches exactly: "/posts" is /posts and nothing under it, and "/"
// is only the home page. End a path with a {name...} wildcard to take
// everything under it. Beyond that, paths are net/http ServeMux patterns,
// with its wildcards and its rule that the more specific pattern wins.
type Router struct {
	app    *App
	parent *Router
	prefix string
	mw     []Middleware
}

// Use adds middleware, which runs in the order added. On the App it wraps
// every request, including those no route takes, which suits logging and
// recovery. On a group it wraps the group's routes, inside the middleware
// of the groups around it, including routes added before the Use.
func (r *Router) Use(mw ...Middleware) {
	r.app.mustNotServe()
	r.mw = append(r.mw, mw...)
}

// Group returns a Router for routes under prefix, such as "/admin", that
// run mw after the middleware of r. An empty prefix groups routes for the
// middleware alone.
func (r *Router) Group(prefix string, mw ...Middleware) *Router {
	r.app.mustNotServe()
	if prefix != "" && !strings.HasPrefix(prefix, "/") {
		panic(fmt.Sprintf("tug: group prefix %q must start with /", prefix))
	}
	return &Router{
		app:    r.app,
		parent: r,
		prefix: r.prefix + strings.TrimRight(prefix, "/"),
		mw:     slices.Clone(mw),
	}
}

// Get adds a route for GET requests to path, which also takes HEAD.
func (r *Router) Get(path string, h HandlerFunc, mw ...Middleware) *Route {
	return r.Handle(http.MethodGet, path, h, mw...)
}

// Post adds a route for POST requests to path.
func (r *Router) Post(path string, h HandlerFunc, mw ...Middleware) *Route {
	return r.Handle(http.MethodPost, path, h, mw...)
}

// Put adds a route for PUT requests to path.
func (r *Router) Put(path string, h HandlerFunc, mw ...Middleware) *Route {
	return r.Handle(http.MethodPut, path, h, mw...)
}

// Patch adds a route for PATCH requests to path.
func (r *Router) Patch(path string, h HandlerFunc, mw ...Middleware) *Route {
	return r.Handle(http.MethodPatch, path, h, mw...)
}

// Delete adds a route for DELETE requests to path.
func (r *Router) Delete(path string, h HandlerFunc, mw ...Middleware) *Route {
	return r.Handle(http.MethodDelete, path, h, mw...)
}

// Options adds a route for OPTIONS requests to path.
func (r *Router) Options(path string, h HandlerFunc, mw ...Middleware) *Route {
	return r.Handle(http.MethodOptions, path, h, mw...)
}

// Any adds a route for requests to path under any method.
func (r *Router) Any(path string, h HandlerFunc, mw ...Middleware) *Route {
	return r.Handle("", path, h, mw...)
}

// Handle adds a route for method requests to path; an empty method takes
// them all. mw wraps this route alone, inside its groups' middleware.
//
// Handle panics when path is not a valid pattern, or takes the same
// requests as a route already added, as ServeMux does: both are mistakes
// to fix before the app can start.
func (r *Router) Handle(method, path string, h HandlerFunc, mw ...Middleware) *Route {
	a := r.app
	a.mustNotServe()
	if !strings.HasPrefix(path, "/") {
		panic(fmt.Sprintf("tug: route path %q must start with /", path))
	}

	full := r.prefix + path
	if path == "/" && r.prefix != "" {
		full = r.prefix // a group's own page is "/admin", not "/admin/"
	}
	if strings.HasSuffix(full, "/") {
		full += "{$}" // alone, a trailing slash would make ServeMux take everything under it
	}

	rt := &Route{app: a, group: r, method: method, path: full, h: h, mw: slices.Clone(mw)}
	rt.shown = rt.String()
	if a.config.DevTools {
		if f, ok := devtools.Caller(); ok {
			at := devtools.At(f)
			rt.added = &at
		}
	}
	pattern := full
	if method != "" {
		pattern = method + " " + full
	}
	a.mux.Handle(pattern, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Answered as it's matched, before its middleware, which may
		// answer for it.
		route.Answered(req.Context(), &rt.shown)
		if a.devtools != nil {
			if rec := devtools.From(req.Context()); rec != nil {
				rec.Answered(rt.devtools())
			}
		}
		rt.handler.ServeHTTP(w, req)
	}))
	a.routes = append(a.routes, rt)
	if method == "" && isCatchAll(full) {
		a.catchAll = true
	}
	return rt
}

// isCatchAll reports whether path is "/{name...}", which takes every path.
func isCatchAll(path string) bool {
	return strings.HasPrefix(path, "/{") && strings.HasSuffix(path, "...}") && strings.Count(path, "/") == 1
}

// Route is a route added to a Router. Name it to build its URL.
type Route struct {
	app    *App
	group  *Router
	method string
	path   string
	name   string
	h      HandlerFunc
	mw     []Middleware

	// handler is h inside its middleware. It is put together when the app
	// starts serving, so middleware added to a group after its routes
	// still wraps them.
	handler http.Handler

	// shown is the route as String says it, which RouteOf returns for a
	// request it answered.
	shown string

	// added is where the app added the route, for Inertia's DevTools, under
	// Config.DevTools.
	added *devtools.Source

	// input is the struct the handler binds, as Takes declares it.
	input reflect.Type
}

// devtools is the route as Inertia's DevTools show it: its path as it was
// added, its name, and its handler, by the name Go gives the function, a
// method's without the -fm of its value, with where the app added it,
// which the function of the app's that answered, when one did, replaces.
func (rt *Route) devtools() devtools.Route {
	r := devtools.Route{URI: strings.TrimSuffix(rt.path, "{$}"), ActionSource: rt.added}
	if rt.name != "" {
		r.Name = &rt.name
	}
	if fn := runtime.FuncForPC(reflect.ValueOf(rt.h).Pointer()); fn != nil {
		name := strings.TrimSuffix(fn.Name(), "-fm")
		r.Action = &name
	}
	return r
}

// Takes declares the struct the route's handler binds, by a value of it,
// LoginInput{}, which a wrapper around the handler, as one that checks
// someone has logged in, keeps tug from seeing. tug gen writes it as what
// the route is sent, Inputs['login.store'] in routes.ts, which a form typed
// by it is checked against, as React's <Form<Inputs['login.store']>> is:
// its fields' keys, and its errors'.
//
// The route needs its name first, as Inputs is by name: Takes panics on a
// route with none, and on a value that isn't a struct or a pointer to one.
// Bind fails, as a 500 whose error names both, a request of the route
// whose handler binds a struct of body fields other than input: Takes has
// drifted from the handler, and would type its forms against a struct the
// handler no longer reads.
func (rt *Route) Takes(input any) *Route {
	rt.app.mustNotServe()
	if rt.name == "" {
		panic(fmt.Sprintf("tug: %s takes an input, and needs a name first, as tug gen types inputs by name: Name(...).Takes(...)", rt))
	}
	t := reflect.TypeOf(input)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		panic(fmt.Sprintf("tug: %s takes %T, which isn't a struct", rt, input))
	}
	rt.input = t
	return rt
}

// Name names the route, for App.URL and Ctx.RedirectRoute. A name belongs
// to one route: naming a second route "posts.show" panics.
func (rt *Route) Name(name string) *Route {
	a := rt.app
	a.mustNotServe()
	if other, ok := a.names[name]; ok && other != rt {
		panic(fmt.Sprintf("tug: route name %q is already %s", name, other))
	}
	delete(a.names, rt.name)
	rt.name = name
	a.names[name] = rt
	return rt
}

// String describes the route by its method and path as it was added, "GET
// /posts/{id}", or "ANY /files/{path...}" for a route of any method.
func (rt *Route) String() string {
	path := strings.TrimSuffix(rt.path, "{$}") // tug's own, which keeps a path ending in a slash exact
	if rt.method == "" {
		return "ANY " + path
	}
	return rt.method + " " + path
}

// RouteOf returns the route that answered r, as its String says it, "GET
// /posts/{id}": for the App's own middleware, around the router, to read
// once the handler has run, as the label of the request's metrics, which
// has as many values as the app has routes, where its path has as many as
// the app has posts. The route answers as it's matched, so its own
// middleware, and its group's, answer as it. A request no route answered,
// a 404, a 405, or a redirect to its path without its trailing slash, has
// "", as does one no router has seen.
func RouteOf(r *http.Request) string {
	if shown, placed := route.Of(r.Context()); placed {
		return shown
	}
	// An App with no middleware of its own places no route, as nothing
	// outside the router needs it, and inside it ServeMux has its pattern.
	return shownPattern(r.Pattern)
}

// shownPattern is a route's ServeMux pattern as the route's String says
// it: "" for none, and for "/", which is tug's own, the misses'.
func shownPattern(pattern string) string {
	if pattern == "" || pattern == "/" {
		return ""
	}
	pattern = strings.TrimSuffix(pattern, "{$}")
	if strings.HasPrefix(pattern, "/") {
		return "ANY " + pattern // a route of any method has none in its pattern
	}
	return pattern
}

// chain wraps the route's handler in its own middleware, then in each
// enclosing group's, so the outermost group's runs first. The App's own
// middleware isn't here: it wraps the whole ServeMux.
func (rt *Route) chain() http.Handler {
	h := wrap(rt.app.adapt(rt, rt.h), rt.mw)
	for g := rt.group; g.parent != nil; g = g.parent {
		h = wrap(h, g.mw)
	}
	return h
}

// wrap puts h inside mw, with mw[0] outermost.
func wrap(h http.Handler, mw []Middleware) http.Handler {
	for _, m := range slices.Backward(mw) {
		h = m(h)
	}
	return h
}

// URL builds the path of the route named name, filling its wildcards with
// params in order: URL("posts.show", 42) is "/posts/42". Values are
// escaped, except for the slashes in a {name...} wildcard's value.
func (a *App) URL(name string, params ...any) (string, error) {
	rt, ok := a.names[name]
	if !ok {
		return "", fmt.Errorf("tug: no route is named %q", name)
	}
	var b strings.Builder
	used := 0
	for seg := range strings.SplitSeq(rt.path[1:], "/") {
		b.WriteByte('/')
		switch {
		case seg == "{$}":
			// The end of a path that ends in a slash.
		case strings.HasPrefix(seg, "{"):
			if used == len(params) {
				return "", fmt.Errorf("tug: route %q needs a value for %s", name, seg)
			}
			v := fmt.Sprint(params[used])
			used++
			switch {
			case strings.HasSuffix(seg, "...}"):
				parts := strings.Split(v, "/")
				for i, p := range parts {
					parts[i] = url.PathEscape(p)
				}
				b.WriteString(strings.Join(parts, "/"))
			case v == "":
				return "", fmt.Errorf("tug: route %q got an empty value for %s", name, seg)
			default:
				b.WriteString(url.PathEscape(v))
			}
		default:
			b.WriteString(seg)
		}
	}
	if used < len(params) {
		return "", fmt.Errorf("tug: route %q takes %d values, not %d", name, used, len(params))
	}
	return b.String(), nil
}
