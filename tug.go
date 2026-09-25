// Package tug is a web framework for Go apps whose frontend is Inertia.js.
//
// This is the HTTP core the rest builds on. Routing is net/http's ServeMux,
// handlers return errors, and middleware has net/http's own shape,
// func(http.Handler) http.Handler, so anything written for net/http plugs in:
//
//	app := tug.New()
//	app.Use(middleware.RequestID(), middleware.Logger(), middleware.Recover())
//	app.Get("/posts/{id}", showPost).Name("posts.show")
//	if err := app.Run(); err != nil {
//		log.Fatal(err)
//	}
//
// A handler gets a *Ctx for its request and returns an error. The app's
// ErrorHandler turns that error into the response, so a handler says what
// went wrong, as NewHTTPError(http.StatusNotFound), and never writes an
// error page itself.
//
// The rest is in packages of their own, which don't import this one:
// inertia renders pages, session keeps sessions in a cookie, validate
// checks structs, vite puts a Vite build into pages, auth has the parts of
// accounts where a slip is a security hole, mail sends mail, and middleware
// has RequestID, Logger, Recover and CSRF. The guide is in the repository's
// docs directory: https://github.com/cuonggt/tug/tree/main/docs.
package tug

import "net/http"

// HandlerFunc handles a request. A returned error is answered by the app's
// ErrorHandler: return an *HTTPError to choose the status.
type HandlerFunc func(c *Ctx) error

// Middleware wraps a handler. It is net/http's own shape, so middleware
// written for net/http works as it is.
type Middleware = func(http.Handler) http.Handler

// WrapHandler turns an http.Handler into a HandlerFunc, to put something
// written for net/http, such as http.FileServer, on a route.
func WrapHandler(h http.Handler) HandlerFunc {
	return func(c *Ctx) error {
		h.ServeHTTP(c.Response(), c.Request())
		return nil
	}
}
