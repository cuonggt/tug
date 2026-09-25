# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

tug is a Go web framework for apps whose frontend is Inertia.js v3: Go
handlers render React pages with props, with no API in between. It is built
in milestones, and `docs/roadmap.md` has the plan, the decisions behind it
and where it stands: M1 to M4 are done, which is the HTTP core, Inertia
pages with Vite, forms and validation, and the rest of the v3 protocol.
`README.md` is the front door. Change the pages with the behaviour.

## Commands

```bash
go test ./...                              # a few seconds, no network or Node
go test -race ./...                        # what CI runs; a server is concurrent
go test -run '^$' -bench . -benchmem .     # tug next to ServeMux alone
go vet ./... && gofmt -l .
ADDR=127.0.0.1:8080 go run ./examples/api
```

In `examples/inertia`:

```bash
npm install
npm run dev                                # Vite, writing public/hot while it runs
ADDR=127.0.0.1:8080 go run .               # the Go server, from this directory
npm run typecheck && npm run build
npx playwright test                        # after a build; uses the installed Chrome
```

Manual runs should set `ADDR=127.0.0.1:...`: the default `:8080` listens on
every interface, which sets off the macOS firewall prompt. A `public/hot`
left behind by a Vite that didn't exit cleanly points the Go server at a
dev server that isn't there: delete it.

## Architecture

- `tug`, the root package:
  - `app.go`: `Config` (`ConfigFromEnv` reads ADDR, PORT and APP_DEBUG),
    `App`, `Run` and `Serve` with graceful shutdown, and misses. At the first
    request, `freeze` adds `/` as a catch-all, unless a route already takes
    every path under every method. The catch-all answers trailing-slash
    redirects, 405s (probing the mux with the other methods for `Allow`) and
    404s, all through the ErrorHandler.
  - `router.go`: `Router`, `Route`, `URL`. A route goes into the ServeMux
    when it's added, so a bad or clashing pattern panics at the call that
    added it. Middleware chains are put together in `freeze`, so a group's
    `Use` after its routes still wraps them; after that, adding anything
    panics. Paths match exactly: `Handle` adds `{$}` to a path ending in
    `/`, and a group's `/` is the prefix itself. App middleware wraps the
    whole mux, so it sees 404s; group and route middleware wrap the route.
  - `ctx.go`: `Ctx`, the responses, `Param` and `Query`, and `Redirect`,
    which is a 302 after GET and a 303 after anything else, as Inertia needs.
  - `bind.go`: `Bind` reads the body (JSON, urlencoded, multipart), then the
    query, then path values, so the URL wins. A value that doesn't parse is
    a `*BindError` inside an `*HTTPError`: 400, or 404 for a path value. A
    field Bind can't fill at all is a plain error, a 500.
  - `errors.go`: `HTTPError`, `BindError`, `PanicError`,
    `DefaultErrorHandler`, and `errorPage`, which renders
    `Config.ErrorPage` with `RenderStatus` for browsers and Inertia's
    client, unless Debug is showing a 500's details. `adapt` in app.go
    recovers handler panics into `*PanicError`, and re-panics
    `http.ErrAbortHandler`.
  - `pages.go`: `Page[P]`, a string type whose type parameter ties a
    component to its props, `Ctx.Inertia` and `Ctx.Location`.
    `Config.Inertia` puts the Inertia middleware inside the App's own.
  - `forms.go`: `BindValid`/`Validate` (tags, then the handler's checks;
    a Precognition request is answered in `precognition` and returns
    `errAnswered`, which `adapt` keeps from the ErrorHandler),
    `answerInvalid` (flash the errors and go `back`, or a 422), and the
    glue between sessions and pages: `Flash`, `ClearHistory`,
    `PreserveFragment`, and `pageRequest`, which hands a render the errors
    and flash data from the session (`tug.errors`, `tug.flash`,
    `tug.clear_history`, `tug.preserve_fragment`) and unflashes what it
    shows. `carryFlash` sends them on when `Ctx.Redirect` redirects again;
    `keepFlashOnReload` reflashes before the 409 for another build.
    `Config.Session` puts the session middleware outside Inertia's.
- `inertia`: the v3 protocol for any net/http router, with no import of
  tug. `inertia.go` renders (HTML first visit, JSON after, `RenderStatus`
  for other statuses), and holds the middleware (Vary, the 409 for another
  build, and `redirects`: 302 → 303, and a redirect to a #fragment → 409
  with X-Inertia-Redirect) and the context helpers. `props.go` has the
  prop types: generic structs, each with its `behavior` (first load,
  always, deferred group, rescue, merge, once, scroll) behind the
  unexported interface `prop`, and function options (`MergeOption`,
  `OnceOption`). Its `resolver` follows inertia-laravel's `PropsResolver`,
  which is the reference when the spec is unclear. `level` walks one
  level of the props, `pathWanted` is the two-way partial-reload match,
  `labeled` decides which props get metadata, `leftOutOfFirstLoad`
  handles optional, deferred and once props, and `collect*` gathers the
  page's metadata. Props nest in maps and in structs whose type holds
  props (`holdsProps`); other values are data for encoding/json. Siblings
  resolve concurrently. `empty.go` copies whatever holds a nil slice or map
  so it goes out as `[]` or `{}`, caching which types can't hold one.
  `protocol_test.go` has a test for each rule M4 added.
- `session`: the cookie store, with no import of tug. AES-256-GCM with a
  key derived by HKDF from each of `Config.Keys`, the first encrypting;
  the cookie name is the AAD. `cookieWriter` sets the cookie when the
  response starts. Flash: `next` is what this request flashes, `now` what
  the one before did; values go through JSON.
- `validate`: `Struct` over one go-playground validator that names fields
  by json tag; `path` turns its namespace into dotted paths, and `message`
  its tags into sentences. `Errors` is `map[string]string`, first message
  per field.
- `vite`: dev-server tags while the hot file exists (read on each render),
  manifest tags otherwise, `Version` from the manifest's hash, `ServeHTTP`
  for the build. No import of tug or inertia; it meets them through
  template funcs.
- `internal/rw`: the ResponseWriter wrapper that records status and size,
  and keeps Flush, Hijack, ReadFrom and `Unwrap`.
- `middleware`: plain `func(http.Handler) http.Handler`, with no import of
  tug: `RequestID`, `Logger`, `Recover`, `CSRF`.
- `examples/api`: a JSON API on the core. Its tests are the end to end check.
- `examples/inertia`: React pages on tug. `main.go` embeds `app.html` and
  `public/` (the build lands in `public/build`; `.gitkeep` lets it compile
  before one). `main_test.go` runs without Node, against a fake manifest;
  `e2e/` drives the real build in a browser, in order: the later tests
  change the posts. `resources/js/types.ts` mirrors the Go props structs
  by hand, and tells Inertia the shared props and flash types.

tug logs through `slog.Default()` and never sets it; that's the app's call.

## Conventions

- **Comments say why**, and name the thing that went wrong when there was
  one. Test names are sentences about behaviour.
- **Error text is for the person who gets it**: "age must be a whole
  number", "post not found". No Go types, and no raw errors from deep down;
  details that aren't theirs go to the log.
- **tug stays on the standard library.** A dependency needs a reason; the
  one so far is go-playground/validator, behind package `validate`, which
  is also why Go 1.26 is the minimum.
- **The decisions in `docs/roadmap.md` are settled.** Ask before reopening
  one.
- **Commit subjects are one line**, imperative.
