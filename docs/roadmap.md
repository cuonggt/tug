# Roadmap

tug is built in milestones, each ending in something that runs. It targets
the Inertia.js v3 protocol (v3.0.0, March 2026), written against
[the spec](https://inertiajs.com/the-protocol).

|     | Milestone                  | Status |
|-----|----------------------------|--------|
| M1  | HTTP core                  | done   |
| M2  | Inertia core + Vite        | done   |
| M3  | Forms and validation       | done   |
| M4  | The full v3 protocol       | done   |
| M5  | CLI                        | done   |
| M6  | v0.1.0                     | done   |
| M7  | The auth starter, whole    | done   |
| M8  | Background jobs            | done   |
| M9  | Scheduled jobs             | done   |
| M10 | Server-side rendering      | done   |
| M11 | Testing                    | done   |
| M12 | Time zones and unique jobs | done   |
| M13 | Passkeys                   | done   |
| M14 | Vue and Svelte starters    | done   |
| M15 | The queue, whole           | done   |
| M16 | Files                      | done   |

M1 to M3 is the minimum usable version: a create, edit and delete app, end
to end.

## Why tug

[gonertia](https://github.com/romsar/gonertia) already implements much of
the protocol on net/http. What Go lacks is a Laravel-style framework built
around Inertia:

- **Types from Go to TypeScript:** prop structs become TypeScript types, and
  named routes a typed `route()` helper.
- **Already wired up:** sessions, flash messages, validation errors,
  Precognition, CSRF and auth all work with Inertia without setup.
- **One command, one binary:** `tug dev` runs Go and Vite together, and
  `tug build` embeds the frontend in the binary.
- **Plain Go underneath:** net/http, standard middleware, the standard
  library.

## M1 · HTTP core — done

The App (config from the environment, slog, graceful shutdown); a Router on
ServeMux (groups, middleware, named routes, URL building); Ctx (binding,
responses); one ErrorHandler for errors, panics, 404s and 405s; and the
RequestID, Logger, Recover and CSRF middleware. tug adds about 35 ns and one
allocation to a request over ServeMux alone.

## M2 · Inertia core + Vite — done

- Package `inertia`, usable with any router: `Render`, `Middleware`,
  `Location`. A first visit gets HTML with the page object in
  `<script data-page="app" type="application/json">`; a visit from the
  client gets it as JSON. `Vary: X-Inertia` goes on every response.
- A GET from another build gets a 409 with `X-Inertia-Location` and
  `X-Inertia-Version`, before the handler runs.
- Partial reloads, shared props (`Share`, `ShareFunc`, and `WithProps` for
  middleware, listed in `sharedProps`), and `Lazy`, `Optional`, `Always`
  and `Defer` props. The props that go out in one response are worked out
  concurrently, and a panic in one becomes an error.
- A 302 after PUT, PATCH or DELETE becomes a 303; `encryptHistory` and
  `clearHistory`, sent only when true.
- Nil slices and maps, at any depth, go out as `[]` and `{}`.
- Package `vite`: tags from the dev server (with the React refresh preamble)
  while `public/hot` exists, and from the manifest otherwise; the version is
  a hash of the manifest; `ServeHTTP` serves the build.
- `tug.Page[P]`, `Ctx.Inertia`, `Ctx.Location`; `Config.Inertia` puts the
  middleware around every route.
- `examples/inertia`: React pages, a deferred prop, a partial reload, a
  DELETE link, and Playwright tests in Chrome, which CI runs.

Choices made on the way:

- `X-Inertia-Location` is relative, where the spec's example is absolute:
  behind a proxy that ends TLS, an absolute URL would have to guess the
  scheme.
- The dev server announces itself in a file, as laravel-vite-plugin does,
  so `npm run dev` switches the Go server to it without a restart. The
  example's `vite.config.ts` has the ten-line plugin that writes it.
- The example keeps Laravel's layout: the build in `public/build`, served
  under `/build/`, and embedded with the rest of `public/`.
- The root template loads the page's own component chunk with the app's, so
  a first visit doesn't wait for one before fetching the other.
- The example resolves pages with `import.meta.glob` rather than
  `@inertiajs/vite`, which adds little without SSR. SSR will bring it in:
  its dev server answers at `/__inertia_ssr`.

## M3 · Forms and validation — done

- Package `session`: the session in a cookie, AES-256-GCM with a key derived
  (HKDF) from `APP_KEY`, which can be rotated with `APP_PREVIOUS_KEYS`, as
  Laravel names them. Flash data lasts one request (`Flash`, `Flashed`,
  `Reflash`, `Unflash`), and the cookie's expiry slides with each response.
- Package `validate`: go-playground/validator's tags, fields named by their
  json tags and dotted paths (`lines.1.quantity`), and messages in the
  style of the bind errors: "title must be at most 80 characters".
- `Ctx.BindValid` and `Ctx.Validate`, with checks of the handler's own, such
  as a title that's taken. A value that doesn't parse is a field error.
- A form that doesn't validate goes back to its Referer (on this site) with
  the errors flashed, under the error bag the client names; an API client
  gets a 422 with `message` and `errors`.
- Precognition requests are answered inside BindValid, 204 or 422, for the
  fields named in `Precognition-Validate-Only`, and the handler stops there.
- `Ctx.Flash` reaches the next page shown, in this request or after a
  redirect, and only that one; a 409 for another build keeps it for the
  reload. `Ctx.ClearHistory` survives a redirect the same way.
- The example creates, edits and deletes posts with Inertia's `<Form>`,
  checking each field as it's left, and shows a flash message after each.

Changed on the way:

- The Go minimum is 1.26, not 1.25. go-playground/validator and every
  current golang.org/x module require it, and x/crypto is one to keep up
  to date. It's also what Go supports, with 1.27.
- Dropped: sending the CSRF 403 through the ErrorHandler. With token-free
  CSRF, a real user never meets it: there's no token to expire, as there is
  behind Laravel's 419. Only a cross-site forgery does, and plain text is
  enough for that.
- Bind keeps a JSON body it has read, so a handler can bind twice, and binds
  every field it can after one that doesn't parse, so the checks after it
  see the rest.

## M4 · The full v3 protocol — done

Written against the spec and against inertia-laravel 3.3.4's
`PropsResolver`, the reference for what the spec leaves open, and checked
with Inertia's client in the example's browser tests.

- Props nest: prop types work inside maps, and inside structs that declare
  them, at any depth, with metadata under their dotted paths
  (`stats.visits`). A partial reload's paths reach inside, both ways:
  asking for `user.name` looks into `user`.
- `Merge(v, ...)` and `Lazy(fn).Merge(...)`, with `Prepend`, `DeepMerge`,
  `MatchOn`, `AppendAt` and `PrependAt`; `Defer(fn).Merge()`. A prop in
  `X-Inertia-Reset` goes out without a label, so the client replaces it.
- `Once(fn, As(key), Until(t), Fresh(when))`, and `.Once()` on Defer,
  Optional and Merge props: left out for a client whose
  `X-Inertia-Except-Once-Props` names it, but always sent to a partial
  reload that asks.
- `Scroll(fn)` for `<InfiniteScroll>`: `{"data": items}`, with
  `scrollProps`, and merged at `data`, appended or prepended as
  `X-Inertia-Infinite-Scroll-Merge-Intent` says; `PageNumbers` for numbered
  pages; `.Defer()` and `.MatchOn()`.
- `Defer(fn).Rescue()`: a failure, or a panic, leaves the prop out, names
  it in `rescuedProps`, and is logged.
- A redirect to a `#fragment` becomes a 409 with `X-Inertia-Redirect`,
  except for a prefetch; `Ctx.PreserveFragment` for the page after a
  redirect.
- `Config.ErrorPage`: errors are shown as an Inertia page with their own
  status, props `status` and `message`, to browsers and Inertia's client;
  API clients still get JSON, and Debug still shows a 500's details.
- `inertia.RenderStatus`, for pages with a status other than 200.
- The example's list scrolls, ten posts at a time, and it has an error page.

Choices made on the way:

- A plain struct in props is data: it goes out as encoding/json writes it,
  and a partial reload asking for a path inside it gets all of it. The
  client deep-merges the result, so it ends up the same, and custom
  MarshalJSON methods keep working.
- Laravel's top-level dot keys (`'user.name' => ...`) aren't copied: in Go
  a nested map or struct says the same thing.
- The prop options are functions (`Merge(v, MatchOn("id"))`,
  `.Once(Until(t))`) rather than chained methods, so each prop type gets
  the options that apply to it without the same method copied onto every
  type.
- Flash data carries on through a chain of redirects, as Laravel's adapter
  does: a page that only redirects shows nothing.

## M5 · CLI — done

- `tug gen` writes `resources/js/tug/pages.ts`, the props of every page that
  `tug.Page` declares, the shared props and a `PageProps<'Component'>`
  helper, and `routes.ts`, the named routes with a typed `route()`. A
  route name that isn't there, or a missing `{id}`, doesn't type-check.
- `tug new` makes an app from the starter in `cmd/tug/starter`: a page with
  a form that checks itself, an error page, tests, a Dockerfile for a
  distroless image, and a `.env` with a fresh APP_KEY. It installs the Go
  and npm packages and writes the types.
- `tug dev` reads `.env` and runs Vite and the app. On a change to Go,
  go.mod or a template it rebuilds, writes the types again, restarts the
  app and reloads the browser. It keeps the last good build running while
  a build fails, and takes the next free port when 8080 is taken.
- `tug build` writes the types, type-checks and builds the frontend, and
  builds one static, stripped binary with it inside.
- `examples/inertia` uses the written types and `route()`, and CI fails when
  they're out of date with the Go.

Measured on a Mac with warm caches: `tug new blog` takes 4 to 8 seconds,
`tug dev` serves in under a second after, and a rebuild after a change
takes under a second. The binary of the starter is 9.6 MB.

Choices made on the way:

- tug gen learns the app by running it, as Laravel's route tools boot the
  app, rather than by reading its source: route groups and prefixes only
  exist at run time. `tug.Page` is now a function that records each page
  and its props type, with the same call as before; `App.Run`, started
  with TUG_GEN set to a file, writes the TypeScript there instead of
  serving. Whatever main does before Run, it does for tug gen too.
- The TypeScript is written from reflection on the Go types, as
  encoding/json writes them: json tags, omitempty, embedded structs,
  pointers as `| null`, `time.Time` as a string, and the prop types as what
  they hold, deferred and optional ones as `?`.
- Props shared per request (ShareFunc) have no type tug can see: an app
  adds them to `SharedProps` in a file of its own.
- tug dev polls for changes rather than depending on fsnotify, and tells
  Vite to reload the browser through a file its plugin watches, which is
  ten lines in the app's vite.config.ts.
- The starter's templates use `[[ ]]`, so the `{{ }}` of the app's own
  html/template, and of JSX, stay as they are.
- A tug built from a checkout (its version is `(devel)` or a
  pseudo-version) makes apps that build against that checkout, with a
  `replace`; a release makes apps that require the release.

## M6 · v0.1.0 — done

- `tug new -auth` makes an app with accounts: register, log in, log out,
  a forgotten password reset by a mailed link, and a dashboard for users
  only. Every page gets `auth.user`. The users are in SQLite, through
  `database/sql`, and the handlers are the app's own code, in `auth.go`.
- Package `auth` has the parts of that where a slip is a security hole:
  `HashPassword` and `CheckPassword` (argon2id), `Login`, `Logout`,
  `UserID` and `Current` (who a session is logged in as, tied to the
  password), `Resets` (reset tokens), `Throttle` (tries at logging in), and
  `SetIntended`/`Intended` (back to the page after logging in).
- Package `mail` sends mail through SMTP, or writes it out in development,
  from the same variables Laravel's are.
- The guide, in [docs/](README.md): getting started, the CLI, routing,
  pages, forms, auth, TypeScript and deployment.
- A tug installed with `go install ...@version` makes apps that require
  that version, pseudo-versions included: the module's checksum says it
  came from the proxy.
- Fixed on the way, as the guide was checked against the code: `validate`
  names the fields of an anonymous input struct, `var in struct{...}`, as
  it does a named one's, where it had lost a nested field's first segment
  and panicked on `eqfield`; tug gen types a `[N]byte` as the numbers
  encoding/json writes; the starters' images listen on a platform's `PORT`,
  which their `ADDR` had hidden; and `tug new` takes its flags after the
  directory as well as before it.

Choices made on the way:

- Passwords are hashed with argon2id at OWASP's settings (19 MiB, two
  passes, about 30 ms), in the PHC string format that PHP's and other
  libraries' hashes are in, so users can move over from them. Hashes run
  one per CPU at most, which caps the memory a burst of logins takes.
- A login keeps the user's ID in the session and a fingerprint of their
  password hash, as Laravel's `AuthenticateSession` keeps the hash itself:
  a new password logs out every session made with the old one, which is
  the one way to end a login held in someone else's copy of a cookie.
- Reset tokens are signed rather than stored, as Django's are: made with
  the app's key for a user's password hash, so they work once, and expire
  after an hour. No table, and nothing to clean up.
- The reset form says the same thing whether the email has an account or
  not, and sends the mail without the request waiting for it, so neither
  the answer nor its timing tells who has one. A wrong login is checked
  against no hash at all when there's no user, which takes as long.
- Reset links are made from `APP_URL`, which the auth starter needs outside
  development: a link made from the request's `Host` could point to any
  site the request names.
- `Throttle.Try` checks and counts in one step, and the login counts a try
  before it checks the password: tries sent at the same moment can't all
  get in under the limit while each waits 30 ms for its hash.
- The auth starter encrypts the history the browser keeps for Back, and
  logging out clears it, so the next person at the browser can't go Back
  to the last one's dashboard. Where the browser can't encrypt, over plain
  HTTP other than localhost, Inertia's client keeps the history as it is.
- A route for users is a handler that takes the user, wrapped in
  `usersOnly`, rather than middleware: a handler that needs a user doesn't
  compile unwrapped, and the user needs no trip through the context. The
  auth prop is shared with `ShareFunc` so that error pages have it too, and
  with `Share("auth", Auth{})` besides, which is a guest's value and how
  tug gen learns its type.
- The starter's SQLite is modernc.org/sqlite, which is pure Go, so `tug
  build` still makes a static binary.
- tug new's two starters are one laid over the other: `starter-auth`'s
  files replace the plain starter's of the same name, and add the rest.

After v0.1: SSR through a Node or Bun process beside the binary, Vue and
Svelte starters, and passkeys. Background jobs are M8, and SSR, through
Node beside the binary, M10.

## M7 · The auth starter, whole — done

Enough of what an app with accounts needs to ship one, learnt from
Laravel's React starter kit, as it was in September 2026, but for passkeys.
Released as v0.2.0.

- Email verification: a link mailed at registering and at each new email,
  signed with `auth.Verifications` for the user and the email, and good
  for a day. `verified` keeps the dashboard and the security settings from
  users who haven't followed it, and a page asks them to, with "send
  another" once a minute.
- "Remember me": `session.Session.SetLifetime`, a lifetime of the
  session's own in place of the `Store`'s. A remembered login lasts a
  month without a visit.
- Asking for the password again: `auth.SetPasswordConfirmed` and
  `PasswordConfirmed`, and `passwordConfirmed`, which sends a user who last
  typed it over three hours ago to a page that asks, and back.
- Two-factor logins: `auth.TwoFactor` makes secrets and their `otpauth://`
  URLs, checks TOTP codes (RFC 6238) that work once, and seals secrets and
  recovery codes with a key derived from the app's; `StartTwoFactor` and
  `TwoFactorPending` hold a login back until the code comes. The security
  settings turn it on with a QR code and a code from the app, show the
  recovery codes, make new ones, and turn it off.
- Settings: the profile, with a new email verified again; the password,
  which ends every other login; deleting the account; and light, dark or
  the system's appearance.
- The frontend: Tailwind and shadcn/ui, lucide's icons, and sonner's
  toasts for flash messages; an app layout with a user menu, a card for the
  pages of `Auth/`, and the settings' own, picked by page name with
  Inertia v3's `layout` option and given titles with static layout props.
- The database changes with the app: migrations, run in order at start
  and counted in SQLite's `user_version`. `GET /up` answers health checks.
  Mail goes as text and HTML, and `main` waits for the mail still on its
  way as the app stops. An `https://` `APP_URL` makes the cookie secure.
- `Ctx.RedirectBack`, to the page a form was sent from. `auth.SetIntended`
  keeps, for a form sent while logged out or unconfirmed, the page it was
  on, by its `Referer`, where it kept nothing.
- `tug new -auth` leaves out the plain starter's files the auth starter has
  no use for: its `Layout.tsx`.

Choices made on the way:

- Tailwind and shadcn/ui are the auth starter's alone: the plain starter
  keeps its hand-written CSS and three npm packages. The shadcn components
  are the registry's, with `cn` pointed at `@/lib/utils` and no
  `"use client"`, as its CLI writes them with `components.json`'s
  `rsc: false`, so `npx shadcn add` fits them.
- Passkeys are left for later: WebAuthn is a lot of code where a slip is a
  security hole, CBOR, COSE keys and attestation, or a large dependency.
- "Remember me" is a longer-lived session, not a second cookie as
  Laravel's is: the whole session is in its cookie anyway, and a new
  password ends it as it ends any login.
- A verification link works without a login, in any browser: its token
  shows the email reaches the user, which is all verifying is. A password
  reset verifies the email too, for the same reason.
- The profile asks for the password again as well as the security
  settings, where Laravel's asks for the security settings alone: a new
  email decides who can reset the password. Logging in counts as typing
  it, so it rarely asks.
- A secret is kept in the session until a code from the app confirms it,
  so the database never has one that doesn't work. Secrets and recovery
  codes are sealed rather than hashed, so the codes can be shown again,
  behind the password, as Laravel and GitHub show them. As the app starts
  with more than one key, it seals them all again with the first: a
  rotated `APP_KEY` can go without taking anyone's second factor.
- A code works once: the time step of the last one that worked is stored,
  and the update only stores a later one, so of two logins with one code
  at once, one gets in. A password reset for a user with two-factor logins
  on goes on to the code: the mail is one factor.
- The appearance is the browser's, in `localStorage`, applied by a script
  in the root template before the page paints. Laravel keeps a cookie too,
  so its server renders the right one; with tug's SSR, which came in M10,
  the server renders the system's, and the page settles on the choice as
  it hydrates.
- The toasts listen for Inertia's `flash` event from the start of
  `app.tsx`, not in a component's effect, which would miss the flash of a
  page's first load, as after a link in mail.
- Inertia's `<Form>` sends a form as JSON with every value a string, `"on"`
  from a checkbox and `"42"` from a number input, which `encoding/json`
  won't put in a bool or a number. `Bind` reads such a string as it reads a
  form's value, times from date inputs too, and `""` leaves the field
  alone. It does so only for a body that fails to decode as it is, and
  finds fields by `encoding/json`'s own rules, so a JSON API's `true` and
  `42` bind as they always have, with no second reading.
- The migrations are the starter's own code, a list of SQL steps, as no
  ORM is in the core.

## M8 · Background jobs — done

Work a request starts and doesn't wait for, kept until it has run, and run
again when it fails: the auth starter's mail first, which a mail server
that was down, or a restart, used to lose. Released as v0.3.0.

- Package `queue`. `Handle` gives a queue the handler for a kind of job,
  by name, and returns a `Kind[T]`, whose `Push` and `PushAt` keep the
  value as JSON. `Run` runs jobs `Workers` at a time, woken by a push
  through the queue, or else by its `Poll`. A job that fails runs again
  after attempt⁴ seconds, 10 times over about four hours, and is then kept
  as failed, with its error; `Permanent` fails one at once, and a panic is
  a failed attempt. `Drain` runs what's due, for tests.
- `Store`: push, claim, done, retry and fail. A claim holds a job for the
  longest `Timeout` and a minute, and a late claim can't change a job a
  later one has taken. `queuetest.TestStore` checks that a Store keeps
  these promises, and `queuetest.Memory` is one in memory, for tests.
- `App.Go`: work beside the server, started with it, told to stop as it
  shuts down, and waited for. An error from it shuts the app down.
- The auth starter keeps its jobs in SQLite, in a `jobs` table its
  migrations make, and sends its mail by jobs. `QUEUE_WORKERS` is how many
  run at once, and 0 runs none. The forgotten-password form takes five
  asks a minute from an address.

Choices made on the way:

- The queue has no SQL: `Store` is an interface, and the auth starter's
  `jobs.go` is the SQLite one, as its users are its own code. tug has no
  database code anywhere; the starter counts its migrations in
  `user_version`, beside which a table of tug's would need migrations of
  its own; and SQL tested in tug would put a driver in its go.mod. What a
  slip in a Store costs, a job run twice or lost, `queuetest.TestStore`
  tests, so the starter's SQLite and anyone's Postgres run the same tests.
- A job runs at least once, not exactly once: one whose worker was killed
  runs again once its hold is up, so a handler is safe to run twice.
  Exactly once would need the job and what it does in one transaction,
  which a mail can't be in.
- The workers run in the app's binary, beside the server, as one binary is
  tug's point. With a database on a server of its own, instances share
  the work, and `QUEUE_WORKERS=0` keeps one out of it.
- The starter's mail jobs carry IDs, and make the mail, token and all, as
  they run: no reset or verification token waits in the database, and a
  mail sent late still has a fresh link.
- The forgotten-password form pushes a job whatever the email, and the job
  looks it up: pushed only for an account, the job would be a write only
  for an account, and how long the form took would say who has one.
- As the app stops, the jobs running get a grace period, then are
  canceled and put back, due at once, rather than left held until their
  hold runs out: a deploy doesn't hold them up for minutes.
- A queue with no handler for a kind runs it again later, rather than
  failing it: in a rolling deploy, an instance from before it may claim a
  kind that's new, and one from after runs it.
- The starter's claim reads before it writes, since SQLite has one writer,
  and an `UPDATE` that matches nothing still takes its lock.

## M9 · Scheduled jobs — done

Jobs that come round on their own, every night or every hour, and run
once however many instances of the app there are: the first of what M8
left out. Released as v0.4.0.

- `Kind.Schedule(s, v)` pushes a job of the kind, with `v`, at each time
  `s` names: `queue.Every(d)` at each multiple of `d`, and
  `queue.Cron(expr)` at the times of a cron expression, in UTC, parsed by
  tug: five fields, with ranges, lists, steps and names, and `@daily` and
  the like. An expression that isn't one, or that never comes round,
  panics as the app starts.
- `queue.ScheduleStore` is a Store that also keeps each schedule's run
  pushed last: `PushScheduled` pushes a run unless it, or a later one, is
  pushed already, and keeps the run and the job together.
  `queuetest.TestStore` checks it of a Store that is one, and `Memory` is.
- The auth starter keeps its schedules in a `schedules` table, and every
  night deletes the jobs that failed over a month ago, with `prune-jobs`.

Choices made on the way:

- A schedule's runs are pushed ahead, each as a job due at its time, once
  the one before it has come round. A run due while the app is down, as
  during a deploy, is already in the Store, so it runs as the app comes
  back: once, however many runs were missed. A new schedule waits for its
  next time, rather than running at once for one that has passed.
- Once across instances comes from the Store, not from electing one
  instance to run the schedules: every instance pushes each run, and the
  Store lets one push win, with an upsert that only moves a schedule
  forward, in a transaction with the job. There's no leader to lose and
  replace.
- `ScheduleStore` is an extra a Store may have, not a new method on
  `Store`: an app made with v0.3.0 has its own Store, in `jobs.go`, which
  still compiles, and `Schedule` panics as the app starts when the Store
  can't keep schedules.
- tug parses cron itself, as it stays on the standard library, and in
  UTC: time zones bring runs that a change of the clocks skips or
  repeats, which is for later.
- Unique jobs in general are left for later: a schedule's runs are unique
  by their schedule and time, which is all scheduling needs.

## M10 · Server-side rendering — done

A first visit's page rendered on the server, by Inertia's own SSR in Node,
which the app runs beside it: for search engines, link previews, and pages
that show before their scripts run. It's optional, as the decisions below
have it, and an app without it is as it was. Released as v0.5.0.

- `inertia.Config.SSR`, a `Renderer`, which a first visit asks for the
  page's head and body: the body where `{{ .Inertia }}` goes, and the head
  in `{{ .InertiaHead }}`. A page it doesn't render renders in the browser,
  and a failure is logged. `WithoutSSR` skips it for a request.
- Package `ssr`. `Gateway` renders through the Vite dev server's
  `/__inertia_ssr` while it runs, and otherwise through `SSR_URL`, or the
  Node that `Server` runs. `Server.Run`, for `App.Go`, runs the SSR build
  the binary embeds with Node, on a free port at 127.0.0.1, starts it again
  when it stops, and stops it with the app. `vite.Vite.DevServer` tells
  the gateway where the dev server is.
- `tug new -ssr`: `@inertiajs/vite`, `ssr.tsx`, a build of both bundles,
  the SSR build embedded, the wiring in `main.go`, and a Dockerfile on
  distroless Node. `tug dev` gives the app `TUG_DEV=1`, as Vite renders
  its pages then.
- Both starters make their app in `inertia.tsx`, which `app.tsx` and
  `ssr.tsx` call, and keep what needs a browser in `app.tsx`. The auth
  starter's dashboard formats its date the same on the server as in a
  browser, whose own language may differ.

Choices made on the way:

- The app runs Node itself, from the SSR build in its binary, which it
  writes to a directory of its own as it starts: `tug build` still makes
  one binary, and a deploy runs one command, with Node in the image.
  `SSR_URL` points at an SSR server run apart instead, as Laravel's
  `inertia:start-ssr` runs one.
- The SSR server is Inertia's own, and tug speaks its protocol, the page
  posted to `/render` and its head and body back, as inertia-laravel does.
  `ssr.tsx` starts it itself, where Laravel's kit has `@inertiajs/vite`
  wrap `app.tsx`: the plugin writes the port into the build, and the app
  picks a free one as it starts.
- A page that isn't rendered on the server renders in the browser, never
  as a 500: with no server there, without a word, and on a failure, with
  one in the log.
- The SSR build takes its packages with it, so none are installed where it
  runs; in development, Vite loads them from `node_modules`, as React's
  server build, which is CommonJS, needs.
- The SSR build has no source maps, which would be megabytes in every
  binary; errors in development map to the sources anyway.
- The build goes to `ssr/build`, beside a tracked `ssr/.gitkeep`, as
  `public/build` does, so `//go:embed all:ssr` compiles before a build.
- The root template has `{{ .InertiaHead }}` before its own `<title>`:
  browsers take the first, the page's, and a page without one has the
  app's.

## M11 · Testing — done

A test of an app's pages talks to it as Inertia's client does: visits with
`X-Inertia`, the session cookie kept from one to the next, and the page
object read back. The example and both starters each had a client of
their own for it, and dug props out of `map[string]any`, so every app
`tug new` made started with a copy to keep up. Package `tugtest` is that
client, in tug. Released as v0.6.0.

- `tugtest.New(t, app)` is a browser with the app open. `Get`, `Post`,
  `Put`, `Patch` and `Delete` are visits as Inertia's client makes them:
  with the version of the build it runs, the page it's on as the
  `Referer`, and the cookies the app has set. `FirstVisit` loads a page
  whole and reads the page object out of the HTML, and `Do` sends any
  other request.
- A `Response` has the page it shows, where it sends the client
  (`Location`), the validation errors it carries (`Errors`), and `Follow`,
  which follows its redirects as the browser does, the protocol's 409s
  included. It prints as the request and its answer: `POST /register: 303
  to /dashboard`.
- Props read into Go types: `tugtest.Props(r, Dashboard)` reads a page's
  props into the struct its `tug.Page` declares, and fails the test on
  another page; `tugtest.Prop[User](r, "auth.user")` reads one prop, by
  its path, and `tugtest.Flash` flash data.
- Partial reloads of the page the client is on, `Reload` with `Only`,
  `Except` and `Reset`; error bags; and Precognition, with `Validate`.
- `Client.Session` changes the client's session as a request would, to
  start a test logged in without the pages it takes.
- The example's tests, both starters' and the guide's use it, and
  [Testing](testing.md) is its page of the guide. The auth starter's 45
  tests are the same flows, on `tugtest`.

Choices made on the way:

- The client is a browser, not a mock of one: it sends the page it's on as
  the `Referer`, so a form that doesn't validate goes back to the page it
  was sent from, where the old clients sent the same `Referer` with every
  request. A test that wants another sets it with `tugtest.Header`.
- Redirects are followed only with `Follow`: where a response sends the
  client is as often what a test checks as the page after, as Laravel's
  tests don't follow them unless asked.
- A client that hasn't been shown a page takes the app's version from the
  409 its first visit gets, which the middleware already sent it in, and
  makes the visit again: a browser has the app open before its first
  visit, and a test shouldn't have to load the HTML to know the build.
- The fields of package inertia's prop types hold functions, which JSON
  can't bring back, so `Props` leaves them as they are, and `Prop` reads
  what went out for them. The prop types could have learnt to decode
  themselves, with a way to read the value back, but that's API on every
  app's props for the tests' sake alone.
- A test that asks the client for what isn't there, a prop the page
  doesn't have, or a redirect to follow from a response that isn't one,
  fails with `t.Fatalf`, and a message with the response: `GET /dashboard:
  302 to /verify-email, not the page Dashboard`. The test couldn't go on
  anyway, and the helpers would otherwise each return an error to check.
- The client keeps cookies by name, whatever their `Secure`, `Domain` or
  `Path`: it's one browser on one site, and the auth starter's session
  cookie is `Secure` for its `https://` `APP_URL` while its tests' requests
  are plain HTTP.
- It's package `tugtest`, not a part of package `inertia`: `Props` takes
  the `tug.PageOf` that has a page's props type, and `Session` takes
  package session's Store. Package tug's own tests keep a small client of
  their own, as they can't import a package that imports tug.

## M12 · Time zones and unique jobs — done

What M9 left for later: schedules on a time zone's clock, and jobs that
wait once however often they're pushed. Released as v0.7.0.

- `queue.CronIn(zone, expr)` is `Cron` on a zone's clock, the zone named
  as the IANA database names it, or `"Local"` for the server's own. Where
  the clock goes forward or back, it does as cron does: a fixed time, with
  no `*` in its minute or hour, runs as the clock goes forward over it,
  and the first time the clock shows it when it goes back; a time with
  `*` follows the clock, so every 15 minutes stays every 15 minutes. A
  zone that doesn't load panics as the app starts.
- `queue.Unique()` makes a kind's jobs unique by their value: while a job
  waits, a push of the same value does nothing, and the one that waits
  runs, at its own time. Once a worker has it, a push is pushed.
- `queue.UniqueStore` is a Store that keeps the keys: `PushUnique` pushes
  a job unless one of its kind and key waits, a claim lets the key go,
  and a unique kind's scheduled run is the job with its key that waits.
  `queuetest.TestStore` checks it of a Store that is one, and `Memory` is.
- The auth starter's `jobs` table keeps the key in a `unique_key` column,
  under a partial unique index, which the claim clears.

Choices made on the way:

- Where the clock changes, tug follows cron's own rule, Vixie cron's,
  which cronie keeps, rather than robfig/cron's, which Kubernetes'
  CronJobs use, and which skips a fixed time the clock skips: a nightly
  job shouldn't miss one night a year.
- Within one offset from UTC, a zone's clock runs as UTC's does, so
  `Next` searches each stretch of one offset in turn, with the offsets
  and the changes taken from the zone: Go's `time.Date` picks a side of a
  change without saying which. UTC keeps the search it had, so `Cron`'s
  times are what they were.
- `CronIn` takes a zone's name, not a `*time.Location`, so that a zone
  that doesn't load panics as the app starts, as an expression that isn't
  one does, and `"Local"` still names the server's. The `CRON_TZ=` some
  crons read inside the expression isn't read: one way to say it is
  enough.
- tug doesn't import `time/tzdata`, 450 KB in every binary: the starters'
  images have the zone database. An app whose instances run on different
  machines imports it, so that they all work out the same times.
- A job is unique while it waits, not until it's done: a push while it
  runs is kept, as the running job may have read what the push is about.
  Laravel's `ShouldBeUnique` holds until the job is done, and loses that
  push; tug's is its `ShouldBeUniqueUntilProcessing`.
- Jobs are the same when their values are: a job's key is its value's
  SHA-256, with no key to write, and `Unique` is one of `Handle`'s options.
  The first push wins, and keeps its time.
- A claim clears the job's key, so that "waits" is a column an index can
  see: "no claim holds it" depends on the time, which an index can't.
- `UniqueStore` is an extra a Store may have, as `ScheduleStore` is, and
  `Unique` panics as the app starts on a Store that isn't one: an app's
  own `jobs.go`, from v0.6.0, still compiles.
- Two runs of one value at once, and a push that moves a waiting job
  later, are left for later.

## M13 · Passkeys — done

Logging in with a passkey, which a phone, a laptop or a password manager
keeps, and unlocks with a PIN, a fingerprint or a face: what the auth
starter still lacked of Laravel's kit, left out of M7 as a lot of code
where a slip is a security hole. Released as v0.8.0.

- `auth.Passkeys`: WebAuthn, on the standard library. `StartRegistration`
  and `StartLogin` make the options of the browser's `navigator.credentials`,
  as JSON, and keep their challenge in the session; `FinishRegistration`
  and `FinishLogin` check the browser's answer: the challenge, answered
  once within five minutes, the site's origin and domain, the user there
  and the device unlocked, the signature, and a count that goes on. Keys
  are ES256, Ed25519 and RSA, as COSE keys, in CBOR, which a small, strict
  reader reads, fuzzed.
- `auth/passkeytest`, a passkey authenticator in software, for tests: real
  keys and real signatures, a `Clone` whose count goes back, and one that
  syncs, one the user doesn't unlock, and one on another origin.
- The auth starter: passkeys added, listed and removed on the security
  page; a login with one, from a button or the email field's autofill,
  with no email or password, which asks for no code from the phone; the
  password confirmed with one; and a mail when one is added. A `passkeys`
  table, and a random handle for each user.
- `tug dev` shows the app at `http://localhost`, where browsers make
  passkeys, as they don't for an IP address; it listens on 127.0.0.1 as it
  did.

Choices made on the way:

- A passkey stands for the password, not for a second factor after it:
  it's two factors on its own, the device and what unlocks it, and the
  options ask for it unlocked each time, so its login skips the code from
  the phone, and confirms the password too.
- WebAuthn is tug's own, on the standard library: `crypto/ecdsa`,
  `crypto/ed25519` and `crypto/rsa` check the signatures, and CBOR is read
  only as far as WebAuthn writes it, definite lengths and no tags or
  floats, rather than by a library that reads all of it. A general WebAuthn
  library would be the largest dependency tug has.
- Attestation isn't checked: the options ask for none, as most sites do,
  and an answer's attestation, whatever its format, is left alone. Which
  make of authenticator a user has is theirs to choose.
- A login's answer names its passkey, and the user by the handle kept with
  it: the login asks for no email, so nothing tells whether an email has
  an account. The handle is random, and never the user's ID or email, as
  the authenticator keeps it where anyone who has the device can read it.
- A passkey's count going back fails its login, as the passkey has been
  copied. A passkey that syncs counts nothing, and keeps 0, which is let
  be.
- The browser's side does its own base64url, rather than lean on the
  newest browsers' `PublicKeyCredential` JSON helpers.
- A new passkey sends a mail: it's a way into the account that lasts, and
  its owner should hear of one they didn't add.
- An account has ten passkeys at most: confirming with one allows only the
  user's own, whose IDs wait in the session, which has to fit in a cookie
  of about 4 KB.
- The site is `APP_URL`'s, or in development the request's, as the links
  in mail are, so passkeys made at `http://localhost:8080` work there.
- Tested end to end in Chrome, with its virtual authenticator standing in
  for a phone: adding a passkey, logging in with the autofill and with
  the button, and confirming with it.

## M14 · Vue and Svelte starters — done

The last of what v0.1 left for later, after SSR and passkeys: `tug new`
makes the same apps with Vue or Svelte in place of React, as Laravel's
starter kits come in all three. tug itself needed nothing for it: tug
gen's types extend `@inertiajs/core`, which Inertia's Vue and Svelte
adapters read as React's does; the React refresh preamble is a template
function an app calls or doesn't; the SSR gateway talks to Inertia's SSR
server, whichever framework it renders; tug dev watches only the Go and
the templates; and tug build runs whatever `npm run typecheck` is. The
work was in the starters. Released as v0.9.0.

- `tug new -vue` and `tug new -svelte`, with `-auth` and `-ssr` as they
  are: the plain app and the app with accounts, with pages rendered on the
  server or not. React stays the default, with no flag of its own, and
  `-vue` with `-svelte` is an error.
- Vue apps: Vue 3.5, pages as single-file components with
  `<script setup lang="ts">`, built by `@vitejs/plugin-vue` and checked by
  vue-tsc. The auth starter's components are shadcn-vue's, on Reka UI, with
  `@lucide/vue`'s icons, vue-sonner's toasts and qrcode.vue, as Laravel's
  Vue kit has them.
- Svelte apps: Svelte 5, with runes, built by
  `@sveltejs/vite-plugin-svelte` and checked by svelte-check. The auth
  starter's components are shadcn-svelte's, on Bits UI, with
  `@lucide/svelte`'s icons, svelte-sonner's toasts and `@svelte-put/qr`.
- The same app in each: the pages have the same names, props, labels and
  words, but for the frontend's own name and files where the landing page
  and the dashboard say them, so the Go is the same, and so are its
  tests. What isn't a component is written once: `lib/passkeys.ts`, the
  flash types in `types.ts`, the Tailwind theme in `app.css`, and the root
  template's script that picks light or dark before the page paints.
- Pages rendered on the server: an `ssr.ts` for each, on
  `@inertiajs/vue3/server` with Vue's `renderToString`, and on
  `@inertiajs/svelte/server` with Svelte's `render`. The bundle is still
  `ssr/build/ssr.mjs`, so `ssr.Server` and the Dockerfile are as they are.
- `cmd/tug/e2e`: one Playwright suite for the auth starter in every
  frontend, which makes an app of each with tug new and runs it as it's
  deployed, rendered on the server: registering and verifying the email
  by the mailed link, logging in and out, a forgotten password, two-factor
  logins with a code and a recovery code, passkeys from the email field's
  autofill and from the button, through Chrome's virtual authenticator,
  the settings, the appearance, and deleting the account. CI runs it.
- The guide: getting started and the CLI have the flags, TypeScript shows
  a page's props in Vue and Svelte, SSR no longer has them as not here
  yet, and Accounts has their frontends. Its other examples, and
  `examples/inertia`, stay React's, as the decisions put React first.

Choices made on the way:

- `tug new` lays up to four directories, each over the ones before, as it
  laid `starter-auth` over `starter`: `starter`, then the frontend's own,
  `react`, `vue` or `svelte`; with `-auth`, `starter-auth`, then
  `react-auth`, `vue-auth` or `svelte-auth`. React's files moved as they
  were, and its apps come out as they did, byte for byte. A file every
  frontend has, but for a line or two, takes a condition, as `-ssr`'s
  files do: `app.html` has the entry's extension, and `viteReactRefresh`
  for React alone, and `vite.config.ts` the frontend's plugin.
- The flags are `-vue` and `-svelte`, not `-frontend vue`: they read as
  `-auth` and `-ssr` do.
- The Vue and Svelte starters have TypeScript 6, where React's has 7,
  which is tsc rewritten in Go, and has no compiler API for other tools to
  call. vue-tsc and svelte-check are built on that API, and Vue's compiler
  reads a type imported from another file through it: with TypeScript 7,
  neither checker starts, and a Vue page whose props are tug gen's types
  doesn't build. As tried in September 2026, with vue-tsc 3.3,
  svelte-check 4.7 and Vue 3.5.
- A Vue page's props are `defineProps<Pages['Home'] & SharedProps>()`,
  not `defineProps<PageProps<'Home'>>()`. Vue's compiler turns the type
  into the component's runtime props, and can't work out `Pages[C]` for a
  type parameter, in 3.5 or in 3.6's release candidate: the page
  type-checks, and doesn't build. Spelled out, it builds, to the same
  props, and tug gen stays the same for every frontend. A Svelte page
  takes `PageProps<'Home'>` from `$props()`, as a React page takes it.
- The toasts are mounted once in the browser, on an element of their own
  beside the app, in Vue and in Svelte, where React's `withApp` puts them
  at the app's root: Vue's and Svelte's Inertia have no place there for a
  component, and in a layout they'd go as a flash arrives with a change of
  layout, as "You've logged out." does, on the landing page.
- Svelte's Inertia has no `Head`, and no title callback, so the Svelte
  starters' pages have a `Head.svelte` of their own, which gives the title
  as React's and Vue's do, with the app's name after it, through
  `<svelte:head>`.
- `lib/passkeys.ts` is a template for the one line that differs, the
  frontend's import of Inertia's router, rather than importing it from
  `@inertiajs/core`, which the apps don't install themselves.
- shadcn's components are as each registry has them, with shadcn/ui's
  changes made in each: the toaster takes the app's appearance. Where the
  registries' components differ, they differ: shadcn-vue's card titles are
  headings, and Bits UI doesn't hide the page behind a dialog from a
  screen reader as Radix does. Vue's input is changed too: Inertia's
  `Form` empties a field without the input event its `v-model` listens
  for, and it put a wrong password back.
- shadcn-svelte's CLI adds its newer styles by default, from 1.2, so the
  Svelte starter's components are from its classic registry, which look
  as React's and Vue's do, and the command its README gives for more
  names that registry in `COMPONENTS_REGISTRY_URL`.
- Inertia's Vue and Svelte `Form`, in 3.7.1, read a form that's gone when
  a field's Precognition check, waiting out its 300ms, runs after the
  page has changed, and throw, where React's checks for it. The register
  page drops the check as it goes.
- The QR code for an authenticator app is qrcode.vue's in Vue, and
  `@svelte-put/qr`'s in Svelte, on Rich Harris's headless-qr: it renders
  the same SVG on the server as in the browser, where `qrcode`, the
  package every framework could share, is CommonJS, with yargs and pngjs.
- The browser suite runs the apps as they're deployed, rendered on the
  server, and fails a test whose page says anything in the console, where
  a page that doesn't hydrate as it was rendered says so. Chrome's virtual
  authenticator answers the login page's passkey autofill at once, as a
  person would pick their passkey from it, so the button's test turns
  autofill off.
- A new app of each kind is made and built, as before, but not every kind
  in every frontend: React keeps its four, and Vue and Svelte make two
  each, the plain app without SSR and the auth app with it, which builds
  each of their layers and both sides of `-ssr`. Eight apps, where every
  kind would be twelve. A change to one frontend is made to the three in
  the same commit, and the browser suite is what says they still match.

## M15 · The queue, whole — done

What the page on jobs still had as not here yet, left from M8 to M12: a
job pushed in the app's own transaction, so that it's kept with what the
request wrote or not at all; a push that moves a unique job; jobs of one
value that never run at once; and the jobs that failed, listed and run
again from the app's own binary. The first was a gap in what the starter
promised, not a feature: it added a passkey, then pushed the mail that
tells its owner, and a push that failed there left a way into the account
that no one heard of. Released as v0.10.0.

- **In the app's transaction.** `Kind.In(store)` is the kind, pushing to a
  Store the app gives it, as the auth starter's `jobs.in(tx)`, its jobs
  table written through the handler's `*sql.Tx`: the job is kept with what
  the transaction wrote as it commits, and with none of it as it rolls
  back. `q.Wake()`, after the commit, has it run at once, rather than at
  the next poll. The starter runs a handler's writes in `a.inTx`, with the
  stores made from its transaction, `a.users.in(tx)` and the rest, and
  pushes each mail in the transaction that writes what the mail is about:
  a new account and the link that verifies its email, a passkey and the
  mail to its owner, a new email and its link.
- **The latest push says when.** With `queue.Unique(queue.Latest())`, a
  push of a value whose job waits gives the job its own time, earlier or
  later, so a reindex pushed for a minute on at each edit runs a minute
  after the last. `queue.LatestStore` moves the job, with `PushLatest`.
- **One at a time.** With `queue.Unique(queue.OneAtATime())`, a job isn't
  claimed while another of its kind and value runs: it runs once that one
  is done, or has failed, or has lost its worker. A push while one runs is
  still kept, as M12 has it. A job of such a kind is pushed with
  `OneAtATime`, and a `queue.OneAtATimeStore` keeps its key for as long as
  it keeps the job, where the claim lets the unique key go.
- **Failed jobs.** `queue.FailedStore` lists the jobs that failed for
  good, with their errors and when, and puts one back to run from its
  first attempt: `Failed` and `RunAgain`. The auth starter's binary takes
  `jobs`: `./blog jobs` lists them, and `./blog jobs retry 42`, or
  `retry all`, runs them again, in place of the server, on its database.
  In its image, that's `docker exec <container> /server jobs`.
- **Stores.** `queuetest.Memory` has every extra, and so has the auth
  starter's SQLite, with an `alone_key` column and the claim's check of
  it, an upsert for `PushLatest`, and the list and the rerun of the jobs
  that failed; `queuetest.TestStore` checks each extra's promises of a
  Store that has it.
- **The guide:** Background jobs has pushing in a transaction, the jobs
  that failed and their command, `Latest` and `OneAtATime`, the new
  extras, and the starter's SQL for each; Accounts and Deployment have the
  starter's transactions and the command.

Choices made on the way:

- `Kind.In` takes a Store made from the transaction, not the transaction,
  so the queue stays without SQL, and any database's transaction works.
  Not a transaction carried in the context: tug keeps what a handler works
  with in its arguments, as `usersOnly` hands it the user.
- What a handler writes while its transaction is open goes through it.
  SQLite has one writer, and the starter's transactions take the lock as
  they begin, so a write outside one waits for it, until the busy
  timeout, and fails. So the tables' methods go through `dbtx`, the
  database or a transaction, each store has `in(tx)`, and registering
  hashes the password before its transaction begins: 30 ms of argon2id
  with SQLite's lock held would hold up every other write.
- A push through `In` doesn't wake the queue, whose claims can't see the
  job until the commit, where a wake would find nothing. `Wake` after the
  commit does, as `a.inTx` does; without it, the next poll finds the job.
- `Latest` gives the waiting job the push's time, earlier or later: the
  last push says when. A scheduled run doesn't move a job that waits: a
  schedule's time isn't a push's.
- One at a time is the claim's, not a lock's: the claim passes over a job
  whose kind and key a live claim holds, in the same query, and so does
  the read the starter's claim makes first, so a job waiting for another
  of its key costs no write at every poll. A job whose worker died lets
  the next go once its hold runs out, as any claim does.
- `OneAtATimeStore`'s method, `KeepsOneAtATime`, does nothing: its promise
  is in the Store's pushes and its claim, which already have methods, where
  each other extra adds a push of its own. It's how `Handle` knows.
- `Latest` and `OneAtATime` are options of `Unique`, which takes them as
  `Unique(opts ...UniqueOption)`: they mean something only for jobs of
  one value, and `Unique()` is as it was.
- Extras, not new methods, again: an option panics as the app starts on a
  Store that isn't the extra it needs, and `In` on one that isn't what the
  kind needs, and an app's own `jobs.go` from v0.9.0 compiles and runs as
  it did, as does its database, which a migration step gives `alone_key`.
- A command, not a page: a page needs someone who may see every user's
  jobs, and the starter's users are users. The command is the app's, in
  its `main.go` and `jobs.go`, not tug's: a deployed app runs as its
  binary, where tug isn't, with the database it's given. It needs no
  `APP_KEY`, only the database.
- The queue sets a job's `FailedAt` as it fails it; the starter's table
  keeps its own clock's, `CURRENT_TIMESTAMP`, as `prune-jobs` compares it
  with the database's.
- `TestStore` checks claims of one key at once, which SQLite, with one
  claim at a time, can't get wrong, but a Postgres Store, whose claims run
  at once, can: each takes a job of the key, as neither sees the other's
  hold until it commits. The page on jobs says so, without a recipe tug
  hasn't run.
- What `TestStore` can't check, as it knows no database, the starter's
  tests do: a transaction rolled back leaves no job, and a registration or
  a passkey whose mail can't be pushed leaves nothing written. A trigger
  turns the jobs table's inserts away, as a database can, while the
  queue's claims go on.

## M16 · Files — done

Uploads, from an Inertia form to where they're kept and back to the page:
what nearly every app needs, and where tug stopped halfway. `c.Bind` put an
uploaded file in a `*multipart.FileHeader` field, as Inertia's client
sends a form with one, but there was nowhere to keep it, no link to serve
it back by, public or private, nothing checked its size or its type, and a
test couldn't send one. Released as v0.11.0.

- **Package `storage`.** A `Disk` keeps files by key: `Put` streams one
  in from a reader, with its size and type, `Open` reads it back, `Delete`
  removes it, and `URL` is a link to it, which for a private disk lasts
  until an expiry. `storage.Local` is a directory, whose files the app
  serves at a route of its own, the disk itself, and `storage.S3` any
  service that speaks S3's API: AWS, Cloudflare R2, MinIO and the rest.
  `storage.PutUpload` keeps an upload under a new key of the app's.
  `storage.FromEnv` picks a disk by the variables Laravel's filesystem
  reads: `FILESYSTEM_DISK`, and `AWS_BUCKET`, `AWS_DEFAULT_REGION`,
  `AWS_ENDPOINT` and the rest for S3.
- **Private files by signed links.** A private local disk's link is signed
  as the links in mail are, with a key derived from the app's, over the
  route, the file and an expiry, and its route checks it; S3's are its own
  presigned links. A public disk's link is its base URL and the key: a
  CDN's, or the app's route.
- **Checking uploads.** `validate`'s `file_max` and `file_type`, the type
  read from the file's first bytes, not from its name or from what the
  browser says, as
  `validate:"required,file_max=2MB,file_type=image/png image/jpeg"`, with
  messages as the other tags have: "photo must be at most 2 MB", "photo
  must be a PNG or JPEG image".
- **tugtest.** A map body with a `tugtest.File` in it goes as multipart,
  as Inertia's client sends one, with its other values as form fields,
  written as the client writes them, so a test uploads as a browser does.
- **The auth starter.** A profile photo: uploaded on the profile page, with
  the upload's progress; kept on the app's disk, `files/`, which the image
  puts on its volume, `/data/files`, or S3; shown in the user menu in
  place of the initials; replaced, removed, and deleted with the account.
  In all three frontends, and in the browser suite.
- **The guide:** Files, a page for uploads, checks, disks, links and
  tests, and Forms, Routing, Testing, Deployment and Accounts where they
  meet them.

Choices made on the way:

- S3 is on the standard library. Its signatures, SigV4, are a chain of
  HMACs, and a disk needs four of its calls: an object's put, get and
  delete, and a presigned link. An AWS SDK would be tug's largest
  dependency by far. A put streams, signed as `UNSIGNED-PAYLOAD`, so a file
  is neither read twice to hash it nor held in memory. Every header a
  request has is signed, which is what lets the tests check the signer
  against each of AWS's published examples, the presigned link's among
  them.
- A small Disk: `Put`, `Open`, `Delete` and `URL`, where Laravel's has
  lists, copies, moves and visibility besides. What an app keeps per
  record, a user's photo or a post's attachment, it keeps by a key beside
  the record. `URL` takes no context: making a link is arithmetic, for
  both disks.
- Keys are the app's, never the upload's name: `PutUpload` makes a random
  one, 26 letters and digits in lower case, with the extension of the type
  it was checked as, so a name such as `../../app.db` or `photo.html`
  means nothing; the name the user gave is the record's, to show. A key is
  a path `fs.ValidPath` takes, with no backslash, on either disk, and a
  local disk opens every path through an `os.Root`.
- Types by what the file is: the check sniffs the file's first bytes, as
  `http.DetectContentType` does, and `internal/filetype`, which validate
  and storage share, knows what each type it tells is called and is
  named with. A tag naming a type it can't tell, such as `image/svg+xml`,
  panics, as it would turn every file away. An SVG is `text/xml`: a
  document, not a photo.
- What's served can't act as the app. A local disk's files go out with
  the type their bytes say, sniffed again as they're served, rather than a
  type kept beside them, with `X-Content-Type-Options: nosniff`, anything
  but an image as an attachment, and, besides what the plan had,
  `Content-Security-Policy: sandbox`, so that even a file a browser would
  render runs nothing. An S3 keeps a file's type, and anything but an
  image to be served as an attachment; its links are on the bucket's
  host, not the app's.
- The same key and expiry make the same link, so the browser keeps a
  photo cached from page to page, where a new link at every render would
  be fetched afresh. An S3 link is presigned for the seven days that end
  at its expiry, whenever it's made; the starter's links run to the end of
  the next day, in UTC, which is the same link all day, and "signed for a
  day" at the least.
- Whether the files are public is the app's to say, not the
  environment's: the S3 that `FromEnv` makes is public when the local
  disk it was given is.
- No resizing: the starter keeps a photo as sent, up to 2 MB, a size
  counted in 1024s, as Laravel counts a file's.
- Files and transactions: a disk has none, so the starter puts the file
  before the transaction that writes its key, and deletes it again if the
  transaction fails; the photo it replaces, or the one an account had, is
  deleted by a `delete-file` job pushed in the transaction that drops it,
  with M15's `In`. The key a job deletes is read in the transaction, not
  from the request's user, so two uploads at once leave no file behind.
- A file over its field's limit is that field's error; a body over
  `Config.BodyLimit` is still a 413.
- Multipart as Inertia's client sends it was already how `Bind` read one:
  lists as `tags[]`, booleans as `1` and `0`, a file input left empty as
  a file with no name, which leaves its field nil, and a nested object, as
  `user[name]`, unread. Its tests say so now. `validate` names a field by
  its `form` tag when it has no `json` one, so that an upload's field,
  `form:"photo"`, has its error where the form has the input.
- In the frontends, an avatar is keyed by the photo: one keeps the image
  it loaded after the image is gone, which in Vue left an empty circle
  where the initials should have come back.
- The tests: `storage`'s own, `Local` on a temporary directory, SigV4
  against AWS's published examples, with no network, and `S3` against a
  MinIO, which CI runs beside the tests. MinIO's own images are gone from
  Docker Hub, and quay.io's want a login, so it's Bitnami's last,
  `bitnamilegacy/minio`, pinned: MinIO all the same. The starter's photo
  in its Go tests with `tugtest.File`, and in the browser suite in all
  three frontends.

## Decisions

- tug has its own Inertia adapter. gonertia and inertia-laravel are
  references, not dependencies: typed pages, sessions and Precognition need
  to hook deep into it.
- Handlers are `func(*tug.Ctx) error`; middleware is
  `func(http.Handler) http.Handler`.
- React 19 and TypeScript first. The core works with any frontend Inertia
  supports.
- No ORM in the core; apps use whatever database library they like.
- SSR comes after v0.1 and stays optional, since it needs a JavaScript
  runtime beside the binary.
- Go 1.26 at least: `http.CrossOriginProtection` needs 1.25, and the
  current golang.org/x modules need 1.26.
