# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

tug is a Go web framework for apps whose frontend is Inertia.js v3: Go
handlers render React, Vue or Svelte pages with props, with no API in
between. It is built in milestones, and `docs/roadmap.md` has the plan,
the decisions behind it and where it stands: M1 to M32 are done, which is
the HTTP core, Inertia pages with Vite, forms and validation, the rest of
the v3 protocol, the CLI, v0.1.0 (the auth starter and the guide), the
auth starter made whole (v0.2.0): email verification, remember me,
password confirmation, two-factor logins, settings, and a Tailwind and
shadcn/ui frontend, and background jobs (v0.3.0): package `queue`, which
the auth starter sends its mail with, jobs on a schedule (v0.4.0),
server-side rendering (v0.5.0): package `ssr`, with Node beside the app,
and `tug new -ssr`, tests of an app's pages (v0.6.0): package `tugtest`,
which the starters' tests use, cron in time zones and unique jobs
(v0.7.0), in package `queue`, passkeys (v0.8.0): `auth.Passkeys`, and in
the auth starter, Vue and Svelte starters (v0.9.0): `tug new -vue` and
`-svelte`, and a browser suite that drives the auth starter in each, and
the queue made whole (v0.10.0): jobs pushed in the app's own transaction,
a unique job at its latest push's time or one at a time, and the jobs
that failed listed and run again by the auth starter's `jobs` command,
files (v0.11.0): package `storage`, a local disk or S3, with signed
links, `validate`'s tags for uploads, `tugtest.File`, and the auth
starter's profile photo, Postgres and MySQL (v0.12.0): `tug new
-auth -postgres` and `-mysql`, the auth starter's SQL in a layer per
database, jobs claimed with `SKIP LOCKED`, and `tug.Generating`, and
throttles across instances (v0.13.0): `auth.ThrottleStore`, the auth
starter's throttles counted in its database, and `tug.Limit`, a limit
for a route, proxies and signed links (v0.14.0):
`middleware.TrustProxies` and `c.IP`, `Config.URL` from `APP_URL`, which
`tug dev` sets, `AbsoluteURL`, and `SignedURL` with `tug.Signed`,
languages (v0.15.0): package `lang`, what tug says, a form's errors among
it, and `c.T`, in the request's language, fields named by their `label`
tags or their keys in words, `validate.Rule`, and `tug lang`,
pagination (v0.16.0): `tug.Paginate`, `SimplePaginate` and
`CursorPaginate`, with the app's own queries, commands (v0.17.0):
`app.Command`, which `Run` runs in place of serving, as the auth
starter's `jobs`, cache and locks (v0.18.0): package `cache`, with
`cache.Remember`, and locks that hold across instances, and the auth
starter's cache in its database, and mail made whole (v0.19.0): copies,
a `Bcc` no one sees, replies to another address, files, the app's own
headers, and a link to unsubscribe in one click, in `mail.Message`, and
`mailtest.Outbox`, which the auth starter's tests use, and downloads and
streams (v0.20.0): `c.File` and `FileFS`, with ranges, `c.Download`, a
file to save by its name however it's written, `c.Stream` and
`StreamDownload`, a body made as it's sent, and `c.Events`, server-sent
events, which the app's shutdown ends, and encryption (v0.21.0):
package `crypt`, the app's own values sealed under a key for their
purpose alone, bound to their owner, with `Stale` for a rotation,
`auth.TwoFactor` sealing the same way, and `tug key`, and API tokens
(v0.22.0): `auth.AccessTokens`, kept as their hashes, `middleware.CORS`,
`CSRF`'s paths let through, and in the auth starter, a settings page of
tokens, and `/api`, which takes one in place of a login, and
broadcasting (v0.23.0): package `broadcast`, events on channels, to the
pages that follow them on every instance, through `c.Events`, carried by
the app's database, `NOTIFY` or a table, in its transaction or not at
all, and in the auth starter, the page that asks to verify the email
moving on once it's verified elsewhere, and the queue further (v0.24.0):
a kind's jobs so many at once, `queue.AtOnce`, or so many a time,
`queue.Rate`, through a `Limiter`, as `auth.Throttle`, on every instance
together, and `queue.OnFail`, when a job has failed for good, and
authorization (v0.25.0): `auth.NewAbility`, what a user may do with a
thing, whose no, an `auth.Denial`, tug answers with a 403 by its
`StatusCode`, and an `auth.Gate` asked first, and in the auth starter,
admins, whom the `admins` command makes, and their page of the jobs that
failed, and notifications (v0.26.0): in the auth starter, each change to
an account that could hand it to someone else kept in its transaction,
heard on the user's channel by the header's bell, listed, and mailed, the
old email told of a new one, and `mailtest.Outbox`'s `NextTo`, and
security headers (v0.27.0): `middleware.Headers`, and `middleware.CSP`,
a Content-Security-Policy that runs the scripts with a nonce made for the
response, which inertia hands the root template as `.Nonce`, and Vite's
tags take first, and logs the reports it answers itself, and the
starters' pages under it. `README.md` is the front door, and `docs/` the guide, a page per part of
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

Package storage's S3 tests run against a MinIO when `TUG_TEST_S3` names
one, as CI's does, and are skipped otherwise. MinIO's own images are
gone from Docker Hub, and Bitnami's last is kept:

```bash
docker run -d --rm --name tug-minio -p 127.0.0.1:9000:9000 -e MINIO_ROOT_USER=tug \
  -e MINIO_ROOT_PASSWORD=tug-secret-key bitnamilegacy/minio:2025.7.23-debian-12-r5
