# tug

A web framework for Go apps whose frontend is [Inertia.js](https://inertiajs.com):
React pages rendered with props straight from Go handlers, with no API in
between, and the whole app shipped as one binary.

The name: a tugboat is small, and moves ships many times its size.

**Status: early.** The HTTP core and Inertia pages with Vite are done; forms
and validation are next. [docs/roadmap.md](docs/roadmap.md) has the plan.

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
```

[`examples/inertia`](examples/inertia/main.go) is the whole app: React
pages, a deferred prop, the Vite dev server with hot reload, and the
frontend embedded in the binary for production. Handlers that aren't pages
look like this:

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

- **Inertia pages** (package `inertia`), to the v3 protocol: a first visit
  gets HTML with the page object, later visits get JSON. Partial reloads,
  shared props, and lazy, optional, always and deferred props, worked out
  concurrently. A browser running an old build reloads, and a 302 after a
  PUT, PATCH or DELETE becomes a 303. Nil slices go out as `[]`, never
  `null`. `tug.Page[Props]` ties a component to the props it takes.
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

tug needs nothing beyond the standard library, and Go 1.25.

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
