# Roadmap

tug is built in milestones, each ending in something that runs. It targets
the Inertia.js v3 protocol (v3.0.0, March 2026, and what its client added
to 3.8.0, October 2026), written against
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
| M17 | Postgres and MySQL         | done   |
| M18 | Throttles across instances | done   |
| M19 | Proxies and signed links   | done   |
| M20 | Languages                  | done   |
| M21 | Pagination                 | done   |
| M22 | Commands                   | done   |
| M23 | Cache and locks            | done   |
| M24 | Mail, whole                | done   |
| M25 | Downloads and streams      | done   |
| M26 | Encryption                 | done   |
| M27 | API tokens                 | done   |
| M28 | Broadcasting               | done   |
| M29 | The queue, further         | done   |
| M30 | Authorization              | done   |
| M31 | Notifications              | done   |
| M32 | Security headers           | done   |
| M33 | Migrations                 | done   |
| M34 | Hooks for metrics          | done   |
| M35 | Inertia DevTools           | done   |
| M36 | Compressed assets          | done   |
| M37 | Typed forms                | done   |
| M38 | Typed flash                | done   |
| M39 | Request IDs in jobs        | done   |
| M40 | Debug error page           | done   |
| M41 | Route list                 | done   |
| M42 | Inertia 3.8                | done   |
| M43 | Development mailbox        | later  |

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
- Inertia's Vue and Svelte `Form`, in 3.7.1, throw when a field's
  Precognition check, waiting out its 300ms, runs after the form is sent:
  once registering has logged the browser in, `guestsOnly` redirects it,
  which isn't Precognition's answer, and once the page has changed, they
  read a form that's gone. React's sets its timeout afresh as it renders,
  which drops the check, and checks for the form. The register page drops
  the check as the form is sent, and as it goes.
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

## M17 · Postgres and MySQL — done

The auth starter kept its users, passkeys and jobs in SQLite, which is
right for one machine, and where most apps start. Past one, SQLite was
what held the starter back: M16 put the files where several instances
can share them, in a bucket, and the database was what was left. And the
page on jobs warned that two claims at once in Postgres can each take a
job of one key, without a recipe, as tug had never run on one. `tug new`
asks which database, as `laravel new` does: SQLite, unless it's told
Postgres or MySQL. Released as v0.12.0.

- **`tug new -auth -postgres` and `-auth -mysql`.** The same app, with its
  users, passkeys and jobs in Postgres or in MySQL, through `database/sql`
  and a driver of the app's own: pgx for Postgres, go-sql-driver for
  MySQL, both pure Go, so the binary stays static. SQLite stays the
  default, and the plain starter, which has no database, takes neither.
- **A layer per database.** The starter's SQL moved out of the files that
  handle requests into a layer of its own, `sqlite/`, `postgres/` or
  `mysql/`, laid over the auth starter as a frontend's layer is: `db.go`,
  the connection and the migrations; `users_db.go`, `passkeys_db.go` and
  `jobs_db.go`, each table's SQL, the jobs' Store's among them; and
  `db_test.go`, the tests' databases. The handlers, and the types they
  share, are the same whichever it is, in `users.go`, `passkeys.go` and
  `jobs.go` as before.
- **Jobs on a database whose claims run at once.** Postgres's and MySQL's
  Stores claim with `FOR UPDATE SKIP LOCKED`, so the claims of several
  instances take different jobs without waiting on each other, and keep a
  `OneAtATime` kind's jobs of one key apart, which SQLite, with one
  writer, never had to. `queuetest.TestStore`, with its test of claims of
  one key at once, runs on each.
- **The connection, by Laravel's variables:** `DB_URL`, or `DB_HOST`,
  `DB_PORT`, `DB_DATABASE`, `DB_USERNAME` and `DB_PASSWORD`. The app has
  a `compose.yaml` that runs its database for development, which the
  `.env` that `tug new` writes names, and SQLite's `DB_PATH` stays as it
  is.
- **`tug.Generating`**, which the plan didn't have: tug gen runs `main` up
  to `app.Run`, and the starter's `main` opened its database, which for
  SQLite made `app.db` and for a server needed one running, so `tug new`,
  which writes the types, failed on Postgres and MySQL before anyone had
  run `docker compose up`, and so would `tug build` in CI. `main` leaves
  the database alone while `tug.Generating()` says tug gen started it, and
  `newApp` doesn't reseal the two-factor secrets then.
- **The guide:** Accounts, Background jobs, Deployment, Testing and the
  CLI, where they meet the databases, and the SQL of each Store, with how
  it keeps one at a time, in place of the warning.

Choices made on the way:

- **Chosen as the app is made, not by the environment.** The SQL is the
  app's own, written for one database, as its handlers are its own: no
  dialect layer or query builder, and no `DB_CONNECTION` to switch at run
  time. An app that moves to another database takes that layer's files.
  `main.go` is the same on each but for a comment, as it calls the
  layer's `dbFromEnv`, and the rest that differs is the configuration:
  `.env.example`, the Dockerfile, the ignore files and the README.
- **tug stays without SQL.** M8's decision holds: the Stores are the
  starter's, one in each layer, and the drivers are in the app's
  `go.mod`, not tug's. tug's own tests make an app on each database with
  `tug new`, writing its types with no database running, and run its
  tests on a server when one is named.
- **SQLite's layer is the SQL it had, moved.** Its migrations are the
  same steps, so an app made before carries on, and needs none of this.
- **Postgres is close to SQLite.** `RETURNING`, `ON CONFLICT` upserts,
  partial indexes, and steps that roll back whole all carry over. What
  changes: `$1` placeholders, identity columns, `timestamptz` and `bytea`,
  and an email that's unique whatever its case, by a unique index on
  `lower(email)`, which the queries compare with. `UPDATE OR IGNORE`
  became the unique violation it is, which ends the transaction it's in,
  as the handler rolls it back anyway. A photo's key is read `FOR UPDATE`,
  as SQLite's transaction held the one lock for writing from its start.
- **MySQL is further.** No `RETURNING`, so an insert reads its ID back,
  and the claim reads the job in the query that finds it; `ON DUPLICATE
  KEY UPDATE` for upserts; no partial indexes, where a unique index lets
  NULLs repeat anyway; `VARCHAR` for what's indexed, and `DATETIME(6)` in
  UTC, the time zone each connection sets. An email's column compares
  with `utf8mb4_0900_as_ci`: whatever its case, as SQLite's `NOCASE`, but
  not whatever its accents, as MySQL's default collation would, which
  makes `ann@example.com` and `ánn@example.com` one account. The other
  strings compare byte for byte, with `utf8mb4_0900_bin`, a kind's and a
  key's among them. MySQL sets an `UPDATE`'s columns in order, each from
  the row as the ones before left it, so the profile's update reads the
  old email before it sets the new one.
- **MySQL at `READ COMMITTED`,** which each connection sets, as Postgres
  reads: a claim's second look, once it has its key's lock, has to see
  the hold the claim before it committed, where `REPEATABLE READ` would
  see what was there as the transaction began, and it takes fewer locks
  on the gaps between rows, which the claims of a queue would otherwise
  meet in. And the driver writes a query's arguments into it, escaped for
  utf8mb4, so each goes in one round trip, rather than three: to prepare
  it, run it and close it.
- **MySQL's `PushLatest` isn't an upsert.** The plan had one, returning
  the job it met with `LAST_INSERT_ID(id)`, but an upsert locks the key's
  index before the job's row, and a claim the row before the index, so a
  push and a claim of the same job at once could each wait for the other.
  It finds the job by its key, then moves it by its ID, and looks again
  when a claim or another push got there first. A unique kind's push is a
  plain insert, whose error, 1062, the transaction goes on after.
- **Migrations,** counted in a table, `schema_version`, in place of
  SQLite's `user_version`. In Postgres, each step runs in a transaction
  with its count, which locks the count's row, so instances starting at
  once take turns; two that both make the table at the first start fail
  one, which leaves the table there all the same. MySQL commits each
  statement that changes a table on its own: each step is one statement,
  which MySQL 8's atomic DDL does whole or not at all, and instances take
  turns under `GET_LOCK`, named for the database and held on one
  connection for the whole run. `schema_version`'s `running` has the step
  under way: a crash in the instant between a step and its count leaves
  it, and the next start stops, saying which, rather than guess, as
  Laravel's migrations are on MySQL too. A step that fails did nothing,
  and is left to run again.
- **One at a time, under claims at once.** The claim of a `OneAtATime`
  job locks a row of its kind and key, in a `job_locks` table, before it
  checks that no other job of the key is held, and keeps the lock until
  it commits: a second claim of the key waits for the first, then sees
  its hold, and passes over the job, trying again for another. The lock is
  an upsert whose update changes nothing, which locks the row whether it
  makes it or finds it; in MySQL, an insert that met the row, then a lock
  taken on it, would lock it for reading first, which two claims could
  both hold. A row rather than an advisory lock: Postgres's last as long
  as the transaction, but MySQL's as long as the connection, which
  `database/sql` hands on to whoever's next. `prune-jobs` deletes the rows
  no job needs any more. Without the lock, `TestStore`'s claims of one key
  at once got two jobs, or all eight, in three of twenty runs on
  Postgres, and six on MySQL.
- **Tests on a server.** On Postgres or MySQL, the starter's tests each
  make a database of their own on the server `DB_URL` names, or else
  `compose.yaml`'s, named for the app and the test, as `blog_test_5f3a9c0e`,
  and drop it after; with no server they fail, saying to start one, where
  skipping would pass with nothing tested: `docker compose up -d`, then
  `go test ./...`. tug's CI runs a Postgres and a MySQL beside the tests,
  named by `TUG_TEST_POSTGRES` and `TUG_TEST_MYSQL`, and its tests of
  those apps are skipped without them, as `storage`'s S3 is without a
  MinIO. The browser suite stays on SQLite: the pages are the same.
- **Versions.** CI runs Postgres 18 and MySQL 8.4, the older of MySQL's
  two long-term releases, 9.7 being the newer, and the guide names those
  as what's tested. MariaDB isn't: its collations aren't MySQL's, and an
  app on it picks its email column's.
- **`compose.yaml`, for development only.** It runs the database, on
  127.0.0.1 with a volume, and nothing else: `tug dev` doesn't start it,
  as a database outlives a dev server. Its password is `secret`, and its
  user the database's own, `postgres` or `root`, who can make the tests'
  databases; its database is the app's name, as SQL takes one without
  quotes. A deployed app gets its `DB_URL` from its environment, with the
  driver's own settings in the URL's query, such as Postgres's `sslmode`.
- **The image.** On Postgres or MySQL, the Dockerfile's `/data` volume
  keeps the photos alone, and the database is wherever `DB_URL` says. The
  health check at `/up` pings it, as before.
- **Flags:** `-postgres` and `-mysql`, as `-vue` and `-svelte` are: one at
  most, and only with `-auth`.

## M18 · Throttles across instances — done

`auth.Throttle` counted tries in memory: each instance of the app counted
its own, and a restart forgot. With four instances, a login got 20 tries
a minute where it should get 5, and "Before going live" warned of it.
M17 made several instances on one database the way the starter runs past
one machine, which left the throttles as the last of its state that
wasn't shared, and it's the state that slows guessing a password. And tug
had no limit for a route, as Laravel's `throttle` middleware is. Released
as v0.13.0.

- **A store for the counts.** `auth.Throttle` has a `Store`, an
  `auth.ThrottleStore`: where the counts are kept, such as a table in the
  app's database, which every instance reads and writes. Without one, the
  counts are in memory, as before. A store's `Hit` counts a try, and says
  how many the key has had in its window and when the window ends, in one
  step, so tries at once, from any of the instances, can't all get in
  under `Max`; its `Tries` reads a key's count without adding to it, for
  `Wait`, and its `Clear` forgets it.
- **The auth starter's throttles, in its database.** A `throttles` table,
  with its SQL in each database's layer, `throttles_db.go`, for the five
  throttles the starter has, each named: `logins`, by email and address,
  `passwords`, of someone logged in, `codes`, of a login waiting for its
  second factor, `mails`, and `reset-asks`. A step at the end of each
  layer's migrations makes it, and `prune-throttles` empties it of the
  counts that count nothing any more, every hour.
- **`auth/throttletest`:** `TestStore`, a store's promises as tests, as
  `queuetest.TestStore` has a queue Store's, which `auth`'s tests run on
  the memory store and the starter's on its table, in each database.
- **A limit for a route.** `tug.Limit(limiter, key, handler)` counts a try
  for each request, by a key the app makes from it, such as the address it
  came from, and answers one over the limit with a 429, and
  `Retry-After`, through the app's `ErrorHandler`: an Inertia visit gets
  the error page, and an API's client JSON. `examples/api` limits its
  writes by address.
- **The guide:** Accounts, with a section on the starter's throttles and
  the store in package `auth`'s, Routing, for the limit, Deployment and
  Testing, and "Before going live" lost its warning.

Choices made on the way:

- **`Try`, `Wait` and `Clear` take a context and return an error.** A store
  can fail, as a database can, and a try that can't be counted is the
  handler's error to answer, not the throttle's to guess: let it in, and
  the throttle stops nothing while its store is down; turn it away, and a
  database that blinks locks everyone out. So the calls changed, the
  starter's and those of every app made from it, as
  `wait, err := a.logins.Try(c.Context(), key)`: the release's one change
  that breaks an app, which its notes say how to make. The starter's
  `checkPassword` takes a context for it, and a forgotten password asks
  the address's throttle, then the email's.
- **A throttle on a store has a name,** such as `logins`, which its keys
  are counted under: the starter's `passwords` and `codes` both count by
  user ID, and in one table would count as one. In memory, each throttle
  has its own counts, and needs none.
- **The store is given a hash, not the key:** the SHA-256 of the throttle's
  name, a NUL, and the key, 32 bytes. A key is an email, an address, or
  both, and a table of counts has no need to show whoever reads it who
  tried to log in, or from where. The column is `key_hash`, as `key` is
  MySQL's word, and in MySQL a `BINARY(32)`.
- **The window stays fixed,** from a key's first try, as before and as
  Laravel's `RateLimiter` has it: a store keeps a count and the time the
  window ends, which one statement moves.
- **Every try counts,** the ones turned away too, where the memory throttle
  left those out: the store's one step is to add one and read back, and
  the `Throttle` compares with `Max`, which the store needn't know. A try
  turned away changes nothing that shows, as the window's end doesn't
  move with it, which a test of the throttle now says in place of the one
  that counted the tries.
- **The app's clock,** which the `Throttle` hands the store, as a queue
  hands its Store the time it claims at: a store compares the times it's
  given, and a test gives the ones it wants.
- **The starter's table** has a row for each key, its hash, its tries, and
  when its window ends, `ends_at`, in Unix milliseconds, as the jobs'
  times are. In SQLite and Postgres, the count is an upsert with
  `RETURNING`: a new window where the old one is over, and one try more
  otherwise. MySQL has no `RETURNING`, and sets an `UPDATE`'s columns in
  order: its upsert sets the tries before the end, in a transaction that
  reads back what it wrote. No index but the key's: `prune-throttles`
  reads a table that holds an hour of counts at most.
- **All five of the starter's throttles go to the table,** on SQLite as
  well: a restart, as each deploy is, no longer gives a guesser a fresh
  minute, and the starter has one way to count, which its tests check on
  each database.
- **Pruning:** a count whose window is over is dead, and `prune-throttles`,
  every hour, `queue.Every(time.Hour)`, deletes those rows.
- **A limit is a wrapper, not middleware.** A refusal is an error for the
  app's `ErrorHandler`, as a 404 is, with its status, where middleware
  answers before the handler, with no `Ctx`, and could only write a
  response of its own, which an Inertia visit shows in a modal.
  `tug.Limit` wraps a `HandlerFunc`, as the starter's `usersOnly` does, and
  takes a `tug.Limiter`, the one method it calls, `Try`, which
  `*auth.Throttle` has, so the core imports no `auth`.
- **The key is the app's,** a function of the request's `Ctx`: an address,
  as the starter's `clientIP` reads it, or a user's ID. tug has no address
  of its own to offer: behind a proxy, the request's is the proxy's, which
  only the app knows to read past.
- **`Retry-After`, alone,** in whole seconds, rounded up, which HTTP has for
  a 429; not Laravel's `X-RateLimit-` headers, which aren't HTTP's. The
  error says how long: "too many requests: wait 30 seconds, and try
  again", or "1 second".
- **Tests:** `throttletest.TestStore` on the memory store, in `auth`'s own
  tests, and on the starter's table in each database, with 20 tries at
  once among them, which a store that reads and then writes fails; and in
  the starter, two instances of the app on one database, with a login's
  wrong passwords shared between them: five across the two, and the sixth
  waits. tug's CI runs them on its Postgres and MySQL, as it runs M17's.

## M19 · Proxies and signed links — done

Behind a proxy, a request comes from the proxy's address. The auth
starter's `clientIP` counted by it all the same, so behind a load
balancer every visitor was one: five wrong passwords for an email, from
anyone, made everyone wait, and five asks for a reset link a minute were
the whole site's. Deployment had each app paste in its own reading of
`X-Forwarded-For`, which is easy to get wrong: read from the wrong end,
and whoever writes the header picks the address a limit counts. Laravel
has `TrustProxies`, told which proxies to believe. And a link out of the
app, in mail or anywhere else, was made from `APP_URL` by the starter's
own `base` and `link`, while the links that mustn't be forged were each
signed their own way, reset and verification links by `auth`, and files
by `storage`: an app that mailed an invitation, or a link to
unsubscribe, had nothing to sign it with, where Laravel has
`URL::temporarySignedRoute` and its `signed` middleware. Released
as v0.14.0.

- **The client's address.** `middleware.TrustProxies(proxies...)` names
  the proxies the app believes, by address or range, as `10.0.0.0/8`, or
  `*` for whatever connects: a request from one has its `RemoteAddr`
  replaced by the client's, from `X-Forwarded-For`, so whatever reads it,
  a key for `tug.Limit` or the app's own log, gets the client. `c.IP()` is
  the address alone, in place of the starter's `clientIP` and
  `examples/api`'s `byAddress`. Both starters read the proxies from
  `TRUSTED_PROXIES`, and believe none without it, as before.
- **The app's address.** `Config.URL` is `APP_URL`, which `ConfigFromEnv`
  reads, and `app.AbsoluteURL(name, params...)` a named route's whole
  link, as `app.URL` is its path; `Ctx` has both. `tug dev` gives the app
  `APP_URL`, the address it shows, unless `.env` names one.
- **Signed links.** `app.SignedURL(name, expires, params...)` is a named
  route's whole link, with its expiry and a signature, made with
  `Config.Keys`, the app's keys, as `session.KeysFromEnv` reads them.
  `tug.Signed(h)` wraps a handler that only a link the app signed
  reaches: any other request is a 403 for the `ErrorHandler`, "this link
  has expired" or "this link isn't valid".
- **The auth starter** counts by `c.IP()`, behind the proxies
  `TRUSTED_PROXIES` names, and makes the links in its mail, and its
  passkeys' site, from `Config.URL`: `base` and `link` went, and so did
  the `Base` its mail jobs carried. It gives `Config.Keys` its keys, so an
  app made from it signs links with nothing more to wire.
- **The guide:** Deployment's "Behind a proxy" sets a variable where it
  had code to paste, Routing has the client's address and signed links,
  Accounts has the starter's, and the CLI `tug dev`'s `APP_URL`.

Choices made on the way:

- **The app names its proxies, and tug reads past them.** This changed a
  choice of M18's, that tug has no address to offer, as only the app
  knows what's in front of it. The app still says what is; but reading
  the header is a part where a slip is a security hole, and those are
  tug's to keep, as `auth` keeps the ones of accounts.
