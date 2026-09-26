# tug

A web framework for Go apps whose frontend is [Inertia.js](https://inertiajs.com):
React pages rendered with props straight from Go handlers, with no API in
between, and the whole app shipped as one binary.

The name: a tugboat is small, and moves ships many times its size.

**Status: early.** v0.2.0 is the latest release: the framework, its CLI,
and a starter with accounts, from registering to two-factor logins.
[The guide](docs/README.md) covers all of it, and
[docs/roadmap.md](docs/roadmap.md) has what's next.

```sh
go install github.com/cuonggt/tug/cmd/tug@latest
tug new blog                                        # a new app, in ./blog; tug new -auth blog for accounts
cd blog
tug dev                                             # http://127.0.0.1:8080, rebuilt and reloaded as it changes
```

```go
type PostsIndexProps struct {
	Posts []Post                   `json:"posts"`
	Stats inertia.DeferProp[Stats] `json:"stats"` // fetched after the page shows
}

var PostsIndex = tug.Page[PostsIndexProps]("Posts/Index") // resources/js/pages/Posts/Index.tsx

func index(c *tug.Ctx) error {
	return PostsIndex.Render(c, PostsIndexProps{
		Posts: store.List(),
		Stats: inertia.Defer(store.Count),
	})
}

type PostInput struct {
	Title string `json:"title" validate:"required,max=80"`
	Body  string `json:"body" validate:"required"`
}

func create(c *tug.Ctx) error {
	var in PostInput
	if err := c.BindValid(&in); err != nil {
		return err // back to the form with its errors; a field being filled in is checked here too
	}
	post := store.Add(in)
	c.Flash("success", "Post created") // usePage().flash on the next page
	return c.RedirectRoute("posts.show", post.ID)
}
```

[`examples/inertia`](examples/inertia/main.go) is the whole app: React
pages, a deferred prop, forms that check each field as it's left, flash
messages, the Vite dev server with hot reload, and the frontend embedded in
the binary for production. Handlers that aren't pages look like this:

```go
func main() {
	app := tug.New() // ADDR or PORT, and APP_DEBUG, from the environment
	app.Use(middleware.RequestID(), middleware.Logger(), middleware.Recover(), middleware.CSRF())

	app.Get("/posts/{id}", showPost).Name("posts.show")

	if err := app.Run(); err != nil { // graceful shutdown on SIGINT and SIGTERM
		log.Fatal(err)
	}
}

func showPost(c *tug.Ctx) error {
	var in struct {
		ID int64 `path:"id"`
	}
	if err := c.Bind(&in); err != nil {
		return err // /posts/abc is a 404
	}
	post, ok := posts[in.ID]
	if !ok {
		return tug.NewHTTPError(http.StatusNotFound, "post not found")
	}
	return c.JSON(http.StatusOK, post)
}
```

[`examples/api`](examples/api/main.go) is a complete JSON API.

## What's here

- **The CLI**, `tug`: `tug new` makes an app, ready to run, with a .env
  and a fresh APP_KEY. `tug dev` runs it with Vite: Go is rebuilt and
  restarted as it changes, and the browser reloaded. `tug gen` writes the
  TypeScript of each page's props and of the named routes, with a typed
  `route()`, so the frontend is checked against the Go. `tug build` makes
  one static binary with the frontend in it, and the app comes with a
  Dockerfile for a distroless image.
- **Inertia pages** (package `inertia`), to the whole v3 protocol: a first
  visit gets HTML with the page object, later visits get JSON.
  `tug.Page[Props]` ties a component to the props it takes, and props nest
  at any depth.
  - **Loading:** `Lazy`, `Optional`, `Always` and `Defer` props, worked
    out concurrently. `Defer(...).Rescue()` turns a failure into Inertia's
    rescue slot rather than a failed page.
  - **Merging:** `Merge` props, prepended or deep-merged, and matched on a
    key. `Scroll` pages a list for `<InfiniteScroll>`.
  - **Once:** `Once` props stay on the client until they expire.
  - **Partial reloads** reach nested props by path.
  - **Redirects:** a browser running an old build reloads. A 302 after a
    PUT, PATCH or DELETE becomes a 303, and a redirect to a `#fragment`
    keeps it.
  - **Error pages:** errors show as a page (`Config.ErrorPage`) with their
    own status.
  - **Empty lists:** nil slices go out as `[]`, never `null`.
- **Forms**: `c.BindValid` binds and checks a request by `validate` tags
  (package `validate`, go-playground/validator's rules) and checks of the
  handler's own. A form that doesn't validate goes back with its errors in
  the `errors` prop, under an error bag when the form names one; an API
  client gets a 422. Precognition, a form checking each field as it's left,
  is answered without running the rest of the handler.
- **Sessions** (package `session`): in an encrypted cookie, keyed by
  `APP_KEY`, with flash data. `c.Flash` reaches the next page shown, after
  a redirect or not, and only that one.
- **Accounts**: `tug new -auth` makes an app where people register and
  verify their email, log in, with a code from an authenticator app too
  once they turn that on, reset a forgotten password by email, and change
  their profile, password and appearance in settings, with the users in
  SQLite and a frontend of Tailwind and shadcn/ui, as Laravel's React
  starter kit has. Its handlers are the app's own code, on package `auth`,
  which has the parts where a slip is a security hole: argon2id password
  hashes, logins that end when the password changes, signed tokens for
  reset and verification links, two-factor codes and recovery codes kept
  encrypted, asking for the password again, and a throttle on guessing.
- **Mail** (package `mail`): through an SMTP server, or in development
  written out where `tug dev` shows it, links and all.
- **Background jobs** (package `queue`): work a request starts and doesn't
  wait for, such as a mail, kept by a store such as a table in the app's
  database, so a failure or a restart doesn't lose it. A job that fails
  runs again after a wait that grows, and the workers run beside the
  server with `app.Go`, finishing what they have as the app stops. The
  auth starter sends its mail this way, with its jobs in SQLite.
- **Vite** (package `vite`): tags from the dev server while it runs, with
  the React refresh preamble, and from the build's manifest otherwise, with
  CSS and preloads. The built files are served, and cached for a year.
- **Routing** on net/http's `ServeMux`: method routes, groups with their own
  middleware, and named routes with `app.URL("posts.show", 42)`. Paths match
  exactly, so `/` is only the home page, and `{name...}` takes everything
  under a path. A trailing slash redirects to the route without it, and a
  wrong method is a 405 with `Allow`.
- **Handlers return errors.** `tug.NewHTTPError(404, "post not found")`
  picks the status. Any other error is a 500 whose details stay in the log,
  or show in the response with `APP_DEBUG=true`. A panic is a 500 with its
  stack in the log. Errors are JSON for a client that asks for JSON.
- **`c.Bind`** fills a struct from path values, the query, and a JSON or
  form body, files included, by struct tags. A value that doesn't parse is
  a 400 that names the field, "age must be a whole number"; a bad path value
  is a 404.
- **Middleware** is `func(http.Handler) http.Handler`: `RequestID`, `Logger`
  (through slog), `Recover`, and `CSRF`, which is Go's
  `http.CrossOriginProtection`, so there are no tokens.
- **`app.Run`** listens on `ADDR` or `PORT`, and on SIGTERM stops taking
  connections and lets the requests in flight finish.

tug needs Go 1.26. Its dependencies are go-playground/validator, for
package `validate`, and golang.org/x/crypto, for argon2id in package `auth`;
the rest is the standard library.

## Development

```sh
go test -race ./...
go test -run '^$' -bench . -benchmem .    # tug next to ServeMux alone
ADDR=127.0.0.1:8080 go run ./examples/api
```

The Inertia example, from `examples/inertia`:

```sh
npm install
npm run dev                               # the Vite dev server
ADDR=127.0.0.1:8080 go run .              # in another terminal
npm run build && npx playwright test      # end to end, in Chrome
```
