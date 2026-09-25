# tug

A web framework for Go apps whose frontend is [Inertia.js](https://inertiajs.com):
React pages rendered with props straight from Go handlers, with no API in
between, and the whole app shipped as one binary.

The name: a tugboat is small, and moves ships many times its size.

**Status: early.** The HTTP core is done: routing, handlers, errors, binding
and middleware. The Inertia adapter is next; [docs/roadmap.md](docs/roadmap.md)
has the plan.

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

The core needs nothing beyond the standard library, and Go 1.25.

## Development

```sh
go test -race ./...
go test -run '^$' -bench . -benchmem .    # tug next to ServeMux alone
ADDR=127.0.0.1:8080 go run ./examples/api
```
