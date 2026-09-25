package tug

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/cuonggt/tug/inertia"
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
		if err := h(c); err != nil {
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

// Run serves on Config.Addr until SIGINT or SIGTERM, then shuts down
// gracefully: it stops taking connections and waits up to
// Config.ShutdownTimeout for the requests in flight to finish.
func (a *App) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ln, err := net.Listen("tcp", a.config.Addr)
	if err != nil {
		return err
	}
	return a.Serve(ctx, ln)
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

	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}

	slog.Info("shutting down", "timeout", a.config.ShutdownTimeout)
	sctx, cancel := context.WithTimeout(context.Background(), a.config.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		srv.Close()
		return fmt.Errorf("tug: requests still running after %v: %w", a.config.ShutdownTimeout, err)
	}
	if err := <-served; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
