package tug

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/cuonggt/tug/inertia"
	"github.com/cuonggt/tug/internal/typegen"
	"github.com/cuonggt/tug/session"
)

// Config is how an App behaves. Every field has a default, so the zero
// value works.
type Config struct {
	// Addr is where Run listens. Default ":8080".
	Addr string

	// Debug puts error details and panic stacks into 500 responses. It is
	// for development: in production they belong in the log only.
	Debug bool

	// ErrorHandler answers the errors handlers return, and the 404s and
	// 405s of requests no route takes. Default DefaultErrorHandler.
	ErrorHandler func(c *Ctx, err error)

	// BodyLimit is the largest request body Bind reads, in bytes; a larger
	// one is a 413. Default 32 MiB.
	BodyLimit int64

	// ShutdownTimeout is how long Run waits for requests in flight after
	// SIGINT or SIGTERM. Default 10 seconds.
	ShutdownTimeout time.Duration

	// Inertia renders the app's pages, for Ctx.Inertia and Page.Render.
	// With it set, every request also goes through its Middleware, inside
	// the App's own middleware.
	Inertia *inertia.Inertia

	// ErrorPage is the Inertia page component that errors are shown with,
	// such as "Error", with the props status and message: for a 404, a
	// 403, or a 500 in production. API clients still get JSON, and with
	// Debug on a 500 still shows its details. Without it, errors are plain
	// text, which Inertia's client shows in a dialog.
	ErrorPage string

	// Session keeps each visitor's session, for Ctx.Session. It's what
	// carries flash data and validation errors from a request to the page
	// after it. With it set, every request goes through its Middleware,
	// inside the App's own middleware.
	Session *session.Store
}

// ConfigFromEnv reads the settings a deployment sets: ADDR, or PORT as
// platforms such as Cloud Run and Fly.io set it, and APP_DEBUG.
func ConfigFromEnv() Config {
	var c Config
	if addr := os.Getenv("ADDR"); addr != "" {
		c.Addr = addr
	} else if port := os.Getenv("PORT"); port != "" {
		c.Addr = ":" + port
	}
	c.Debug, _ = strconv.ParseBool(os.Getenv("APP_DEBUG"))
	return c
}

// App is a web application: its routes, middleware and settings. It is an
// http.Handler, so an http.Server set up by hand can serve it too.
type App struct {
	*Router

	config Config
	mux    *http.ServeMux
	routes []*Route
	names  map[string]*Route

	// catchAll is set when a route takes every path under every method,
	// which leaves no request for the app to answer with a 404 or 405.
	catchAll bool

	start   sync.Once
	handler http.Handler
	serving atomic.Bool

	// background is what Go runs beside the server.
	background []func(ctx context.Context) error
}

// New returns an App. Without a Config it reads one from the environment
// (ConfigFromEnv); a Config passed in is used as it is.
func New(config ...Config) *App {
	var cfg Config
	switch len(config) {
	case 0:
		cfg = ConfigFromEnv()
	case 1:
		cfg = config[0]
	default:
		panic("tug: New takes one Config at most")
	}
	if cfg.Addr == "" {
		cfg.Addr = ":8080"
	}
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = DefaultErrorHandler
	}
	if cfg.BodyLimit == 0 {
		cfg.BodyLimit = 32 << 20
	}
	if cfg.ShutdownTimeout == 0 {
		cfg.ShutdownTimeout = 10 * time.Second
	}

	a := &App{config: cfg, mux: http.NewServeMux(), names: make(map[string]*Route)}
	a.Router = &Router{app: a}
	return a
}

// ServeHTTP answers a request. The first request fixes the routes and
// middleware: adding either after it panics.
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.start.Do(a.freeze)
	a.handler.ServeHTTP(w, r)
}

func (a *App) freeze() {
	a.serving.Store(true)
	for _, rt := range a.routes {
		rt.handler = rt.chain()
	}
	if !a.catchAll {
		// ServeMux answers a miss with its own plain-text 404 or 405. A
		// pattern that matches everything only wins when nothing else
		// does, and sends the misses to the ErrorHandler instead.
		a.mux.Handle("/", a.adapt(a.miss))
	}
	var h http.Handler = a.mux
	if a.config.Inertia != nil {
		h = a.config.Inertia.Middleware(h)
	}
	if a.config.Session != nil {
		if a.config.Inertia != nil {
			h = keepFlashOnReload(a.config.Inertia, h)
		}
		h = a.config.Session.Middleware(h)
	}
	a.handler = wrap(h, a.Router.mw)
}

func (a *App) mustNotServe() {
	if a.serving.Load() {
		panic("tug: routes and middleware are fixed once the app is serving; add them before the first request")
	}
}

// adapt turns a HandlerFunc into an http.Handler. It gives the handler a
// Ctx, and hands what it returns, or a panic, to the ErrorHandler.
func (a *App) adapt(h HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := &Ctx{app: a, r: r}
		c.rw.ResponseWriter = w
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v) // net/http's way of dropping a response on purpose
				}
				a.config.ErrorHandler(c, &PanicError{Value: v, Stack: debug.Stack()})
			}
		}()
		if err := h(c); err != nil && !errors.Is(err, errAnswered) {
			a.config.ErrorHandler(c, err)
		}
	})
}

// miss answers a request no route took: a redirect when only a trailing
// slash is in the way, a 405 when the path has routes under other methods,
// and a 404 otherwise.
func (a *App) miss(c *Ctx) error {
	r := c.Request()
	if to, ok := a.withoutTrailingSlash(r); ok {
		http.Redirect(c.Response(), r, to, http.StatusTemporaryRedirect)
		return nil
	}
	if allowed := a.allowedMethods(r); len(allowed) > 0 {
		c.Response().Header().Set("Allow", strings.Join(allowed, ", "))
		return NewHTTPError(http.StatusMethodNotAllowed)
	}
	return NewHTTPError(http.StatusNotFound)
}

