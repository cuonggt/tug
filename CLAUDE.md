# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

tug is a Go web framework for apps whose frontend is Inertia.js v3: Go
handlers render React, Vue or Svelte pages with props, with no API in
between. It is built in milestones, and `docs/roadmap.md` has the plan,
the decisions behind it and where it stands: M1 to M18 are done, which is
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
for a route.
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
  - `app.go`: `Config` (`ConfigFromEnv` reads ADDR, PORT and APP_DEBUG),
    `App`, `Run` and `Serve` with graceful shutdown, and misses. At the first
    request, `freeze` adds `/` as a catch-all, unless a route already takes
    every path under every method. The catch-all answers trailing-slash
    redirects itself, with a 307, and 405s (probing the mux with the other
    methods for `Allow`) and 404s through the ErrorHandler. `Go` adds work
    to run beside the server (`background`): `Serve` starts it with a
    context canceled as shutdown begins, waits for it, and shuts down when
    it fails first; `stopped` drops its `context.Canceled`. `Generating`
    says tug gen started the app (`TUG_GEN`), for `main` to leave out what
    only serving needs, as the auth starter's database.
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
  - `limit.go`: `Limit`, a HandlerFunc wrapper, not middleware, so a
    refusal is a 429 `*HTTPError` for the ErrorHandler, with
    `Retry-After`; it takes a `Limiter`, the `Try` `*auth.Throttle` has,
    so the core imports no auth.
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
  fails, which it logs; `WithoutSSR` skips it), and holds the middleware (Vary, the 409 for another
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
  paths, and `message` its tags into sentences. `Errors` is
  `map[string]string`, first message per field. Two tags are tug's own,
  for a `*multipart.FileHeader` (`uploaded`): `file_max` (`sizeLimit`
  reads `2MB` and says it `2 MB`) and `file_type`, the type by the file's
  first bytes; a type sniffing can't tell, a size that isn't one, or a
  field that isn't an upload panics.
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
  makes the same link. `env.go`: `FromEnv`, Laravel's variables. Its tests
  check SigV4 against AWS's published examples (`sigv4_test.go`), and run
  against a MinIO (`minio` in `s3_test.go`) when `TUG_TEST_S3` names one.
- `internal/filetype`: what a file is by its first 512 bytes, as
  `http.DetectContentType` says, without parameters (`Sniff`, `Of`), and
  what's known of each type it tells (`kinds`): its name for a message
  (`Names`: "a PNG or JPEG image"), its extension, and whether it's an
  image a browser shows. validate and storage share it.
- `vite`: dev-server tags while the hot file exists (read on each render),
  manifest tags otherwise, `Version` from the manifest's hash, `ServeHTTP`
  for the build, and `DevServer`, the dev server's URL, for package ssr.
  No import of tug or inertia; it meets them through template funcs.
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
  changes (`watch`, `snapshot`), touches `.tug/reload` for the starter's
  Vite plugin to reload the browser, and shows 127.0.0.1 as localhost
  (`shown`), where browsers make passkeys; `build.go`; `new.go`
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
  new and runs them, their mail in `app.log`; `apps.ts` has the ports):
  the three are one app, word for word, so a change to one frontend is
  made to all three. The auth starter's handlers are in `auth.go.tmpl`
  (who's logged in, and the wrappers `usersOnly`, `verified`,
  `passwordConfirmed` and `guestsOnly`), `verify.go.tmpl`,
  `twofactor.go.tmpl`, `passkeys.go.tmpl` (the `passkeys` table, and the
  handlers of adding them, logging in and confirming with them, on
  `auth.Passkeys`, whose site is `APP_URL`'s or the request's, as mail's
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
  upsert, for its locks' order. `jobs.in(tx)` is the Store that pushes in
  a handler's transaction, and `jobsCommand` the `jobs` command, which
  `main` runs in place of the server when it's given one (`command`).
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
  share a login's count.
  The mail goes by jobs (`VerifyMail` in `verify.go.tmpl`, `ResetMail` in
  `auth.go.tmpl`) that carry IDs and make the mail, token and all, as they
  run; `main` runs the queue with `app.Go`, unless `QUEUE_WORKERS` is 0,
  and the tests on their own, with an `outbox` that can be down. Its
  frontend is Tailwind and shadcn/ui: the registry's components in
  `components/ui`, layouts picked by page name in `inertia.tsx`, toasts
  from the flash event. Vue's and Svelte's are shadcn-vue's and
  shadcn-svelte's (its classic registry, `COMPONENTS_REGISTRY_URL`, as the
  CLI's default is its newer styles), with the toasts mounted by `app.ts`
  beside the app, as their Inertia has no `withApp` for a component; Vue's
  Input.vue takes the value Inertia's Form sets, and both register pages
  drop a waiting Precognition check as the form is sent, as `guestsOnly`
  redirects it once registering has logged the browser in, and as they
  go, as their Form, in 3.7.1, reads the form that's gone. An avatar is
  keyed by the user's photo, in all three, as an avatar keeps the image it
  loaded once the image is gone.
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
  `Stale` for rotation. `throttle.go`: `Throttle`, whose `Try`, `Wait`
  and `Clear` take a context and return the store's error, over a
  `ThrottleStore` (`Hit` counts and reads back in one step, `Tries`,
  `Clear`), given the SHA-256 of the throttle's `Name` and the key
  (`hash`), and the app's clock; without a `Store`, `memoryThrottles`, a
  map swept as it doubles. Every try counts, the refused too: the store
  needn't know `Max`. `throttletest`: `TestStore`, which auth's own
  tests run on the memory store (`export_test.go`). `passkeys.go`: WebAuthn, the
  options as JSON, the challenge in the session (`tug.auth.passkey`,
  answered once within five minutes, which `Login` drops), and the checks
  of an answer (`checkClientData`, `checkAuthData`, the signature, the
  count); `cose.go`, the keys (ES256, Ed25519, RSA) and their signatures;
  `cbor.go`, a strict reader of the CBOR WebAuthn writes, fuzzed.
  `passkeytest`: an authenticator in software, for tests, with its own
  CBOR writer.
- `mail`: `Message`, `SMTP` on net/smtp (STARTTLS, TLS on 465, deadlines
  from the context), `Log`, which writes mail out, and `FromEnv`.
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
  while a held job of its kind has the key; `KeepsOneAtATime` is a marker)
  and `FailedStore` (`Failed` and `RunAgain`, for an app's command).
  `queue.go`: `Run`, one goroutine that claims while a worker slot is free,
  woken by a push through the Queue (`poke`, `wake`) or else by `Poll`;
  stopping gives the jobs running `Grace`, then cancels their context, and
  `run` puts them back. `run` records how a job went: Done, Retry after its
  `backoff` (attempt⁴ seconds), or Fail after its last attempt or a
  `Permanent` error. `hold` is how long a claim holds a job, the longest
  Timeout and a minute. `Drain` runs what's due in the caller. `Wake` pokes
  Run, for jobs pushed in a transaction of the app's, once it has
  committed. `pushScheduled`, at the top of `Run`'s loop, pushes each
  schedule's next run once the one pushed last has come round; every
  instance does, and `ScheduleStore.PushScheduled` (store.go) lets one win.
  `kind.go`: `Handle`, `Kind[T]` with `Push` and `PushAt` (the value as
  JSON; `PushUnique` for a `Unique` kind, `PushLatest` for a `Latest` one)
  and `Schedule`, `In` (the kind pushing to a Store the app made from its
  transaction, which doesn't poke), and the options, `Unique`'s own
  `UniqueOption`s among them (`Latest`, `OneAtATime`). `schedule.go`:
  `Schedule`, `Every` (time.Truncate's multiples), `Cron` (UTC, fields as
  bitsets, `clockAfter` searching from the month down) and `CronIn`, whose
  `nextIn` searches each stretch of one offset from UTC in turn, found with
  `ZoneBounds`, as cron has it: a fixed time, no `*` in minute or hour
  (`minuteStar`, `hourStar`), runs as the clock goes forward over it and
  the first time when it goes back; a time with `*` follows the clock.
  `queuetest`: `Memory`, and `TestStore`, the Store's promises as tests,
  which every Store's own tests run. `queue_test.go` is an external
  package, for `Memory`, with `export_test.go` for the clock.
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
  tests change the posts. `resources/js/tug` is written by tug gen and committed
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
  database drivers and its npm packages (Tailwind, shadcn/ui's Radix,
  lucide, sonner, qrcode.react) are.
- **The decisions in `docs/roadmap.md` are settled.** Ask before reopening
  one.
- **Commit subjects are one line**, imperative.