- **From the right.** Each proxy adds the address it saw to the end of
  `X-Forwarded-For`, so it's read from the end, back past the proxies the
  app names, to the first address that isn't one: the client, as far as
  the proxies can tell. What's before it, the client wrote. A request from
  an address that isn't a proxy keeps it, whatever its headers say.
- **`*` believes the peer alone,** as Laravel's does: for a platform
  whose proxy has no address the app can name, where only the proxy can
  reach the app. The client is then the address the proxy added. An app
  that anything else can reach names its proxies instead, as `*` takes the
  word of whatever connects.
- **The address alone.** Not `X-Forwarded-Proto` or `X-Forwarded-Host`:
  the scheme and host of the app's links are `APP_URL`'s, the cookie is
  `Secure` by `session.Config.Secure`, which the starters set from it,
  and the proxy passes `Host` on, as Deployment asks. Nor RFC 7239's
  `Forwarded`, which proxies don't set unless they're told to.
- **The header, as proxies write it:** an address with its port, which
  some add, and IPv4 written as IPv6, `::ffff:10.0.0.5`, read as IPv4 on
  both sides, so that `10.0.0.0/8` has it. An entry that isn't an address
  stops the walk, and the request keeps the last proxy's address: past a
  proxy that wrote nothing it could have seen, nothing can be believed.
- **`RemoteAddr`, rewritten,** rather than a value of tug's in the
  context: what's written for net/http finds the client where it always
  looks, and `middleware` still imports no tug. It keeps its form, an
  address and a port, the port 0, as the client's is the proxy's to know,
  on a copy of the request, so the caller's, as a test's, stays as it was.
- **A blank proxy is skipped,** so a list split from `TRUSTED_PROXIES`
  unset believes no one, and a proxy that isn't an address or a range
  panics as the app starts, as `CSRF`'s origins do: a mistyped setting
  stops the app, rather than leave it believing no one.
- **Links from `APP_URL` alone,** never from a request's `Host`, which can
  name any site, as M6 had the starter's mail. In development too: `tug
  dev` gives the app the address it shows, `http://localhost:8080` or the
  port it took, where the starter took the request's `Host`, and M13 took
  passkeys' site from it. It's the same address, made one way. `tug dev`'s
  takes the place of one that's set but empty, as `.env.example` has it.
- **The auth starter stops without `APP_URL`, but under tug gen,** which
  makes no links, where before it went on with `APP_DEBUG` on: `tug new`
  and `tug build` need no address, as they need no database.
- **The app's address is a scheme and a host,** `http` or `https`, and
  `New` panics on anything else, a path, a query or a user among them, as
  a mistyped `APP_URL` should stop the app as it starts; a slash after the
  host is dropped. tug's routes are at the root, so an address with a path
  would make links that miss them.
- **`app.URL` stays a path,** for redirects and the pages' own links,
  where the scheme and host a proxy answers on aren't the app's to know.
  A whole link is asked for by name: `AbsoluteURL` or `SignedURL`. Without
  `Config.URL`, either is an error that says to set `APP_URL`.
- **A signed link is whole,** `Config.URL` and its path: it's for
  somewhere else, a mail, a QR code or another service, and a page can
  show one all the same.
- **What's signed:** the link's path, escaped as it's sent, and its
  expiry, each part preceded by its length, with HMAC-SHA256 and a key
  derived from the app's for signed links alone, as `storage`'s links and
  `auth`'s tokens have their own. Unescaped, `/share/a%2Fb`, one value,
  and `/share/a/b`, two, are one path, and a link for the one mustn't open
  the other. The query has `expires` and `signature`, as a private disk's
  links do. A link is checked with each of the app's keys, so a rotated
  `APP_KEY` leaves the links made with the old one working until it's
  dropped.
- **Nothing may be added.** A query with anything else in it isn't the
  link that was signed, and fails: `Bind` would put what was added in the
  handler's struct.
- **Every link expires.** `expires` is a time, as a private disk's links
  take: a link in mail lasts as long as the mail, which can be forwarded,
  and one that has to work for years, as a link to unsubscribe, is given
  years. It works until then, as often as it's followed: what must happen
  once, as an invitation accepted, is the app's to record.
- **Only a real link is told it has expired:** the signature is checked
  first, so "this link has expired" is for a link the app made, and
  anything else "isn't valid". A route `Signed` wraps on an app with no
  keys is a 500, the app's mistake, rather than a 403 for every link.
- **A wrapper, not middleware,** as `tug.Limit` is: a refusal is an error
  for the `ErrorHandler`, so an Inertia visit gets the error page.
- **`auth`'s tokens stay.** A reset link works once, as its token is
  signed with the password hash the reset changes, which a signed link has
  no part in, and the starter's links carry their tokens in their paths
  already.
- **Jobs pushed before the release** carry a `base` the starter's jobs no
  longer read: JSON leaves it be, and the link is `APP_URL`'s, which is
  what `base` held outside development.
- **The plain starter** has `TrustProxies` too, a cookie that's `Secure`
  by an `https://` `APP_URL`, and its keys in `Config.Keys`, as the auth
  starter has, and both variables in its `.env.example`.
- **Tests:** the walk past the proxies, with a client that writes the
  header itself, one that writes a proxy's address, a chain of proxies,
  `*`, IPv6, IPv4 written as IPv6, an address with its port, and the
  header on two lines, and the requests it leaves alone; a signed link
  changed in its path, its expiry or its signature, one with a parameter
  added, one expired, one signed with a key since rotated, and one
  route's signature on another's path; `c.IP()`; `tug dev`'s `APP_URL`;
  `examples/api`'s writes counted by client through a proxy; and in the
  starter, on each database, logins behind a load balancer: a guesser
  waits, and the account's owner, from another address through it, logs
  in.

## M20 · Languages — done

Everything tug said to a person was in English: `validate`'s messages,
`Bind`'s, as "age must be a whole number", a 429's wait, and the status
on an error page, and an app could only say them otherwise by replacing
them. Nor could it add a rule to the tags, as `slug` or a phone number,
other than as a check in each handler, as `validate`'s validator is its
own; and a message named a field by its key, "first_name is required".
Laravel has all three: an app's own rules, the names people read, its
`attributes`, and every message in `lang/`, in the request's language,
with `__()` for the app's own words. Released as v0.15.0.

- **Package `lang`.** A language is a JSON file of texts, each under what
  it says in English, as Laravel's `lang/vi.json` is, in a directory the
  app embeds, `lang/`, which `lang.Load` reads into a `lang.Catalog`, with
  the app's default language. `catalog.In(locale)` is a language's
  `lang.Words`, whose `T(text, args...)` says a text, its `:name`
  placeholders filled from `args`, in pairs as slog takes them, and a
  text the language doesn't have says itself, in English.
  `Config.Lang` is the app's catalog.
- **The request's language.** `c.Locale()`: the app's choice for the
  request, `Config.Locale`, from what a user picked; or else the best of
  its languages for the browser's `Accept-Language`; or else its
  default. `c.T(text, args...)` says a text in it, for a flash message,
  and `app.Locale(r)` is the same for code with the request alone, as a
  props function every page shares.
- **tug says it in the request's language:** `BindValid`'s and
  `Validate`'s errors, `Bind`'s, a 429's wait, a signed link's, and an
  error page's status. A message names a field as a person would, by its
  `label` tag, or else by its key made into words, `first_name` and
  `firstName` as "first name", and in the language's words for that.
- **Rules of the app's.** `validate.Rule(name, check, message)` adds a
  tag, as `slug`, with its check of a field's value, as the value's own
  type, and its message in English, which a language's file translates.
- **Plurals,** as Laravel's `trans_choice` has them: `Choice(text, n)`
  says a text's form for a count, of those split by `|`, by the
  language's rules, or by the counts a form names, `{0}` or `[2,*]`, as
  tug's messages with a number pick theirs: "at most 1 character", "at
  most 80 characters".
- **`tug lang vi`** writes `lang/vi.json`, with every text tug says, the
  messages of the app's rules, its fields' names and the texts its Go
  gives `T` and `Choice` in quotes, in English, to be translated, and adds
  to a file that's there the texts it hasn't got.
- **The starters** have a `lang/`, empty but for its `.gitkeep`, embedded
  as `public/` is before a build, loaded in `newApp` with `APP_LOCALE`,
  the default, so tug's words in another language are a file an app adds.
- **The guide:** a page, Languages, and Forms, Routing, the CLI and the
  README where they meet it.

Choices made on the way:

- **JSON, keyed by the English,** as Laravel's `lang/*.json`: a file a
  translator edits without Go, which a frontend's i18n library can read
  as well, and the English stays in the code where it's said, so a text
  no one has translated still says something. Not a Go function per
  language, which only a programmer edits, nor `.po` files, which need a
  library to read. A text left empty, as `tug lang` writes one, isn't
  translated yet.
- **`:name` placeholders,** as Laravel's, filled by name, as a language
  puts them where its grammar has them: ":field is required", and "Vui
  lòng nhập :field". `:Field` fills with the value capitalized, as a
  sentence starts, and `:FIELD` in capitals, as Laravel's do. They're
  filled in one pass, so a value, a user's name, can't name another.
- **The app's choice is a function, `Config.Locale`,** where the plan
  had a middleware of the app's set it with `lang.WithLocale`: the
  session, where a choice is kept, is inside the App's middleware, which
  couldn't read it, and a 404's page, which no group's middleware runs
  for, would be in the browser's language. The function runs inside the
  session's middleware, for every request.
- **A language's words are `catalog.In(locale)`,** a `lang.Words`, where
  the plan had `T(locale, text, args...)`: the text is always `T`'s first
  argument, which `tug lang` reads from the source, and a job says all
  its texts in the one language it carries.
- **`Choice` for a count,** beside `T`, fills `:count` with it, as
  Laravel's `trans_choice` does; `T` says a text with forms as it's
  written.
- **Plural rules are Laravel's,** a switch of languages by how they
  count, written in Go, where CLDR's would bring `golang.org/x/text`. A
  text that falls back to English takes English's rules, so a language of
  one form doesn't say "5 comment".
- **The app's languages are its default and its files.** English is
  offered as the default, or with an `en.json`, so an app in Vietnamese
  alone answers a browser in English in Vietnamese. A text comes from the
  language, then its language without the region, `pt.json` for `pt-BR`,
  then `en.json`, which rewords tug's English for every language that
  hasn't words of its own, then the English itself.
- **`Accept-Language` is tug's to read,** with its weights, a region
  matched to its language, `vi-VN` to an app's `vi`, and a language to
  the one region the app has, `pt` to `pt-BR`, rather than by
  `golang.org/x/text/language`'s matcher, which is large for one header.
  It reads 32 languages at most, and one weighted 0, or with a weight
  that isn't one, is left out.
- **The app's choice first,** then the browser's: someone who picked a
  language keeps it on every browser. Where the choice is kept, a user's
  row or the session, is the app's.
- **Words for people, not for programmers.** What a person reads is
  translated: a form's errors, a 429, an error page. What a programmer
  reads isn't: the log, panics, and a misused call's errors, as "tug: no
  route is named ...". An `HTTPError`'s message is shown as it's given,
  which a handler says with `c.T`; without one, the status's text is
  said in the request's language.
- **Readable names change English messages:** "password_confirmation must
  match password" became "password confirmation must match password",
  which the release's notes say, for tests that compare messages. Keys
  split at underscores, dashes and a change of case, `userID` as "user
  id", `URLPath` as "url path". A value `Bind` can't parse is named the
  same way, its path's last part, "age must be a whole number" for
  `author.age`, where it had the whole path, as validate names a nested
  field; and by its `label` tag, found down a JSON error's path, which
  Go writes without the lists' indexes.
