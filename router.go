package tug

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
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
	pattern := full
	if method != "" {
		pattern = method + " " + full
	}
	a.mux.Handle(pattern, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
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

// String describes the route by its method and path, "GET /posts/{id}".
func (rt *Route) String() string {
	if rt.method == "" {
		return "ANY " + rt.path
	}
	return rt.method + " " + rt.path
}

// chain wraps the route's handler in its own middleware, then in each
// enclosing group's, so the outermost group's runs first. The App's own
// middleware isn't here: it wraps the whole ServeMux.
func (rt *Route) chain() http.Handler {
	h := wrap(rt.app.adapt(rt.h), rt.mw)
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
