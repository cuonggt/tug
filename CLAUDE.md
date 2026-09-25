# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

tug is a Go web framework for apps whose frontend is Inertia.js v3: Go
handlers render React pages with props, with no API in between. It is built
in milestones, and `docs/roadmap.md` has the plan, the decisions behind it
and where it stands: M1, the HTTP core, is done. `README.md` is the front
door. Change the pages with the behaviour.

## Commands

```bash
go test ./...                              # a few seconds, no network
go test -race ./...                        # what CI runs; a server is concurrent
go test -run '^$' -bench . -benchmem .     # tug next to ServeMux alone
go vet ./... && gofmt -l .
ADDR=127.0.0.1:8080 go run ./examples/api
```

Manual runs should set `ADDR=127.0.0.1:...`: the default `:8080` listens on
every interface, which sets off the macOS firewall prompt.

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
    `DefaultErrorHandler`. `adapt` in app.go recovers handler panics into
    `*PanicError`, and re-panics `http.ErrAbortHandler`.
- `internal/rw`: the ResponseWriter wrapper that records status and size,
  and keeps Flush, Hijack, ReadFrom and `Unwrap`.
- `middleware`: plain `func(http.Handler) http.Handler`, with no import of
  tug: `RequestID`, `Logger`, `Recover`, `CSRF`.
- `examples/api`: a JSON API on all of the above. Its tests are the end to
  end check.

tug logs through `slog.Default()` and never sets it; that's the app's call.

## Conventions

- **Comments say why**, and name the thing that went wrong when there was
  one. Test names are sentences about behaviour.
- **Error text is for the person who gets it**: "age must be a whole
  number", "post not found". No Go types, and no raw errors from deep down;
  details that aren't theirs go to the log.
- **The core stays on the standard library.** A dependency needs a reason;
  go-playground/validator in M3 is the one planned.
- **The decisions in `docs/roadmap.md` are settled.** Ask before reopening
  one.
- **Commit subjects are one line**, imperative.