- **Rules can be added whenever, under a lock,** where the plan had one
  added after the first check panic: go-playground's validator takes a
  tag only before it checks anything, so `Rule` takes a lock that
  `Struct` reads under, and a rule added again replaces the one before.
  An app's tests make the app afresh, and add its rules again. A check
  takes the field's value as its type, as `func(s string, param string)
  bool`, not go-playground's `FieldLevel`, which stays behind `validate`,
  and a field of a type that isn't it, or of its kind, panics.
- **`tug lang` asks the app, and reads it:** tug's texts, and the app's
  rules' messages, come from the same run of the app as tug gen's, which
  writes them beside the types, so they're the app's tug's; the rest from
  the app's Go, but its tests and its dependencies' directories: `T` and
  `Choice` literals, the names of fields with validate, form, query or
  path tags, the fields eqfield compares with, and the file types
  file_type names, as "a PNG or JPEG image", a text too. It writes the
  file sorted, with `<` and `&` as they are, and only when it adds.
- **`tug dev` builds again** when a file in `lang/` changes, as the
  binary embeds them.
- **The frontend's words are the frontend's.** A page's own words are in
  its components, which the frontend's i18n library translates, from the
  same files if it likes, as `laravel-vue-i18n` reads Laravel's, and the
  app shares the request's language as a prop. tug translates what the
  server says.
- **The starters' own words stay English:** they're in three frontends,
  and making a starter ready for many languages, when most apps have one,
  is the app's to choose.
- **Tests:** the language from the app's choice, from the header with its
  weights and regions, and the default; tug's texts, translated and not,
  in a form's errors, a bind error by its label, a 429, a 404, a signed
  link and a body that isn't JSON; the texts tug gen's run writes; a rule
  of the app's, a rule added again, one on a field of another type, and
  one named as the validator's own; plurals in English, and in languages
  of one form and of three, and a form's own counts; the catalog's files,
  and what's wrong with them; and `tug lang`'s search of an app's Go, and
  its file, written and added to.

## M21 · Pagination — done

A list longer than a page is in most apps, and tug had `inertia.Scroll`,
for `<InfiniteScroll>`, and nothing for pages by number: each app read
`?page`, checked it, worked out the offset, counted the rows, and made
the links to the pages, and `examples/inertia` read `page` itself for its
scroll. Laravel's `paginate()`, `simplePaginate()` and `cursorPaginate()`
do it all, and give a page its list with where it sits, as JSON that
pagers read. tug does it without SQL: the app runs its query, with the
limit and offset it's given. Released as v0.16.0.

- **By number.** `tug.Paginate(c, perPage, count, fetch)` reads the page
  asked for from `?page`, calls `count` for how many there are, then
  `fetch` with that page's limit and offset, and returns a
  `tug.Paginated[T]`: the items, the page, the last page, how many in
  all, and links to the pages around it, the one before and the one
  after.
- **Without a count,** `tug.SimplePaginate(c, perPage, fetch)`, which
  fetches one more than a page, to know whether there's a next: for a
  list too long to count at each visit.
- **By cursor,** `tug.CursorPaginate(c, perPage, fetch, cursor)`: `cursor`
  is where an item sits, a value of the app's, such as its time and ID,
  and `fetch` takes the last item's, for the page after it.
- **Scrolling.** Each result's `Paging()` is where it sits for
  `inertia.Scroll`, so one query feeds numbered pages and
  `<InfiniteScroll>` alike.
- **`examples/inertia`** has its posts in numbered pages too, at
  `/posts`, with a pager, `Pager.tsx`, and its scroll fed by
  `tug.Paginate`, with a browser test of each.
- **The guide:** Pages, a section on pagination, and its scroll by
  `tug.Paginate`.

Choices made on the way:

- **Laravel's JSON,** `data`, `current_page`, `last_page`, `per_page`,
  `total`, `from`, `to`, `first_page_url`, `last_page_url`,
  `prev_page_url`, `next_page_url`, `path` and `links`, in Laravel's
  order, so a pager written for Laravel's reads tug's. `data` is `[]`,
  never `null`, for a page with none, as a JSON API's client gets it too,
  and `from` and `to` are `null` then.
- **`links` are Laravel's pager's pages,** every one when there are under
  14, and otherwise the first two and the last two, and three on each
  side of the page, with `"..."`, with no URL, for those left out, as its
  window has them. But not its "&laquo; Previous" and "Next &raquo;",
  HTML, which a page would have to render as HTML: `prev_page_url` and
  `next_page_url` are those.
- **A type for each,** `Paginated[T]`, `SimplePaginated[T]` and
  `CursorPaginated[T]`, which tug gen writes as it writes any generic
  struct, as `Paginated_Post`: a list without a count has no `total` in
  its TypeScript, where one type for all three would have it `null`. The
  simple one has Laravel's `current_page_url`, and no `links`; the cursor
  one `next_cursor` and `next_page_url`, and none of Laravel's
  `prev_cursor`, as its cursors go forward.
- **The page size is the app's,** not the query's: a client that could
  pick it would pick how much the database reads. One under 1 is the
  program's mistake, and panics.
- **A page that isn't a number, or is under 1, is the first,** as Laravel
  has it, and a page past the last is empty, with `last_page` for the
  pager, not a 404: a list can shrink between two visits. The count comes
  first, so a page past the last runs no query for its items, however far
  past it is. Without a count, a page so far on that its offset doesn't
  fit an `int` runs none either.
- **Links keep the query,** the filters and the order the list was shown
  with, and change `page` alone, where Laravel's keep it only when asked,
  with `withQueryString()`. They're paths, as tug's redirects are, with
  the query written as Go's `url.Values` writes it, its keys in order.
- **Functions, not a query builder:** `count` and `fetch` are the app's
  SQL, or anything else's, so tug stays without SQL, as M8 and M17 have
  it, and without an ORM. A `fetch` that brings more than it's asked for
  is cut to the page.
- **A cursor is the app's value,** as JSON, in base64url in the link, and
  isn't signed, as Laravel's aren't: it's only where to start, and one
  made up starts somewhere else within what the query lets the page see.
  One that doesn't decode is the first page.
- **Cursors go forward.** A page before one, by cursor, runs the query in
  the other order, which is the app's to write; an `<InfiniteScroll>`
  that starts in the middle of a list takes numbered pages. A cursor
  page's `Paging` has the cursor it was asked with as its current, and
  the next's.
- **`PageName`,** as `inertia.Paging` has it, for two lists on one page:
  `?comments_page=2`, an option of all three, whose default is `page`, and
  `cursor` for a cursor.
- **In `tug`, not `inertia`,** as it reads the request, through `Ctx`,
  and isn't the protocol's.
- **Tests:** the page asked for and its links, with the list's query;
  the first page for one that isn't a number, a page past the last, with
  no query for its items, and a list with none; Laravel's window, at each
  end and between; the JSON, key by key; one more than a page without a
  count, and a page past any offset; cursors there and back, one that
  doesn't decode, and one made up; `PageName`; `Paging` of each; a count
  that fails, and a page size of 0; and the TypeScript of each kind. In
  `examples/inertia`, the archive's pages in Go, and its pager, and the
  scroll, in the browser.

## M22 · Commands — done

An app's binary does more than serve: the auth starter's `./blog jobs`
lists the jobs that failed, and an app grows more, a fix to its data, an
import, a user made an admin. Each was `os.Args`, read by `main` before
anything else, as the starter's `command` read it, with one command,
whose error named it. Laravel's Artisan runs an app's own commands in the
app, with its routes, database and queue. The `tug` CLI can't be where
they run: a deployed app is its binary, where tug isn't. Released
as v0.17.0.

- **`app.Command(name, summary, run)`** adds a command, whose `run` takes
  a context and the arguments after its name. `Run`, given one, as in
  `./blog jobs retry 42`, runs it in place of serving, with a context
  canceled on SIGINT or SIGTERM, and returns its error; `./blog help`
  lists them, with their summaries, and a name that isn't a command is an
  error that says to ask `help`.
- **The starter's `jobs`** is one, which `newApp` adds, and `command` in
  its `main` went.
- **The guide:** Routing has commands, after `Run` and `Serve`, and
  Deployment, Background jobs and Accounts run one in the image. Jobs'
  example of a failed job lost the `base` its payload carried before M19.

Choices made on the way:

- **A command runs in the app `main` made,** with its routes, for the
  links it mails, its queue's kinds, for the jobs it pushes, and its
  database. This changed a choice of M15's, that `jobs` needs only the
  database, and no `APP_KEY`: a command needs the environment the server
  has, which it has in the image, where `docker exec` runs it.
- **The command alone:** neither the server nor what `Go` runs starts. A
  job it pushes runs on the instances that serve, as `jobs retry` has it.
- **The arguments are the command's,** to read with `flag` or by hand:
  tug parses none, as each command's flags are its own.
- **No commands of tug's** in an app's binary, but `help`, which lists
  the app's, as do `-h` and `--help`: the other names are the app's to
  choose. A name is a word of letters, digits and `:-_`, as
  `users:admin`, which a flag, starting with a dash, isn't; one taken, or
  `help`, panics, as a command added once the app is serving does.
- **An app with no commands serves,** whatever its arguments, as before;
  and under tug gen, `Run` writes the types, whatever they are.
- **A command's error is `Run`'s,** which `main` stops with, as it stops
  with the server's: `log.Fatal`'s time comes before it. What a command
  says, it writes itself, where it likes: the starter's `jobs`, to the
  standard output.
- **Tests:** a command run with its arguments, with what `Go` runs not
  started; its error; the list, by `help`, `-h` and `--help`; a name that
  isn't one; a context canceled by SIGINT, and an app without commands,
  given one, serving until it, on Unix, whose signals a test can send
  itself; tug gen's types whatever the arguments; the names that panic;
  and the starter's `jobs` run through `Run`, as its binary runs it.

## M23 · Cache and locks — done

An app that showed what was slow to work out, a dashboard's counts or a
feed from another service, worked it out at each visit, or kept it in a
map of its own, which each instance had apart and a restart emptied. And
work that mustn't run twice at once, an import or a report, had nothing
to hold across instances: a job had `OneAtATime`, and a command or a
handler nothing. Laravel's `Cache` keeps values in a store every
instance shares, the database among them, with `Cache::remember` and
`Cache::lock`. tug has it now as it has the queue and the throttles: an
interface for the store, and its SQL in the starter's layers. Released
as v0.18.0.

- **Package `cache`,** with no import of tug. A `cache.Cache` keeps
  values under keys, each until it expires, in its `Store`: `Set` keeps
  one, as JSON, `cache.Get` reads it into its type, and `Delete` drops
  it. Without a `Store`, they're in memory, as a `Throttle`'s counts are
  without one: for one instance, and for tests.
- **Remembering.** `cache.Remember(ctx, a.cache, "stats", time.Hour,
  a.stats)` returns the value kept under `"stats"`, or runs `a.stats`
  and keeps what it returns for an hour, of the type it returns.
- **Locks.** A cache's `Lock(name, ttl)` is a lock that every instance on
  its store shares: `Try` takes it if it's free, `Wait` waits for it
  until a context is done, and `Release` lets it go, only for whoever
  holds it. A holder that dies lets go when its time is up.
- **A store for them.** A `cache.Store` keeps a value under a key until a
  time, and has the two steps a lock needs, each in one statement: `Add`,
  which keeps a value only where there's none, or one that has expired,
  and `DeleteIf`, which deletes one only while it's the value given.
- **`cache/cachetest`:** `TestStore`, a store's promises as tests, as
  `throttletest.TestStore` has a throttle store's, which `cache`'s tests
  run on the memory store and the starter's on its table.
- **The auth starter's cache, in its database.** A `cache` table, with
  its SQL in each database's layer, `cache_db.go`, which a step at the
  end of each layer's migrations makes, and `prune-cache`, every hour,
  which deletes what has expired. `newApp` gives the app `a.cache`, which
  the starter keeps nothing in: it's there for the app, as Laravel's
  `cache` table is.
- **The guide:** a page, Cache, and Accounts, for the starter's table,
  Deployment, where the instances share it, and Background jobs, where a
  lock is for what isn't a job.

Choices made on the way:

- **A store the app's database keeps,** as M8's jobs and M18's throttles
  are, so tug stays without SQL: the interface is tug's, and the SQL the
  starter's, in each database. Not Redis, which a deploy would run beside
  the database for this alone: an app that has one writes a `Store` on
  it, of five methods.
- **The store is given a hash, not the key:** the SHA-256 of `value` or
  `lock`, a NUL, and the key or the lock's name, as a throttle's store is
  given one, 32 bytes for any key, which fit one index on each database,
  and keep an email in a key out of the table. The plan had a value's be
  of its key alone, which a value keyed `lock`, a NUL and a name would
  have shared with that lock.
- **Every value expires,** as every signed link does: a time to live of
  0 or less panics, as a page size under 1 does. A table of values no one
  asks for again only grows, and one that must last is given days.
- **JSON,** as the session's values and a job's payload are: readable in
  the table, and a value comes back as the type it's read into, where gob
  would tie the table to the Go types of the day. A value that isn't its
  type's JSON, as one an older build kept may not be once the type has
  changed, is none: `Get` says so, and `Remember` works it out again, in
  its place.
- **`Remember` runs its function once at a time for a key,** in an
  instance: callers that miss while it runs wait for its value, as
  `golang.org/x/sync`'s singleflight has it, written again in a few
  lines. The wait takes in the store's read too, so callers at once make
  one query, whether the value is there or not. Across instances, each
  may run it once; where that costs more than waiting, the function takes
  a lock.
- **Callers that waited for a run that failed work it out themselves,**
  rather than take its error, which may be the first caller's alone, as
  a client that went away; a run that panics lets them go too, and the
  panic is its caller's, as a handler's is. A caller whose own context is
  done stops waiting.
- **A value's time starts once it's worked out,** however long the
  function took.
- **A store that fails is a miss,** for `Remember`: it runs the function,
  returns its value, and logs the store's error, as the site works
  without its cache, only slower, where a throttle that guessed would
  stop nothing. It logs nothing for a context that's done, as a client
  that went away is no failure of the store's. `Get` and `Set` return the
  error, for an app that wants it, and a lock never guesses: one that
  can't be taken, for an error, returns it.
- **A lock's owner is a random value,** kept as the lock's value:
  `Release` deletes the lock only while it's the owner's, so a holder
  whose time ran out, and whose lock another took, can't let go of the
  other's; it says nothing of that, as the lock is the other's to hold. A
  lock lasts the time it's taken for, and work that may take longer asks
  for longer. A `Lock` that holds its lock can't take it again.
- **`Wait` asks again every 250 milliseconds,** as Laravel's `block`
  does, until the lock is taken or the context is done, whose error it
  returns: a store has no way to say it's free, and
  `context.WithTimeout` says how long to wait.
- **The queue keeps `OneAtATime`,** where a job waits its turn without
  holding a worker; a lock is for what isn't a job, as a command or a
  handler, or for part of one.
- **Throttles keep their store,** as M18 has it: a count is one step,
  `Hit`, which a cache's get and set would make two.
- **No counters, tags or prefixes,** which Laravel's cache has: counting
  is a throttle's, tags need a store that lists keys, and the table is
  the app's own, with no other app's keys to keep apart from.
- **The memory store sweeps what has expired** as it doubles, as a
  throttle's does, by the latest time a `Get` or an `Add` was given: `Set`
  is given none, and the wall clock isn't the app's, as a test's isn't.
- **The table:** a row for each key, its hash, the value, and when it
  expires, `expires_at`, in Unix milliseconds, as the throttles' times
  are, with no index but the key's. `Get` leaves out a value that has
  expired, so pruning is for room, and hourly will do. `Add` is an upsert
  whose update replaces only a value that has expired: in SQLite and
  Postgres, `ON CONFLICT DO UPDATE` with a `WHERE`, and in MySQL, `IF`s,
  the value's first, as it reads the time the second one changes, and a
  row left as it was counts as none affected, as the jobs' `PushScheduled`
  has it. MySQL's value is a `MEDIUMBLOB`, up to 16 MB, where a `BLOB`'s
  64 KB would fail a value of a megabyte, as a test's does.
- **Tests:** `cachetest.TestStore` on the memory store, and on the
  starter's table in each database, with 20 `Add`s at once among them, of
  which one keeps its value, and a megabyte of every byte; `Remember`
  with callers at once, in `testing/synctest`, whose clock waits for them,
  a run that fails, one that panics and a caller that stops waiting; a
  store that fails, and one that fails for a context that's done; a
  value past its time, one of another type, and one JSON can't hold;
  locks taken and released, one whose time ran out, one waited for and
  one given up on; the hashes a store is given; and in the starter, two
  caches on one database, as two instances have, which share a value and
  a lock, and the prune.

## M24 · Mail, whole — done

A `mail.Message` had who it's from and who it's to, a subject, and a
body in text and HTML: no copies, no address for replies, no files, and
no headers of the app's. An invoice goes as a PDF, a contact form's mail
replies to whoever filled it in, and mail sent in bulk says how to
unsubscribe in one click, which Gmail and Yahoo have asked of it since
2024. And each app's tests wrote a mailer that keeps what it's sent, as
the auth starter's `outbox` did. Laravel's mailables have copies,
replies, files and headers, and `Mail::fake()` keeps what's sent, for
tests. Released as v0.19.0.

- **More recipients.** `Cc`, `Bcc` and `ReplyTo`, each a list of
  addresses, written as `To` is. `Bcc` goes to the server with the rest,
  and in no header, so no one who gets the mail sees it.
- **Files.** `Attachments`, each a `mail.Attachment`, its name, its type
  and its bytes, which the mail carries in base64, after its text and
  HTML.
- **Headers of the app's,** as `X-Campaign`, in `Headers`.
- **Unsubscribing in one click.** `Unsubscribe` is a link that
  unsubscribes the recipient, which the mail carries as RFC 8058 has it,
  in `List-Unsubscribe` and `List-Unsubscribe-Post`, so the mail program
  shows a button that POSTs to it. A link `SignedURL` makes, to a route
  `tug.Signed` wraps, is one no one can forge, for a recipient who
  needn't log in.
- **`mail/mailtest`:** `Outbox`, a `Mailer` that keeps what it's sent, for
  a test to read mail by mail, waiting for the job that sends it, and
  that can be down for a number of mails, as a mail server can. The auth
  starter's tests use it, and their `outbox` went.
- **`Log`** writes the copies, the address for replies, the link to
  unsubscribe, the files' names and sizes, and `Bcc`, which the mail
  itself doesn't show.
- **The guide:** Accounts' Package mail, with an invoice, a contact
  form's reply and a newsletter's link to unsubscribe, and Testing, a
  section on mail.

Choices made on the way:

- **SMTP stays the one way to send:** SES, Postmark, Resend, Mailgun and
  SendGrid all take it. A provider's HTTP API is a `Mailer` of the app's,
  one method, where each would be a client in tug.
- **An address gets a mail once,** in however many of `To`, `Cc` and
  `Bcc` it is, whatever its case. A message to `Bcc` alone, as a list's
  often is, has no `To`; one to no one at all is still an error.
- **Files are bytes,** in memory, as the whole mail is before it's sent,
  of the type given, or else the one their first bytes say, as
  `http.DetectContentType` has it, with its charset, which a text file
  needs, where a disk's files go by `internal/filetype`'s, without. A job
  that mails a file reads it as it runs, from its disk or from where it's
  made. A file has a name, and one without is an error: it's what the
  recipient saves it as.
- **A file's name is written by `mime.FormatMediaType`,** which encodes a
  name with anything but printable ASCII, `filename*=utf-8''...`, as RFC
  2231 has it, so a name in Vietnamese reaches the recipient as it was,
  and a line break in one from a user can't end its header. The name is
  in its `Content-Type` too, as `name`, for the mail programs that look
  there.
- **`multipart/mixed` around the rest:** the text and the HTML, as
  `multipart/alternative`, then each file, in base64 in lines of 76, as
  MIME has them. No images inside the HTML, by `cid:`, which is
  `multipart/related` too: an image in a mail is a link to one of the
  app's, as mail sent in bulk has them, and each mail stays small.
- **`Headers` can't say what the message says,** `From`, `To`, `Bcc`,
  `Subject`, `List-Unsubscribe` and the rest, nor any `Content-` header,
  as the body's are the message's to say, nor have a line break in a
  value, as the subject can't: a value from a form can't add a
  recipient. A name is RFC 5322's, printable ASCII without a colon, and a
  value with more than ASCII in it is encoded, as the subject is. They're
  written in the order of their names.
- **`Unsubscribe` is an `https` link,** as RFC 8058 has it, with a host,
  and nothing that would end the angle brackets it goes in: a `<`, a
  `>`, a space or a line break. The app's route answers its POST, whose
  body is `List-Unsubscribe=One-Click`. The POST comes from the mail
  provider's servers, with no `Origin`, which `CSRF` lets through, as it
  does any request that isn't a browser's, so the signature is what
  guards it. And it's a POST, as a GET is what a scanner follows before
  anyone reads the mail: the link in the mail's own text goes to a page
  of the app's that asks.
- **DKIM is the provider's,** which signs what it sends with the domain's
  key. RFC 8058 asks that the signature cover both headers, which the
  `h=` of a sent mail's `DKIM-Signature` shows, as the guide says.
- **Mail goes by the app's jobs,** as the starter's does: a job carries
  IDs and makes its mail as it runs, as M8 has it. Not a queue of
  messages, which would keep whole mails, files and all, in the jobs
  table.
- **`mailtest.Outbox` is the starter's `outbox`, made tug's:** `Next`,
  the next mail, waiting five seconds for it, as a job sends it, and
  failing the test when none comes; `None`, that no more comes in 100
  milliseconds; `Down(n)`, the next n mails failing, with
  `mailtest.ErrDown`; and `Sent`, all of them. It keeps its mail in
  order, where the starter's channel of 20 held a send up once full, and
  refuses a message SMTP wouldn't send, as `Log` does, which it asks, so
  a mail that couldn't go in production fails its test. The starter's
  tests call it, `app.outbox.Next(t)`, where they had `mail` and
  `noMail` of their own.
- **`Log`'s sizes are 1,024 bytes a KB,** as `validate`'s `file_max` has
  them: 512 bytes, 48 KB, 1.5 MB.
- **Tests:** a message with copies, a `Reply-To` and files, read back by
  `net/mail` and `mime/multipart`, as a mail program reads it, with `Bcc`
  in the envelope and nowhere else, each address once, and `Bcc` alone;
  a file's name that isn't ASCII, one with a line break, a file with no
  name, and a type that isn't one; the app's headers, one not in ASCII,
  and the ones refused; the unsubscribe headers, and the links refused;
  SMTP's recipients through a fake server; `Log`; `mailtest.Outbox`,
  in `testing/synctest` for what waits, down and up, and a message it
  refuses; and the starter's tests on it.

## M25 · Downloads and streams — done

A handler wrote a body it had whole: `c.Blob` takes bytes, and anything
else was `c.Response()`, by hand. A file of the app's, an invoice made as
a PDF, or every post as CSV, went with a `Content-Disposition` each app
wrote itself, which is easy to get wrong: a name from a user, an
upload's, can end the header, and one in Vietnamese arrives garbled.
Nothing seeked, so a download that broke off started again, and a list
too long to hold was made whole before it was sent. And a page that
follows work as it goes, an import's progress, asked again and again.
Laravel's responses have `download()`, `streamDownload()`, `file()` and
`eventStream()`. Released as v0.20.0.

- **Files.** `c.File(path)` and `c.FileFS(fsys, name)` send a file of the
  app's, as its type, through `http.ServeContent`: with ranges, so a
  download that broke off goes on where it stopped, and with
  `If-Modified-Since`. A file that isn't there, or a directory, is a 404
  for the `ErrorHandler`.
- **Downloads.** `c.Download(name, content)` sends `content`, an
  `io.Reader`, as a file to save as `name`: a file, bytes, or what a
  disk's `Open` reads, under the name it was uploaded with. Content that
  seeks, a file or a `bytes.Reader`, has ranges too.
- **Streams.** `c.Stream(contentType, write)` sends a body as `write`
  makes it, a row at a time as the rows are read, without holding them
  all, and `c.StreamDownload(name, contentType, write)` has it saved as
  `name`: every post, as CSV.
- **Events.** `c.Events(fn)` is a stream of server-sent events, which the
  browser's `EventSource` reads: `fn` sends each, a `tug.Event`, with its
  name, its data as JSON and its ID, until the client goes or the app
  shuts down. A page follows work with one, and reloads a prop when an
  event says, with Inertia's `router.reload`.
- **`examples/inertia`** has its posts as CSV, streamed, at `/posts.csv`,
  linked from the archive, with a browser test of the file saved and its
  name.
- **The guide:** Routing, a section each on files and downloads, streams
  and events, and Pages, a download's plain link and events on a page.

Choices made on the way:

- **The name, as RFC 6266 has it:** `attachment` and the name, written by
  `mime.FormatMediaType`, which encodes a name with anything but
  printable ASCII, as `filename*`, in UTF-8 and percent escapes, which
  every browser reads: a name in Vietnamese arrives as it was, and a line
  break becomes `%0A`, never the end of the header. What's before a
  name's last `/` or `\` is dropped: the browser saves a name, not a
  path.
- **The type, as `ServeContent` finds it:** by the name's extension, or
  else by the first bytes, with `nosniff`. A stream's is the app's to
  give, as it has no bytes yet.
- **`c.File` is for the app's own files,** which it shows as what they
  are, HTML as a page: a file someone uploaded is sent by their disk's
  route, which sends it sandboxed, as M16 has it, or by `Download`, to be
  saved.
- **A name a request gives is `FileFS`'s:** one part of a path can carry
  a slash, as `%2F`, so `..%2Fapp.db` reaches a handler as `../app.db`,
  which `File` would open, and an `fs.FS`, as `os.DirFS`'s or an
  `os.Root`'s, turns away, as a name that isn't one: a 404, as for a file
  that isn't there. The guide says so, with the example.
- **Not `http.ServeFile`,** which lists a directory, and answers a path
  ending in `/index.html` with a redirect: `File` opens the file and
  hands it to `ServeContent`, and one that isn't there is tug's 404, with
  the app's error page.
- **What doesn't seek goes as it's read,** through `Stream`, as a
  download of a response from another service does, and a file of an
  `fs.FS` that can't seek, as a zip's: no ranges, and its type by its
  extension or else its first bytes, read ahead. Content with a `Stat`, as
  a file, has its time for `If-Modified-Since`.
- **The handler closes what it opened:** `Download` reads `content` to
  its end, and leaves it open, as the `Open` that made it was the
  handler's.
- **Each write of a stream goes as it's made,** flushed, so a client that
  reads as it comes, a `fetch` of lines, gets each one. A writer of many
  small writes is buffered by whoever makes them, as `csv.Writer` is. A
  stream that writes nothing is an empty body.
- **An error once the body has started can't be an error page:** it goes
  to the log, and the connection is cut, with `http.ErrAbortHandler`,
  which `adapt` passes on to net/http, so the browser says the download
  failed, where it would save half a file as the whole. Before the first
  write, it's the `ErrorHandler`'s, as any handler's error is, without the
  `Content-Disposition`, as the error page isn't a file to save. A client
  that went away is no failure, and isn't logged.
- **An event's data is JSON,** always, as props are, for the page's
  `JSON.parse`, with a data line for every event, `null` for none, as the
  browser drops an event without one. Its name and ID are text on one
  line, and one with a line break is `send`'s error, as is an ID with a
  NUL, which the browser drops.
- **The headers go at once,** flushed, so the page's `EventSource` opens
  before the first event.
- **A comment every 20 seconds,** `:` alone, keeps a quiet stream open
  through the proxies in front of the app, which close a connection idle
  for a minute, as nginx and AWS's load balancers do unless told
  otherwise. With `Cache-Control: no-cache`, and `X-Accel-Buffering: no`,
  so nginx doesn't hold the events back.
- **A stream ends as the app shuts down:** `Shutdown` waits for the
  requests in flight, and one that never ends would hold it for all of
  `ShutdownTimeout`. The app has a context of its own, done as `Serve`
  begins to shut down, beside the one what `Go` runs has, and `fn`'s is
  done with it; `send` fails once it is. The browser's `EventSource`
  connects again, to an instance that's up, with the last event's ID in
  `Last-Event-ID`, for the app to go on from. A server the app didn't
  start, which calls `ServeHTTP`, has no shutdown of tug's, and its
  streams end with their clients.
- **`fn`'s error is `Events`'**, for the `ErrorHandler`, which logs it, as
  the response has started, unless the client went away, or the app is
  shutting down, which is how a stream ends.
- **Events come from where the app has them:** tug has no way yet to send
  one instance's event to the pages open on another. That's
  broadcasting, a milestone of its own, which this is the ground for.
- **Not WebSockets:** events go one way, the server's, which is what a
  page that follows work needs, over plain HTTP, which every proxy
  passes; the page sends with Inertia, as it always does. And the
  standard library has no WebSockets.
- **A download is a plain link,** `<a href>`, not Inertia's `<Link>`,
  whose visit expects a page, and shows anything else in a dialog.
- **`examples/inertia`'s CSV quotes a formula:** a cell of a title that
  starts as one does, with `=`, `+`, `-` or `@`, would run as one when a
  spreadsheet opened the file, and a `'` before it keeps it text, as
  OWASP has it.
