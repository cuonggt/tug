# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

tug is a Go web framework for apps whose frontend is Inertia.js v3: Go
handlers render React pages with props, with no API in between. It is built
in milestones, and `docs/roadmap.md` has the plan, the decisions behind it
and where it stands: M1 to M7 are done, which is the HTTP core, Inertia
pages with Vite, forms and validation, the rest of the v3 protocol, the
CLI, v0.1.0 (the auth starter and the guide), and the auth starter made
whole: email verification, remember me, password confirmation, two-factor
logins, settings, and a Tailwind and shadcn/ui frontend.
`README.md` is the front door, and `docs/` the guide, a page per part of
tug. Change them with the behaviour.

## Commands

```bash
go test -short ./...                       # a few seconds, no network or Node
go test ./...                              # also makes an app of each kind with tug new: needs npm
go test -race ./...                        # what CI runs; a server is concurrent
go test -run '^$' -bench . -benchmem .     # tug next to ServeMux alone
go vet ./... && gofmt -l .
ADDR=127.0.0.1:8080 go run ./examples/api
```

The CLI, from a checkout, and in `examples/inertia`:

```bash
go build -o /tmp/tug ./cmd/tug            # not -trimpath: tug new finds this checkout by its own path
/tmp/tug new /tmp/blog && cd /tmp/blog && /tmp/tug dev    # -auth for the starter with accounts
go run ../../cmd/tug gen                  # write resources/js/tug again, after changing Go types
go run ../../cmd/tug dev                  # Vite and the app, rebuilt as it changes
npm install
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
    redirects itself, with a 307, and 405s (probing the mux with the other
    methods for `Allow`) and 404s through the ErrorHandler.
  - `router.go`: `Router`, `Route`, `URL`. A route goes into the ServeMux
    when it's added, so a bad or clashing pattern panics at the call that
    added it. Middleware chains are put together in `freeze`, so a group's
    `Use` after its routes still wraps them; after that, adding anything
    panics. Paths match exactly: `Handle` adds `{$}` to a path ending in
    `/`, and a group's `/` is the prefix itself. App middleware wraps the
    whole mux, so it sees 404s; group and route middleware wrap the route.
  - `ctx.go`: `Ctx`, the responses, `Param` and `Query`, and `Redirect`,
    which is a 302 after GET and a 303 after anything else, as Inertia needs;
    `RedirectBack` goes to the Referer when it's this site's (`back`, in
    forms.go).
  - `bind.go`: `Bind` reads the body (JSON, urlencoded, multipart), then the
    query, then path values, so the URL wins. A value that doesn't parse is
    a `*BindError` inside an `*HTTPError`: 400, or 404 for a path value. A
    field Bind can't fill at all is a plain error, a 500. A JSON body that
    fails to decode is read again by `jsonAsForm` (`jsonform.go`), with its
    strings for bools, numbers and times parsed as form values, as
    Inertia's `<Form>` sends every value as a string; it finds fields by
    `encoding/json`'s rules (`jsonFieldsOf`), and `bindJSON` restores `dst`
    before the second decode.
  - `errors.go`: `HTTPError`, `BindError`, `PanicError`,
    `DefaultErrorHandler`, and `errorPage`, which renders
    `Config.ErrorPage` with `RenderStatus` for browsers and Inertia's
    client, unless Debug is showing a 500's details. `adapt` in app.go
    recovers handler panics into `*PanicError`, and re-panics
    `http.ErrAbortHandler`.
  - `pages.go`: `Page[P]`, which declares a component with its props
    type in the registry tug gen reads (`declare`, `declaredPages`) and
    returns a `PageOf[P]` that renders only those props, `Ctx.Inertia`
    and `Ctx.Location`.
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
  the one before did; values go through JSON. `SetLifetime` gives one
  session a lifetime of its own, kept in the payload (`l`), which `Clear`
  drops.
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
- `internal/typegen`: TypeScript from reflect.Type, as encoding/json writes
  values: `pages.ts` (an interface per named struct, `SharedProps`,
  `Pages`, `PageProps`, and the `InertiaConfig` augmentation) and
  `routes.ts` (the route table, `Params`, `route()`). The prop types are
  found by package path and generic name (`propOf`). The app runs it: see
  `App.gen` in app.go, reached from Run when TUG_GEN names a file, and the
  page registry `declare`d by `tug.Page` in pages.go.
- `cmd/tug`: the CLI, on the stdlib flag package. `gen.go` builds the app
  into `.tug/app` and runs it with TUG_GEN; `dev.go` runs Vite and the app
  as processes in their own groups (`proc`, `proc_unix.go`), polls for
  changes (`watch`, `snapshot`), and touches `.tug/reload` for the
  starter's Vite plugin to reload the browser; `build.go`; `new.go`
  writes `starter/`, and with `-auth` lays `starter-auth/` over it (its
  files replace the ones of the same name), with the `.tmpl` files filled
  in between `[[ ]]` (two brackets in Go, as `OptionalProp[[]string]`,
  are written by a placeholder, `[[ "[[" ]]`); `notInAuth` lists the plain
  starter's files an auth app leaves out. An app requires the tug that
  made it when that's a release or was fetched by the go command
  (`release`, `fetched`: the build's module checksum), and otherwise
  `replace`s it with the checkout it was built from (`checkoutDir`). The starters' Go files are `.tmpl` so
  the go tool doesn't build them in place; `tug_test.go` makes a real app
  of each kind. The auth starter's handlers are in `auth.go.tmpl` (who's
  logged in, and the wrappers `usersOnly`, `verified`,
  `passwordConfirmed` and `guestsOnly`), `verify.go.tmpl`,
  `twofactor.go.tmpl` and `settings.go.tmpl`, its mail in `mail.go.tmpl`,
  and its users in SQLite (modernc.org/sqlite, pure Go), with migrations
  counted in `user_version`, in `users.go.tmpl`. Its frontend is Tailwind
  and shadcn/ui: the registry's components in `components/ui`, layouts
  picked by page name in `app.tsx`, toasts from the flash event.
- `middleware`: plain `func(http.Handler) http.Handler`, with no import of
  tug: `RequestID`, `Logger`, `Recover`, `CSRF`.
- `auth`: the parts of accounts where a slip is a security hole, with no
  import of tug and no idea what a user is. `password.go`: argon2id at
  OWASP's settings, PHC strings, a check against a decoy when there's no
  hash, and `hashing`, which runs one hash per CPU. `auth.go`: the login
  in the session (`tug.auth.id`, and `tug.auth.check`, a fingerprint of
  the password hash that `Current` compares), the intended page, the time
  the password was last confirmed (`tug.auth.confirmed`), and a login held
  back for its second factor (`tug.auth.pending`), which `Login` drops.
  `tokens.go`: tokens for links in mail, signed with an HKDF key for their
  purpose alone, over the expiry and their parts; `reset.go` (the ID and
  the password hash) and `verify.go` (the ID and the email) use it.
  `twofactor.go`: TOTP (RFC 6238, the last step used kept by the app),
  secrets and recovery codes sealed with AES-GCM under an HKDF key, and
  `Stale` for rotation. `throttle.go`: counts per key in a
  map, swept as it doubles; `Try` checks and counts under one lock, so
  tries at the same moment can't all get in.
- `mail`: `Message`, `SMTP` on net/smtp (STARTTLS, TLS on 465, deadlines
  from the context), `Log`, which writes mail out, and `FromEnv`.
- `examples/api`: a JSON API on the core. Its tests are the end to end check.
- `examples/inertia`: React pages on tug. `main.go` embeds `app.html` and
  `public/` (the build lands in `public/build`; `.gitkeep` lets it compile
  before one). `main_test.go` runs without Node, against a fake manifest;
  `e2e/` drives the real build in a browser, in order: the later tests
  change the posts. `resources/js/tug` is written by tug gen and committed
  (CI checks it's current); `resources/js/types.ts` has only the flash type.

tug logs through `slog.Default()` and never sets it; that's the app's call.

## Conventions

- **Comments say why**, and name the thing that went wrong when there was
  one. Test names are sentences about behaviour.
- **Error text is for the person who gets it**: "age must be a whole
  number", "post not found". No Go types, and no raw errors from deep down;
  details that aren't theirs go to the log.
- **tug stays on the standard library.** A dependency needs a reason; the
  ones so far are go-playground/validator, behind package `validate`,
  which is also why Go 1.26 is the minimum, and golang.org/x/crypto, for
  argon2id. The starters' apps can have their own, as the auth starter's
  SQLite driver and its npm packages (Tailwind, shadcn/ui's Radix, lucide,
  sonner, qrcode.react) are.
- **The decisions in `docs/roadmap.md` are settled.** Ask before reopening
  one.
- **Commit subjects are one line**, imperative.
