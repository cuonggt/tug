# tug

A web framework for Go apps whose frontend is [Inertia.js](https://inertiajs.com):
React pages rendered with props straight from Go handlers, with no API in
between, and the whole app shipped as one binary.

The name: a tugboat is small, and moves ships many times its size.

**Status: early.** v0.29.0 is the latest release: the framework, its CLI,
background jobs, on a schedule in any time zone too, pushed in the app's
own transactions, and so many at once or a second across the instances,
server-side rendering, tests of an app's pages, uploads kept on the app's
disk or in S3, limits on routes and on guessing passwords, counted across
the app's instances, the client's address behind its proxies, signed
links, what tug and the app say in the request's language, lists in
pages, the app's own commands, a cache and locks across the instances,
mail with copies, files and a link to unsubscribe, downloads, streams and
server-sent events, the app's own values encrypted, events broadcast to
the pages open on every instance, what a user may do with a thing,
headers that say what a browser may do with a page, a
Content-Security-Policy that runs its own scripts alone, the database's
tables made and changed by files of SQL, each run once, the route that
answered each request and how each job ran, for the app's metrics, and
starters in React, Vue or Svelte, one with accounts, from registering to
two-factor logins, passkeys, a profile photo, API tokens, admins, and
notifications in the app and by mail, in SQLite, Postgres or MySQL.
[The guide](docs/README.md) covers all of it, and
[docs/roadmap.md](docs/roadmap.md) has what's next.

```sh
go install github.com/cuonggt/tug/cmd/tug@latest
tug new blog                                        # a new app, in ./blog; tug new -auth blog for accounts
cd blog
tug dev                                             # http://localhost:8080, rebuilt and reloaded as it changes
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
messages, posts that show in every browser open as they're made, the Vite
dev server with hot reload, and the frontend embedded in the binary for
production. Handlers that aren't pages look like this:

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
  and a fresh APP_KEY, and a frontend of React, or with `-vue` or
  `-svelte`, of Vue or Svelte. `tug dev` runs it with Vite: Go is rebuilt and
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
  - **Pagination:** `tug.Paginate` is the page of a list `?page` asks for,
    from the app's own count and query, with where it sits and a pager's
    links, under the keys Laravel's paginators write; `SimplePaginate`
    without a count, and `CursorPaginate` by cursor. Each feeds `Scroll`
    too.
  - **Once:** `Once` props stay on the client until they expire.
  - **Partial reloads** reach nested props by path.
  - **Redirects:** a browser running an old build reloads. A 302 after a
    PUT, PATCH or DELETE becomes a 303, and a redirect to a `#fragment`
    keeps it.
  - **Error pages:** errors show as a page (`Config.ErrorPage`) with their
    own status.
  - **Empty lists:** nil slices go out as `[]`, never `null`.
  - **DevTools:** under `tug dev`, each request is kept for Inertia's
    DevTools, the browser's panel: its route and the function that
    answered, where the page was rendered, each prop with its type and
    value, and the headers and bodies, with secrets redacted.
- **Server-side rendering** (package `ssr`), when an app wants it, as
  `tug new -ssr` makes one: a first visit's page comes with its HTML, for
  search engines and pages that show before their scripts run, rendered by
  Inertia's own SSR in Node, which the app runs beside it, from the SSR
  build it embeds. Without Node, pages render in the browser, as they
  would anyway.
- **Forms**: `c.BindValid` binds and checks a request by `validate` tags
  (package `validate`, go-playground/validator's rules, and the app's own,
  with `validate.Rule`) and checks of the handler's own. A form that
  doesn't validate goes back with its errors in the `errors` prop, under an
  error bag when the form names one; an API client gets a 422. A message
  names a field as a person reads it, "first name is required", by its
  `label` tag or its key made into words. Precognition, a form checking
  each field as it's left, is answered without running the rest of the
  handler.
- **Languages** (package `lang`): what tug says to a person, a form's
  errors, a 429, an error page's status, and what the app says with `c.T`,
  in the request's language: the app's choice, or else the browser's
  `Accept-Language`, from a JSON file for each language, keyed by the
  English, as Laravel's `lang/vi.json` is, with plurals by each language's
  rules. `tug lang vi` writes the file, with every text the app says.
- **Encryption** (package `crypt`): the app's own values, as a token for
  another service kept in a column, sealed with AES-256-GCM, under a key
  for their purpose alone, derived from `APP_KEY`, bound to their row if
  the app likes, and moved to a new key after a rotation, as tug's own
  are. `tug key` makes a key.
- **Sessions** (package `session`): in an encrypted cookie, keyed by
  `APP_KEY`, with flash data. `c.Flash` reaches the next page shown, after
  a redirect or not, and only that one.
- **Files** (package `storage`): uploads, from an Inertia form with its
  progress, kept in a directory the app serves, or in S3 or any service
  that speaks its API, Cloudflare R2 and MinIO among them, with S3's
  signatures on the standard library. `validate` checks an upload's size
  and what it is, from its first bytes rather than its name, and a page
  gets a link to it, public or signed until it expires. What's served
  can't run as the app: its type is its bytes', and anything but an image
  is a download.
- **Accounts**: `tug new -auth` makes an app where people register and
  verify their email, log in, with a code from an authenticator app too
  once they turn that on, or with a passkey and no password at all, reset
  a forgotten password by email, change their profile and photo, password
  and appearance in settings, and make API tokens, for a script or another
  service to call the app's API with, hear of each change to their
  account that could hand it to someone else, at once in the app, by its
  bell, and by mail, the old email too, and where admins, whom a command
  makes, have a page of the jobs that failed, with the users in SQLite,
  or with `-postgres` or `-mysql`, in Postgres or MySQL, and a frontend of
  Tailwind and shadcn's components, as Laravel's starter kits have, in
  React, Vue or Svelte. Its handlers are the app's own code, on package
  `auth`, which has the parts where a slip is a security hole: argon2id
  password hashes, logins that end when the password changes, signed
  tokens for reset and verification links, two-factor codes and recovery
  codes kept encrypted, passkeys, WebAuthn's checks on the standard
  library, asking for the password again, API tokens kept as their hashes,
  throttles on guessing, whose counts every instance of the app shares,
  and abilities, what a user may do with a thing, whose no is a 403 that
  says why.
- **Migrations** (package `migrate`): the database's tables, made and
  changed by files of SQL, named for when they were made, which `tug
  migrate new` writes, each run once, in order, as the app starts, and by
  its binary's `migrate` command, which lists them and undoes the last.
  What ran is kept with a hash of its SQL, so a file changed after it ran
  is caught, and instances starting at once take turns. The auth
  starter's tables are its migrations, in SQLite, Postgres or MySQL.
- **Mail** (package `mail`): through an SMTP server, or in development
  written out where `tug dev` shows it, links and all, with copies, a
  `Bcc` no one sees, replies to another address, files, and a link to
  unsubscribe in one click, as Gmail and Yahoo ask of mail sent in bulk.
  `mailtest.Outbox` keeps what an app sends, for its tests, which read it
  in order, or by who it went to.
- **Background jobs** (package `queue`): work a request starts and doesn't
  wait for, such as a mail, kept by a store such as a table in the app's
  database, so a failure or a restart doesn't lose it. A job that fails
  runs again after a wait that grows, and the workers run beside the
  server with `app.Go`, finishing what they have as the app stops. Jobs
  run on a schedule too, with `Every` or a cron expression, on UTC's
  clock or a time zone's, through its daylight saving as cron goes, once
  across all instances. A unique kind keeps one job waiting for each
  value, however often it's pushed, at the latest push's time if it likes,
  and runs them one at a time if it likes. A kind runs so many at once at
  most, or starts so many a second, counted on all the instances, and
  says, with `OnFail`, when a job has failed for good, and with `Observe`,
  how each run went, for the app's metrics. A job pushed in the app's own
  transaction is kept with what else it writes, or not at all.
  The auth starter sends its mail this way, with its jobs in its
  database, where on Postgres or MySQL the claims of several instances
  skip each other's jobs rather than wait for them, and its binary lists
  the jobs that failed for good, and runs them again: `./blog jobs`.
- **Cache** (package `cache`): what's slow to work out, kept for a while
  where every instance of the app finds it, such as a table in its
  database, as the auth starter has, with `cache.Remember`, which works a
  value out once however many ask for it at once, and locks that hold
  across the instances, for an import or a command that mustn't run twice
  at once.
- **Broadcasting** (package `broadcast`): events on channels, as `posts`
  or `users.42`, to the pages that follow them on every instance of the
  app, through `c.Events`, carried by its database, as the auth starter
  has it: Postgres's `NOTIFY`, or a table in SQLite and MySQL, and in the
  transaction that makes the change, or not at all. A post made in one
  browser shows in the others, and the auth starter's page that asks to
  verify the email moves on once the link is followed on the phone.
- **Tests** (package `tugtest`): Inertia's client, for Go's tests of an
  app's pages, which need no browser or frontend build. Its visits keep
  the cookies the app sets and follow its redirects, upload files as the
  browser does, and a page's props read into the struct its `tug.Page`
  declares: `tugtest.Props(r, Dashboard).User`. The starters' tests use
  it.
- **Vite** (package `vite`): tags from the dev server while it runs, with
  the React refresh preamble, and from the build's manifest otherwise, with
  CSS and preloads, the scripts with the page's nonce. The built files are
  served, and cached for a year.
- **Routing** on net/http's `ServeMux`: method routes, groups with their own
  middleware, and named routes with `app.URL("posts.show", 42)`. Paths match
  exactly, so `/` is only the home page, and `{name...}` takes everything
  under a path. A trailing slash redirects to the route without it, and a
  wrong method is a 405 with `Allow`. Links that leave the app, as in
  mail, are whole, from `APP_URL`, never the request's `Host`, and
  `app.SignedURL` makes ones only the app could have, until they expire,
  for a route `tug.Signed` wraps. `tug.Limit` puts a limit on a route, by
  address or user: a 429, with `Retry-After`, past it. `tug.RouteOf` says
  which route answered a request, `GET /posts/{id}`, for the app's
  middleware to label its metrics by, and `Logger` logs it.
- **Handlers return errors.** `tug.NewHTTPError(404, "post not found")`
  picks the status. Any other error is a 500 whose details stay in the log,
  or show in the response with `APP_DEBUG=true`. A panic is a 500 with its
  stack in the log. Errors are JSON for a client that asks for JSON.
- **Files, downloads and streams:** `c.File` sends one of the app's
  files, with ranges, `c.Download` a file to save, its name however it's
  written, Vietnamese, quotes and all, `c.StreamDownload` a CSV made a row
  at a time, and `c.Events` server-sent events, for a page to follow work
  as it goes, ended as the app shuts down.
- **`c.Bind`** fills a struct from path values, the query, and a JSON or
  form body, files included, by struct tags. A value that doesn't parse is
  a 400 that names the field, "age must be a whole number"; a bad path value
  is a 404.
- **Middleware** is `func(http.Handler) http.Handler`: `RequestID`, `Logger`
  (through slog), `Recover`, `CSRF`, which is Go's
  `http.CrossOriginProtection`, so there are no tokens, `CORS`, for an API
  other sites' pages call,
  `TrustProxies`, which reads the client's address past the app's load
  balancers, from the end of `X-Forwarded-For`, for `c.IP()`, `Headers`,
  which says what a browser may do with a response, as be framed by
  another site, and holds it to HTTPS, and `CSP`, a
  Content-Security-Policy that runs only the scripts that carry a nonce
  made for the response, which the root template gives Vite's tags and its
  own, and logs what browsers report it blocked.
- **`app.Run`** listens on `ADDR` or `PORT`, and on SIGTERM stops taking
  connections and lets the requests in flight finish. Given one of the
  app's commands, as `./blog jobs`, which `app.Command` adds, it runs
  that in place of serving, in the app as `main` made it.

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