- **Tests:** a file with a range, `If-Modified-Since`, one that isn't
  there, and a directory; a file of an `fs.FS`, and a name with `..` in
  it, as `%2F` carries it; a download's name in Vietnamese, with quotes, a
  line break or a path in it; content that seeks, with its time, and
  content that doesn't; a stream read as it's written, through a real
  server, with a client that gives up rather than wait, so a stream that
  isn't flushed fails its test rather than hang it; one that fails before
  its first write and after; events as `EventSource` reads them; events
  that can't be sent; a quiet stream's comments, in `testing/synctest`;
  a client that goes, and a stream the app's shutdown ends, through
  `Serve`; and in `examples/inertia`, the CSV, in Go and saved by the
  browser.

## M26 · Encryption — done

`APP_KEY` encrypts the session's cookie, seals two-factor secrets, and
signs links and tokens, each with a key derived from it for that alone.
An app had nothing to encrypt its own with: a token for another service
that a user connects, kept in a column, is there for whoever reads a
copy of the database, such as a leaked backup, and the one sealer tug
had, `auth.TwoFactor`'s, is for two-factor secrets, with their key.
Laravel has `Crypt`, with `APP_PREVIOUS_KEYS` for a key being rotated,
and `key:generate` for a new one. Released as v0.21.0.

- **Package `crypt`,** with no import of tug. `crypt.New(keys, purpose)`
  is a `crypt.Box`, whose `Seal` encrypts a value for keeping, with a key
  derived from the app's for that purpose alone, and whose `Open`
  returns it. The first key seals and each opens, so what an old key
  sealed opens while it's in `APP_PREVIOUS_KEYS`, and `Stale` says what
  to seal again with the new one, as `auth.TwoFactor`'s does.
- **Where a value belongs.** `Seal` and `Open` can be given what a value
  belongs to, as its table and row, `"users", "42"`: one copied to
  another row doesn't open there.
- **`auth.TwoFactor` seals as `crypt` does,** through `internal/seal`, in
  the form it had, so every secret sealed before opens as it did.
- **`tug key`** prints a new key, `base64:` and 32 random bytes, as
  `APP_KEY` takes it, for a deploy's secrets or a rotation, where the
  guide had `head -c 32 /dev/urandom | base64`.
- **The guide:** a page, Encryption: the app's keys, what each one seals
  or signs, package `crypt`, and rotating the key, step by step; and
  Accounts, Forms, the CLI, Getting started and Deployment where they
  meet it.

Choices made on the way:

- **A purpose, always,** as tug's own keys each have one, `tug session`,
  `tug two-factor` and `tug signed link` among them: a key for each is
  derived from the app's with HKDF, so a value sealed for one purpose
  doesn't open as another's, and a derived key that leaks gives away no
  other. A `Box`'s is derived with `tug crypt ` and its purpose, which
  none of tug's own begins with, so an app's purpose can't name one of
  them, and seal what tug would open as its own.
- **One way to seal, `internal/seal`,** which `crypt` and
  `auth.TwoFactor` share: AES-256-GCM under a key HKDF derives from each
  of the app's with an info, the first sealing and each opening, the
  nonce first, in base64url. `TwoFactor`'s info is `tug two-factor`, as
  it was, and its form the same, which a secret sealed by its code before
  shows, in a test that opens it. The session's cookie seals the same
  way, with its name bound to it, and keeps its own code: moving it was
  no part of this.
- **AES-256-GCM,** with a random nonce for each value, as the session's
  cookie has it: the same value sealed twice is two texts, and a text
  changed by a byte doesn't open. A sealed value is base64url, for a
  column, a cookie or a link.
- **What a value belongs to is checked, not kept,** as the session's
  cookie is sealed with its name: GCM's additional data. Without it,
  whoever can write the table can copy one user's sealed token into
  their own row, and the app would use it as theirs. It's any number of
  parts, `"users", "42"`, each after its length and a colon, as tug signs
  its links' parts, so that `"users42"` and `"users4", "2"` are others; a
  value sealed with no owner opens with none.
- **An error, not a panic,** from `crypt.New` for no purpose, no keys, or
  a key that isn't 32 bytes, as `session.New` has it: keys come from the
  environment. `Open`'s error is `crypt.ErrOpen`, whatever the reason, and
  a value that doesn't open isn't `Stale`. `TwoFactor`, which panicked for
  no keys, panics for a key that isn't 32 bytes too, where it derived from
  any length: a shorter key is a weaker one, and `session.KeysFromEnv`'s
  are 32.
- **Bytes in, text out:** a value is bytes, and a struct is the app's
  JSON first, as a job's payload is.
- **Not Laravel's format,** JSON of an IV, the AES-CBC text and an HMAC:
  a value moved from a Laravel app is opened there, and sealed again.
- **Jobs carry IDs,** as M8 has the starter's: a secret a job needs, it
  reads, sealed, from where the app keeps it. Not Laravel's
  `ShouldBeEncrypted`, which seals a payload with the secret in it.
- **No cookies of tug's:** what the app keeps in a browser, sealed, is in
  the session, and a cookie of its own, as a theme a script reads, is
  `http.SetCookie`'s, as a script can't read a sealed one.
- **`tug key` prints, and writes nothing:** production's key is set where
  the platform keeps its secrets, and `.env`, development's, has the one
  `tug new` wrote, which `newKey` now makes for both. A rotation is a
  deploy: the new key in `APP_KEY`, the old one first in
  `APP_PREVIOUS_KEYS`, and the old one dropped once what it sealed has
  moved. `session.ErrNoKey`, and the starters' `.env.example`, say to
  run it where they had `head -c 32 /dev/urandom | base64`, which the
  guide keeps for CI, where there's no tug.
- **Moving to the new key is the app's,** by `Stale`, as the starter
  moves its two-factor secrets as it starts: tug can't know where an app
  keeps what it sealed.
- **Tests:** a value sealed and opened, twice as two texts, and opened
  with a key since rotated, which `Stale` finds, and not once it's
  dropped; one changed in each of its bytes, and text that isn't one; one
  sealed for another purpose, and for another owner, its parts run
  together among them; a `Box` named for two-factor secrets, which opens
  none; a `Box` without a purpose or keys, or with a short key; seals at
  once; a two-factor secret sealed by the code before, opening; and `tug
  key`'s key, as `session.ParseKey` reads it. The guide's CI example lost
  an `APP_DEBUG` it hasn't needed since M19.

## M27 · API tokens — done

An app with an API, for its mobile app, a script or another service, had
no way to let a caller in but the session's cookie, which only a browser
keeps. Laravel's Sanctum gives each user tokens they make and revoke,
each with what it may do, which a request sends in `Authorization:
Bearer`. A token is a password for the API, and made, kept and checked
where a slip is a security hole, in `auth`. And an API that another
site's pages call needs CORS, which tug had none of. Released as
v0.22.0.

- **Tokens:** `auth.AccessTokens` makes a token, random, after the app's
  prefix, as `blog_`, and the hash the app keeps in its place; `Hash`
  finds a token sent by its hash, and `auth.BearerToken(r)` reads one
  from `Authorization`.
- **What a token may do:** its abilities, as `posts:write`, kept with it,
  which a route checks, `auth.Abilities`'s `Can`, and `*` for everything.
- **CORS:** `middleware.CORS(origins...)` answers the browser's preflight,
  and says which sites' pages may call the app, and with what.
- **The auth starter:** an `access_tokens` table in each database's
  layer; a settings page, API tokens, where a user makes one, named, with
  its abilities and an expiry, shown once, sees when each was last used,
  and revokes one; and an `/api` group, whose routes take a token, never
  the session, with `GET /api/user`, the user a token is theirs.
- **The guide:** Accounts, a section on the starter's API tokens and one
  on package `auth`'s, Routing, CORS and `CSRF`'s paths, and Deployment,
  `CORS_ORIGINS`.

Choices made on the way:

- **Kept as a SHA-256, not argon2id:** a token is 32 random bytes, which
  no one guesses, so a fast hash is enough, and one lookup by it finds
  the token, where a password's slow hash is for a secret people choose.
  The database keeps no token a copy of it could use.
- **A prefix of the app's,** so a token pasted where it shouldn't be, in
  a repository or a log, says what it is, and a scanner of secrets can
  look for it. It's letters and digits, and `AccessTokens` panics on
  anything else; the starter's is its name in them, `myblog` for
  `my-blog`, `TokenPrefix` in `tug new`'s data.
- **Two-factor secrets' base32, in lower case:** 52 letters and digits,
  which a double click selects whole, where base64's `-` and `_` would cut
  it. `Hash` is nil for what can't be one of the app's, another prefix, a
  length or letters that aren't a token's, which no lookup needs to find.
- **Shown once,** in the flash, as the recovery codes are, and as GitHub
  shows its tokens: the app keeps the hash alone.
- **The token alone, never the session:** an `/api` route reads the
  token, and no cookie, so another site's page can't borrow a login.
  `CSRF` lets `/api/` through, as `http.CrossOriginProtection` can, and
  CORS says which sites' pages may call it. `CSRF` kept its signature: an
  entry that's a path, `CSRF("/api/")`, is a pattern of `ServeMux`'s whose
  requests pass unchecked, and the rest are origins, as before; one it
  can't take panics.
- **A preflight is answered before any route:** an API's routes have no
  `OPTIONS` of their own, and a group's middleware runs for its routes
  alone, so `CORS` is the app's, and answers an `OPTIONS` from a site it
  names with a 204, the method and headers the browser asked for, and an
  answer the browser keeps for two hours, the most Chrome does. A
  request from another site passes as it came. `Vary: Origin`, unless it's
  `*`, and `Retry-After` exposed, for a limit's 429.
- **No cookies across sites:** CORS never allows credentials, as the API
  takes tokens.
- **What the API says:** a 401 without a token, or with one that isn't
  the app's, has expired or was revoked, with `WWW-Authenticate: Bearer`,
  as RFC 6750 has it, and a 403 that names the ability the token lacks, as
  JSON to a client that asks for it, as tug's errors are.
- **Last used, once a minute at most,** so an API's every request isn't
  a write. The last use and the expiry are Unix milliseconds, as the jobs'
  times are, which compare as numbers in every database.
- **A new password leaves the tokens be:** they're the user's to revoke,
  one by one, as GitHub's are. Deleting the account deletes them, with
  the table's foreign key.
- **An expiry, if the user gives one:** 30 days, a year, or none, as
  Sanctum's `expiration` has it; 30 days unless they choose.
- **No ability chosen for them:** the page's checkboxes start empty, and
  a token needs one, so a token gets what its user gives it and no more,
  and one there isn't is refused.
- **The page posts what it has,** with `router.post`, as the passkeys'
  dialog does, where Inertia's `<Form>` would need an array's name for
  its checkboxes; the expiry is a `<select>`, styled as the inputs are, as
  the starters have no select of shadcn's.
- **Limits by token:** each has 60 requests a minute, counted in the
  `throttles` table by its ID, with the starter's other throttles.
- **Tests:** a token found by its hash, and not by another; what can't be
  a token; a prefix that isn't letters and digits; `Authorization` as
  sent, and abilities; `CSRF`'s paths, and one it can't take; CORS's
  preflight, for a site named and one that isn't, `*`, none, and an
  origin that isn't one; and in the starter, on each database, a token
  made on the page and sent alone, its hash kept, one expired, one
  revoked, another user's, one without the ability, one refused an
  ability there isn't, the 61st request, its last use, the account
  deleted, the page asking for the password again, and CORS; and in each
  frontend, a token made, used, shown once and revoked.

## M28 · Broadcasting — done

`c.Events` sent a page the events its own instance had, and nothing of
another's: a post one instance saved, the pages open on the others didn't
hear of. Laravel broadcasts an event on a channel, through Reverb or
Pusher, and every page listening on it hears it, through Echo. tug does
it with no server of its own: the app's database carries an event
between the instances, as it carries the jobs, and `c.Events` takes it
to the pages. Released as v0.23.0.

- **Package `broadcast`,** with no import of tug. A `broadcast.Hub`
  publishes an event on a channel, as `posts` or `users.42`, and every
  subscriber to the channel, on any instance, gets it: `Publish(ctx,
  channel, name, data)`, and `Subscribe(ctx, channels...)`, which a route
  of `c.Events` sends on; `In(store)` publishes in the app's own
  transaction, and `Run`, which the app runs with `App.Go`, listens.
- **A store for them,** a `broadcast.Store`, `Publish` and `Listen`,
  which carries what one instance publishes to the others: none, for one
  instance, `broadcasttest.Memory` for tests, and the starter's database,
  Postgres's `LISTEN` and `NOTIFY`, and in SQLite and MySQL a table of
  what's new. `broadcasttest.TestStore` checks a store's promises.
- **Who may listen** is the app's: a channel's route checks its user may,
  as the starter's `/broadcasts` is the user's own channel.
- **The auth starter:** the page that asks to verify the email moves on
  once it's verified, in another tab or on another device, by an event on
  the user's channel, in each frontend; and `a.broadcasts`, for an event
  in a handler's transaction.
- **`examples/inertia`:** a post made, changed or deleted in one browser
  shows in the others: the archive reloads its page, and the list its
  stats, with `useEvents`, a hook of its own.
- **The guide:** a page, Broadcasting, and Pages, where one listens,
  Routing's events, Accounts, Testing and Deployment.

Choices made on the way:

- **Over server-sent events, as M25 has them:** not WebSockets, nor a
  server of its own, as Reverb is. A page's requests carry what it says
  back.
- **At most once:** an event reaches the subscribers there are as it's
  published, and nothing is replayed. A page that wasn't following, or
  that connects again after it, reloads its props, which have what the
  event said: the starter's page and the example's reload as their
  `EventSource` opens again.
- **A subscriber that falls behind is closed,** with 16 events it hasn't
  taken, rather than hold the others up, or lose an event and go on: its
  stream ends, and its page connects again and reloads. When the store
  stops listening, as a connection drops, `Run` closes every subscriber,
  as each may have missed one, and listens again, after a second,
  doubling to a minute.
- **The app's database carries it,** as it carries the jobs: Postgres's
  `NOTIFY`, which every listening connection gets as the transaction that
  sent it commits, and elsewhere a table, which each instance reads every
  quarter of a second, from its newest row as it starts, and which a
  job, `prune-broadcasts`, rids of what's a minute old every minute. Not
  Redis.
- **With what it's about, or not at all:** `Hub.In` publishes through a
  store of the app's transaction, the starter's `a.broadcasts.in(tx)`, as
  `Kind.In` pushes a job in one: a `NOTIFY` goes as the transaction
  commits, and a row is there as it commits.
- **MySQL's IDs out of order:** MySQL hands out an ID as a row is written,
  not as it commits, a rolled-back row's never comes, and a server that
  counts by more than one, as one of several primaries, leaves some out.
  So an ID below the highest read is waited for, 10 seconds from when it
  was first missed, and then passed over, and each event is handed on
  once. SQLite writes a transaction at a time, and its `AUTOINCREMENT`
  keeps an ID from coming again once its row is pruned, so its IDs come
  in the order they commit.
- **An event says what changed, not the whole of it:** a `NOTIFY` holds
  8000 bytes, so the hub refuses an event over `MaxEvent`, 7000, its
  channel, name and data together, whatever the store, and a page
  reloads what it shows.
- **One listener an instance,** the hub's, which hands each event to its
  own subscribers, where a connection each would take one from the pool
  for every page open. Postgres's is a connection taken out of the pool
  as long as it listens, and dropped after, as one that `LISTEN`ed is no
  use to anything else.
- **Channels are names,** the app's, and a private one is a route that
  checks: no protocol of their own, as Echo's `/broadcasting/auth` is.
  The starter's `/broadcasts` takes no channel from the request: it's the
  user's own, `users.` and their ID.
- **The verify page's event is a nicety:** the email is verified whether
  the page hears or not, so a publish that fails is logged, not the
  request's error, and the page moves on as it's next reloaded.
- **The example's list is left as it is:** its pages, which scroll, merge
  by ID, so a reload would keep a post that's gone, and fetch the page in
  view where a new post goes on the last. The archive, whose page a
  reload replaces, shows a post where it falls, and the list reloads its
  stats.
- **Tests:** `broadcasttest.TestStore` on the memory store and the
  starter's, in each database: every listener hears, its own too, in
  order, nothing from before it listened, events published at once each
  once, one of `MaxEvent` bytes whole, and `Listen` ending with its
  context; the hub's channels, a subscriber that falls behind, one whose
  context ends, what it can't publish, an event the store carries that
  isn't one, and `Run` listening again; in the starter, an event in a
  transaction that rolls back, which no one hears, one that commits after
  a later one's, on Postgres and MySQL, two instances on one database,
  the verify page's event, a guest turned away, a user's stream that
  carries their channel alone, and the table pruned; the example's
  stream of posts made, changed and deleted; and in the browser, the
  starter's verification in another tab, in each frontend, and
  `examples/inertia`'s post in another browser.

## M29 · The queue, further — done

A kind's jobs shared the queue's workers, with nothing to hold them back
but one at a time for each value: a hundred photos to resize took every
worker, and a newsletter's ten thousand mails went as fast as the workers
took them, past what the mail provider allows a second. And a job that
failed for good was logged, with nothing else done about it, where a job
of Laravel's has a `failed` method, which marks what it was about.
Released as v0.24.0.

- **At most so many at once:** `queue.AtOnce(n)`, an option of a kind's:
  no more than n of its jobs run at once, on all the instances.
- **At most so many a time:** `queue.Rate(limiter)`, as `tug.Limit` takes
  one: a kind's jobs start no faster than the limiter lets them, as an
  `auth.Throttle` on the starter's table counts them on every instance.
- **When a job fails for good:** `queue.OnFail(fn)`, a kind's: `fn` gets
  the job's value and its error once the job is kept as failed, to mark
  what it was about, or to tell someone.
- **Stores:** `queue.AtOnceStore` and `queue.HoldBackStore`,
  `queuetest.TestStore`'s promises for them, and the starter's claims in
  each database.
- **The guide:** Background jobs.

Choices made on the way:

- **On all the instances:** a limit on each would grow with them, where
  a limit is what a resource, a CPU or a provider, can take.
- **The claim counts,** as it passes over a `OneAtATime` job while one of
  its value is held: a job of a kind with n held waits. The job carries
  its kind's limit, `Job.AtOnce`, in an `at_once` column, as a
  `OneAtATime` job carries its key, so the claim needs nothing but the
  job, and every held job of the kind counts, one pushed before the kind
  had a limit too. In Postgres and MySQL, a claim takes the kind's
  `job_locks` row before it counts, the row whose key is empty, as no
  job's is, after its key's row, as claims at once would otherwise each
  find room.
