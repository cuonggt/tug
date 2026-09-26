# Roadmap

tug is built in milestones, each ending in something that runs. It targets
the Inertia.js v3 protocol (v3.0.0, March 2026), written against
[the spec](https://inertiajs.com/the-protocol).

|    | Milestone               | Status |
|----|-------------------------|--------|
| M1 | HTTP core               | done   |
| M2 | Inertia core + Vite     | done   |
| M3 | Forms and validation    | done   |
| M4 | The full v3 protocol    | done   |
| M5 | CLI                     | done   |
| M6 | v0.1.0                  | done   |
| M7 | The auth starter, whole | done   |
| M8 | Background jobs         | done   |

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
Svelte starters, and passkeys. Background jobs are M8.

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
  for SSR, which tug doesn't have.
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