// withoutTrailingSlash returns where r would be routed without its
// trailing slash, as "/posts/" is a link to a "/posts" route.
func (a *App) withoutTrailingSlash(r *http.Request) (string, bool) {
	if !strings.HasSuffix(r.URL.Path, "/") {
		return "", false
	}
	u := *r.URL
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = strings.TrimRight(u.RawPath, "/")
	if u.Path == "" {
		return "", false
	}
	probe := *r
	probe.URL = &u
	if !a.routed(&probe) {
		return "", false
	}
	return u.RequestURI(), true
}

// allowedMethods lists the methods a route takes r's path under, for a
// 405's Allow header.
func (a *App) allowedMethods(r *http.Request) []string {
	var allowed []string
	for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions} {
		probe := *r
		probe.Method = m
		if m == r.Method || !a.routed(&probe) {
			continue
		}
		allowed = append(allowed, m)
		if m == http.MethodGet {
			allowed = append(allowed, http.MethodHead) // ServeMux answers HEAD with the GET route
		}
	}
	return allowed
}

// routed reports whether a route, rather than the app's miss handler,
// would take r.
func (a *App) routed(r *http.Request) bool {
	_, pattern := a.mux.Handler(r)
	return pattern != "" && pattern != "/"
}

// Go runs fn beside the server, for work that isn't a request, such as a
// job queue's workers: Run and Serve start it as they start serving, with
// a context that's done as the app shuts down, and wait for it to return.
// An error it returns before then shuts the app down, and Serve returns
// it: an app whose jobs have stopped shouldn't carry on as though they
// hadn't. ServeHTTP, as tests call it, starts nothing, and nor does tug
// gen.
//
// Go panics once the app is serving.
func (a *App) Go(fn func(ctx context.Context) error) {
	if a.serving.Load() {
		panic("tug: Go is for before the app is serving: what it runs starts with the server")
	}
	a.background = append(a.background, fn)
}

// Run serves on Config.Addr until SIGINT or SIGTERM, then shuts down
// gracefully: it stops taking connections, waits up to
// Config.ShutdownTimeout for the requests in flight to finish, and waits
// for what Go runs to return.
//
// Run is also where tug gen learns about the app: started by it, with
// TUG_GEN set to a file, Run writes the TypeScript for the app's pages and
// named routes there, and returns without serving.
func (a *App) Run() error {
	if path := os.Getenv("TUG_GEN"); path != "" {
		return a.gen(path)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ln, err := net.Listen("tcp", a.config.Addr)
	if err != nil {
		return err
	}
	return a.Serve(ctx, ln)
}

// gen writes tug gen's TypeScript for the app to the file at path: that of
// the pages Page declared, the props shared with inertia.Share, and the
// named routes.
func (a *App) gen(path string) error {
	in := typegen.Input{Pages: declaredPages()}
	if a.config.ErrorPage != "" {
		in.Pages = append(in.Pages, typegen.Page{Component: a.config.ErrorPage, Props: reflect.TypeFor[ErrorPageProps]()})
	}
	if a.config.Inertia != nil {
		in.Shared = a.config.Inertia.Shared()
	}
	for name, rt := range a.names {
		in.Routes = append(in.Routes, typegen.Route{Name: name, Method: rt.method, Path: rt.path})
	}
	data, err := json.Marshal(typegen.Generate(in))
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Serve serves on ln until ctx is done, then shuts down as Run does. It
// closes ln.
func (a *App) Serve(ctx context.Context, ln net.Listener) error {
	a.start.Do(a.freeze)
	srv := &http.Server{
		Handler: a,
		// Without it, a client that sends its headers a byte at a time
		// holds a connection for as long as it likes.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(slog.Default().Handler(), slog.LevelWarn),
	}

	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	slog.Info("listening", "addr", ln.Addr().String())

	// What Go runs is told to stop as the server shuts down, and stops
	// the server when it fails first.
	work, stopWork := context.WithCancel(ctx)
	defer stopWork()
	errs := make([]error, len(a.background))
	failed := make(chan struct{}, len(a.background))
	var working sync.WaitGroup
	for i, fn := range a.background {
		working.Go(func() {
			if errs[i] = fn(work); errs[i] != nil {
				failed <- struct{}{}
			}
		})
	}

	select {
	case err := <-served:
		stopWork()
		working.Wait()
		return errors.Join(err, stopped(errs))
	case <-ctx.Done():
	case <-failed:
	}

	stopWork()
	slog.Info("shutting down", "timeout", a.config.ShutdownTimeout)
	sctx, cancel := context.WithTimeout(context.Background(), a.config.ShutdownTimeout)
	defer cancel()
	var err error
	if serr := srv.Shutdown(sctx); serr != nil {
		srv.Close()
		err = fmt.Errorf("tug: requests still running after %v: %w", a.config.ShutdownTimeout, serr)
	} else if serr := <-served; !errors.Is(serr, http.ErrServerClosed) {
		err = serr
	}
	working.Wait()
	return errors.Join(err, stopped(errs))
}

// stopped is what the work Go runs returned, but for its being canceled,
// which is how it was told to stop.
func stopped(errs []error) error {
	var failed []error
	for _, err := range errs {
		if err != nil && !errors.Is(err, context.Canceled) {
			failed = append(failed, err)
		}
	}
	return errors.Join(failed...)
}
