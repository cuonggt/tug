# Routing

An app is a `tug.App`: its settings, its routes, and the middleware around
them. Routing is net/http's `ServeMux`, middleware has net/http's own shape,
and a handler takes a `*tug.Ctx` and returns an error:

```go
func main() {
	app := tug.New() // ADDR or PORT, APP_DEBUG and APP_URL, from the environment
	app.Use(middleware.RequestID(), middleware.Logger(), middleware.Recover(), middleware.CSRF())

	app.Get("/posts/{id}", showPost).Name("posts.show")

	if err := app.Run(); err != nil { // until SIGINT or SIGTERM
		log.Fatal(err)
	}
}
```

An app made by `tug new` has the same shape, with pages: see
[getting-started.md](getting-started.md) and [pages.md](pages.md).
[`examples/api`](../examples/api/main.go) is a whole JSON API.

## The app

`tug.New()` returns an `App`, with its `Config` read from the environment
by `tug.ConfigFromEnv`. A `Config` passed in is used as it is, so start from
`ConfigFromEnv` to keep what the environment says:

```go
cfg := tug.ConfigFromEnv()
cfg.ShutdownTimeout = 30 * time.Second
app := tug.New(cfg)
```

Every field has a default, so the zero `Config` works:

| Field             | Default               | What it is |
|-------------------|-----------------------|------------|
| `Addr`            | `":8080"`             | Where `Run` listens. |
| `Debug`           | `false`               | Puts a server error's details, and a panic's stack, into the response. It's for development: in production they belong in the log only. |
| `ErrorHandler`    | `DefaultErrorHandler` | Answers the errors handlers return, and the 404s and 405s of requests no route takes. See [Errors](#errors). |
| `BodyLimit`       | 32 MiB                | The largest request body `Bind` reads; a larger one is a 413. |
| `ShutdownTimeout` | 10 seconds            | How long `Run` waits for the requests in flight after SIGINT or SIGTERM. |
| `Inertia`         | none                  | Renders the app's pages. See [pages.md](pages.md). |
| `ErrorPage`       | none                  | The Inertia page that errors are shown with. See [pages.md](pages.md). |
| `Session`         | none                  | Keeps each visitor's session, which carries flash data and validation errors to the next page. See [forms.md](forms.md). |
| `URL`             | none                  | The app's own address, as `https://example.com`, which the links that leave the app begin with. See [Whole links, and signed ones](#whole-links-and-signed-ones). |
| `Keys`            | none                  | The app's keys, which sign the links `SignedURL` makes. |
| `Lang`            | none: English         | The app's languages, which what tug says, and `c.T`, are said in. See [Languages](languages.md). |
| `Locale`          | none                  | The language the app has chosen for a request, as a user's, before the browser's `Accept-Language`. See [Languages](languages.md#the-requests-language). |
| `DevTools`        | `false`               | Keeps an entry of each request for Inertia's DevTools, a panel of the browser's. It's for development, as `tug dev` runs the app. See [pages.md](pages.md#devtools). |

`ConfigFromEnv` reads `ADDR`, the address to listen on, such as
`127.0.0.1:8080`; or else `PORT`, as platforms such as Cloud Run and Fly.io
set it, so `PORT=3000` is `:3000`; `APP_DEBUG`, where `true` or `1` turns
on `Debug`; `APP_URL`, the app's address; and `TUG_DEV`, which `tug dev`
sets, and which turns on `DevTools`. The default address,
`:8080`, listens on every interface. `tug.New` panics on a `URL` that isn't
a scheme and a host, as a mistyped `APP_URL` should stop the app as it
starts. [deployment.md](deployment.md) has more on the environment in
production.

### `Run` and `Serve`

`app.Run()` listens on `Addr`, returning the error when it can't, and serves
until the process gets SIGINT or SIGTERM. Then it shuts down gracefully: it
stops taking connections and waits up to `ShutdownTimeout` for the requests
in flight. When they finish in time it returns nil; otherwise it closes
their connections and returns an error that says so. `app.Serve(ctx, ln)`
does the same on a listener of your own, until `ctx` is done, and closes
the listener. The server either one starts has a `ReadHeaderTimeout` of 10
seconds, so a client that sends its headers a byte at a time can't hold a
connection for as long as it likes.

Work that isn't a request, such as a job queue's workers, runs beside the
server with `app.Go`:

```go
app.Go(q.Run) // a job queue's: func(ctx context.Context) error
```

`Run` and `Serve` start it as they start serving, with a context that's
done as the app shuts down, and wait for it to return before they do. An
error it returns before then shuts the app down, and `Run` returns it, as
an app whose jobs have stopped shouldn't carry on as though they hadn't.
`ServeHTTP` starts nothing, nor does `tug gen`, and `Go` panics once the
app is serving.
[jobs.md](jobs.md) has the job queue.

`Run` is also how `tug gen` learns the app: it runs the app with `TUG_GEN`
set, and `Run` writes the app's TypeScript instead of serving. So add every
route before calling `Run`; whatever `main` does before it, it does for
`tug gen` too. See [typescript.md](typescript.md).

An `App` is an `http.Handler`, so an `http.Server` set up by hand can serve
it, without `Run`'s signals and shutdown. A test needs no server at all: it
calls `app.ServeHTTP` with an `httptest.NewRecorder()`, as the tests of
`examples/api` do.

### Commands

```go
app.Command("users:admin", "make a user an admin: users:admin <email>", func(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errors.New("users:admin takes the user's email")
	}
	return users.MakeAdmin(ctx, args[0])
})
```

An app's binary does more than serve: a fix to its data, an import, a
user made an admin, and the auth starter's `jobs`, which lists the jobs
that failed. `app.Command(name, summary, run)` adds one, and `Run`, given
its name as the binary's first argument, runs it in place of serving,
with the arguments after the name and a context canceled on SIGINT or
SIGTERM, and returns its error, which `main` stops with:

```
$ ./blog users:admin ann@example.com
$ ./blog help
Without a command, ./blog serves. Its commands:

  ./blog users:admin  make a user an admin: users:admin <email>
  ./blog help         list these commands
```

A command runs in the app `main` made, with its routes, for the links it
makes, its queue, for the jobs it pushes, and its database, and needs the
environment the server does. Neither the server nor what `Go` runs
starts: a job a command pushes runs on the instances that serve. Its
arguments are its own, to read with package `flag` or by hand; tug parses
none. A name that isn't a command is an error that says to ask `help`.

The tug CLI can't run an app's commands, as a deployed app is its binary,
where tug isn't: in a container, `docker exec <container> /server
users:admin ann@example.com`. An app without commands serves whatever its
arguments, and under `tug gen`, `Run` writes the types, whatever they
are. A command's name is a word of letters, digits and `:-_`, and
`Command` panics on one that's taken, on `help`, and once the app is
serving.

## Routes

```go
app.Get("/posts", index)
app.Post("/posts", store)
app.Get("/posts/{id}", show)
app.Put("/posts/{id}", update)
app.Delete("/posts/{id}", destroy)
```

`Get`, `Post`, `Put`, `Patch`, `Delete` and `Options` add a route for their
method, and `Any` one for every method. `Handle(method, path, h)` takes the
method as a string, where `""` is every method. A `Get` route answers HEAD
requests too.

Paths are `ServeMux` patterns. `{id}` is one segment: `/posts/{id}` takes
`/posts/42`, and `c.Param("id")` is `"42"`. `{path...}`, at the end, is the
rest of the path: `/files/{path...}` takes `/files/docs/a.txt`, and
`c.Param("path")` is `"docs/a.txt"`. When two routes take a request, the
more specific one wins: `/posts/latest` over `/posts/{id}`.

Paths match exactly. `/posts` is `/posts` and nothing under it, and `/` is
only the home page. On its own, a pattern that ends in a slash would make
`ServeMux` take every path under it, so tug adds `{$}` to one: `/tags/`
becomes `/tags/{$}`, which is `/tags/` alone. To take everything under a
path, end it with a `{name...}` wildcard. `app.Any("/{path...}", h)` takes
every request no other route does, and `app.Get("/{path...}", h)` every GET,
which leaves the other methods a 405.

A request no route takes is answered in one of three ways:

- When its path ends in a slash, and a route would take the request without
  it, it's redirected there with a 307, query and all, as `/posts/?page=2`
  is most likely a link to `/posts?page=2`. For a route whose path ends in
  a slash, `ServeMux` does the reverse: `/tags` gets a 307 to `/tags/`.
- When routes take its path under other methods, it's a 405, with an
  `Allow` header that lists them, such as `GET, HEAD, POST`.
- Otherwise, it's a 404.

The 405 and the 404 go to the `ErrorHandler` as `*tug.HTTPError`s, so they
look like the app's other errors: JSON for an API client, and the error page
for a browser, when the app has one.

Mistakes panic at the call that makes them, so they stop the app as it
starts rather than showing up later: a route path or group prefix that
doesn't start with `/`, a pattern `ServeMux` can't parse, such as
`/posts/{id`, and a route that clashes with one added before it. Routes
clash when they take the same requests, as `/posts/{slug}` and
`/posts/{id}` do, or when both take some request and neither is more
specific: `Any("/posts/latest", h)` clashes with `Get("/posts/{id}", h)`,
since one has the more specific path and the other the more specific
method.

The first request fixes the routes and middleware, as does `Serve` when it
starts: adding a route, a group, middleware or a route name after that
panics. It's when tug puts each route's middleware together, which is why a
group's `Use` after its routes still wraps them.

## Named routes

```go
app.Get("/posts/{id}", show).Name("posts.show")
app.Get("/files/{path...}", file).Name("files")

path, err := app.URL("posts.show", 42)           // "/posts/42"
path, err = app.URL("files", "docs/read me.txt") // "/files/docs/read%20me.txt"
```

`Name` names a route, so its path can be built rather than written out. A
name belongs to one route: naming a second route the same panics.

`App.URL(name, params...)` fills the route's wildcards with `params`, in
order, each written with `fmt.Sprint` and escaped; a `{name...}` wildcard
keeps the slashes in its value. It returns an error for a name no route has,
for too few or too many values, and for an empty value for a `{name}`
wildcard. In a handler, `c.URL` builds a path the same way, as `examples/api`
does for a `Location` header, and `c.RedirectRoute` redirects to one: a
handler for a form usually ends with
`return c.RedirectRoute("posts.show", post.ID)`.

`tug gen` writes the named routes to TypeScript too, with a `route()` that
builds the same paths in the frontend: `route('posts.show', { id: 42 })`.
See [typescript.md](typescript.md).

### Whole links, and signed ones

```go
cfg := tug.ConfigFromEnv() // APP_URL, the app's address, as https://example.com
cfg.Keys = keys            // session.KeysFromEnv's, from APP_KEY
app := tug.New(cfg)

app.Get("/invitations/{id}", tug.Signed(acceptInvitation)).Name("invitations.accept")

link, err := app.AbsoluteURL("posts.show", 42) // "https://example.com/posts/42"
link, err = app.SignedURL("invitations.accept", time.Now().Add(72*time.Hour), invite.ID)
// "https://example.com/invitations/7?expires=1790000000&signature=..."
```

A path is enough for a link within the app, and it's what redirects and
pages use: the scheme and host a proxy answers on aren't the app's to
know. A link that leaves the app, in mail, a QR code, or another service,
is whole: `AbsoluteURL(name, params...)` is the named route's path after
`Config.URL`, the app's own address, which `ConfigFromEnv` reads from
`APP_URL`. It's never taken from the request's `Host`, which can name any
site, so a link made from it could send its reader anywhere. Without
`Config.URL`, it's an error that says to set `APP_URL`. `tug dev` sets it
to the address it shows the app at, unless `.env` names another.

`SignedURL(name, expires, params...)` is a whole link that only the app
could have made, until it expires: its query has `expires`, a Unix time,
and `signature`, over the link's path and its expiry, with a key derived
from the first of `Config.Keys` for signed links alone. `tug.Signed(h)`
wraps a handler that only such a link reaches: any other request is a
403 for the `ErrorHandler`, "this link has expired" for one the app made
that's past its time, and "this link isn't valid" for the rest, a link
changed anywhere in its path, its expiry or its signature, or with
anything else in its query, which `Bind` would give the handler. It's a
wrapper, as `tug.Limit` is, so an Inertia visit gets the error page.

A link is checked with each of the keys, so one made with a key moved to
`APP_PREVIOUS_KEYS` works until the key is dropped. Every link expires,
and one that has to work for years, as a link to unsubscribe, is given
years. It works until then, as often as it's followed: what should happen
once, as an invitation accepted, is the app's to record. `c.AbsoluteURL`
and `c.SignedURL` make the same links in a handler; a job, with no
request, makes them with the `*tug.App`, as the auth starter's mail does.

## Groups and middleware

```go
app.Use(middleware.RequestID(), middleware.Logger(), middleware.Recover(), middleware.CSRF())

admin := app.Group("/admin", requireAdmin)
admin.Get("/", dashboard).Name("admin")        // /admin
admin.Get("/users", users).Name("admin.users") // /admin/users
admin.Post("/users/{id}/ban", ban, audit)      // requireAdmin, then audit

account := app.Group("", requireLogin) // no prefix: the middleware alone
account.Get("/settings", settings)
```

`Group(prefix, mw...)` returns a `*tug.Router` for routes under the prefix,
which starts with `/`. A group's own page, its `/`, is the prefix itself:
`/admin`, not `/admin/`. Groups nest, and their prefixes and middleware add
up.

Middleware is net/http's own shape, `func(http.Handler) http.Handler`, which
tug names `tug.Middleware`, so middleware written for net/http works as it
is. On the App, with `app.Use`, it wraps every request, including those no
route takes, so it sees the redirects, 405s and 404s too, which suits
logging and recovery. On a group, as arguments to `Group` or with the
group's `Use`, it wraps the group's routes, including those added before
the `Use`, but not a request under the prefix that no route takes. On a
route, as the last arguments to `Get`, `Post` and the rest, it wraps that
route alone.

A request goes through them from the outside in, each in the order it was
added:

1. The App's middleware.
2. The session middleware, when `Config.Session` is set.
3. The Inertia middleware, when `Config.Inertia` is set.
4. The middleware of the route's groups, the outermost group's first.
5. The route's own middleware, then the handler.

The App's middleware is outside the rest, so it sees every request, and
every response as it finally goes out: `Logger` logs the status the client
gets. It also runs before there's a session: `session.From` finds nothing
there. The session middleware is outside Inertia's so that flash data lasts
through the reload Inertia's middleware asks for when a browser has an old
build of the frontend. Group and route middleware run inside both, so they
can read the session, as a check that someone is logged in does, and their
redirects are fixed for Inertia's client as a handler's are: a 302 after a
PUT becomes a 303. So middleware that needs the session goes on a group,
even one with an empty prefix, rather than on the App.

`tug.WrapHandler` puts an `http.Handler` on a route, such as a file server.
It answers for itself: its 404s are its own, not the `ErrorHandler`'s.

```go
app.Get("/static/{path...}", tug.WrapHandler(http.StripPrefix("/static", http.FileServerFS(static))))
```

### The route that answered

```go
app.Use(func(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		route := tug.RouteOf(r) // "GET /posts/{id}", once the handler has run
		// ...
	})
})
```

`tug.RouteOf(r)` is the route that answered `r`, as its `String` says it:
its method and path as they were added, `GET /posts/{id}`, or
`ANY /files/{path...}` for a route of any method. The App's middleware,
outside the router, reads it once the handler has run, as the label of a
request's metrics, which has as many values as the app has routes, where
its path has as many as the app has posts ([Metrics](deployment.md#metrics));
`Logger` logs it. A route answers as it's matched, so a redirect its group's
middleware sends, to log in, say, is the route's. A request no route
answered, a 404, a 405, or a redirect to its path without its trailing
slash, has `""`.

An App with middleware of its own puts a place for the route in each
request's context as the request comes in, which the router fills in,
whatever copies of the request the middleware in between makes. Inside the
router, a handler's `RouteOf(c.Request())` reads it, or, in an App with no
middleware, which makes no place, the pattern `ServeMux` keeps on the
request.

### Package `middleware`

Package `middleware` has eight, each a plain
`func(http.Handler) http.Handler` that works with any router. The order in
`app.Use` above is the usual one: `RequestID` first, so every log line
carries the ID, and `Logger` before `Recover`, so a panic is logged as the
500 that `Recover` makes of it. Behind a proxy, `TrustProxies` goes before
them all. `Headers` and `CSP` go after `Recover`, and `CORS`, for an API
another site's pages call, before `CSRF`, as the starters have them:

```go
app.Use(
	middleware.TrustProxies(strings.Split(os.Getenv("TRUSTED_PROXIES"), ",")...),
	middleware.RequestID(), middleware.Logger(), middleware.Recover(),
	middleware.Headers(middleware.HeadersConfig{HSTS: strings.HasPrefix(cfg.URL, "https://")}),
	middleware.CSP(middleware.CSPConfig{ReportPath: "/csp-reports", DevServer: assets.DevServer}),
	middleware.CSRF(),
)
```

- `TrustProxies(proxies...)` gives a request that came through the app's
  proxies, such as a load balancer, the address of the client it came
  from, in `RemoteAddr`, where it had the proxy's. See
  [The client's address](#the-clients-address).
- `RequestID()` gives each request an ID: the `X-Request-ID` it came with,
  as a proxy in front may set one, when that's short and plain (up to 64
  letters, digits and `-_.:+/=`), or else a new random one. The ID goes back
  in the response's `X-Request-ID`, and into the request's context, where
  `middleware.RequestIDFrom(ctx)` finds it.
- `Logger()` logs a line for each request through `slog.Default()`: its
  method, path, status, size and duration, its `route`, when a route
  answered it (`tug.RouteOf`, above), and its `request_id` when
  `RequestID` ran before. A 5xx logs at Error, a 4xx at Warn, and the rest
  at Info. The path is logged without its query string, which can carry
  tokens.
- `Recover()` answers a panic with a 500 and logs it with its stack. tug
  already recovers a handler's panics and hands them to the `ErrorHandler`;
  `Recover` is for the rest, such as a panic in middleware, which runs
  outside the handler. A response that has started is left as it is.
- `CSRF(trustedOrigins...)` rejects a request a browser sent from another
  origin, with a 403 in plain text, unless it's a GET, HEAD or OPTIONS,
  which must never change anything. It's net/http's
  `CrossOriginProtection`, which reads the `Sec-Fetch-Site` header browsers
  send, or else compares `Origin` with `Host`, so there are no tokens to put
  in forms. A request with neither header, such as one from curl, passes,
  and so does one from a trusted origin, such as
  `"https://admin.example.com"`. `CSRF` panics on a trusted origin that
  isn't one, such as `admin.example.com` without its scheme, so that a
  mistyped setting stops the app as it starts. An entry that's a path,
  such as `"/api/"`, is a pattern of `ServeMux`'s whose requests pass
  unchecked: routes that take a token, never the session's cookie, which
  another site's page can't borrow, as the auth starter's API's do. See
  [forms.md](forms.md).
- `CORS(origins...)` lets the pages of the sites named, as
  `"https://app.example.com"`, or `"*"` for any, call the app from the
  browser, as another site's pages call an API. A request from one of them
  gets `Access-Control-Allow-Origin`, and the browser's preflight, an
  `OPTIONS` that asks first, is answered with a 204 and the methods and
  headers it asked for, before any route, as an API's routes have no
  `OPTIONS` of their own. A request from another site passes as it came,
  and its browser keeps the response from its page. Credentials, the
  app's cookies, are never allowed: an API another site's pages call takes
  tokens, which a page sends itself. `Retry-After` is exposed, for a
  limit's 429. A blank origin is skipped, and one that isn't an origin
  panics, as `CSRF`'s do.
- `Headers(cfg)` says what a browser may do with each response:
  `X-Content-Type-Options: nosniff`, `Referrer-Policy:
  strict-origin-when-cross-origin`, `Cross-Origin-Opener-Policy:
  same-origin` and `X-Frame-Options: SAMEORIGIN`, and with `HSTS`,
  `Strict-Transport-Security` for a year. A handler that needs another sets
  its own.
- `CSP(cfg)` sends a Content-Security-Policy, which runs the scripts that
  carry a nonce made for the response, and what they load, and lets a
  page load the app's own alone. `middleware.NonceFrom(ctx)` reads the
  nonce, which package `inertia` hands the root template as `.Nonce`.
  `ReportOnly` sends it as `Content-Security-Policy-Report-Only`,
  `ReportPath` is where the browser reports what it blocks, which `CSP`
  answers itself and logs, `Sources` adds to its directives, and
  `DevServer` lets Vite's dev server in. It panics on a source or a path it
  can't put in the header. See
  [deployment.md](deployment.md#security-headers).

### The client's address

```go
app.Use(middleware.TrustProxies("10.0.0.0/8"), middleware.RequestID(), middleware.Logger())

func search(c *tug.Ctx) error {
	slog.Info("a search", "from", c.IP()) // "203.0.113.9", the client's, not the load balancer's
	...
}
```

`c.IP()` is the address a request came from, without its port. Behind a
proxy, a load balancer, a CDN or a platform's router, that's the proxy's,
and every client comes from it: a limit by address counts them all as
one. `middleware.TrustProxies(proxies...)` names the proxies the app
believes, each an address or a range, as `10.0.0.5` or `10.0.0.0/8`, and
gives a request from one of them the client's address, in `RemoteAddr`,
where `c.IP()`, and anything else written for net/http, finds it. The
starters read the proxies from `TRUSTED_PROXIES`, comma separated, and
believe none without it.

Each proxy adds the address it saw to the end of `X-Forwarded-For`, so the
header is read from its end, back past the proxies named, to the first
address that isn't one: the client, as far as the proxies can tell. What
comes before it, the client wrote itself, and could be anything, which is
why reading the header from its start, as the easy way does, lets anyone
pick the address a limit counts. A request from an address that isn't a
proxy keeps its own, whatever its headers say.

`*` believes whatever connects, and it alone: the client is the address
that proxy added. It's for a platform whose proxy has no address the app
can name, where nothing but the proxy can reach the app. An app that
anything else can reach names its proxies: with `*`, whatever connects
picks its own address. `TrustProxies` panics on a proxy that isn't an
address or a range, such as a host's name, so that a mistyped setting
stops the app as it starts, rather than leave it believing no one.

It reads the address alone, not `X-Forwarded-Proto` or
`X-Forwarded-Host`: the app's links are `APP_URL`'s, its cookie is
`Secure` by `session.Config.Secure`, and the proxy passes `Host` on (see
[Deployment](deployment.md#behind-a-proxy)).

### A limit for a route

```go
searches := &auth.Throttle{Name: "searches", Max: 30, Window: time.Minute}
app.Get("/search", tug.Limit(searches, (*tug.Ctx).IP, search)).Name("search")
```

`tug.Limit(limiter, key, handler)` counts a try for each request, by the
key the function makes of it, and one over the limit doesn't reach the
handler: it's a 429 for the `ErrorHandler`, as a 404 is, with
`Retry-After`, the wait in seconds, rounded up, and "too many requests:
wait 30 seconds, and try again". So an Inertia visit gets the error page,
and an API's client JSON. A limiter's error, as when the database that
counts is down, is as though the handler had returned it.

It's a wrapper around a handler, as the auth starter's `usersOnly` is,
rather than middleware: middleware answers before the handler, with no
`Ctx`, and could only write a response of its own, which an Inertia visit
shows in a modal. The limiter is anything with `Try`, a `tug.Limiter`,
as an `auth.Throttle` is ([Accounts](auth.md#throttle)): in memory, this
process's own counts, or with a `Store`, counts that every instance of
the app shares.

The key is the app's to make from the request: the address it came from,
`c.IP()`, which behind a proxy takes `TrustProxies` (see
[The client's address](#the-clients-address)), or a user's ID, for a
limit per account. A throttle used on more than one route counts them
together, unless the key says which.

## Handlers and `Ctx`

```go
func show(c *tug.Ctx) error {
	id := c.Param("id")   // "42" for /posts/42, on "/posts/{id}"
	tab := c.Query("tab") // "comments" for ?tab=comments
	return c.String(http.StatusOK, "post "+id+", tab "+tab)
}
```

A handler is a `tug.HandlerFunc`, `func(c *tug.Ctx) error`. It writes its
response through `c`, or returns an error for the `ErrorHandler` to answer:
a handler says what went wrong, and never writes an error page itself. A
`Ctx` belongs to its request, so don't keep it after the handler returns,
as a goroutine that outlives the handler would.

| Method        | Returns |
|---------------|---------|
| `Request()`   | The `*http.Request`. |
| `Response()`  | The `http.ResponseWriter`, for headers, or to write a response by hand. |
| `Context()`   | The request's context, which is canceled when the client goes away. |
| `Param(name)` | The value of the path wildcard `{name}`. |
| `Query(name)` | The first value of the query parameter `name`. |
| `IP()`        | The address the request came from, without its port: the client's, behind the proxies `TrustProxies` names. |
| `Locale()`    | The request's language, as `"vi"`: the app's choice for it, or else the browser's, or else the app's default. `c.T` and `c.Choice` say texts in it ([Languages](languages.md)). |
| `Written()`   | Whether the response has started. After that, its status and headers can't change. |

The responses set the status and the `Content-Type`, and write the body.
Each returns an error, so a handler can end with `return c.JSON(...)`.

| Method                       | Writes |
|------------------------------|--------|
| `JSON(code, v)`              | `v` as JSON. A value `encoding/json` can't encode is an error, returned before anything is written. |
| `String(code, s)`            | `s` as plain text. |
| `HTML(code, html)`           | `html` as a page, exactly as given: anything from a user in it must already be escaped. |
| `Blob(code, contentType, b)` | `b`, with the given content type. |
| `NoContent(code)`            | A status and no body, such as 204. |

`c.Redirect(to)` sends the client to another URL: with 302 Found after a GET
or HEAD, and 303 See Other after anything else. A 303 makes browsers, and
Inertia, follow a PUT, PATCH or DELETE with a GET, where a 302 may repeat
the method. `c.RedirectRoute(name, params...)` redirects to a named route,
and `c.RedirectBack()` to the page the request came from, by its `Referer`,
when that's a page of this app, and to `/` otherwise, since a `Referer` can
name any site. It's for a form that more than one page has, which goes back
to whichever it was sent from, as the auth starter's "send the link again"
does.

A `Ctx` has more for pages, such as `Inertia` and `Location`, in
[pages.md](pages.md), and for forms, such as `BindValid`, `Session` and
`Flash`, in [forms.md](forms.md).

### Files and downloads

```go
app.Get("/terms", func(c *tug.Ctx) error {
	return c.File("legal/terms.pdf") // shown in the browser, as a PDF is
})

app.Get("/invoices/{id}", func(c *tug.Ctx) error {
	pdf, err := a.invoicePDF(c.Context(), c.Param("id"))
	if err != nil {
		return err
	}
	return c.Download("invoice-"+c.Param("id")+".pdf", bytes.NewReader(pdf))
})
```

- `c.File(path)` sends one of the app's files, shown as its type: a PDF
  as a PDF, HTML as a page. `http.ServeContent` sends it, with ranges, so
  a download that broke off goes on where it stopped, and with
  `If-Modified-Since`, for the browser's cache. A file that isn't there,
  or a directory, is a 404 for the `ErrorHandler`.
- `path` is the app's, never a request's. `c.File("reports/" +
  c.Param("name"))` would send `app.db` to a request for
  `/reports/..%2Fapp.db`, as one part of a path can carry a slash, as
  `%2F`. `c.FileFS(fsys, name)` sends a file
  of an `fs.FS`, such as `os.DirFS("reports")`, or an `os.Root`'s `FS`,
  which keeps symbolic links inside as well, where a name that would
  leave it isn't there, a 404. It's how an embedded file is sent too.
- `c.Download(name, content)` sends `content`, an `io.Reader`, as a file
  to save as `name`: the browser saves it rather than shows it. The name
  goes in `Content-Disposition` however it's written, in Vietnamese, with
  quotes or with a line break, which can't end the header, and what's
  before its last `/` or `\` is dropped, as the browser saves a name, not
  a path. Its type is its name's extension's, or else what its first
  bytes say. Content that seeks, a file or a `bytes.Reader`, has ranges,
  and a file its time for `If-Modified-Since`; other content, as a
  response from another service, goes as it's read. Closing it is the
  handler's.
- A file someone uploaded goes by its disk's route
  ([Files](files.md#links)), which sends it as what its bytes are, or by
  `Download`, under the name it was uploaded with: `File` would show HTML
  as a page of the app's.

### Streams

```go
app.Get("/posts.csv", func(c *tug.Ctx) error {
	return c.StreamDownload("posts.csv", "text/csv; charset=utf-8", func(w io.Writer) error {
		out := csv.NewWriter(w)
		err := a.posts.each(c.Context(), func(p Post) error {
			return out.Write([]string{strconv.FormatInt(p.ID, 10), p.Title})
		})
		out.Flush()
		return cmp.Or(err, out.Error())
	})
})
```

`c.Stream(contentType, write)` sends a body as `write` makes it, for one
too long to hold, made as it's read: every post as CSV, a row at a time,
as the query returns them. `c.StreamDownload(name, contentType, write)`
sends it as a file to save, named as `Download` names one. Each write
goes to the client as it's made, flushed, so a client that reads as it
comes, a `fetch` of lines, gets each one; many small writes are buffered
by whoever makes them, as `csv.Writer` does.

`write`'s error before its first write is the handler's, for the
`ErrorHandler`, as any is. Once the body has started, there's no error
page to show: the error goes to the log, and the connection is cut, so
the browser says the download failed, where it would have saved half a
file as the whole. A stream takes as long as it takes, as the server has
no timeout for a response, and the app's shutdown waits for it, up to
`ShutdownTimeout`, as it does for any request.

### Events

```go
app.Get("/imports/{id}/progress", func(c *tug.Ctx) error {
	return c.Events(func(ctx context.Context, send func(tug.Event) error) error {
		for p := range a.imports.progress(ctx, c.Param("id")) {
			if err := send(tug.Event{Name: "progress", Data: p}); err != nil {
				return err
			}
		}
		return send(tug.Event{Name: "done"})
	})
})
```

`c.Events(fn)` sends server-sent events, which a page reads with the
browser's `EventSource` ([Pages](pages.md#downloads-and-events)): an
import's progress, or a change a page should show. `fn` sends each, a
`tug.Event`: its `Name`, which the page listens for, its `ID`, and its
`Data`, as JSON, which the page reads with `JSON.parse`. A name or ID with
a line break, or data JSON can't hold, is `send`'s error, and isn't sent.

- The stream lasts as long as `fn`. `fn`'s context is done when the
  client goes away, and when the app shuts down, as `Serve` waits for the
  requests in flight, which a stream would hold up; `send` fails once it
  is.
- An `EventSource` connects again when its stream ends, the app's
  shutdown's among them, to an instance that's up, and sends the last
  event's ID in `Last-Event-ID`, which `c.Request().Header` has, for the
  app to go on from. A page that has what it waited for closes its
  `EventSource`.
- A quiet stream sends a comment every 20 seconds, which the browser
  passes over, so the proxies in front of the app, which close a
  connection that says nothing for a minute, don't. The stream has
  `Cache-Control: no-cache`, and `X-Accel-Buffering: no`, so nginx doesn't
  hold the events back.
- `fn`'s error is logged, as the response has started; a client that went
  away, or the app's shutdown, isn't an error.
- The events are the instance's own: a page on one instance doesn't hear
  an event another instance has. Package `broadcast` sends one to every
  page that follows a channel, whichever instance it's on
  ([Broadcasting](broadcasting.md)).

## Binding

```go
type UpdatePost struct {
	ID    int64    `path:"id"`     // the {id} in "/posts/{id}"
	Draft bool     `query:"draft"` // ?draft=1
	Title string   `json:"title"`  // the body, JSON or a form
	Tags  []string `json:"tags"`
}

func update(c *tug.Ctx) error {
	var in UpdatePost
	if err := c.Bind(&in); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, in)
}
```

`c.Bind(&in)` fills a struct from the request. Each field's tags say where
its value comes from:

| Tag            | From |
|----------------|------|
| `path:"id"`    | The path wildcard `{id}`. |
| `query:"page"` | The query string. |
| `json:"title"` | A JSON body, through `encoding/json`. In a form, it's the field's key when there's no `form` tag. |
| `form:"title"` | A form body, urlencoded or multipart. |

The body is read by its `Content-Type`. JSON, `application/json` or any
`+json` type, goes through `encoding/json`. A form,
`application/x-www-form-urlencoded` or `multipart/form-data`, fills a field
by its `form` tag, or else its `json` tag's name, or else its name.
`form:"-"` leaves a field out of a form, as `json:"-"` does for a field
with no `form` tag. A body of any other type is a 415.

The body is bound first, then the query, then the path, and each overwrites
the one before where it has a value. So the URL wins, and a body can't
change the `{id}` in it. A body can still fill a `query` or `path` field
that the URL has no value for: `examples/api` keeps a body away from its ID
with `` ID int64 `path:"id" json:"-"` ``.

In a query or a form, a key that repeats, or ends in `[]`, fills a slice:
`tags=a&tags=b`, or `tags[]=a&tags[]=b`. An empty value leaves its field
alone, since an empty input means no value rather than zero. A multipart
form's files go to fields of type `*multipart.FileHeader`, or
`[]*multipart.FileHeader` for several, and a file input left empty, sent
as a file with no name, leaves its field nil. Up to 32 MiB of files is
held in memory, and the rest in temporary files, which net/http removes
after the request. Inertia's client sends a form with a file in it as a
multipart form, with a list's items under `tags[]`, true and false as `1`
and `0`, which bind, and a nested object's values under `user[name]`,
which don't: a form with a file keeps its fields flat. See
[Files](files.md).

Values from the path, the query and forms fill strings, bools, ints, uints,
floats, `[]byte`, `time.Time`, any type with an `UnmarshalText` method, such
as `netip.Addr`, and pointers and slices of these. A bool takes `1`, `true`,
`on` or `yes`, and `0`, `false`, `off` or `no`, so a checkbox's `on` binds.
A time takes RFC 3339, or what HTML's `date` and `datetime-local` inputs
send, as UTC. The fields of an embedded struct with no tag bind as the outer
struct's own, as `encoding/json` promotes them. A field of a type `Bind`
can't fill, such as a map, is the program's mistake rather than the
client's: when a value comes for it, `Bind` returns a plain error, a 500.

Inertia's `<Form>` sends a form as JSON, with every value a string, as the
browser's `FormData` has it: `"on"` from a ticked checkbox, `"42"` from a
number input, `"2026-09-25"` from a date input, and `""` from one left
empty. `encoding/json` takes none of those for a bool, a number or a
`time.Time`, so a JSON body that doesn't decode is read again, with each of
those strings read as the form's value would be: `"on"` is `true`, `"42"`
is `42`, a date is one in UTC, and `""` leaves its field alone, or, in a
list, is left out. That goes for fields at any depth, in nested structs,
lists and maps, found by `encoding/json`'s own rules for which field a key
fills. A string that still doesn't parse is the error it was: "age must be
a whole number". A body that decodes as it is, as an API client's `true`
and `42` do, binds just as `encoding/json` has it, and a field that reads
its own JSON or text, such as `netip.Addr`, or one tagged `json:",string"`,
is given the string as it came.

A value that doesn't parse is a 400 whose message names the field: "age
must be a whole number", or "author.age must be a whole number" from a JSON
body. The error is an `*HTTPError` that wraps a `*BindError`. The fields
after it are still bound, and the first such error is the one returned. A
path value that doesn't parse is a 404, whatever else is wrong, since
`/posts/abc` is a page that isn't there. JSON that doesn't parse at all is a
400, "invalid JSON", and a body over `Config.BodyLimit` is a 413.

A handler can bind more than once, as one that reads the `{id}` to find a
post and then binds the form does: a JSON body is kept after the first read.

`c.BindValid(&in)` binds as `Bind` does, then checks the struct's `validate`
tags, and a value that doesn't parse becomes one of the form's errors rather
than a 400. See [forms.md](forms.md).

## Errors

```go
func show(c *tug.Ctx) error {
	post, err := db.FindPost(c.Context(), c.Param("id"))
	if errors.Is(err, db.ErrNotFound) {
		return tug.NewHTTPError(http.StatusNotFound, "post not found")
	}
	if err != nil {
		return err // a 500, with the details in the log
	}
	return c.JSON(http.StatusOK, post)
}
```

A handler's error goes to the app's `ErrorHandler`, which turns it into the
response. `tug.NewHTTPError(code, message...)` returns an `*HTTPError`,
which chooses the status; without a message, it says the status's text, so
`NewHTTPError(http.StatusNotFound)` is a 404 that says "Not Found". An
`HTTPError` has three fields. `Message` is shown to the client, so it says
what went wrong in their terms. `Err` is the cause, which is for the log:
it's logged with a server error, and shown only by `Debug`.

```go
return &tug.HTTPError{Code: http.StatusBadGateway, Message: "the payment service didn't answer", Err: err}
```

An `*HTTPError` anywhere in an error's chain counts, so
`fmt.Errorf("loading post: %w", tug.NewHTTPError(http.StatusNotFound))` is
still a 404. Two more types reach the `ErrorHandler`: a `*BindError`, which
`Bind` returns inside an `*HTTPError`, with the `Field`, the `Reason`
("must be a whole number") and the parse error, `Err`; and a `*PanicError`,
a handler's panic recovered by the app, with its `Value` and `Stack`. A
panic with `http.ErrAbortHandler`, net/http's way of dropping a response on
purpose, isn't recovered.

`DefaultErrorHandler` answers them like this:

- An `*HTTPError` chooses the status and the message. So does an error
  that says its status, with a `StatusCode() int` method, as package
  `auth`'s no does ([Authorization](authorization.md#a-no-is-a-403)), with
  its own words as the message, under 500, and the status's text from 500
  up, as its words may not be the client's. Anything else is a 500 that
  says "Internal Server Error".
- Errors of 500 and up are logged through `slog.Default()` as "request
  failed", with the method, path and error, and a panic's stack. With
  `Debug` on, the response shows the error and the stack too.
- The body is JSON, `{"message":"post not found"}`, when the request's
  `Accept` header asks for JSON first, as API clients do, and plain text
  otherwise.
- An error after the response has started can't change it: it's logged,
  and the response goes out as it began.
- The `validate.Errors` of `BindValid` go back to the form, or to an API
  client as a 422. See [forms.md](forms.md).
- With `Config.ErrorPage` set, browsers and Inertia's client see errors as
  an Inertia page, unless `Debug` is showing a server error's details. See
  [pages.md](pages.md).

An `ErrorHandler` of your own gets all of these, and the 404s and 405s of
requests no route takes, as `*HTTPError`s. Handle the errors that are
yours, and hand the rest to `DefaultErrorHandler`:

```go
cfg := tug.ConfigFromEnv()
cfg.ErrorHandler = func(c *tug.Ctx, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		err = tug.NewHTTPError(http.StatusNotFound)
	}
	tug.DefaultErrorHandler(c, err)
}
app := tug.New(cfg)
```

One that writes a response itself checks `c.Written()` first: once a
response has started, only the log is left.

## Logging

tug logs through `slog.Default()` and never sets it: the handler, the
format and the level are the app's call. For JSON lines, say, set it in
`main`, before `Run`:

```go
slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
```

`Run` and `Serve` log "listening", with the address, and "shutting down",
and send the `http.Server`'s own errors there at Warn. `DefaultErrorHandler`
logs "request failed" for server errors. In package `middleware`, `Logger`
logs a "request" line for each request, and `Recover` logs "panic". A line
about a request is logged with the request's context, so a `slog.Handler`
of the app's own can add `middleware.RequestIDFrom(ctx)` to it.