- **A rate through a `Limiter`,** the `Try` that `auth.Throttle` has, so
  the queue imports no `auth`, and counts where the throttles do: tried
  by the kind's name, for each job as it's claimed.
- **A job held back isn't an attempt:** it goes back as it was before its
  claim, through a `HoldBackStore`'s `HoldBack`, its attempt taken back
  and its key given back, due as it was, so it keeps its place. And its
  whole kind is held back until the limit lets one start, in a
  `held_kinds` table, which the claim checks: with the job alone held
  back, the newsletter's thousands would each be claimed and refused in
  turn, many a second on every instance, where now one is, each time the
  limit is up.
- **A limiter that fails holds the job back** for 10 seconds: the
  throttles' store is the database, which is down, and the job neither
  runs past the limit nor loses an attempt.
- **`OnFail` runs once,** after the store has kept the job as failed, on
  the instance that ran it, with a context of its own, done after the
  kind's `Timeout`; its own error, and a panic, go to the log. It's
  `OnFail[T]`, whose type `Handle` checks is the kind's, as an `Option`
  has none; a job that failed as its value didn't fit has none to give
  it, and it isn't run.
- **An `AtOnce` kind's job wakes the queue as it ends,** as a
  `OneAtATime` kind's now does too, where it waited for the next poll: the
  next of its kind, which the claims passed over, runs at once. Two at
  once of jobs of 200 ms run ten a second, not two.
- **`Drain` stops once its context is done:** a Store in memory ignores
  it, and a job held back again and again would have it claim for ever.
- **MySQL counts the kinds held back,** where the others say `NOT
  EXISTS`: MySQL makes a `NOT EXISTS` beside the claim's other conditions
  a join, whose jobs it sorts, locking every one it reads, so the claims
  beside it found them all locked. `TestStore`'s claims at once found it.
- **Not chains or batches,** as Laravel's `Bus` has: a job that pushes
  the next when it's done is a chain, and a batch needs a table of its
  own, with nothing yet asking for one.
- **Tests:** the limits on the memory store and the starter's, in each
  database, with claims at once, as from two instances: a kind held to
  its `AtOnce`, taking the next once one is done, put back, failed or
  lost, with other kinds beside it and a job without a limit counted; a
  job held back claimed again at its time as it was, its kind held and
  other kinds claimed, its key given back unless another has it, and a
  late hold back that leaves the job alone. In the queue, jobs at once up
  to `AtOnce`, the next as one ends with no poll to wake it, and other
  kinds beside; a rate that holds back, its limiter tried once a job, the
  attempts kept, on two instances, and a limiter that fails; `OnFail`
  with the value and the error, once, after the store kept the job, its
  error and its panic logged, a value that doesn't fit, and its type
  checked. In the starter, a rate through its throttles table, with the
  job's attempts and the kind's hold, and the held kinds pruned; a kind
  with neither, as before, in the rest.

## M30 · Authorization — done

tug said who's logged in, and nothing of what they may do: each app wrote
its checks its own way, and its own 403s, and a page that hid a button its
user might not press asked its handler for a flag of its own. Laravel's
gates say what a user may do with a thing, `authorize` turns a no into a
403, and an Inertia page gets what its user may do as props. And the auth
starter's users were all alike, so the jobs that failed for good were
listed by its binary's command alone: a page for them waited, in
Background jobs, for someone who may see every user's jobs. Released as
v0.25.0.

- **Abilities, in package `auth`:** `auth.NewAbility`, what a user may do
  with a thing, as edit a post: a check of the user and the thing that
  says yes, or no, and why not with `auth.Deny`. `Can` asks, and `Check`
  returns the no as an error, an `*auth.Denial`, "you may not edit this
  post", for the handler to return.
- **A no is a 403:** tug answers an error that says its status with it,
  and its message, as it does a `*tug.HTTPError`: the error page in a
  browser, and JSON for an API.
- **Before the abilities:** an `auth.Gate`, whose `Before` answers every
  ability made with it first, as an admin may do anything, or a suspended
  account nothing.
- **On the page:** what its user may do, in props of its own, `can`, so
  the page shows the buttons its user may press, and the handler checks
  again when one is.
- **The auth starter:** admins, whose `admin` column the `admins`
  command sets, `./blog admins add ann@example.com`, and a page for them,
  `/admin/failed-jobs`, of the jobs that failed for good, with their
  errors, which runs them again, as the `jobs` command does.
- **The guide:** a page, Authorization; Accounts, the admins; Routing,
  the errors that say their status; and Background jobs, the page.

Choices made on the way:

- **In `auth`,** beside the logins, with no idea what a user is: an
  ability is generic over the app's user and the thing, both the app's
  own types, `*auth.Ability[*User, *Post]`, which the check's types say.
- **Abilities are values,** `var editPost = auth.NewAbility(...)`, used
  where they're checked, where Laravel's gates are names in strings: a
  mistyped one doesn't compile. `NewAbility`, as `Ability` is the type's
  name.
- **A check is `(bool, error)`:** most say yes or no, `p.Author ==
  u.ID`, and one that says why returns `false, auth.Deny("the post is
  locked")`; any other error is a failure, a 500, as a query that fails
  isn't a no. `Can` returns the failure as its error, and never counts
  it a yes.
- **Policies are the app's:** the abilities over one type are a struct of
  them, or a file of them; Go has no discovery by name to hang a
  convention on.
- **The no says why,** to the person who gets it: the check's reason when
  it gives one, or else "you may not" and what the ability is.
- **A guest may do nothing an ability names:** the zero user, a nil
  `*User`, is a no, with neither the gate nor the check asked, so a check
  reads its user without looking for nil.
- **By its status, not its type:** tug answers an error with a
  `StatusCode() int`, as auth's no has, so `auth` imports no tug, as the
  core imports no `auth`; from 500 up, with the status's text, as the
  error's own words may not be the client's.
- **The gate's `Before` is `(bool, error)` too:** true lets the user do
  anything, a `Deny` nothing, and false leaves it to the ability.
- **`can` is shared,** what's the app's as a whole, for the header's nav,
  worked out with `auth` as each page is rendered; what's one thing's
  goes in its page's props.
- **One admin column,** not tables of roles and permissions: an app with
  roles checks them in its abilities. The command is `admins`, with
  `add` and `remove`, and a list, as `jobs` is, rather than a `users`
  command of one verb.
- **The admins' page asks a verified email** too, as the dashboard does,
  and a job run again from it wakes the queue, where the command's waits
  for the next poll of the app that serves.
- **Tests:** an ability's yes, no and why, a reason left empty, the gate
  before it, a failure that isn't a no, and a guest, whom neither is
  asked; errors that say their status as text, JSON and the error page,
  wrapped too, and a server error's words kept back; and in the starter,
  the admins command, the page refused to a guest and a user, and not in
  their `can`, and an admin's, its list, a job run again, and again, and
  all of them; and in the browser, in each frontend, a user's 403 and an
  admin, made by the command, finding the page in the header.

## M31 · Notifications — done

An app tells its users what happened to their account or their work: a
passkey added, a report ready, a reply. The auth starter mailed the few it
had, so a user who didn't read the mail saw nothing in the app, and the
owner of an account whose email was changed heard nothing at the old
one. Laravel's notifications go by mail, into the database, where the app
lists them, and to the pages open, each by the channels it names. tug had
each part, mail, the queue, the database's stores and broadcasting, and
the auth starter puts them together. Released as v0.26.0.

- **Kept for the user:** a `notifications` table in each database's
  layer, of a notification's kind, its data as JSON, and when it was made
  and read, kept in the transaction that makes the change it tells of.
- **Told at once:** published on the user's channel as the transaction
  commits, so the header's bell counts it without a reload.
- **And mailed,** by a job pushed in the same transaction, made from the
  notification as it runs.
- **A bell and a list:** the header's bell, with the unread count, a
  shared prop, `bell`; a page of the user's notifications, the newest
  first, 20 at a time, each with its line and its link; and marking them
  read as the page shows them.
- **The account's changes:** a new password, in the settings or by a
  reset, a new email, told at the old one too, two-factor logins turned
  off, a passkey added and an API token made: each a notification and a
  mail, so the owner hears of a change they didn't make.
- **`mailtest.Outbox.NextTo`,** the next mail to an address, as a change
  that mails two addresses mails them in any order.
- **The guide:** Accounts, a section on notifications; Testing, `NextTo`;
  and Broadcasting.

Choices made on the way:

- **The app's code, not a package:** its kinds, their text and their
  channels are the app's, on tug's mail, queue and broadcast, as its mail
  is: `notices`, in `notifications.go`, a line, a page and a mail for
  each, and `a.notify`, what a handler calls.
- **Kept, then told:** in the transaction with the change, so a
  notification of a change that rolled back is never shown, mailed or
  heard. So the changes that wrote on their own now write in one: a new
  password, a reset, two-factor logins turned off, a token made.
- **One mail job for them all,** `notification-mail`, which carries the
  notification's ID, and makes the mail from it as it runs. The passkey's
  mail, a job of its own, is its notification's now, in the same words.
- **The line at read time,** from the kind and its data, in the reader's
  language, `c.T`: a notification outlasts the language it was made in.
  Its mail is in the app's words, as the rest of its mail is.
- **The old email hears of a new one,** as the address the owner may
  still read, with a link to log in; the new one gets its link to verify,
  as before.
- **Read as the list shows it,** by the range of the IDs it shows, which
  a notification made since is past: no button, as the bell's count drops
  as the page renders, and a dot says which were new to it.
- **A kind the version doesn't have is still shown,** with a line of its
  own, and marked read: left out, it would shorten a page, which
  `SimplePaginate` takes for the last, and stay in the bell for good.
- **The bell is shared as `bell`,** and the page's list is `list`, as a
  page's prop of a shared one's name would hide the shared.
- **One connection a page:** `listen`, in `resources/js/lib/broadcasts.ts`,
  shared by all three frontends, keeps one `EventSource` for whatever
  listens, the bell, the list and the verify page, and calls each
  listener as its event comes and as the connection is made again. The
  verify page reloads on `verified`, as its handler sends a verified user
  to the dashboard.
- **What comes in the flash, the page keeps:** the bell's reload is a
  visit, and empties the page's flash, and a token's own notification
  rings it: the new token showed for a moment, then went, as CI's browser
  suite caught. The tokens page and the recovery codes keep what came
  until the page is left, rather than the bell's count coming apart from
  the page's props.
- **The bell waits for a visit in flight,** since v0.31.1: a run of CI's
  browser suite caught it after v0.31.0, two toasts of "Two-factor logins
  are off.": the change notifies, and the bell's reload, when the event
  came as the form's visit was still loading the page it goes back to, ran
  beside it, as Inertia's reloads are async, so both read the session with
  the flash the form left, and both showed it. `listen` holds its
  listeners' reloads while a request of Inertia's is in flight, counted
  from the `inertia:start` and `inertia:finish` it fires on the document
  for each, and runs each once the last finishes. The flash can't be kept
  from it on the server: the two requests carry one cookie.
- **A partial reload leaves the flash,** since v0.33.1: the bell's wait
  is in its own tab, and a run of CI's browser suite caught it across
  two, after v0.33.0: a token made in one tab rang the bell of the
  other, whose reload, as the first tab's redirect loaded its page, read
  the session with the new token as that did, and showed it there. A
  partial reload of the page it renders shows none of the session's
  flash, or a form's errors, and `Session.Pass` writes no cookie for a
  request that changes nothing, so the flash is there for the page it
  was left for, whichever comes first, and shown once: a reload that
  kept it, by reflashing, would bring it back as the cookie it wrote
  came after the page's.
- **Kept a while:** a notification read over 90 days ago goes, by a
  scheduled job, `prune-notifications`, every night.
- **Not each user's choice of channels yet:** each kind has its own.
- **Tests read mail by address** where two can come at once: `NextTo`,
  which takes the next mail to an address, and leaves the others for
  `Next`. And a test that claims jobs of its own stops the app's queue
  first, `stopQueue`, as the admins' test of M30 didn't, and now and then
  lost its job to the queue, which runs beside it.
- **Tests:** each change's notification, in the list, the bell and the
  mail, the new email's at the old one; one whose change was rolled back,
  never kept; one heard on the user's channel; the list, the user's own,
  the newest first, a page at a time, marked read as shown, with a kind
  long gone; the prune, and its schedule; `NextTo`, taking one and
  leaving the rest; and in the browser, in each frontend, a token made in
  another tab ringing the bell, and the list showing it read, and a new
  token and the recovery codes still shown as it rings.

## M32 · Security headers — done

A page tug serves says nothing of what a browser may do with it: be put
in another site's frame, send its whole URL to the next site as the
Referer, or run a script it didn't bring. An escaping template stops most
injected scripts, and a Content-Security-Policy stops the rest, but only
once every script the app runs carries the response's nonce, which is
hard to add to an app that has grown. Rails sets the headers and helps an
app write a policy with a nonce; Laravel leaves both to packages.
Released as v0.27.0.

- **`middleware.Headers`:** the headers every response carries:
  `X-Content-Type-Options: nosniff`, a `Referrer-Policy`, a
  `Cross-Origin-Opener-Policy`, no framing by other sites, and
  `Strict-Transport-Security` over HTTPS.
- **A Content-Security-Policy with a nonce:** a new one for each
  response, which the root template's scripts carry, `{{ .Nonce }}`, as
  Vite's tags and the page's head from SSR do, so the app's own scripts
  run, and no others.
- **Report-only first:** the policy as
  `Content-Security-Policy-Report-Only`, with a route that logs what it
  would have blocked, for an app to see before it enforces.
- **The starters** send both, the appearance script in their `app.html`
  with the nonce, and `tug dev` lets Vite's dev server in.
- **The guide:** Deployment, a section on them; Pages, the root
  template's nonce; Routing, the two middlewares; and SSR, Files and
  Accounts, what's theirs.

Choices made on the way:

- **Middleware, as the rest is,** with no import of tug: two, `Headers`
  and `CSP`, each with a config of its own, as the policy has its nonce,
  its reports and its sources, and an app may send the headers without
  it.
- **The nonce in the request's context,** which inertia hands the
  template: in `internal/nonce`, which `CSP` sets and inertia reads, as
  neither imports the other, the way `internal/rw` is shared;
  `middleware.NonceFrom` is the app's.
- **Scripts by nonce, with `'strict-dynamic'`,** as the modules Vite's
  entry loads carry none; not hashes, as a template's scripts change.
