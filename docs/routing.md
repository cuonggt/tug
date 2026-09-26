# Routing

An app is a `tug.App`: its settings, its routes, and the middleware around
them. Routing is net/http's `ServeMux`, middleware has net/http's own shape,
and a handler takes a `*tug.Ctx` and returns an error:

```go
func main() {
	app := tug.New() // ADDR or PORT, and APP_DEBUG, from the environment
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

`ConfigFromEnv` reads `ADDR`, the address to listen on, such as
`127.0.0.1:8080`; or else `PORT`, as platforms such as Cloud Run and Fly.io
set it, so `PORT=3000` is `:3000`; and `APP_DEBUG`, where `true` or `1`
turns on `Debug`. The default address, `:8080`, listens on every interface.
[deployment.md](deployment.md) has more on the environment in production.

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

`Run` is also how `tug gen` learns the app: it runs the app with `TUG_GEN`
set, and `Run` writes the app's TypeScript instead of serving. So add every
route before calling `Run`; whatever `main` does before it, it does for
`tug gen` too. See [typescript.md](typescript.md).

An `App` is an `http.Handler`, so an `http.Server` set up by hand can serve
it, without `Run`'s signals and shutdown. A test needs no server at all: it
calls `app.ServeHTTP` with an `httptest.NewRecorder()`, as the tests of
`examples/api` do.

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

### Package `middleware`

Package `middleware` has four, each a plain
`func(http.Handler) http.Handler` that works with any router. The order in
`app.Use` above is the usual one: `RequestID` first, so every log line
carries the ID, and `Logger` before `Recover`, so a panic is logged as the
500 that `Recover` makes of it.

- `RequestID()` gives each request an ID: the `X-Request-ID` it came with,
  as a proxy in front may set one, when that's short and plain (up to 64
  letters, digits and `-_.:+/=`), or else a new random one. The ID goes back
  in the response's `X-Request-ID`, and into the request's context, where
  `middleware.RequestIDFrom(ctx)` finds it.
- `Logger()` logs a line for each request through `slog.Default()`: its
  method, path, status, size and duration, and its `request_id` when
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
  mistyped setting stops the app as it starts. See [forms.md](forms.md).

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
`[]*multipart.FileHeader` for several. Up to 32 MiB of files is held in
memory, and the rest in temporary files, which net/http removes after the
request.

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

- An `*HTTPError` chooses the status and the message. Anything else is a 500
  that says "Internal Server Error".
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