TUG_TEST_S3=http://tug:tug-secret-key@127.0.0.1:9000 go test ./storage
```

The CLI's tests make the auth starter on Postgres and on MySQL, and run
its tests on the servers `TUG_TEST_POSTGRES` and `TUG_TEST_MYSQL` name,
as CI's do, and skip them otherwise. Ports of their own, as this machine
may have a MySQL on 3306:

```bash
docker run -d --rm --name tug-postgres -p 127.0.0.1:55432:5432 -e POSTGRES_PASSWORD=secret postgres:18
docker run -d --rm --name tug-mysql -p 127.0.0.1:53306:3306 -e MYSQL_DATABASE=tug -e MYSQL_ROOT_PASSWORD=secret mysql:8.4
TUG_TEST_POSTGRES='postgres://postgres:secret@127.0.0.1:55432/postgres?sslmode=disable' \
  TUG_TEST_MYSQL=mysql://root:secret@127.0.0.1:53306/tug go test -run OnPostgresOrMySQL ./cmd/tug
```

An app made with `-postgres` or `-mysql` runs its own tests on the
server its `DB_URL` names, `DB_URL=... go test ./...` in the app.

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

The browser suite of the auth starter, in `cmd/tug/e2e`, which makes an
app of each frontend with tug new and runs it as it's deployed:

```bash
npm install && npx playwright test                           # minutes the first time: it makes three apps
TUG_E2E_DIR=/tmp/tug-e2e FRONTENDS=vue npx playwright test   # keeps the apps for the next run; one frontend
```

Manual runs should set `ADDR=127.0.0.1:...`: the default `:8080` listens on
every interface, which sets off the macOS firewall prompt. A `public/hot`
left behind by a Vite that didn't exit cleanly points the Go server at a
dev server that isn't there: delete it.

## Architecture

- `tug`, the root package:
  - `app.go`: `Config` (`ConfigFromEnv` reads ADDR, PORT, APP_DEBUG and
    APP_URL; `New` panics on a `URL` that `appURL`, in links.go, doesn't
    take), `App`, `Run` and `Serve` with graceful shutdown, and misses. At
    the first request, `freeze` adds `/` as a catch-all, unless a route
    already takes every path under every method. The catch-all answers
    trailing-slash redirects itself, with a 307, and 405s (probing the mux
    with the other methods for `Allow`) and 404s through the ErrorHandler.
    `Go` adds work to run beside the server (`background`): `Serve` starts
    it with a context canceled as shutdown begins, waits for it, and shuts
    down when it fails first; `stopped` drops its `context.Canceled`.
    `stopping`, the App's own context, is done as `Serve` shuts down, for
    the event streams, which `Shutdown` would otherwise wait for.
    `Generating` says tug gen started the app (`TUG_GEN`), for `main` to
    leave out what only serving needs, as the auth starter's database.
  - `router.go`: `Router`, `Route`, `URL`. A route goes into the ServeMux
    when it's added, so a bad or clashing pattern panics at the call that
    added it. Middleware chains are put together in `freeze`, so a group's
    `Use` after its routes still wraps them; after that, adding anything
    panics. Paths match exactly: `Handle` adds `{$}` to a path ending in
    `/`, and a group's `/` is the prefix itself. App middleware wraps the
    whole mux, so it sees 404s; group and route middleware wrap the route.
  - `ctx.go`: `Ctx`, the responses, `Param`, `Query` and `IP` (the
    RemoteAddr's address, which `middleware.TrustProxies` has made the
    client's), `Locale` (`App.Locale`'s, in app.go: `Config.Locale`, the
    app's choice, then `Accept-Language`, then `Config.Lang`'s default;
    kept once picked), `T` and `Choice` (through `words`), and
    `Redirect`, which is a 302 after GET and a 303 after anything else, as
    Inertia needs; `RedirectBack` goes to the Referer when it's this
    site's (`back`, in forms.go).
  - `bind.go`: `Bind` reads the body (JSON, urlencoded, multipart), then the
    query, then path values, so the URL wins. A value that doesn't parse is
    a `*BindError` inside an `*HTTPError`: 400, or 404 for a path value. A
    field Bind can't fill at all is a plain error, a 500. A JSON body that
    fails to decode is read again by `jsonAsForm` (`jsonform.go`), with its
    strings for bools, numbers and times parsed as form values, as
    Inertia's `<Form>` sends every value as a string; it finds fields by
    `encoding/json`'s rules (`jsonFieldsOf`), and `bindJSON` restores `dst`
    before the second decode. `Bind` says its messages in the request's
    language (`said`, `bindMessage`: ":field " and one of `bindReasons`),
    naming the field by its `label` tag (`field.label`, or `jsonLabel`
    down a JSON error's path, which leaves out indexes) or its key's last
    part in words.
  - `texts.go`: `texts`, what tug says to a person, in English, with
    validate's, which `App.gen` writes beside the types, for tug lang.
  - `errors.go`: `HTTPError`, `BindError`, `PanicError`,
    `DefaultErrorHandler`, which answers a `statusError`, an error with a
    `StatusCode`, as auth's `Denial`, with its status, and its words under
    500, and `errorPage`, which renders
    `Config.ErrorPage` with `RenderStatus` for browsers and Inertia's
    client, unless Debug is showing a 500's details. `adapt` in app.go
    recovers handler panics into `*PanicError`, and re-panics
    `http.ErrAbortHandler`.
  - `commands.go`: `Command` adds one of the app's commands (`command`,
    in the App's `commands`), which `Run` runs in place of serving
    (`runCommand`) when it has any and the binary has an argument, with
    the rest and `Run`'s signal context; `help` lists them
    (`listCommands`), and a name that isn't one is an error. What `Go`
    runs doesn't start, and tug gen's run writes the types first.
  - `limit.go`: `Limit`, a HandlerFunc wrapper, not middleware, so a
    refusal is a 429 `*HTTPError` for the ErrorHandler, with
    `Retry-After`; it takes a `Limiter`, the `Try` `*auth.Throttle` has,
    so the core imports no auth.
  - `links.go`: links that leave the app. `AbsoluteURL` is `Config.URL`,
    never the request's Host, and `URL`'s path; `SignedURL` adds
    `expires` and `signature` (`signLink`: HMAC-SHA256 with an HKDF key,
    "tug signed link", from the first of `Config.Keys`, over the escaped
    path and the expiry, each after its length, as storage's links are);
    `Signed`, a wrapper like `Limit`, lets only such a link through
    (`checkSigned`: each key, then the expiry; a query with anything else
    in it fails, as Bind would read it), and is a 500 with no keys.
  - `paginate.go`: `Paginate`, `SimplePaginate` and `CursorPaginate`,
    over the app's own count and fetch (limit and offset), or its
    cursor, as JSON in base64url, and the three types they return, with
    Laravel's keys; `pageNumber` reads `?page` (1 unless more),
    `pageLinks` makes paths with the request's query and the page
    changed, and `pageWindow` Laravel's pager window, three on each side;
    `span` is `from` and `to`, and the items never nil; `Paging` of each
    is where it sits for `inertia.Scroll`; `PageName` names another
    parameter.
  - `files.go`: `File` and `FileFS` (`serveFile`: `http.ServeContent`,
    with nosniff; `missing` is a 404 for a file that isn't there, or a
    name that isn't one, and a directory is too), and `Download`
    (`attachment`, the Content-Disposition, by `mime.FormatMediaType`, of
    `baseName`, the name after its last slash or backslash; content that
    seeks goes by `ServeContent`, with its `Stat`'s time, and the rest by
    `sendAll`, typed by `typeOf`: the extension, or else the first bytes).
  - `stream.go`: `Stream` (`streamWriter` flushes each write; an error
    before the first is the handler's, with the Content-Disposition
    dropped, after it a log line and `http.ErrAbortHandler`, unless the
    client went), `StreamDownload`, and `Events` (headers flushed at once,
    `send` through `Event.encode`, a data line always, a comment every
    `keepAlive`, 20 s, from a goroutine, both through one lock, and a
    context done by the client, fn's end, or the App's `stopping`; fn's
    error is returned unless that context was done).
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
  for other statuses; a first visit asks `Config.SSR`, a `Renderer`, for
  the page's head and body, and keeps the browser's when it has none or
  fails, which it logs; `WithoutSSR` skips it; `TemplateData.Nonce` is
  the context's nonce, `internal/nonce`'s, which `nonced` gives the scripts
  of the head from SSR), and holds the middleware (Vary, the 409 for another
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
  by json tag, or else form tag; `path` turns its namespace into dotted
  paths, and `message` its tags into sentences, in the `lang.Words`
  `Struct` is given, or English: its texts are `plain`, `valued` and
  `bounds` (a text's characters, a list's items, a number), with a form
  for one and for more; the field is named by `internal/label`
  (`fieldName`, `sibling` for eqfield's other, both through
  `structField`). `Rule[T]` registers a tag of the app's under `mu`,
  which `Struct` reads under, as go-playground takes one only before it
  checks; `rules` keeps their messages, and `Texts` lists them all.
  `Errors` is `map[string]string`, first message per field. Two tags are
  tug's own, for a `*multipart.FileHeader` (`uploaded`): `file_max`
  (`sizeLimit` reads `2MB` as `2` and `MB`) and `file_type`, the type by
  the file's first bytes; a type sniffing can't tell, a size that isn't
  one, or a field that isn't an upload panics.
- `lang`: an app's languages, with no import of tug. `lang.go`: `Load`
  reads a JSON file per language, keyed by the English (`canonical`
  tags, `validTag`), into a `Catalog` whose languages are the default and
  the files; `Find` resolves a tag (exact, its language, a region of it),
  `Match` an `Accept-Language` (`accepted`, by weight); `In` is a
  language's `Words`, whose `T` and `Choice` `lookup` the text in the
  language, its language without the region, `en.json`, or else say the
  English, and `fill` its `:name` placeholders (`value`, with `:Name` and
  `:NAME`) in one pass. `plural.go`: `choose` a form of `|` for a count,
  a form's own counts (`interval`, `{0}`, `[2,*]`) first, then
  `pluralIndex`, Laravel's rules by language (`pluralRules`). A nil
  Catalog, and the zero Words, are English.
- `crypt`: the app's own values, sealed, with no import of tug. `New`
  makes a `Box` for a purpose, whose key is derived with the info `tug
  crypt ` and the purpose, so none is one of tug's own; `Seal`, `Open`
  (`ErrOpen`) and `Stale` take an owner, whose parts, each after its
  length and a colon (`ad`), are the AEAD's additional data.
- `internal/seal`: AES-256-GCM under a key HKDF derives from each of the
  app's with an info, the first sealing and each opening (`Open` says
  which), the nonce first, in base64url; `crypt` and `auth.TwoFactor`
  seal with it.
- `internal/label`: how a message names a field: `Of`, its label tag or
  `Readable`, its key in words (`first_name` and `firstName` as "first
  name", `URLPath` as "url path"), which validate, Bind and tug lang
  share.
- `storage`: files, with no import of tug. `storage.go`: `Disk` (`Put`,
  `Open`, `Delete`, `URL`), `checkKey` (an `fs.ValidPath`, no backslash),
  `PutUpload`, which keeps an upload under a random key with its sniffed
  type's extension, and `exactly`, which fails a put whose bytes don't all
  come. `local.go`: `Local`, a directory, through an `os.Root`, put by a
  temp file, synced and renamed; its `ServeHTTP` is the route, which checks
  a private link's signature (`sign`, HMAC with an HKDF key over the
  route's path, the key and the expiry, like auth's tokens) and serves the
  file as its sniffed type, with nosniff, `Content-Security-Policy:
  sandbox`, and anything but an image as an attachment. `s3.go`: `S3`,
  SigV4 on the standard library (`sign`, the Authorization header, every
  header signed; `presign`, the query), a put streamed as
  UNSIGNED-PAYLOAD, `failure` reading S3's XML errors; a private link is
  presigned for the seven days that end at its expiry, so the same expiry
  makes the same link; `Origin` is the address its links are at, for a
  page's policy. `env.go`: `FromEnv`, Laravel's variables. Its tests
  check SigV4 against AWS's published examples (`sigv4_test.go`), and run
  against a MinIO (`minio` in `s3_test.go`) when `TUG_TEST_S3` names one.
- `internal/filetype`: what a file is by its first 512 bytes, as
  `http.DetectContentType` says, without parameters (`Sniff`, `Of`), and
  what's known of each type it tells (`kinds`): its name for a message
  (`Names`: "a PNG or JPEG image"), its extension, and whether it's an
  image a browser shows. validate and storage share it.
- `vite`: dev-server tags while the hot file exists (read on each render),
  manifest tags otherwise, `Version` from the manifest's hash, `ServeHTTP`
  for the build, and `DevServer`, the dev server's URL, for package ssr
  and `middleware.CSP`. No import of tug or inertia; it meets them through
  template funcs, `vite` and `viteReactRefresh`, which take the page's
  nonce first (`Tags`, `ReactRefresh`; `isNonce` refuses an entry, as a
  template from before names first), for the scripts and preloads to
  carry (`nonceAttr`).
- `internal/nonce`: the response's Content-Security-Policy nonce in the
  request's context, which `middleware.CSP` sets and inertia reads,
  neither importing the other.
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
  into `.tug/app` and runs it with TUG_GEN (`runForGen`, which reads back
  the types and tug's texts, `generated`); `lang.go` is tug lang: tug's
  texts from that run, and the app's own from its Go (`appTexts`: `T` and
  `Choice` literals, and `fieldTexts`, the names of fields with validate,
  form, query or path tags, eqfield's others, and file_type's names),
  added to `lang/<lang>.json` (`addTexts`, sorted, only when it adds);
  `key.go` is tug key, which prints `newKey`, as `tug new`'s `.env` has
  one; `dev.go` runs Vite and the app
  as processes in their own groups (`proc`, `proc_unix.go`), polls for
  changes (`watch`, `snapshot`), touches `.tug/reload` for the starter's
  Vite plugin to reload the browser, and shows 127.0.0.1 as localhost
  (`shown`), where browsers make passkeys, which is the app's `APP_URL`
  unless it has one (`devEnv`), and rebuilds on `lang/*.json` too
  (`watched`); `build.go`; `new.go`
  (`writeStarter`) lays directories over each other, a later one's files
  replacing an earlier one's of the same name: `starter/`, the Go and what
  every frontend uses, then the frontend's own, `react/`, `vue/` or
  `svelte/` (`-vue`, `-svelte`); with `-auth`, `starter-auth/`, then
  `react-auth/`, `vue-auth/` or `svelte-auth/`, then the database's,
  `sqlite/`, `postgres/` or `mysql/` (`-postgres`, `-mysql`), whose
  conditions are `[[ if .SQLite ]]` and the rest; `DBName` is the app's
  name as SQL takes it, and `DevURL` compose.yaml's database, which the
  `.env` tug new writes names. The `.tmpl` files are
  filled in with `starterData` between `[[ ]]` (two brackets in Go, as
  `OptionalProp[[]string]`, are written by a placeholder, `[[ "[[" ]]`,
  and a bracket before an action, as `plugins: [react()`, by trimming the
  space between them, `[ [[- .Frontend ]]()`); `notInAuth` lists the plain
  starter's files an auth app leaves out, and `onlySSR` those only an app
  with `-ssr` has, by the frontend's extensions (`Script`, `Component`).
  `-ssr` is `[[ if .SSR ]]` in the templates, and a frontend
  `[[ if .Vue ]]`, written `[[- if ]]` before an indented line, as `-]]`
  would eat its indent. A shared file that differs between frontends by a
  line takes a condition, as `app.html` (`viteReactRefresh` is React's
  alone) and `vite.config.ts` do; one that's a frontend's own is in its
  layer.
  `tug dev` gives the app `TUG_DEV=1`, so an SSR app leaves rendering to
  Vite. An app requires the tug that made it when that's a release or was
  fetched by the go command (`release`, `fetched`: the build's module
  checksum), and otherwise `replace`s it with the checkout it was built
  from (`checkoutDir`). The starters' Go files are `.tmpl` so the go tool
  doesn't build them in place; `tug_test.go` makes a real app of each
  kind, React's four and two each of Vue's and Svelte's, and runs an SSR
  one's binary for a page rendered on the server, and one on each of
  Postgres and MySQL, which writes its types with no database running. Every starter makes its
  app in `resources/js/inertia.tsx` (`.ts` in Vue and Svelte:
  `createApp`), which `app.tsx`, the browser's, and `ssr.tsx`, the
  server's, call. Vue and Svelte are on TypeScript 6, as vue-tsc and
  svelte-check need its compiler API, which 7 hasn't; a Vue page's props
  are `defineProps<Pages['Name'] & SharedProps>()`, as Vue's compiler
  can't resolve `PageProps<'Name'>`. `e2e/` is one Playwright suite for
  the auth starter in every frontend (`setup.ts` makes the apps with tug
  new and runs them, their mail in `app.log`; `apps.ts` has the ports and
  `dotEnv`; `tests/helpers.ts`'s `command` runs an app's command, as
  `admins add`):
  the three are one app, word for word, so a change to one frontend is
  made to all three. The auth starter's handlers are in `auth.go.tmpl`
  (who's logged in, and the wrappers `usersOnly`, `verified`,
  `passwordConfirmed` and `guestsOnly`), `verify.go.tmpl`,
  `twofactor.go.tmpl`, `passkeys.go.tmpl` (the `passkeys` table, and the
  handlers of adding them, logging in and confirming with them, on
  `auth.Passkeys`, whose site is `APP_URL`'s (`passkeySite`), as mail's
  links are; the browser's side is `resources/js/lib/passkeys.ts`),
  `settings.go.tmpl` and `photos.go.tmpl` (a user's photo, on the disk
  `main` makes with `storage.FromEnv`: `files/`, or `FILES_PATH`, served
  at `/files` by its own route, or S3; `updatePhoto` puts the file before
  the transaction that names it, and deletes it when that fails, and the
  photo it replaces goes by a `delete-file` job pushed in that
  transaction; `withPhoto`, which `a.user` calls, gives each page's user a
  link signed to the end of the next day, the same all day, for the
  browser's cache), its mail in `mail.go.tmpl`,
  and its users in the database its layer has, SQLite
  (modernc.org/sqlite), Postgres (pgx) or MySQL (go-sql-driver), all pure
  Go. `users.go.tmpl`, `passkeys.go.tmpl` and `jobs.go.tmpl` have the
  types and the Go that's the same on each, and every layer the same
  files of SQL: `db.go.tmpl` (`dbFromEnv`, from `DB_PATH` or `DB_URL`,
  `openDB`, and the migrations, counted in `user_version`, or in
  `schema_version` under Postgres's row lock or MySQL's `GET_LOCK`, with
  `running` for the step under way, and `taken`, a unique index's error),
  `users_db.go.tmpl`, `passkeys_db.go.tmpl`, `jobs_db.go.tmpl`,
  `db_test.go.tmpl` (`testDB`, on a server a database per test, made on
  `DB_URL`'s or compose's and dropped, `jobsDown`, `failedDaysAgo`, and
  the migrations' tests), and on a server, `compose.yaml.tmpl`. The jobs
  are in the same database: `jobs_db.go.tmpl` is a `queue.ScheduleStore`
  and a `queue.UniqueStore`, which `jobs_test.go.tmpl` runs
  `queuetest.TestStore` on, with a `schedules` table whose upsert only
  moves forward, in a transaction with the job, and a `unique_key` column
  under a unique index, which the claim clears, and an `alone_key`
  column, a `OneAtATime` job's key for its whole life, which `claimable`,
  the claim's condition, checks no held job of its kind has. Postgres's
  and MySQL's claims are transactions, `FOR UPDATE SKIP LOCKED`, and lock
  a `job_locks` row of a `OneAtATime` job's key, then look again
  (`claim`, `again`); MySQL's connections are `READ COMMITTED` in UTC
  (`config`), and its `PushLatest` moves a job by its ID, not by an
  upsert, for its locks' order. It's an `AtOnceStore` too (`KeepsAtOnce`
  in `jobs.go.tmpl`): an `at_once` column, which `claimable` compares with
  a count of its kind's held jobs, by the `jobs_held` index, and which
  Postgres's and MySQL's claims count again under the kind's own
  `job_locks` row, its key empty, after a key's; and a `HoldBackStore`:
  `HoldBack` upserts the kind's `held_kinds` row, only later, which
  `claimable` checks, then takes the claim's attempt back, and gives the
  key back, unless another job has it (SQLite's one statement, or the
  others' `taken` fallback). MySQL's `claimable` counts the held kinds, as
  a `NOT EXISTS` there becomes a join whose sort locks every job it reads.
  `prune` deletes the held kinds whose time has passed. `jobs.in(tx)` is the Store that pushes in
  a handler's transaction, and `jobsCommand` the `jobs` command, which
  `newApp` adds with `app.Command`, for `Run` to run in place of the
  server.
  Under tug gen, `main` opens no database, and `env.DB` is nil. `a.inTx` runs a handler's writes in one
  transaction, through the stores' `in(tx)` (the tables' methods go
  through `dbtx`, the database or a transaction), and wakes the queue
  after the commit: registering, adding a passkey and a new email push
  their mail in the transaction that writes what it's about;
  `prune-jobs` runs every night and deletes jobs that failed a month ago.
  Its five `auth.Throttle`s, each named, count in the database: every
  layer's `throttles_db.go` is `throttles`, an `auth.ThrottleStore` (an
  upsert with `RETURNING`, or in MySQL a transaction that reads back),
  whose `prune` runs every hour as `prune-throttles`; `throttles_test.go`
  runs `throttletest.TestStore` on it, and two instances on one database
  share a login's count. Every layer's `cache_db.go` is `cacheTable`, a
  `cache.Store` (`Add` an upsert that replaces only a value that has
  expired: `ON CONFLICT ... DO UPDATE ... WHERE`, or MySQL's `IF`s, whose
  row unchanged counts as none affected), which `newApp` gives the app as
  `a.cache`, keeping nothing in it itself, and whose `prune` runs every
  hour as `prune-cache`; `cache_test.go` runs `cachetest.TestStore` on
  it, and two caches on one database share a value and a lock. Every
  layer's `tokens_db.go` is the `access_tokens` table's (`apiTokens`,
  whose Go is in `tokens.go.tmpl`: a token's hash, whose, its name, its
  abilities, space separated, and its last use and expiry in Unix
  milliseconds, `unixMilli`, `fromUnixMilli`; `used` writes at most once
  a minute): the settings page `Settings/Tokens` makes a token
  (`createToken`, shown once in the flash's `token`), lists and revokes
  them, and `a.tokenUsers(ability, h)` makes a route of `/api`, which
  reads the token alone (`sentToken`), 401 without one, 403 without the
  ability, 60 a minute by the token's ID (`apiRequests`); `tokenPrefix` is
  `starterData.TokenPrefix`, the app's name in letters and digits, and
  `abilities` lists what the page offers, `user:read`, which `/api/user`
  asks for. `tokens_test.go` and `e2e/tests/tokens.spec.ts` test it.
  `abilities.go.tmpl` has the app's `gate`, which lets a user whose
  `admin` column is set do anything, `seeFailedJobs`, `Can`, which
  `shareAuth` shares as `can` beside `auth` (`can`), and `only`, the
  wrapper of a route by an ability of no thing; `admin.go.tmpl` has
  `Admin/FailedJobs` (`failedJobsPage`, `retryFailedJob`,
  `retryFailedJobs`, through the `jobs` FailedStore, waking the queue) at
  `/admin/failed-jobs`, in each frontend's nav for an admin, and
  `adminsCommand`, the `admins` command (`users.setAdmin`, `admins`, in
  each layer's `users_db.go`); `admin_test.go` and
  `e2e/tests/admin.spec.ts` test them.
  `notifications.go.tmpl` has the `notifications` table's Go (each
  layer's `notifications_db.go`: a user's notifications, their kind,
  `noticeData` as JSON, and times in Unix milliseconds): `notices`, each
  kind's line, made as it's read with `c.T`, its page, and its mail, which
  `mailNotification` sends by the `notification-mail` job (`someKind` for
  a kind the version has none of); `a.notify`, which a handler calls in
  its transaction, keeping the notification, publishing `notification`
  on the user's channel, and pushing the mail; `Notifications`, the page
  at `/notifications`, `SimplePaginate`d, which marks read what it shows
  (`markRead`, by the IDs' range); and `Bell`, shared as `bell`, the
  unread count. A new password, in the settings or by a reset, a new
  email, told at the old one, two-factor logins off, a passkey added (its
  own mail's job gone) and a token made notify; `prune-notifications`
  deletes what was read 90 days ago. The browser's side: each layout's
  bell, and `resources/js/lib/broadcasts.ts`'s `listen`, one connection
  to `/broadcasts` a page, whose listeners reload as their event comes
  and as it connects again, as the verify page, the bell and the list do.
  A reload is a visit, and empties the page's flash, so the tokens page
  and the recovery codes keep, in their own state, what came in it.
  `notifications_test.go` and `e2e/tests/notifications.spec.ts` test it.
  Every layer's `broadcasts_db.go` is `broadcasts`, a `broadcast.Store`,
  which `main` gives the `broadcast.Hub` it runs with `app.Go`, and
  `newApp` the app as `a.hub`, with `a.broadcasts` for `in(tx)`: in
  Postgres a `pg_notify` on `tug_broadcasts`, which `Listen` hears on a
  connection it takes from the pool (`Raw`, pgx's `WaitForNotification`)
  and discards (`driver.ErrBadConn`); in SQLite and MySQL a `broadcasts`
  table read every `pollEvery`, 250 ms, past the last ID read, from its
  `MAX(id)` as it starts, which `prune` empties of what's a minute old
  every minute as `prune-broadcasts` (not in Postgres); MySQL's IDs
  can commit out of order, so an ID below the `highest` read is
  `missing`, waited for `gapWait`, 10 s, from when it was first missed,
  then passed over, and `seen` hands each on once. `broadcasts.go.tmpl`
  is the user's own channel, `userChannel`, `users.` and the ID, at
  `/broadcasts` (`userEvents`), which `verifyEmail` publishes `verified`
  on, and each frontend's `Auth/VerifyEmail` follows with `EventSource`,
  visiting the dashboard as it hears it, and reloading as it connects
  again; `broadcasts_test.go` runs `broadcasttest.TestStore` on it, and
  `e2e/tests/accounts.spec.ts` follows the link in another tab. The
  throttles count by `c.IP()`, behind the proxies `TRUSTED_PROXIES`
  names, which both starters hand `TrustProxies`; the links in mail are
  `a.routes.AbsoluteURL`'s, and `newApp` stops without `APP_URL`, but
  under tug gen. Both starters send `middleware.Headers`, HSTS by an
  https:// `APP_URL`, and `middleware.CSP`, enforced, reporting to
  `/csp-reports`, with Vite's `DevServer`, and the auth starter's bucket's
  `Origin` in `img-src`; their `app.html`'s scripts, and Vite's tags, carry
  `.Nonce`. `e2e/tests/headers.spec.ts` checks the headers, and a script
  the page didn't bring blocked, and reported to the app's log, and
  `tug_test.go`'s `rendersOnTheServer` a served page's scripts' nonce.
  Both starters embed `lang/` (a
  `.gitkeep` until there's a file) and load it in `newApp`, with
  `APP_LOCALE` for the default.
  The mail goes by jobs (`VerifyMail` in `verify.go.tmpl`, `ResetMail` in
  `auth.go.tmpl`) that carry IDs and make the mail, token and all, as they
  run; `main` runs the queue with `app.Go`, unless `QUEUE_WORKERS` is 0,
  and the tests on their own, with a `mailtest.Outbox` for the mail,
  which can be down. Its frontend is Tailwind and shadcn/ui: the
  registry's components in `components/ui`, layouts picked by page name
  in `inertia.tsx`, toasts from the flash event. Vue's and Svelte's are
  shadcn-vue's and shadcn-svelte's (its classic registry,
  `COMPONENTS_REGISTRY_URL`, as the CLI's default is its newer styles),
  with the toasts mounted by `app.ts` beside the app, as their Inertia
  has no `withApp` for a component; Vue's Input.vue takes the value
  Inertia's Form sets, and both register pages drop a waiting
  Precognition check as the form is sent, as `guestsOnly`
  redirects it once registering has logged the browser in, and as they
  go, as their Form, in 3.7.1, reads the form that's gone. An avatar is
  keyed by the user's photo, in all three, as an avatar keeps the image it
  loaded once the image is gone.
- `middleware`: plain `func(http.Handler) http.Handler`, with no import of
  tug: `Headers` (`headers.go`: nosniff, `Referrer-Policy`,
  `Cross-Origin-Opener-Policy`, `X-Frame-Options`, and HSTS when its config
  says), `CSP` (`csp.go`: `policy`, the directives, which `Sources` adds
  to, with a nonce, `rand.Text`, in script-src for each response, in its
  context through `internal/nonce`, which `NonceFrom` reads, the dev
  server's sources, `devSources`, from `DevServer` at each request, and
  `logReport`, which answers a POST to `ReportPath`, report-uri's, before
  any route; `ReportOnly` sends it as `-Report-Only`), `RequestID`,
  `Logger`, `Recover`, `CSRF` (an entry that's a path is
  `CrossOriginProtection`'s bypass, `bypass`, and the rest trusted
  origins), `CORS` (`cors.go`: `isOrigin`; a preflight from an origin
  named answered with a 204 before any route; credentials never), and
  `TrustProxies`
  (`proxies.go`), which rewrites a copy of the request's `RemoteAddr` to
  the client's, port 0, read from the end of `X-Forwarded-For` past the
  ranges named (`trusted.client`); `*` believes the peer alone, an entry
  that isn't an address stops the walk at the last proxy, and IPv4 in
  IPv6 is unmapped on both sides.
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
  secrets and recovery codes sealed by `internal/seal` (`box`, with the
  info `tug two-factor`, as before it, so what it sealed opens), and
  `Stale` for rotation. `throttle.go`: `Throttle`, whose `Try`, `Wait`
  and `Clear` take a context and return the store's error, over a
  `ThrottleStore` (`Hit` counts and reads back in one step, `Tries`,
  `Clear`), given the SHA-256 of the throttle's `Name` and the key
  (`hash`), and the app's clock; without a `Store`, `memoryThrottles`, a
  map swept as it doubles. Every try counts, the refused too: the store
  needn't know `Max`. `throttletest`: `TestStore`, which auth's own
  tests run on the memory store (`export_test.go`). `access.go`:
  `AccessTokens`, a token (`New`: the `Prefix`, letters and digits, `_`,
  and 32 random bytes in `b32`, lower case) and its SHA-256 (`Hash`, nil
  for what can't be one), `BearerToken` and `Abilities`. `ability.go`:
  `Ability[U, T]` (`NewAbility`, `Can`, `Check`), what a user may do with
  a thing, whose check's false, or `Deny`'s `*Denial`, is a no in its words
  or "you may not" and what (`no`), with `StatusCode` 403, and whose other
  errors are failures; a `Gate`'s `Before` is asked first, and the zero
  user, a guest, is a no with neither asked (`reflect`'s `IsZero`); `None`
  is the thing of an ability of none. `passkeys.go`:
  WebAuthn, the options as JSON, the challenge in the session
  (`tug.auth.passkey`, answered once within five minutes, which `Login`
  drops), and the checks of an answer (`checkClientData`,
  `checkAuthData`, the signature, the count); `cose.go`, the keys
  (ES256, Ed25519, RSA) and their signatures; `cbor.go`, a strict reader
  of the CBOR WebAuthn writes, fuzzed.
  `passkeytest`: an authenticator in software, for tests, with its own
  CBOR writer.
- `mail`: `Message` (with `Cc`, `Bcc`, `ReplyTo`, `Attachments`,
  `Headers` and `Unsubscribe`), `SMTP` on net/smtp (STARTTLS, TLS on 465,
  deadlines from the context), `Log`, which writes mail out, and
  `FromEnv`. `build` writes a message as a server takes it, and returns
  its envelope: the headers, the app's among them, which `checkHeader`
  refuses when they're a name the fields set (`ownHeaders`, and any
  `Content-`) or have a line break; `List-Unsubscribe` and
  `List-Unsubscribe-Post`, RFC 8058's, for an `https` `Unsubscribe`; the
  recipients, To, Cc and Bcc, each address once, Bcc in no header; and
  the body, `writeBody`: `writeText`'s quoted-printable text, with the
  HTML as `multipart/alternative`, inside `multipart/mixed` with each
  file, in base64 in lines of 76 (`writeBase64`), its type and name
  written by `mime.FormatMediaType` (`Attachment.header`), its type
  sniffed when it has none. `mailtest`: `Outbox`, a Mailer for tests,
  whose `Next` waits for a mail (`next`, on `arrived`, the first `read`
  isn't set for that it `wants`), `NextTo`, the next to an address, which
  leaves the rest for `Next`, `None`, `Down` (`ErrDown`) and `Sent`; it
  refuses what `Log` does.
- `ssr`: server-side rendering through Inertia's own SSR, with no import of
  tug. `ssr.go`: `Gateway`, an `inertia.Renderer`: the dev server's
  `/__inertia_ssr` while it runs, otherwise `URL` (SSR_URL) or `Server`'s
  `/render`; no server, or a dev server still loading (`null`), renders
  nothing, and Inertia's error JSON becomes the error (`failure`).
  `server.go`: `Server.Run`, for `App.Go`, copies the embedded bundle to a
  temp dir, runs Node with `SSR_PORT` on a free port, waits for `/health`,
  starts it again with a growing wait, and stops it with the app;
  `proc_linux.go` has Linux stop it if the app dies. Its tests run a fake
  bundle in real Node.
- `queue`: background jobs, with no import of tug and no idea where jobs
  are kept. `store.go`: `Job` and `Store`, whose Done, Retry and Fail act
  only for the claim that holds a job, told apart by its `Attempts`. A
  unique kind's `Job` has a `Key`, its payload's SHA-256 (`keyOf`), which a
  `UniqueStore` keeps while the job waits and lets go at the claim. The
  other extras a Store may have, each checked for as the kind that needs it
  is handled (`handler.needs`): `LatestStore` (`PushLatest`, which moves
  the job that waits to the push's time), `OneAtATimeStore` (a job pushed
  with `OneAtATime` keeps its key all its life, and a claim passes over it
  while a held job of its kind has the key; `KeepsOneAtATime` is a marker),
  `AtOnceStore` (a job's `AtOnce`, and a claim passes over it while as
  many of its kind are held; `KeepsAtOnce`), `HoldBackStore` (`HoldBack`,
  for `Rate`: the claim undone, and its kind held until a time) and
  `FailedStore` (`Failed` and `RunAgain`, for an app's command).
  `queue.go`: `Run`, one goroutine that claims while a worker slot is free,
  woken by a push through the Queue (`poke`, `wake`) or else by `Poll`;
  stopping gives the jobs running `Grace`, then cancels their context, and
  `run` puts them back. `run` records how a job went: Done, Retry after its
  `backoff` (attempt⁴ seconds), or Fail after its last attempt or a
  `Permanent` error, then the kind's `OnFail` (`handler.failed`); before
  the handler, `held` tries a `Rate` kind's `Limiter` and holds back what
  it refuses, or for `heldOnError` when it fails, and an `AtOnce` or
  `OneAtATime` kind's job pokes Run as it ends. `hold` is how long a claim holds a job, the longest
  Timeout and a minute. `Drain` runs what's due in the caller, until its
  context is done. `Wake` pokes
  Run, for jobs pushed in a transaction of the app's, once it has
  committed. `pushScheduled`, at the top of `Run`'s loop, pushes each
  schedule's next run once the one pushed last has come round; every
  instance does, and `ScheduleStore.PushScheduled` (store.go) lets one win.
  `kind.go`: `Handle`, `Kind[T]` with `Push` and `PushAt` (the value as
  JSON; `PushUnique` for a `Unique` kind, `PushLatest` for a `Latest` one)
  and `Schedule`, `In` (the kind pushing to a Store the app made from its
  transaction, which doesn't poke), and the options, `Unique`'s own
  `UniqueOption`s among them (`Latest`, `OneAtATime`), `AtOnce`, `Rate`,
  with `Limiter`, and `OnFail[T]`, whose type `Handle` checks is the
  kind's (`onFailType`). `schedule.go`:
  `Schedule`, `Every` (time.Truncate's multiples), `Cron` (UTC, fields as
  bitsets, `clockAfter` searching from the month down) and `CronIn`, whose
  `nextIn` searches each stretch of one offset from UTC in turn, found with
  `ZoneBounds`, as cron has it: a fixed time, no `*` in minute or hour
  (`minuteStar`, `hourStar`), runs as the clock goes forward over it and
  the first time when it goes back; a time with `*` follows the clock.
  `queuetest`: `Memory`, and `TestStore`, the Store's promises as tests,
  which every Store's own tests run. `queue_test.go` is an external
  package, for `Memory`, with `export_test.go` for the clock.
- `cache`: values kept for a while, and locks, with no import of tug and
  no idea where they're kept. `cache.go`: `Cache` (`Set`, `Get`, `Delete`,
  and `Remember`, which works a key's value out once at a time in the
  process, the callers meanwhile waiting on its `flight`, and takes a
  store that fails for no value, which `logFailure` logs unless the
  context is done) and `Store` (`Get`, `Set`, `Add`, `Delete` and
  `DeleteIf`), given `hash`: the SHA-256 of `value` or `lock`, a NUL, and
  the key; `mustLive` panics for a time to live of 0 or less. `lock.go`:
  `Lock`, whose `owner` is random, `Try` (`Add`), `Wait` (`Try` every
  `retry`, 250 ms) and `Release` (`DeleteIf`). `memory.go`: `memoryStore`,
  a Cache's without a Store, swept as it doubles, by the latest time a
  `Get` or an `Add` was given. `cachetest`: `TestStore`, which `cache`'s
  own tests run on the memory store (`export_test.go`); the tests of what
  waits run in `testing/synctest`.
- `broadcast`: events on channels between an app's instances, with no
  import of tug and no idea where they're carried. `broadcast.go`:
  `Hub` (`Publish`, `Subscribe`, `In`, which returns a `Publisher`
  through another `Store`, and `Run`) and `Store` (`Publish`, `Listen`);
  an event goes to the store as `wire`, JSON of its channel, name and
  data, `MaxEvent` bytes at most, and `deliver` hands it to the
  `subscriber`s of its channel, a buffer of 16 each, and `drop`s one
  that's full; without a Store, `publish` delivers in place. `Run` calls
  `Listen` again after it fails, `listenAgain`, a second, doubling to a
  minute, having dropped every subscriber. `broadcasttest`: `Memory`, and
  `TestStore`, whose `listen` sends `probe` events until one comes back,
  as a store listens from a moment after `Listen` is called; the hub's
  own tests run it on `Memory`, and those of what waits in
  `testing/synctest`.
- `tugtest`: Inertia's client for an app's Go tests, a browser with the
  app open. `tugtest.go`: `Client`, whose visits (`Visit`, and `Get` to
  `Delete`) send X-Inertia, the `Version` it runs, and the page it's on as
  the Referer; `FirstVisit` and `Do`; the cookies it keeps (`keep`), by
  name alone; and `Session`, which runs a function on the session through
  a `session.Store`. A client not yet shown a page takes the app's version
  from the 409 its first visit gets (`send`), which tug's Inertia
  middleware sends the version in. `response.go`: `Response`, with
  `Location`, `Follow` (redirects and the protocol's 409s) and `Errors`,
  and `Props`, `Prop` and `Flash`, which read the page's JSON into Go
  types; `valuesOnly` drops what went out for inertia's prop types, which
  hold functions, so a page's own props struct takes the rest. It fails
  the test itself, with t.Fatalf, when asked for what isn't there. Package
  tug's own tests can't use it, as it imports tug. `upload.go`: `File`,
  which makes a map body a multipart form (`multipartBody`), written as
  Inertia's client's objectToFormData writes one (`writeForm`).
- `examples/api`: a JSON API on the core, its writes limited by address
  with `tug.Limit`. Its tests are the end to end check.
- `examples/inertia`: React pages on tug. `main.go` embeds `app.html` and
  `public/` (the build lands in `public/build`; `.gitkeep` lets it compile
  before one). `main_test.go` runs on tugtest without Node, against a fake
  manifest; `e2e/` drives the real build in a browser, in order: the later
  tests change the posts. The list's scroll and the archive, `/posts`, in
  numbered pages with `Pager.tsx`, are both `tug.Paginate` of the posts,
  and `/posts.csv` is every post as CSV, `c.StreamDownload`'s (`export`),
  with `cell` putting a quote before one that would run as a formula.
  A post made, changed or deleted is an event on the posts channel of a
  `broadcast.Hub` in memory (`changed`), which `/posts/events` sends on
  (`events`), and which the archive, reloading its page, and the list,
  reloading its stats, follow with `resources/js/useEvents.ts`.
  `resources/js/tug` is written by tug gen and committed (CI checks it's
  current); `resources/js/types.ts` has only the flash type.

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
  database drivers and its npm packages (Tailwind, shadcn/ui's Radix,
  lucide, sonner, qrcode.react) are.
- **The decisions in `docs/roadmap.md` are settled.** Ask before reopening
  one.
- **Commit subjects are one line**, imperative.