- **Vite's tags take the nonce first,** `{{ vite .Nonce
  "resources/js/app.tsx" }}` and `{{ viteReactRefresh .Nonce }}`, as a
  template's function can't see the request. A template from before,
  whose first argument is an entry, is an error that says to pass
  `.Nonce` first, rather than tags that load nothing; an app without a
  policy passes `""`, and its tags have no nonce. The styles' links carry
  none, as `style-src` takes none.
- **Styles as they are:** shadcn's components and Inertia's progress bar
  set styles inline, and an injected style does little a script can't;
  `style-src` allows them.
- **Reports by `report-uri`,** which every browser sends as it happens,
  answered by `CSP` itself, before any route and `CSRF`, as `CORS`
  answers a preflight, and logged as a warning, leaving out what the
  browser left empty; not `report-to`, which Chrome alone sends, batched,
  and to HTTPS alone.
- **The policy, beyond the plan:** `data:` images and fonts, as the
  starters' favicon and Vite's small fonts are, and `blob:` images, for a
  photo shown from the file chosen; `object-src 'none'`; `base-uri
  'none'`, as a `<base>` would send the paths of the scripts that carry
  the nonce to another site; `form-action 'self'`; and `frame-ancestors
  'self'`, with `X-Frame-Options` for a policy that's report-only.
  `Sources` adds to a directive, as a payment provider's frames.
- **The headers:** `Referrer-Policy: strict-origin-when-cross-origin`,
  the origin alone to another site; `Cross-Origin-Opener-Policy:
  same-origin`; and HSTS for a year, without preload, a promise to the
  browsers that's slow to take back, and without `includeSubDomains`, one
  about hosts not the app's. It goes when the app says it's served over
  HTTPS, as an https:// `APP_URL` says in the starters, as the session
  cookie's `Secure` does, behind a proxy that ends TLS.
- **The SSR head's scripts get the nonce,** as a `<Head>`'s script runs in
  the browser, where Inertia makes it from a script with the nonce: in
  the page the server rendered, the nonce alone lets it run. The page
  object is data, and runs as no script.
- **The starters enforce the policy,** as their scripts all carry the
  nonce, and report to `/csp-reports`; report-only is for an app that has
  grown. `tug dev` needs nothing of its own: the policy asks Vite's
  `DevServer`, at each request, as the tags do.
- **Images from the app's disk:** S3's origin, when the files are there,
  as the auth starter's photos are links to it: `(*storage.S3).Origin()`,
  the address its links are at, added to `img-src`.
- **`examples/inertia`** sends both too, and its test finds the header's
  nonce on its scripts.
- **Not yet:** a `Permissions-Policy`, Trusted Types, or styles by nonce.
- **Tests:** each header, on every status, HSTS only when asked for, and
  a handler's own; a nonce for each response, which `NonceFrom` reads;
  the policy's directives, report-only, the sources added, and the dev
  server's; a report logged, in either mode, and what isn't one turned
  away; the settings `CSP` can't send, which panic; the template's nonce,
  and the scripts of the head from SSR, but not the page object; Vite's
  tags, built and from the dev server, with the nonce, and a template
  from before refused; S3's origin; a page, JSON, a file, an error and a
  404 through tug, with the headers; the example's scripts with its
  header's nonce; each SSR app `tug new` makes, served, its scripts with
  the nonce; and in the browser, in each frontend, the headers on a page
  and a 404, a script the page didn't bring blocked, its report in the
  app's log, and every other test's pages with nothing blocked.

## M33 · Migrations — done

The auth starter's tables are a list of SQL steps in its `db.go`, one list
for each database, which the app runs as it starts, counted in SQLite's
`user_version`, or a `schema_version` table. An app adds a table of its
own as a step at the end of the list, in Go; two branches that each add
one both make the next step, and a database that ran one branch's counts
it as run, whichever step takes its place in the merge; and nothing says
which steps a database has, or makes the next one. Laravel's migrations
are files, named for when they were made, which `make:migration` writes,
`migrate` runs, and `migrate:status` lists; Rails' are too. Released as
v0.28.0.

- **Migrations are files:** `migrations/`, a `.sql` file each, named for
  when it was made, as `20261001093000_create_posts.sql`, embedded in the
  binary, as `lang/` is, and each run once, in its name's order.
- **Package `migrate`:** runs the files that haven't run, with no import
  of tug, and no SQL: a Store, the app's, keeps which have run, takes the
  lock that has instances take turns, and runs a file and records it
  together, where its database can. `migratetest.TestStore` has a Store's
  promises as tests, as the other packages have theirs.
- **The commands:** `tug migrate new create_posts` writes a new file, and
  the app's binary runs what hasn't run, `./blog migrate`, and lists what
  has, `./blog migrate status`, commands the starter adds with
  `app.Command`.
- **Undoing the last one:** a file may end with a part that undoes it,
  after a `-- down` line, which `./blog migrate down` runs, for the file
  just written that isn't right yet.
- **The starter's own** are files: each step its layers have in `db.go`
  now, in the order they have them; and each layer has a Store, in its
  database's SQL. An app made before carries on: the steps its
  `user_version` or `schema_version` counts are the files it has run.
- **Still as the app starts,** as now, and by the command for a deploy
  that runs them first, and for a person to see where a database is.
- **The guide:** a page, Migrations; Accounts, the database; Deployment;
  the CLI, `tug migrate`; and the READMEs.

Choices made on the way:

- **M6's choice reopened, at the owner's ask:** the migrations were the
  starter's own code, a list in `db.go`; they're files an app adds, which
  a package of tug's runs. M17's holds: tug has no SQL, and a Store in
  each database's layer keeps what's run, as the jobs' Store does.
- **Files of SQL, not Go:** a migration is SQL, which a person and the
  database's own tools read. A change of data that SQL can't make is a
  job, or a command of the app's.
- **Named for when they were made,** to the second, in UTC, so two
  branches' files don't take one number: each runs, in its name's order,
  whether a later one ran first or not, as Laravel's and Rails' do. The
  starter's own are named for the commits that added their steps, and
  Postgres's and MySQL's first five, from M17's, a second apart.
- **What's run is kept by name and a hash of its SQL,** the part before
  `-- down`, with its lines ending as Unix's, so a file saved on Windows
  is the same: a file changed after it ran stops the next run, before any
  runs, and says which. One that ran but that the files don't have, as a
  newer version's, is left alone, as an old instance of a deploy runs
  beside the new ones.
- **A file is one transaction,** where the database has them for changes
  to tables, SQLite and Postgres, with its record, and a Store's `Run`
  looks again there whether it has run, as SQLite has no lock for the
  instances to take turns under. MySQL commits each such statement on its
  own, so its Store runs a file a statement at a time, split where a line
  ends in `;`, and keeps the one under way, as `schema_version`'s
  `running` did: a crash, or a later statement that failed, stops the
  next run, saying which; a first statement that failed did nothing, and
  leaves nothing. Postgres's instances take turns under an advisory lock,
  MySQL's under `GET_LOCK`, each on one connection.
- **Down for the last one alone,** the name that comes last, when its file
  says how: a step just written is put right, and run again. A migration
  that ran long ago is changed by the next one. The starter's own have no
  down part.
- **The migrate command runs them itself,** where every other run of the
  binary, the server and the other commands, runs them as it starts:
  `main` leaves them to it, `migrating`, so `./blog migrate` in a deploy
  says what it ran, and a new database's status says none has.
- **The count of an app made before is taken over** under the lock, as
  the first start of the new version: the files it counted recorded as
  run, and the count dropped. More counted than there are files is an
  app's own steps, which go in files of their own first.
- **Opening waits its turn,** found by the test of instances starting at
  once: SQLite's `busy_timeout` comes before its `journal_mode`, and a
  new file, whose log the first open makes, is opened again while SQLite
  turns an instance away, as two making it at once can't each wait for
  the other; SQLite's migrations table is made in a transaction that
  takes the lock first, as a statement that reads, then writes, fails at
  once when another wrote meanwhile. Postgres's and MySQL's `openDB`
  connects, so a server that isn't there is said as the app starts, with
  compose's hint, as the migrations' first query said it before.
- **`tug dev` builds again** when a migration is written, which the app
  runs as it starts; a file with no SQL yet stops it, saying which.
- **Tests:** the files loaded in order, split at their down part, and a
  name or a file refused; the run in order, under the lock, a changed
  file stopping it before any runs, a newer version's left alone, a
  failure stopping the rest; down, the last by name, and none that can't;
  the status, and the command; `tug migrate new`'s file, in UTC, a taken
  second, and no `migrations/`; `TestStore`, on each database's table; the
  starter's migrations run once, beside a newer version's, and by four
  instances at once; eight instances opening a new SQLite file at once;
  an app made before, at this version and three steps before, and with
  its own steps not in files; on MySQL, a later statement that failed, a
  migration stopped in its down part, and a step of an app made before
  under way; the command; and the binary's `migrate`, on a new database,
  which runs them itself, and its `jobs`, which runs them as it starts.

## M34 · Hooks for metrics — done

A deployed app's metrics, how many requests it answers, how slowly, and
which fail, by route, and how its jobs go, by kind, are counted by a
client of its monitor's, Prometheus's own Go client, OpenTelemetry's, or
a platform's agent, from what the app sees. Two things it can't see from
outside tug. Which route answered: `ServeMux` keeps the pattern on the
request it's handed, a copy the middleware in between made, so the app's
own middleware, around the router, never has it, and `middleware.Logger`
logs the path, which has as many values as the app has posts. And how
each job went: a handler the app wraps sees its own error, not what the
queue made of it, whether the job runs again, failed for good, or was
held back by its kind's rate, nor how long it waited to run. tug gives
the app both, and leaves the counting to the client it picks: Phoenix
emits events for its reporters to count, and Laravel's Pulse is a
package of its own. Released as v0.29.0.

- **The route that answered:** `tug.RouteOf(r)`, the route that answered
  `r`, as `GET /posts/{id}`, for the app's own middleware, around the
  router, to read once the handler has run: a request's label, with as
  many values as the app has routes.
- **In the log:** `middleware.Logger` logs the route beside the path.
- **How each job went:** `queue.Config`'s `Observe`, a function the
  queue tells as each run of a job ends, a `queue.Ran`: its kind and
  attempt, whether it's done, runs again, failed for good, or was held
  back by its kind's rate, how long it ran, and how long it waited past
  its time.
- **The guide:** Deployment, a section on metrics, with the two hooks
  counted by Prometheus's own Go client in the app, the requests by route
  and the jobs by kind; Routing, `tug.RouteOf`; and Background jobs,
  `Observe`.

Choices made on the way:

- **Hooks, not a package of metrics:** counting, and the format a monitor
  reads, are a client's of the app's choosing, which tug has nothing to
  add to, and an app that takes Prometheus's, with its modules, does so
  in its own `go.mod`, not tug's. Each hook is of use without metrics
  too: the log has the route, and a trace both.
- **`RouteOf`, not `Route`,** which is already the type of a route. It
  says the route as its `String` does, as it was added, `ANY` for a route
  of any method, and without the `{$}` tug adds to a path that ends in a
  slash, which `String` now leaves out too.
- **The route by context, where it's needed:** as a request reaches an
  App with middleware of its own, before it, tug puts a place for the
  route in its context, which the router fills in, so that middleware
  reads it after the handler, whatever copies of the request were made in
  between. That's a copy of the request, and the place, one allocation, a
  context of its own: 2 allocations and about 90 ns a request, which an
  App with no middleware, as the benchmark's, doesn't pay, as only
  middleware outside the router needs it. Inside the router, `RouteOf`
  reads the place, or with none, the pattern `ServeMux` keeps.
- **A route answers as it's matched,** before its own middleware and its
  group's, so a redirect to log in that a group's middleware sends is the
  route's.
- **A miss has no route:** a 404, a 405, or a redirect to the path
  without its trailing slash, which the catch-all answers, has "", for a
  metric to count as it likes. An app's own route of everything is a
  route.
- **What the queue made of it:** `Done`; `Retried`, after its backoff, as
  a job stopped with the queue is too; `Failed`, at its last attempt or
  by a `Permanent` error; or `HeldBack` by its kind's rate, which isn't an
  attempt, and is the attempt it would have been. `Took` is the
  handler's time alone, not the rate's check before it, and `Waited` is
  from the job's time to its claim, which a job held back waits through
  too. An `Outcome`'s `String` is a metric's label: `done`, `retried`,
  `failed`, `held back`.
- **Told once the Store has kept it,** in the worker that ran the job, so
  `Observe` is quick, as a client's counter is, and `queue` imports
  nothing of a monitor's. A run the Store couldn't keep isn't told: the
  job runs again, and that run is. A panic of `Observe`'s goes to the
  log, as `OnFail`'s does, and the worker goes on.
- **The starters change nothing** but what their log lines have: a route
  of `/metrics`, and its token, would be in every app made, for a monitor
  the app may not have.
- **Tests:** the route read by the App's middleware once the handler has
  run, and not before, through a middleware that copies the request, for
  the home page, a path ending in a slash, a route of any method, HEAD of
  a GET route, a group's own page and its routes, and a route whose own
  middleware answers; none for a 404, a 405 and a redirect to the path
  without its slash, nor for a request that came through no App; a
  handler's own route, with and without the App's middleware, and in the
  ErrorHandler of a miss, none; an app's route of everything; the log
  line's route, and none when no route answered or the request came
  through no App; and each way a job's run goes, told once with its kind,
  attempt, error, and the times it ran and waited, a run held back, a run
  the Store couldn't keep, not told, and an `Observe` that panics.

## M35 · Inertia DevTools — done

An Inertia page's props come from the server, and the browser can't say
which were shared, deferred, merged or kept once, nor which route and
handler rendered the page, and where. Inertia's DevTools, a panel of the
browser's own, as an extension, records each visit, the client's side of
it paired with what the server says of it, by a protocol each server
adapter implements, Laravel's being the reference. tug's adapter is its
own, so the server's side is tug's to write, for `tug dev`, where an app
is made. Released as v0.30.0, and with the route's action made right,
as v0.30.1.

- **Recorded under `tug dev`:** each response says its entry's ID and
  batch, `X-Inertia-Devtools-Id` and `X-Inertia-Devtools-Parent-Out`, and
  a first visit's page carries the ID in a tag, for the panel to find
  before any visit; the request's tab, visit, batch, and whether it's a
  deferred prop's or a poll's, come in the extension's own headers.
- **An entry:** the request's kind, as the protocol names them
  (`initial`, `navigate`, `partial`, `deferred`, `poll`, `prefetch`,
  `precognition` or `http`), its method, URL, status, redirect and time;
  its headers and bodies, the page object for an Inertia response; each
  prop's type, as the protocol has it, `defer` with its group, `merge` and
  `scroll` with their direction, `once`, `optional`, `always`, shared or
  not, and where, and rescued; the props' values; the route, its path and
  name, and the app's function that answered, and where it's defined;
  where the page was rendered, and the page's file.
- **The panel's two endpoints:** `GET /_inertia/devtools/entries`, the
  newest first, by component and kind, a part at a time, and
  `GET /_inertia/devtools/entries/{id}`, as the protocol has them.
- **The guide:** Pages, a section on the DevTools; Routing, `DevTools` in
  the Config; Deployment, `TUG_DEV`, to leave unset; and the CLI, `tug
  dev` and `.tug`.

Choices made on the way:

- **In development alone:** on when `tug dev` runs the app, by
  `TUG_DEV`, through `Config.DevTools`, which `ConfigFromEnv` sets; off in
  the binary a deploy runs, where nothing is recorded, no header is sent
  and the endpoints aren't there, and each hook is a nil check, so the
  benchmark of a request is as it was, three allocations. No gate for a
  deployed app's developers, which would need the app's own logins.
- **Kept in files:** `.tug/devtools`, which git leaves out, and `tug
  dev`'s watcher too, as it does every directory starting with a dot; an
  entry each, written aside and renamed, named by its ID, as `tug dev`
  builds the app again at each change, which an instance's memory would
  forget, and an index of their `__meta`s, read from the files at the
  first use. The newest 100 of each tab, and of the requests with none, as
  from a browser without the extension, for a day at most, as Laravel's
  are by default, pruned every five minutes as an entry is kept.
- **Answered before the App's middleware,** as `middleware.CSP` answers
  its reports: the session's flash isn't taken by the panel's fetch, which
  can come between a form's redirect and the page it leads to, the
  endpoints' own requests aren't recorded, and the rest are recorded as
  they went out, the session's cookie and all.
- **Secrets never kept:** values under Laravel's keys, as `password`,
  `token` and `secret`, and `recovery_codes`, which the auth starter's
  flash carries, matched in any case and without their underscores and
  hyphens, as Go's and JavaScript's names are `accessToken` as often as
  `access_token`, in the props, the bodies and the URL's query; and the
  headers of logins, as `Cookie` and `Authorization`; all kept as
  `[REDACTED]`. An upload as its name, size and type, from the form
  `Bind` read; a body that isn't text, is a stream, or is over 256,000
  bytes, left out, saying why, by the protocol's reasons; and the body of
  a write that didn't come from Inertia's client, as Laravel's is.
- **A body as it was read:** the request's, as the handler read it
  through the request, and then what it left, up to the limit, as
  net/http reads it before the connection's next request, so a handler
  that only redirects has its body too: but not from a client that asked
  for a 100-continue, which hasn't sent what wasn't read, nor from a
  connection taken over. An empty body is empty before it's anything
  else, as a redirect's.
- **The function that answered, from the stack:** the auth starter wraps
  nearly every route's handler, as `a.guestsOnly(loginPage)`, so the
  route's own handler is a closure of the wrapper's, `guestsOnly.func1`,
  which says nothing of the page. The action is instead the app's
  function on the stack as the response's status is written, inside the
  route's handler: the first frame that's neither tug's, its tests and
  examples aside, nor the standard library's, before the frame of tug's
  adapter of the handler, whose function's start the adapter records
  (`devtools.Here`). That's the handler that rendered the page or
  redirected, or a wrapper that answered for it, as `guestsOnly` sends one
  who has logged in to the dashboard, with where the function is defined,
  as Laravel's action is the controller's method; one the compiler
  inlined, whose start the runtime doesn't keep, is where it is. A
  response tug wrote for the handler, as for the errors it returned, once
  it had returned, names the function that read the request, which `Bind`
  and `Validate` record, and with none, the route's own handler, without
  a method value's `-fm`, and the line that added the route, as Go keeps
  no line of a method value of its own.
- **tug's frames by their files:** v0.30.0 walked past the handler, to
  the Recorder's frame, and told tug's frames by their functions' names;
  its own sample, a failed login in the auth starter, named
  `main.newApp.Headers.func7.1`, the closure of `middleware.Headers`,
  which the compiler had inlined into `newApp`, so it took the app's
  name, in tug's file, and would have named any middleware of the app's
  that was on the way. A frame is now tug's by its file too, under the
  directory of tug's own, and the standard library's by the directory of
  `net/http`'s, where the build names them; a package's path is the
  rest, a first element with no dot the standard library's, unless it's
  the app's own module's, as `tug new`'s `blog`, from the build's info.
  The walk stops at the adapter, and the frames before it are the
  handler's.
- **Where else it was, from Go:** where `c.Inertia` was called, by the
  same walk of the stack; where `Share` or `ShareFunc` was called, kept as
  they're called, a prop shared by a function at the function's, as the
  page got that one; `errors`, tug's own, shared, with no place; and the
  page's file, `resources/js/pages/<name>`, by each extension a starter's
  page has, when it's there.
- **Inertia's part in package `inertia`,** which imports no tug: the
  resolver tells a hook of each prop as it goes out, a plain value at the
  top, and a prop of a type at any depth; the values are read from the
  page as JSON has it, as the client gets it. A deferred prop is `defer`,
  with its group, on its own fetch, which the extension's
  `X-Inertia-Devtools-Deferred` says, and on a partial reload is as any
  prop, as Laravel's; `scroll` keeps its group when deferred; a merge
  prepends when it's `Prepend`, prepends at a path with none appended, or
  is a scroll's page before, and deep-merges when it's `DeepMerge` or
  matched on a key. A first visit's tag is a 200's alone, as the protocol
  has it, of JSON, which a page's Content-Security-Policy leaves alone,
  as it doesn't run. tug's part, the request, the route and the
  endpoints, meets it in the request's context, as the route's place
  does.
- **Recording never fails a response:** an entry that can't be made or
  kept is dropped, and the log says why, at Debug.
- **IDs as the protocol's,** ULIDs, made on the standard library,
  monotonic within a millisecond, which order the list; an ID that isn't
  one is a 404 before any file is read.
- **Tests:** each response's ID and batch, a prefetch's its own; each
  kind of request; a first visit's tag, and none on a visit after or an
  error's page; each kind of prop, shared or not, by `Share` and by a
  function, where, deferred on its fetch, optional on a partial reload,
  and rescued; the route's action, the function that rendered, a wrapper
  that answered for it, and the route's own for an error tug answered;
  where the page was rendered, and its file; secrets redacted, in props,
  bodies, the query and headers; an upload summarized; bodies left out by
  their reasons, and one the handler didn't read kept; the endpoints,
  their filters, a 404 and a 405, the panel's requests neither recorded
  nor seen by the App's middleware, and no flash taken; the files, their
  limit by tab, their day, and another run reading them; an entry that
  can't be kept leaving the response alone; nothing recorded, sent or
  served outside `tug dev`; which frames are the app's; and the auth
  starter, made by `tug new` and run as `tug dev` runs it, answering the
  panel with its login page's entry.

## M36 · Compressed assets — done

A deployed tug app is one binary that serves its own frontend, the build
Vite made, which `vite.ServeHTTP` sent as it is: nothing in tug
compressed a response, where Laravel's app sits behind a web server that
does. The auth starter's build is 37 files of JavaScript and CSS, 640 KB,
which gzip makes 205 KB, so a first visit downloaded three times what it
needed to, unless a proxy or a CDN in front compressed it. tug sends the
build compressed itself. Released as v0.31.0.

- **The build, gzipped:** `vite.ServeHTTP` sends a file of a type that
  compresses as gzip, `Content-Encoding: gzip`, to a browser whose
  `Accept-Encoding` takes it, and as it is to one that doesn't, both with
  `Vary: Accept-Encoding`, so a cache between keeps them apart, and the
  year's `Cache-Control` the assets had.
- **Brotli, when the build has it:** a file's own compressed copies beside
  it in the build, `app.js.br` and `app.js.gz`, as a compression plugin
  of the app's Vite writes them, go first, to a browser that takes them.
- **Nothing for an app to change:** the starters' route of the build,
  `app.Get("/build/{path...}", tug.WrapHandler(assets))`, stays, and an
  app made before gets it with the new tug.
- **The guide:** Pages, what the Vite section says `ServeHTTP` sends;
  Deployment, a section on compression, what's compressed, and what's
  left to a proxy; and the README.

Choices made on the way:

- **As it's served, not as it's built:** the starters' Dockerfile builds
  with `npm run build` and `go build`, not `tug build`, as an app's own
  pipeline may, so files compressed by `tug build` would have missed most
  deploys. Compressed as it's served, a file the first time it's asked
  for, every pipeline gets it, and the binary carries no second copy. It
  costs the compressed copies in memory, 205 KB for the starter's build,
  kept for the life of the process, which only a file the build has adds
  to, as each is looked for first; and the time to compress each once a
  start: 24 ms for the whole build, 12 ms for its largest file, 371 KB.
  The requests for a file at once compress it once, under its own
  `sync.Once`.
- **gzip, from the standard library:** brotli makes the starter's build
  178 KB, 13% less than gzip, but Go's standard library has no encoder for
  it, and tug stays on that: an app that wants brotli adds a plugin to its
  Vite, and tug serves what it writes. The starters add none, as gzip is
  most of the gain.
- **The build's copies, as they are:** `name.br` and `name.gz` aren't
  checked against the file, and the build's `.gz` takes the place of
  tug's own.
- **By the file's extension:** JavaScript, CSS, SVG, JSON, HTML, plain
  text, source maps, WebAssembly, and fonts not compressed already, TTF
  and OTF. An image, a WOFF or WOFF2 font, audio and video go as they are,
  as their formats compress them, and so does a file gzip doesn't make
  smaller, as the React starter's smallest chunk, an icon of 164 bytes,
  which gzip makes 184, without `Vary`, as it's the same for every
  browser.
- **The file's own type and length:** a compressed copy goes with the
  type the file has as it is, by its extension, or else its first bytes,
  as `http.ServeFileFS` finds it, not what its compressed bytes look
  like. `http.ServeContent` leaves the length out under a
  `Content-Encoding`, for a writer that compresses as it goes, so
  `ServeHTTP` sets it, as its copies are compressed already; a range sets
  its own, and one past the end is refused without it.
- **`Accept-Encoding` by its weights:** `gzip;q=0` refuses gzip, `*`
  takes it, a coding named outweighs `*`, `x-gzip` is gzip, and a weight
  that isn't a number refuses. Of the encodings taken, the build's brotli
  first, then gzip, then none, which `identity;q=0` doesn't make a 406: a
  browser gets the file.
- **A range is of what's sent:** a `Range` of a compressed file is of its
  compressed bytes, the representation sent, as RFC 9110 has it and
  nginx's `gzip_static` serves it.
- **Pages and JSON as they are:** compressing a response that holds a
  secret beside text an attacker can have it say tells the secret by the
  response's size, BREACH. tug's CSRF puts no token in a page, but a
  page's props can hold what's private, as a new API token, and which
  responses do is the app's to know. The build holds nothing secret. A
  proxy or a CDN in front compresses pages when the app chooses, as it
  can terminate TLS; Laravel leaves both to its web server.
- **Only the build:** `c.File`, `c.Download` and storage's files are the
  app's and its users', mostly images and documents, and stay as they
  are.
- **Tests:** a script gzipped for a browser that takes it, by each way of
  saying so, its length, type, `Vary` and year's caching, and as it is,
  with `Vary`, for one that doesn't, or refuses gzip; an image, and a
  script gzip doesn't shrink, as they are, without `Vary`; a range of the
  compressed bytes, one past the end, and HEAD; the build's `.br` and
  `.gz` first, by the browser's weights, in the file's own type; requests
  at once reading a file once; the 404s as before; the auth starter, and
  each starter with `-ssr`, made by `tug new` and built as its Dockerfile
  builds it, sending the first script its page loads gzipped; and in the
  browser suite, each frontend's login page working, its scripts and
  styles over a kilobyte gzipped.

## M37 · Typed forms — done

tug gen typed what a page gets, its props, and where it can go, its
routes, but not what a form sends: the starters' forms name their fields
as text, `name="email"`, and read `errors.email` from errors of any
string's, so a field renamed in Go left its form sending what the handler
no longer read, and its error unshown, with nothing to say so. Inertia's
own types can say it: React's `<Form>` takes the type of what it sends,
`<Form<LoginInput>>`, and then only that type's keys for `errors`,
`resetOnError` and `clearErrors`; `useForm` does in all three frontends;
and a form's `action` takes a route's path and method together. tug gen
writes both from the Go. Released as v0.32.0.

- **A route says what it takes:** `Takes` on a route, as the starter's
  login's `.Name("login.store").Takes(LoginInput{})`, declares the struct
  its handler binds, which the wrapper around the handler,
  `a.guestsOnly(a.login)`, keeps tug from seeing.
- **`Inputs`, by route name:** tug gen writes the inputs' interfaces, and
  `Inputs` in `routes.ts`, each named route's input by its name, for
  `<Form<Inputs['login.store']>>` and `useForm<Inputs['login.store']>`.
- **`form()`:** a named route as a form's action, its path, filled in as
  `route()` fills it, and its method, `{ url, method }`, which `<Form
  action>` and `useForm` take, so a form's method can't come apart from
  its route's.
- **The starters' forms typed:** their 16 routes that bind input take it,
  each form's action is `form()`'s, and React's forms take their route's
  input. Vue's and Svelte's `<Form>` take no type in Inertia 3.7.1, so
  theirs take the action, and their errors stay of any string.
  `examples/inertia`'s post form is typed too.
- **The guide:** TypeScript, a section on forms; Forms, its example with
  `Takes` and `form()`; Routing, `Takes`; and the README.

Choices made on the way:

- **Declared on the route, not by the handler's type:** a handler is a
  `func(*tug.Ctx) error` behind its wrappers, as the starter's
  `a.guestsOnly(a.login)`, whose input tug can't see, and a handler that
  tug bound and validated for, `func(c *tug.Ctx, in LoginInput) error`,
  wouldn't fit the starter's `usersOnly`, whose handlers take the user.
- **Named first, as it's called:** a route needs its name before it takes
  an input, `Name(...).Takes(...)`, as `Inputs` is by name, and `Takes`
  panics there, as the app adds its routes, not when it starts serving:
  `freeze` runs in a `sync.Once` at the first request, which a panic would
  leave half done.
- **Checked as it's bound:** a declaration that has drifted from its
  handler would type the form against a struct the handler no longer
  reads, which is what typing it is for. `bind`, under `Bind` and
  `BindValid`, fails, as a 500 whose error names both, a struct other
  than the route's that has a field read from the body, one with a form
  name and no path or query tag, so the drift shows at its first request,
  in the app's tests. A struct of query or path fields alone, as a list's
  filters, or the example's `postID` beside its `PostInput`, is let
  through, and a route that takes nothing is checked for nothing. A
  handler gets its route on its `Ctx`, from `adapt`.
- **Inputs of their own, in `routes.ts`:** an input's keys are the names
  `validate` gives its errors, a field's json name, else its form name,
  else its own, where `pages.ts` keys a struct as encoding/json writes it,
  so a struct that's a page's props and a route's input could be two
  interfaces; the inputs are a generator of their own, writing to the
  file of the routes. A field whose form or query tag names it otherwise
  is an error tug gen stops on, as its data and its errors would have two
  names. A path field is the route's, in `form()`'s params.
- **A file as a `File`:** a `*multipart.FileHeader` field is `File |
  null`, and a list of them `File[]`, as Inertia sends a `File` as
  multipart, which `Bind` reads.
- **The method as a literal:** `routes` is `as const`, so `form()`'s
  method is the route's own, `'put'` for `posts.update`, which Inertia's
  `Method` takes, and a route of `OPTIONS` can't be a form's action.
- **React's alone typed whole:** Vue's `<Form>` is a component of fixed
  props, and Svelte's hands its slot props of no type, in Inertia 3.7.1,
  so neither takes a type; `useForm` does in both, and their forms get
  `form()`. When theirs take one, the starters follow.
- **The starters' every route that binds:** the three of passkeys too,
  whose browser side is `fetch`, not a form, as their inputs are what
  the app's own code would send. The Vue and Svelte register pages' form,
  which they hold to drop a waiting Precognition check, is `registration`
  now, `form` being the helper's name, and in Vue its template ref's key
  too, which a binding of the setup of that name would take.
- **Names, not attributes:** an uncontrolled input's `name="email"` is
  text TypeScript doesn't read, so a typo there is the form's own tests'
  to catch, not the types'.
- **Tests:** tug gen's `Inputs` and `form()` for routes that take inputs,
  with a file, a list of files, an embedded struct, a field with no tags,
  a field left out, and a path field left to the params, and none for a
  route that takes nothing; a field whose form or query tag names it
  otherwise refused; `Takes` on a route with no name, and of a string,
  panicking; a pointer to a struct taken as it; `Bind` failing a handler
  that binds another struct, the error in the log naming both, and
  letting a struct of query fields through, and a route that takes
  nothing; `examples/inertia` typechecking with its form typed, as
  `errors.titel` doesn't; and the starters, made by `tug new`,
  typechecking with their forms typed, passing their own tests, whose
  forms bind what their routes take, and their browser suite.

## M38 · Typed flash — done

tug gen typed a page's props, its routes, and what its forms send, but
not the flash data a handler leaves for the next page, as a toast's
message: the starters typed it by hand, in `resources/js/types.ts`, as
`flashDataType: { success?: string; error?: string; recoveryCodes?:
string[]; token?: string }` in the auth starter, the keys its handlers
flash with `c.Flash`, so a key added, or a value of another type, left
the file behind, with nothing to say so. Inertia types `usePage().flash`
and the flash event by `InertiaConfig`'s `flashDataType`, as it types the
shared props by `sharedPageProps`, which tug gen writes already.
Released as v0.33.0.

- **A flash key declared in Go:** `tug.Flash[T](key)`, as
  `var Success = tug.Flash[string]("success")`, declares a key and the
  type of its value, as `tug.Page[P]` declares a page, and returns a
  `FlashOf[T]`, whose `Set(c, v)` flashes it, as `c.Flash(key, v)` does,
  with a value of its type alone.
- **`FlashData`, from the keys:** tug gen writes an interface of the keys
  declared, each optional, and `flashDataType: FlashData` beside
  `sharedPageProps` in `pages.ts`'s `InertiaConfig`, so
  `usePage().flash.success` is a `string | undefined`, and a key no
  handler declares is a type error: `Property 'sucess' does not exist on
  type 'FlashData'. Did you mean 'success'?`
- **`c.Flash` checked:** a declared key flashed by `c.Flash` with a value
  of another type panics, as the frontend's type would say otherwise; a
  key not declared flashes as before.
- **The starters' keys declared:** in `flash.go`, `success`, `error`,
  `token` and `recoveryCodes` in the auth starter, and `success` in the
  plain one, each set through its declaration where they flash, 28
  places in the auth starter and one in the plain, and their hand-kept
  `types.ts` gone, as `examples/inertia`'s is.
- **The guide:** Forms, flash messages by a declared key; TypeScript, a
  section on flash data; Getting started and Accounts, `flash.go` in
  place of `types.ts`; and the README.

Choices made on the way:

- **A declaration of the key, as of a page:** a handler's `c.Flash`
  takes a key and a value of any type, which tug can't read the types of
  without running the handler; a declaration, made once, as the app's
  pages are, is what tug gen reads, in the run it reads the pages in,
  from a registry of the process's, as the pages' is.
- **Written once there's a key:** an app that declares none keeps its own
  `flashDataType`, as one made before has in its `types.ts`: tug gen's
  beside it would be two declarations of one property, a type error. An
  app that declares a key drops its own.
- **Every key optional:** a page shows the flash of the request before
  it, some keys or none.
- **A value as JSON writes it:** the flash goes through the session as
  JSON, and to the page as JSON, so its type is written as encoding/json
  writes it, as props are, and a struct's interface is one of the
  props'.
- **One type a key:** a key declared twice, of two types, panics as the
  app starts, as the types tug gen writes would say one of them wrongly;
  twice of one type is the same key.
- **The check, for a declared key alone:** `c.Flash` of a declared key
  panics for a value whose type isn't the key's, or doesn't implement it
  when it's an interface, as `Flash[any]` takes anything; nil is any
  type's. A key flashed once, with no declaration, still flashes,
  untyped.
- **The auth starter's names:** `Success`, `Failure` for the key `error`,
  as `Error` would read as Go's own, `RecoveryCodes`, and `NewToken` for
  `token`, the key its pages read.
- **Two brackets in a template:** `tug.Flash[[]string]` in the auth
  starter's `flash.go.tmpl` began an action of tug new's templates, whose
  delimiters are `[[ ]]`, and failed to make the app, as the browser
  suite's setup found; it's written with the placeholder, `[[ "[[" ]]`,
  as `OptionalProp[[]string]` is.
- **Tests:** tug gen's `FlashData` and `flashDataType` for keys of a
  string, a list and a struct, each optional, and neither for an app that
  declares none; a key declared twice of two types panicking, and of one
  type not; `Set` flashing a string and a list for the page after a
  redirect; `c.Flash` of a declared key with a value of another type a
  500 whose log names both, and of a key of any, nil, and a key not
  declared, flashing as before; `examples/inertia` typechecking with no
  `types.ts`, as `flash.sucess` doesn't; and the starters, made by `tug
  new`, typechecking with no `types.ts`, and their toasts, the tokens
  page and the recovery codes in the browser suite as before.

## M39 · Request IDs in jobs — done

A request's ID, which `middleware.RequestID` gives it and
`middleware.Logger` logs, went no further than the request: a job its
handler pushed, as the auth starter's mail, ran later, on any instance,
and its log lines, and tug's of how it went, said nothing of where it
came from, so a job that failed for good couldn't be traced back to the
request that pushed it. Laravel's Context carries what a request had into
the jobs it dispatches, and OpenTelemetry's propagators carry a trace the
same way. A job takes what the context it's pushed from carries, and the
context it runs in gets it back. Released as v0.34.0.

- **`queue.Carrier`:** a value a job takes from the context it's pushed
  from, and gives back to the context it runs in: `Carry(ctx, into
  map[string]string)` and `Restore(ctx, from map[string]string)
  context.Context`, the shape of OpenTelemetry's propagators, which an
  app's trace adapts to in a few lines, as the guide shows.
  `queue.Config.Carry` lists them.
- **`Job.Carried`:** what a job took, by name, which a Store keeps beside
  the job, and gives back with its claim, its retries, and a failed job's
  listing.
- **The request's ID carried:** `middleware.CarryRequestID`, the Carrier
  of the ID `RequestID` put in the context, by `request_id`, and
  `middleware.WithRequestID`, which puts one in a context, so that
  `RequestIDFrom` reads it in the job's run, and tug's log lines of the
  job, a failure's among them, say it: `a job failed, and will run again
  kind=verify-mail job=6 attempt=1 ... request_id=SCK2WDZAHS33MHC2HQCFU7KWMK`,
  beside the request's own line, of the same ID.
- **The auth starter's queue carries it:** its jobs table keeps what a job
  carried, in a `carried` column, in each database, by a migration, and
  its `jobs` command and its admin page of failed jobs show the request
  each came from: `42  verify-mail, which failed at ... after 10 attempts,
  pushed by request SCK2WDZAHS33MHC2HQCFU7KWMK`.
- **The guide:** Background jobs, a section on what a job carries, with a
  Carrier of OpenTelemetry's propagator, and the starter's `carried`
  column; Deployment, a job's log lines matched to its request's; Routing
  and Accounts, where the ID goes; and the README.

Choices made on the way:

- **Carriers, not the request's ID alone:** the queue imports nothing of
  tug's middleware, and a trace's parent, or a tenant's ID, travels the
  same way, so the queue carries what the app lists, and the request's
  ID is middleware's Carrier, as the queue's `Limiter` is auth's
  `Throttle`. Middleware doesn't import the queue either: its
  `CarryRequestID` is a Carrier by its methods alone.
- **An extra a Store may have:** `CarryStore`, as `UniqueStore` and the
  rest are, with a marker method, `KeepsCarried`, as `KeepsAtOnce` is, as
  the promise is in its pushes, claims and listing. A Store that doesn't
  keep what a job carries runs the job as before, without it, so an
  app's own Store, as a starter's made before, keeps working as it is,
  and `queuetest.TestStore` checks the carried values only of a Store
  that says it keeps them. A push to such a Store takes nothing from its
  context, rather than take what it would lose.
- **Text, by name, kept as JSON:** what's carried is a few strings, as a
  header's values are, kept as JSON in a column of the jobs table, as the
  payload is, NULL for nothing, and a push whose carried values come to
  more than 4 KB as JSON fails, as a header that size would.
- **Carried however it's pushed:** `Push`, `PushAt`, `PushUnique` and
  `PushLatest`, and in a handler's transaction through `In`, each from
  the context it's given. A schedule's runs, which the queue pushes,
  carry nothing, and a job run again from the failed keeps what it
  carried.
- **A job's own push's:** a unique push that finds its job waiting
  pushes nothing, and a latest one moves the job that waits, and either
  way the job keeps what its own push carried, as it keeps its payload:
  the job is that push's, moved. The tables' upserts leave the column as
  it was, and `TestStore` checks a second latest push leaves it.
- **Given back before anything of the job's runs:** a `Rate`'s limiter
  and the handler have what the job carried in their context, and so
  does `OnFail`, whose context is its own, without the job's deadline,
  but with its values.
- **What comes back is checked:** what's carried has been in the
  database, so `CarryRequestID` gives back only an ID as plain as
  `RequestID` keeps one, as it ends up in logs and in the headers of what
  the job sends on.
- **In tug's log lines of a job:** each line the queue writes of a job,
  as one that failed for good, has what it carried, by name, after its
  kind, ID and the rest, so the line says which request pushed the job.
  The app's own lines in the handler have the context, and
  `RequestIDFrom`. `Observe`'s `Ran` leaves it out, as a request's ID is
  no label for a metric.
- **The migration has no down part:** as the starter's own have none
  (M33), `add_carried_to_jobs` only adds the column, and `migrate down`
  leaves it, as the starter's test of the command found.
- **On the failed jobs page, a line of its own words:** ", pushed by
  request" and the ID, in each frontend, after the attempts; Vue's is on
  the line it follows, as a line break in its template would be a space.
- **Tests:** a job pushed from a request's context run with the ID back
  in its own, by `RequestIDFrom`, and in the queue's log line of its
  failure, and in `OnFail`'s context; each way of pushing carrying, and a
  schedule's runs not; a retry and a run again keeping what was carried;
  a second latest push leaving the first's; a Store that keeps nothing
  carried running the job as before; carried values past the limit
  failing the push; an ID that isn't plain not given back; `TestStore`
  on the memory store, and on each of the auth starter's databases,
  after its migration; and in the auth starter, a request's ID, sent as
  `X-Request-ID`, in the claim of the mail it pushed, and the failed
  jobs showing the request, in its command and on its page.

## M40 · Debug error page — done

With `APP_DEBUG` on, a server error's response was the error as plain
text, with a panic's stack after it as `debug.Stack` writes it: a page of
text in a browser, and the same text in the modal Inertia's client opens
for a visit that failed. Laravel shows Ignition's page, and Rails and
Phoenix a page of their own: the error, the app's code where it happened,
and the request. tug has each of them: the error and what it wraps, a
panic's stack, which of its frames are the app's, the route that
answered, and where the app added it. Released as v0.35.0.

- **A page for a server error, while debugging:** under `Config.Debug`,
  `DefaultErrorHandler` answers a 5xx with an HTML page, in place of the
  text, to a client that takes HTML, as a browser and Inertia's client
  do: the status, the error's message, and each error it wraps, with its
  type, those `errors.Join` joins a level further in. Inertia's client
  shows it in its modal.
- **A panic's stack, the app's frames open:** from the frame that
  panicked, each frame's function, file and line, the app's with the
  lines of source around it and its own marked, and the frames of tug,
  the standard library and the app's dependencies folded between them,
  "27 frames of tug and the standard library".
- **Where a returned error came from:** an error a handler returns has no
  stack, so the page shows the route that answered, its name, its
  handler's function by the name Go gives it, and the lines of source
  where the app added it, which name the handler as the app wrote it.
- **The request:** its method, path and query, the route's values, its
  ID, and its headers, the secrets among them `[REDACTED]`, as DevTools
  keeps them.
- **Links to the editor:** with `APP_EDITOR` naming one, `vscode`,
  `cursor`, `zed`, `goland` or `sublime`, or a link with `{file}` and
  `{line}` in it, each frame's file and line are a link that opens them
  there. `Config.Editor` is it.
- **JSON and text too:** a client that asks for JSON first gets the
  error, what it wraps, a panic's frames and the route as JSON, and one
  that takes no HTML, as curl, the text, as before.
- **The guide:** Routing, a section on the page, While debugging, and
  `Editor`; Pages, Deployment and Getting started, what `APP_DEBUG` shows;
  and the README.

Choices made on the way:

- **Under Debug alone:** with `Config.Debug` off, a server error is the
  ErrorPage, or its status's words, as before: the page shows the app's
  source and its request's headers.
- **HTML and CSS, no script:** Inertia's client shows a response that
  isn't a page in a sandboxed frame of the page's own document, under its
  Content-Security-Policy. In a browser, under the starters' policy, a
  script in such a frame ran only with the nonce of the page the visit
  left, and the page's one `<style>`, which carries the response's nonce
  for a policy that asks for one, styled it. `<details>` folds the frames,
  and nothing is loaded from elsewhere.
- **Checked by hand in a browser:** a starter's app, with a route that
  panics and one that returns an error, as no suite has one, shown as a
  page and in Inertia's modal. The browser pane draws no sandboxed frame
  in its screenshots, so the modal's styles were read by a script in the
  frame, given the page's nonce.
- **The stack as the runtime gives it:** `adapt` keeps a panic's program
  counters, `runtime.Callers`', beside the text, for the page's frames,
  from past `runtime.gopanic`'s and the runtime's on the way to it, as a
  nil map's `mapassign` is, to the frame that panicked. A `PanicError`
  an app makes, with no counters, shows its `Stack` as text.
- **Whose a frame is, shared:** DevTools' test of a frame moved to
  `internal/frames`, an `Owner` of four, as DevTools counts a
  dependency's frame as the app's, where the page folds it: the app's,
  `main` or a package of its module, as its build says it, or tug's tests
  and examples; tug's; the standard library's; and a dependency's.
- **Source from the disk:** the lines around a frame are read from its
  file as the page is made, where the app was built; a file that isn't
  there, as in a build with `-trimpath`, leaves its frame without them.
- **Where a route was added, under Debug too:** a route keeps its caller
  under `Config.Debug`, as it does under DevTools.
- **HTML for a client that takes it:** an `Accept` with `text/html`, as a
  browser's and Inertia's client's have; curl's `*/*` gets the text, as
  the log's line does, which reads better in a terminal.
- **A link to an editor is a `template.URL`:** `html/template` lets
  through only the web's schemes in a link; each editor's link is made by
  its own function, as VS Code's takes the path after `file`, with a slash
  first for a Windows path, and an app's own by its `{file}` and
  `{line}`.
- **Go's types shown:** the page is for the app's developer, who reads
  them, where an error's words are for the person who gets it.
- **A handler's errors and panics:** what `DefaultErrorHandler` answers.
  A panic in middleware is `middleware.Recover`'s, which knows nothing of
  tug, and answers as it did. An app's own ErrorHandler that hands
  `DefaultErrorHandler` what it doesn't answer itself gets the page for
  those.
- **Not the body:** the handler has read it, and DevTools' panel shows
  what Inertia's client sent, under `tug dev`.
- **Tests:** a returned error's page, with its message, what it wraps and
  each one's type, the route, its name, and the lines where it was added;
  a panic's, with its value, the app's frames open with their lines and
  their own marked, and tug's and the standard library's folded; the
  request with `Cookie`, `Authorization` and a `token` in the query
  `[REDACTED]`; no `<script>` in it, and its `<style>` with the nonce
  under `middleware.CSP`; the editors' links, by name and by a link of
  the app's; a frame whose file isn't there, without lines; the causes of
  a joined error, and of a panic's value; a `PanicError` without its
  frames; JSON for a client that asks for it, and the text for one that
  takes no HTML; with Debug off, nothing of it; and each frame's owner,
  in `internal/frames`.

## M41 · Route list — done

An app's routes were the lines its `newApp` adds them on, and tug gen
wrote the named ones into `routes.ts`, but nothing listed them: which
handler answers `POST /login`, what it takes, which routes a path has,
and where each was added. Laravel's `route:list` and Rails' `routes`
print them. tug's router has each route's method and path, its name, the
struct it takes, and where the app added it, and the line that added it
names the handler as the app wrote it, wrappers and all. Released as
v0.36.0.

- **`tug routes`:** builds the app and runs it as tug gen does, and
  prints its routes, one a line, in columns: the method, the path, the
  name, the struct the route takes, the file and line that added it,
  `main.go:392` in an app made by `tug new -auth`, and the handler, as
  that line has it, `a.guestsOnly(a.login)`.
- **Every route of the app's:** named or not, as `/up`, a group's, and
  one of any method, as `ANY`; not tug's catch-all for the misses, or
  DevTools' endpoints, which aren't the app's.
- **A filter:** `tug routes login` lists the routes whose path or name
  has `login` in it, in any case; one that lists none is an error.
- **`-json`:** the same as JSON, with the handler's function, as Go
  names it, beside its expression, for a script.
- **The guide:** CLI, `tug routes`; Routing, a pointer to it; and the
  README.

Choices made on the way:

- **From the app's own run:** routes are added as `main` runs, some by
  its environment, as the auth starter's `/files` by its disk; the run
  tug gen makes, with the app's `.env`, lists them as the app has them,
  without the database, which `Generating` leaves out. The app writes
  them, `RouteList`, beside its types and texts, so tug gen, tug lang
  and tug routes make the same run.
- **The handler as the app wrote it:** a route's function is often a
  wrapper's closure, `main.newApp.(*app).guestsOnly.func15`, which says
  nothing of the handler inside it; the line that added the route says
  it, so tug routes reads the call there with `go/parser`, and prints the
  expression the app gave as the handler, on one line, as
  `types.ExprString` writes one, a function literal without its body.
- **The innermost of the router's calls over the line:** the runtime
  gives a call over several lines its first, in a starter's app, and the
  call is found by the lines it takes in, of `Get` to `Any`, and
  `Handle`, whose handler comes third, so a route added over three lines
  is read whole.
- **A variable is the helper's:** a handler that's a variable of the
  function around the call, a parameter or one it declares, as a helper
  of the app's that adds routes has, says nothing, so the route is
  listed by its function's name, at the helper's line.
- **Where it was added, under tug gen too:** a route keeps its caller in
  tug gen's run, as under DevTools and Debug.
- **By path, then method:** a path's routes, and a prefix's, go
  together, its reads before its writes, and a route of any method last;
  the order they were added in is the `ADDED` column's.
- **The handler last:** its width varies most, as a wrapper's handler is
  long, so the columns before it stay narrow, and the line reads as a
  file's line, then what's on it.
- **Not the middleware:** the starters' guards are wrappers of the
  handler, which its expression shows, and a middleware is a function
  value whose name, a constructor's closure, says less than the line that
  added it.
- **Tests:** the handler's expression read from a call on one line, one
  over several from each of its lines, one chained with `Name` and
  `Takes`, a group's, `Handle`'s, a function literal's, and a helper's,
  and a line that adds none, which fall back to the function's name, as
  a file that isn't there does; the routes' order, and those of a text,
  by path or name, in any case; the table, a column each and the handler
  last; package tug's run writing every route, named or not, with where
  it was added and what it takes; and the auth starter, made by `tug
  new`, listing `POST /login` as `login.store`, taking `LoginInput`,
  added in `main.go`, by `a.guestsOnly(a.login)`.

## M42 · Inertia 3.8 — done

tug spoke Inertia's protocol as v3.0.0 had it, in March, and the client
was at 3.8.0, which the starters' `^3.7.1` installed. Two of the things
that came since ask something of the server. A page's head, its title,
its description and the tags a link's preview is made from, can come
from the server, as a prop the client keeps in the head, `serverHead`,
since 3.5.0. And a whole number past JavaScript's safe range, which
ends at 2^53 - 1, can go to the page as a `BigInt`, since 3.8.0, where
it was rounded on the way: a snowflake ID, `900719925474099988`, arrived
as `900719925474100000`. tug has each page's props, the HTML of its
first visit, which it writes without Node, and the Go type of each
value, which tug gen says in TypeScript. Released as v0.37.0.

- **A page's head, from Go:** `c.Head(...)` gives the page the request
  renders the elements of its `<head>`: `inertia.Title`; `inertia.Meta`,
  by name, as the description; `inertia.Property`, Open Graph's, as
  `og:image`; and `inertia.Link`, as the canonical one. They go out as
  the page's `head` prop, which the client, with `serverHead`, keeps in
  the head as it shows the page, until a visit to another page replaces
  them. `inertia.WithHead` is the same for any router, and for
  middleware, as a site's own image on every page.
- **In the first visit's HTML, without SSR:** a search engine's crawler,
  and what makes a link's preview, as Slack and iMessage do, read the
  HTML and run no script. With the head in it, through the root
  template's `{{ .InertiaHead }}`, they see each page's own title and
  description, as with SSR, and no Node.
- **Big integers:** `inertia.BigInt`, an `int64`, goes out as the
  protocol's `{"$bigint": "…"}`, which the client makes a `BigInt`: in
  props, deferred, merged and the rest, and in flash data, on a page that
  says so, `preserveBigIntegers`. tug gen types it `bigint`. The client
  sends one back as its digits, which `Bind` reads, from JSON, a form,
  the query or the path, and tugtest reads one from a page as the number
  it is.
- **The starters, on 3.8:** Inertia at `^3.8.0`, `serverHead` on,
  `{{ .InertiaHead }}` in every root template, not only an SSR one's, and
  the home page's title and description from its handler, in all three
  frontends, with accounts and without. `examples/inertia`'s post page
  has its title and words from its handler.
- **The guide:** Pages, a section on a page's head, and one on numbers
  past the safe range; TypeScript, `bigint`; Testing, a BigInt read back;
  SSR, the head with it and without; the roadmap's first lines; and the
  README.

Choices made on the way:

- **Elements, not HTML:** the client puts the `head` prop's strings in
  the page as HTML, so a description taken from a post, written into
  one, would be a script a stranger wrote. tug writes each element from
  its parts, each value through `template.HTMLEscapeString`, and takes no
  HTML from the app. A title, a meta tag by name or by property, and a
  link are what search engines and previews read; JSON-LD's structured
  data, a script, waits for an app that asks.
- **Keyed as the client keys them:** each element carries `data-inertia`,
  its key: `title`, the meta's name or property, the link's rel. The
  client matches elements by it across visits, so a description replaces
  the one before, and a page's `<Head>` element with that `head-key` wins
  over the server's. Two elements of one key are one, the later, in the
  earlier's place; `Key` gives one its own, for a second `og:image`.
- **For the response, not in the props struct:** a head is made from
  what the handler loaded, as the post's title, and the site's from
  middleware; a field in each page's props would be one more for every
  page to carry, and for tug gen to type. `head` is the client's default
  name, a prop of tug's own, as `errors` is: a page's own `head` wins,
  with none of tug's in its HTML, and tug gen leaves it out of the types,
  as the client reads it, not the page.
- **Not shared:** the head isn't in the page's `sharedProps`, whose props
  the client takes on to the next page in an instant visit, and a shared
  prop named `head` gives way to the request's.
- **A partial reload leaves it out, unless it asks:** the head follows
  the page's props, as Laravel's does. The client syncs the head only as
  a visit goes to another URL, or back, and a reload of the page keeps
  the one it has.
- **The title as the client says it:** the client says a title through
  its `title` callback, `Welcome · blog` in the starters, which a first
  visit without SSR doesn't run, so `inertia.Config.Title` says it the
  same way in Go, for the HTML tug writes, as the starters' `pageTitle`
  does, and a page with no title from Go keeps the template's own
  `<title>`. The two are the app's to keep alike, as the root template's
  and the callback's app name were.
- **The first visit's title has no key, nor the template's:** React's
  and Vue's adapters replace a title without one with their own, the
  same, as the client has since 3.0.1. Svelte puts the document's title
  in the first `<title>` there, and its adapter's head takes away a keyed
  one that isn't the next page's, as a visit leaves a page, the
  document's title with it: the browser suite caught Svelte's login page
  with no title, after the home page.
- **Svelte's title from `Head.svelte`:** Svelte's adapter puts a title
  from the server in as it comes, with no callback, and takes it away,
  as above, after Svelte has put the next page's title in it. The plan
  had Svelte's starters say it in a `title` callback, which Svelte's
  adapter doesn't call. Their `serverHead` is a function that leaves the
  title out, and a page, the home page too, has its title from
  `Head.svelte`, which `pageTitle` says the same way in a first visit's
  HTML; React's and Vue's home pages have theirs from Go alone.
- **With SSR, the client's head:** a page rendered on the server has the
  head from Go in its head already, as the client's own code put it
  there, so tug adds none; a page SSR didn't render, as `WithoutSSR`'s,
  or one whose render failed, gets tug's.
- **An error page, without the handler's:** the head a handler gave the
  page it meant to render, as a post's title, isn't its error's; the
  error page keeps middleware's.
- **Big integers by type, not by value:** inertia-laravel, when it's on,
  sends any integer past the safe range as a BigInt, as PHP's integers
  have no type to tell, so a field's TypeScript would have to be
  `number | bigint`. tug goes by the Go type: an `inertia.BigInt` always
  goes as one, a small one too, and is `bigint`, and an `int64` is a
  `number`, rounded past the safe range, as before, or a string with
  encoding/json's `,string`, which tug gen types `string`.
- **The flag only where there's one:** `preserveBigIntegers` goes on a
  page whose JSON has a BigInt's marker, `{"$bigint":`, which a string's
  escaped quotes can't make, in its props or its flash data: the page is
  written again with it. Every other page's JSON is as it was, for a
  client before 3.8 too.
- **An int64:** a snowflake ID, a count of nanoseconds and a 64-bit
  column are int64s, and Go's arithmetic on one stays an int64's. A
  `uint64` past 2^63, and `big.Int`, are left out: rarer, and not IDs.
- **Read as an int64:** the client sends a BigInt in a request as its
  digits, which Bind reads into an `inertia.BigInt` as it reads an int64:
  a string of them in JSON, as a form sent as JSON, and a form, the query
  and the path by its kind, with "order must be a whole number" for what
  isn't one. It has no `UnmarshalJSON` of its own: CI's Go 1.27.1 caught
  that encoding/json, from 1.27, gives the error of one no field, where
  1.26 gave it the field's path, which Bind names. A page's marker is
  tugtest's to read.
- **tugtest reads the number:** a page that says it has a BigInt is read
  with each marker the number it is, so `Prop[int64]` and `json.Number`
  read one, as `Props` reads an `inertia.BigInt`.
- **The starters' home page from Go:** the landing page is the one a
  search engine sees, so its handler gives it a title and a description;
  a page behind a login keeps its `<Head>`, as no crawler sees it.
- **The example's post:** `examples/inertia`'s post page has its title
  and words from its handler, its first visit's head tested without Node,
  against the fake manifest, and in the browser, on its lock's 3.7.1,
  which has `serverHead`.
- **Tests:** in package inertia, each element written escaped, with its
  key; a later element of a key replacing the earlier; the head out of
  `sharedProps`, over a shared prop, and a page's own `head` winning;
  a partial reload leaving it out unless it asks; the first visit's HTML
  with the head, its title through `Config.Title` without a key; none of
  tug's beside SSR's, and tug's for a page SSR failed or was skipped; a
  BigInt as the marker in props, nested, in a list, lazy and deferred,
  and in flash data, with the page's flag, and a page without one, a
  string with the marker's text in it too, without it. In package tug, a
  handler's head over
  middleware's, and an error page with middleware's alone; Bind reading
  a BigInt from JSON, a form, the query and the path, and a 400 for one
  that isn't one. tug gen's `bigint`, in props, a list, a pointer, flash
  data and a route's input; tugtest reading one into each type; the
  starters' home page's first visit, in an app of each kind made by `tug
  new`, its title and description once each, rendered on the server or
  not; and in the browser suite, the home page's title and description
  in each frontend, the login page's own title and none after a visit,
  and the home page's again, back.

## M43 · Development mailbox — later

Under `tug dev`, with no `MAIL_HOST`, the mail an app sends is written
out to the terminal, as `mail.Log` writes it: who it's from and to, its
subject, and its text, with the link that verifies an email among it, to
copy into the browser. Its HTML, which a person's mail program shows,
isn't seen, nor its files, nor the message a mail server would take.
Phoenix keeps the mail its app sends in a mailbox at `/dev/mailbox`,
Rails' letter_opener opens each one in the browser, and Laravel's Sail
and Herd run Mailpit beside the app. tug has each mail as a server takes
it, which package `mail` builds, a place under `.tug`, where DevTools
keeps its entries, and an App that answers DevTools' endpoints before its
own routes under `tug dev`. To be released as v0.38.0.

- **A mailbox under tug dev:** with no `MAIL_HOST`, and `TUG_DEV` set, as
  `tug dev` sets it, `mail.FromEnv` returns a `mail.Mailbox`, which keeps
  each mail it's sent in `.tug/mail`, as a mail server would take it, and
  writes a line to the log for each: who it's to, its subject, and the
  link to it in the mailbox. Without `TUG_DEV`, as a deploy, a test and
  the browser suite run, it's `Log`, as now.
- **A page of the mail:** `/_tug/mail`, which the App answers under
  `Config.DevTools`, before its own middleware and routes, as it does
  DevTools' endpoints: the mail kept, newest first, each with who it's
  from and to, its subject, and when it came. A mail's own page,
  `/_tug/mail/{id}`, has its headers, its Bcc, which no header has, its
  HTML, as a mail program shows it, its text, its files, to download, and
  its source.
- **The HTML in a frame of its own:** `/_tug/mail/{id}/html`, sandboxed,
  so nothing in it runs, and its links open in a tab of their own, as the
  link that verifies an email does, into the app, in the browser the
  developer is logged in with.
- **The guide:** Accounts, the mail while developing; CLI, what `tug dev`
  keeps; Deployment, `MAIL_HOST` and `TUG_DEV`; Getting started; and the
  README.

Choices, to settle before any code:

- **Under tug dev alone:** a deploy with no `MAIL_HOST` writes its mail
  out, as now, where its platform's log keeps it; the mailbox is for the
  machine the app is made on, which `TUG_DEV` says, as `Config.DevTools`
  reads it. A `MAIL_HOST`, as a Mailpit's, gets the mail, under `tug dev`
  too.
- **A Mailer in package mail, and a page of the App's:** `mail.Mailbox`
  is a Mailer, as `Log` is, with no import of tug. The App serves the page
  from what it kept, through an internal package both use,
  `internal/mailbox`: the files, their names, and their order, as
  DevTools' store has its entries.
- **As a server takes it:** each mail is kept as `build` writes it for
  SMTP, with its envelope beside it, the Bcc among it, so the page shows
  what a mail program would get, its parts read back with the standard
  library's `mime/multipart`, quoted-printable and base64. A mail SMTP
  wouldn't send fails here too, as with `Log`.
- **One line in the log:** the mail is on its page, so the log has a line
  for it, not the mail: who it's to, its subject, and its link, from
  `APP_URL`, which `tug dev` sets, as
  `http://localhost:8080/_tug/mail/01K…`.
- **On the app's address:** the page is at the app's own, `/_tug/mail`,
  beside DevTools' `/_inertia/devtools`, so the mail's links lead into
  the app as the browser has it. Answered before the App's middleware, it
  has no session, CSRF or policy of the app's, and sends a policy of its
  own: its style, and the mail's frame, from the app.
- **No script:** HTML and CSS, as the debug page is, and a reload shows
  the mail that came since. The mail's HTML is in an `<iframe>`, sent
  with a Content-Security-Policy of `sandbox`, with `allow-popups` and
  `allow-popups-to-escape-sandbox`, and with a `<base target="_blank">`
  put in its head, so its links open in a tab of their own, which runs
  the app's scripts, and nothing in the mail runs.
- **Its files as downloads:** each at `/_tug/mail/{id}/files/{n}`, as an
  attachment, by its name, with `nosniff`, so no file is shown as a page;
  and its source, `/_tug/mail/{id}/source`, as text.
- **Kept as DevTools' entries are:** the newest 100, each named by a
  ULID, and the rest deleted as mail comes. `.tug` is git-ignored, and
  outlives the app's restarts, which `tug dev` makes as the app changes.
- **Not previews:** Rails renders a mail without sending it, from a
  preview class of the app's; a tug app sends one to the mailbox from a
  command or a test of its own.
- **Tests:** in package mail, `Mailbox` keeping a mail as it's built,
  its envelope with the Bcc, and its line in the log with its link, and
  refusing what `Log` refuses; `FromEnv` picking it under `TUG_DEV`
  without `MAIL_HOST`, `SMTP` with one, and `Log` otherwise; in
  `internal/mailbox`, the newest 100 kept, newest first; in package tug,
  the list, a mail's page with its headers, Bcc, text, files and source,
  its HTML's response, with its sandbox and its base, a file as a
  download, with `nosniff`, and none of it without `Config.DevTools`; in
  an app made by `tug new -auth`, run under `TUG_DEV`, the mail that
  registering sends listed on the page, with the link that verifies the
  email in its HTML; and by hand in a browser, the page and its frame,
  and the link out of it, as no suite runs `tug dev`.

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
- No account administration in the auth starter: a page of the users,
  suspending an account and acting as a user, built after migrations, was
  too much for a core framework, and every app made with `-auth` would
  carry it; Laravel leaves both to packages. The work is on the branch
  `account-administration`.
- No feature flags: which users get a feature is the app's to decide, and
  needs nothing of tug's internals, as a page gets flags as it gets `can`,
  a shared prop, and a share of the users is a hash of the flag's name and
  the user's ID. Many apps that need more use a service; Laravel ships
  Pennant apart from the framework.
