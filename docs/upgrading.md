# Upgrading

An app `tug new` makes is the starter's code, copied, and from then on the
app's own: its pages, handlers and tables grow from what the starter
wrote. A new tug changes the starter too, a page's layout, a handler's
check, a table for something new, and `tug upgrade` brings the app what
the starter has gained since the tug that made it, merged into what the
app has changed itself. What changed in tug's own API, for the app's own
code, each release says, [below](#the-releases).

## Upgrading an app

With everything in the app committed, install the tug to upgrade to, and
run `tug upgrade` in the app's directory:

```sh
go install github.com/cuonggt/tug/cmd/tug@latest
tug upgrade
```

```
tug upgrade: writing the starter as tug v0.47.0 wrote it
tug upgrade: the starter from v0.47.0 to v0.55.0, with -auth

  updated  auth.go
  merged   main.go
  marked   resources/js/pages/Dashboard.tsx
  added    migrations/20261022012800_create_presences.sql
  added    presences_db.go

The starter changed these migrations, which the app keeps as they are,
as one that has run stops the app once it's changed. Make each change in
a migration of the app's own, with tug migrate new:

  migrations/20260925171438_create_users.sql
    @@ -14,4 +14,5 @@ CREATE TABLE users (
     	passkey_handle    BLOB,
     	photo_key         TEXT,
    +	locale            TEXT NOT NULL DEFAULT '',
     	created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
     );

Put right what's marked, between <<<<<<< and >>>>>>>, then run
go mod tidy, npm install and tug gen.
tug: the upgrade left 1 file marked and 1 migration to write
```

Then:

1. Put right each file it marked, where the app and the starter changed
   the same lines, and run what it says, as it installs and writes the
   types only once nothing's marked.
2. Write each migration it listed, a change of the app's own that makes
   the starter's ([Migrations](#migrations)).
3. Read what the releases in between say of tug's API,
   [below](#the-releases), for the app's own code.
4. Check the change as the app's `AGENTS.md` says: `tug gen`, `npm run
   typecheck`, `go vet ./...` and `go test ./...`, whose databases run
   every migration, the app's new one among them.

`git diff` is everything the upgrade changed, and committing it is the
upgrade done. `git checkout .` and `git clean -fd` undo it.

## What it does

`tug upgrade` writes the starter twice: as the tug that made the app wrote
it, by that tug's own `tug new`, which the go command fetches and builds
once and keeps, and as this tug writes it, both with the app's name and
its flags, which `tug.json` keeps. What changed between the two is laid
over the app, file by file, as git merges a branch:

- **A file the app left as it was** takes the starter's new one:
  `updated`.
- **A file the app changed** has the starter's change merged into it, by
  `git merge-file`: `merged`. Where both changed the same lines, it marks
  them, with the app's lines, what the starter had, and what it has now:
  `marked`, between `<<<<<<< app` and `>>>>>>>`.
- **A file new in the starter** is added: `added`. Where the app has one
  of its own by that name, the two are merged over nothing, and where
  they differ is marked.
- **A file the starter has left out** goes where the app left it as it
  was, `deleted`, and stays where the app changed it, `kept`.
- **A file the app deleted** stays deleted, and is listed, `kept`, where
  the starter has changed it since.
- **What the starter never had**, the app's own pages, handlers and
  migrations, isn't touched.

Some of the starter's files aren't merged at all: `.env`, which has the
app's own key and values, where `.env.example`, which is merged, shows
what's new; `go.mod`, whose tug and `go` lines `tug upgrade` sets, leaving
the rest to `go mod tidy`; `tug.json`, which it writes for the tug it
upgraded to; and the lockfiles and `resources/js/tug`, which the install
and `tug gen` write again.

Unless a file is marked, it then does what `tug new` does once it has
written the starter: `go mod tidy`, the install, with the package manager
whose lockfile the app has, and `tug gen`. A file marked, or a migration
to write, has it exit with status 1, so a script sees it.

It stops, having changed nothing:

- outside a git work tree, or with changes not committed, as the upgrade
  has to be one diff, which git shows and undoes;
- for an app whose starter is newer than this tug: as `tug new` makes an
  app of the tug it is, `tug upgrade` upgrades to it, and never back, so
  an app moves to another version by that version's tug;
- for an app whose starter is this tug's already, which it says.

With `-tug-dir`, it upgrades the app to a checkout of tug, as `tug new
-tug-dir` makes an app of one.

## Migrations

A migration that has run is never changed: the app keeps each one's hash
as it runs, and stops as it starts at one whose SQL has changed since
([Migrations](migrations.md)). `tug upgrade` can't tell which of the
app's migrations have run where, so it changes none of them. A migration
new in the starter, as a table for something new, is added under its own
name, and runs at the app's next start, wherever its name sorts, as every
one that hasn't run does. One the starter changed, as v0.48.0 put a
`locale` column in `create_users.sql`, is left as the app has it, and
listed with how the starter changed it, for the app to make the same
change in a migration of its own:

```sh
tug migrate new add_locale_to_users
```

```sql
ALTER TABLE users ADD COLUMN locale TEXT NOT NULL DEFAULT '';

-- down
ALTER TABLE users DROP COLUMN locale;
```

in its database's own SQL, as the starter's are. A new app has the column
in its first migration, and an upgraded one has it from its own, so the
two end up with the same table.

From v1.0, the starter never changes a migration it has released: a
change to one of its tables comes as a migration of its own, which is
added, so an upgrade within v1 writes no SQL by hand. What's listed is
for an app that crosses an earlier release that did, as v0.48.0 did.

## An app made before `tug.json`

`tug new` writes `tug.json`: the tug whose starter the app has, the name
it was made with, which the starter writes in its files and the directory
needn't keep, and its flags.

```json
{
  "starter": "v0.55.0",
  "name": "blog",
  "flags": [
    "-auth"
  ]
}
```

An app made before v0.55.0 has none, and names the tug that made it once,
with `-from`:

```sh
tug upgrade -from v0.47.0
```

Without it, `tug upgrade` stops and says the likely one: the tug the
app's `AGENTS.md` links the guide at, which is the starter's, or else the
one its `go.mod` requires, which `go get` may have moved since, as every
release's notes below say to run it first. The flags are read from the
app's files: `auth.go` for `-auth`, the driver `db.go` imports for
`-postgres` or `-mysql`, Inertia's adapter in `package.json` for `-vue` or
`-svelte`, and `resources/js/ssr.tsx`, or `ssr.ts`, for `-ssr`; the name
is `package.json`'s. An app that took all of a release's changes by hand,
as its notes said to, names that release, so the upgrade starts from it.

`-from` takes a release, or a commit the go command can fetch. An app made
from a checkout of tug, whose `tug.json` says `v0.0.0`, names the commit
its starter came from.

## Versions

From v1.0.0, within v1, an app that builds and passes its tests on one
release of tug does on every later one: a minor release adds, and a
patch fixes, and what would break an app waits for v2, whose module path
is `github.com/cuonggt/tug/v2`, as Go gives a major version its own.
That covers:

- **The Go API** of every package outside `internal/`, as the guide and
  each package's documentation say it behaves. A name doesn't change or
  go, and an interface an app implements, a store or a `Disk`, gains no
  method: what tug needs of one later comes as an interface beside it,
  which tug looks for. A name that's to go is marked `// Deprecated:`,
  with what to use instead, and stays until v2. CI's gorelease holds
  each push to it.
- **The CLI:** its commands and their flags; `tug.json`; and the files
  it reads and writes in an app, `resources/js/tug`, `lang/<lang>.json`
  and `migrations/`, by their names and a migration's `-- down` line.
  `.tug/` is tug's own, whatever it keeps there.
- **The environment** tug reads, as
  [Deployment](deployment.md#configuration) and
  [the CLI](cli.md#env-and-the-environment) list it.
- **The TypeScript tug gen writes**, which an app's pages import and `npm
  run typecheck` holds them to: `Pages`, `PageProps`, `SharedProps`,
  `FlashData`, `Params`, `Inputs`, `RouteName`, `route`, `form`, `Texts`,
  `t`, `text`, `choice`, `number` and `date`, and how a Go type becomes
  one ([TypeScript](typescript.md)).
- **Go and Inertia:** Go's two newest releases, as Go supports them, so a
  minor release may raise the go line of tug's `go.mod` as a Go release
  goes out of support; and Inertia's v3 protocol, with each 3.x client.

What it leaves out:

- **The starters' code**, once `tug new` has written it: it's the app's,
  to change as it grows, and a release changes the starters as it likes,
  which `tug upgrade` brings to the app merged into what it has done. A
  migration the starter has released is the one thing it never changes,
  as above.
- **The text of errors and of the log**, which a person reads, and the
  pages tug shows while debugging, its error page and the mailbox.
- **DevTools' entries**, which are Inertia's protocol's.
- **`internal/`**, and behaviour the guide calls a bug, which a fix
  changes.
- **The versions of tug's own dependencies**, which a release moves as
  they're fixed, as v0.56.0 moved mimetype's.

## The releases

What each release said of upgrading to it, newest first, from its notes
on GitHub. Until `tug upgrade`, the notes said which of the starter's
files to take from an app made by that release, which `tug upgrade` now
does; what they say of tug's API, and of migrations, is still for the
app to do.

### v0.56.0

`go get github.com/cuonggt/tug@v0.56.0`, and pass the request's context
to `URL` and `UploadURL`, `disk.URL(c.Context(), key, expires)`, where
the compiler points. **A Disk of the app's own, or an `Uploader`, takes
`ctx context.Context` first in both.** An app made by `tug new -auth`
takes the one line of `photos.go` that calls it with `tug upgrade`, with
`-from` the tug that made it if that was before v0.55.0.

### v0.55.0

`go install github.com/cuonggt/tug/cmd/tug@v0.55.0`, and in the app,
with everything committed, `tug upgrade -from` the tug that made it, as
an app made before has no `tug.json`: the one its `AGENTS.md` links the
guide at, unless it has taken a later release's changes by hand. Then put
right what it marks, and write the migrations it lists. Nothing an app
calls changed.

### v0.54.0

`go get github.com/cuonggt/tug@v0.54.0`. **A Disk of an app's own gains
three methods**, `List`, `Copy` and `Move` — an app that keeps its files
on `storage.Local` or `storage.S3` needn't notice, as both have them.
Nothing else changed: a link to read signs as it did, and `Info` gaining a
field is additive.

### v0.53.0

`go get github.com/cuonggt/tug@v0.53.0`. Everything here is new:
`Uploader`, `Stater`, `Conditions`, `UploadKey` and `Checked`, with
`Local` and `S3` both. A read link's signature didn't change, so the links
already in pages and mail keep working, and a link of either kind is no
good as the other. A bucket needs **CORS for the `PUT`** before a browser
can make one — the guide's Deployment says what.

### v0.52.0

`go get github.com/cuonggt/tug@v0.52.0`. **`Props`, `Prop` and `Has` read
the merged page now, not the answer**, so a test that asserts a prop is
*absent* after a partial reload reads `r.Page.Props` instead — the prop
is there, as it is in a browser. A test that sent
`X-Inertia-Except-Once-Props` itself can drop it: the client says what it
holds. Everything else reads as it did.

### v0.51.0

`go get github.com/cuonggt/tug@v0.51.0`. Nothing a hub does changed:
`Hub.Presence` and `Lease` are new fields, and a hub without them works as
it did. An app made by `tug new -auth` before that wants the list takes
`presences_db.go`, the `presences` table **in a migration of its own**,
the `Presence` field on the hub `main` makes, and the pages' `Enter` from
an app made now.

### v0.50.0

`go get github.com/cuonggt/tug@v0.50.0`. An app made by `tug new` before
takes **`dir="{{ .Dir }}"` in its `app.html`** — nothing else is needed
for the document to say it. A page of its own that is laid out by physical
sides mirrors once its utilities are the logical ones, which is the app's
own pass; an app with no right-to-left language has nothing to do.

### v0.49.0

`go get github.com/cuonggt/tug@v0.49.0`, then **`tug gen`**, which writes
`number()` and `date()` into `resources/js/tug/lang.ts`. **`c.Choice`'s
`:count` is written the language's way now**, so a text with a count in it
reads differently in a language the table names — which is the fix, and
worth looking at if a count of yours isn't a quantity. `Config.Zone` is
new and empty is UTC, as before; `APP_TIMEZONE` sets it, and an app on
`scratch` wants `time/tzdata` for the zone's files.

### v0.48.0

`go get github.com/cuonggt/tug@v0.48.0`. Nothing in tug's own API changed.
An app made by `tug new -auth` before adds the column **in a migration of
its own** — never by changing `create_users.sql`, whose hash has already
run — and takes `a.words`, the jobs' texts and `logIn`'s line from an
app made now.

### v0.47.0

`go get github.com/cuonggt/tug@v0.47.0`, then **`tug gen`**, which writes
`text()` into `resources/js/tug/lang.ts` — a committed `lang.ts` from
before is stale without it. Nothing an app calls changed, and an app made
by `tug new` before keeps its own pages: `t()` is for whoever wants a
second language.

### v0.46.0

`go get github.com/cuonggt/tug@v0.46.0`, then `tug gen` to write
`resources/js/tug/lang.ts`. Nothing an app calls changed, and nothing
reads differently until there's a `lang/<tag>.json`: a text with no words
says itself. An app made by `tug new` before takes
`resources/js/lib/lang.ts` and the `texts` prop's two lines in `main.go`
from one made now.

### v0.45.0

`go get github.com/cuonggt/tug@v0.45.0`. Nothing an app calls changed. An
app made by `tug new` before takes one line in `app.html`: `<html lang="{{
.Locale }}">` in place of `<html lang="en">`.

### v0.44.0

`go get github.com/cuonggt/tug@v0.44.0`. Nothing an app calls changed, and
a session with no `Keeper` is the cookie it always was. An app made by
`tug new` before can take `sessions_db.go` and `SESSION_STORE` from one
made now — **its table goes in a migration of the app's own, not in a
change to one that has already run**. Turning a Keeper on keeps the
sessions in the cookies; turning it off logs everyone out.

### v0.43.3

Tug's Go is as it was, so `go get github.com/cuonggt/tug@v0.43.3` changes
nothing in an app. An app made before can add the lines to its
`AGENTS.md`, where its checks are.

### v0.43.2

Tug's Go is as it was, so `go get github.com/cuonggt/tug@v0.43.2` changes
nothing in an app. An app made by `tug new -auth` before can add the rule
to its `AGENTS.md`, among the rules.

### v0.43.1

Tug's Go is as it was, so `go get github.com/cuonggt/tug@v0.43.1` changes
nothing in an app. An app made by `tug new -auth` before can take the fix
from one made now: `TestTheMigrateCommandRunsListsAndUndoesTheMigrations`,
the end of `migrations_test.go`. Any app made before can add its own name
to `.gitignore` and `.dockerignore`, and `go fmt ./...` to its
`AGENTS.md`'s checks.

### v0.43.0

Nothing an app calls changed: `go get github.com/cuonggt/tug@v0.43.0`. An
app made by `tug new` before can take `AGENTS.md` and `CLAUDE.md` from one
made now with the same flags, and change what its own code has grown past.

### v0.42.0

`go get github.com/cuonggt/tug@v0.42.0`. Nothing an app calls changed, but
`middleware.Headers` sends the Permissions-Policy now: **an app whose
pages use the camera, the microphone, the browser's location, payments or
another of the features must allow it in `HeadersConfig.Permissions`**, or
the browser refuses it. Styles by nonce are opt-in. An app made by `tug
new` before takes them from one made now: the meta in `app.html`, the
nonce on the template's own `<style>`, `defaults: { nonce }` in
`inertia.tsx` (`.ts`), and `StyleNonce: true` in `main.go`; and in React
with accounts, `get-nonce` and its `setNonce`, sonner's stylesheet and
`sonnerStyles`. What's still blocked, each page's console says, and the
app's log has the reports.

### v0.41.0

Nothing an app calls changed: `go get github.com/cuonggt/tug@v0.41.0`. An
app made by `tug new -auth` before keeps its accounts as they are; to have
passkey-only ones, it takes them from an app `tug new -auth` makes now:
the registration in `passkeys.go`, `newPassword` and `removePassword` in
`settings.go`, the confirm page's props and the reset mail in `auth.go`,
the `password-set` and `password-removed` notices, `User.Password` in
`users.go`, its database's `users.addWithPasskey`, the routes, the
`registration-asks` throttle and the `registration-mail` job in `main.go`,
and its frontend's `Auth/Register`, `Auth/RegisterPasskey`,
`Auth/ConfirmPassword`, `Settings/Security` and `DeleteAccount`. It needs
no migration. For Inertia 3.9.1, raise `@inertiajs/react`, `vue3` or
`svelte`, and `@inertiajs/vite`, to `^3.9.1`.

### v0.40.0

Nothing an app calls changed, and an app that doesn't set `Config.TLS`
serves as it did: `go get github.com/cuonggt/tug@v0.40.0`. tug's `go.mod`
requires golang.org/x/net now, for package `letsencrypt`, which an app
that doesn't import it doesn't build. An app made by `tug new` before
takes HTTPS from one `tug new` makes now: the lines of `main.go` that give
`Config.TLS` what `letsencrypt.FromEnv` makes, the Dockerfile's `/data`
volume and `CERTS_PATH`, and `certs/` in `.gitignore` and `.dockerignore`.

### v0.39.1

Tug's Go is as it was, so `go get github.com/cuonggt/tug@v0.39.1` changes
nothing in an app. An app made by `tug new -auth` before keeps its admins
and its migrations, which are its own: it doesn't take the new files,
whose tables its database has made already. One made from v0.39.1, `go
install github.com/cuonggt/tug/cmd/tug@v0.39.1`, has the new ones.

### v0.39.0

Nothing an app calls changed: `go get github.com/cuonggt/tug@v0.39.0`. An
app made by `tug new -auth` before keeps its logins in their cookies
alone, as it did; to list them, it takes the browsers from an app `tug new
-auth` makes now: `logins.go` and `logins_test.go`, its database's
`logins_db.go` and `migrations/20261002053504_create_logins.sql`, and the
changes to `auth.go`, `settings.go`, `twofactor.go`, `passkeys.go`,
`notifications.go`, `main.go` and the security page, with its frontend's
`LoginSettings`. Its sessions from before have no login's ID, and log in
again, once.

### v0.38.0

Nothing an app calls changed: `go get github.com/cuonggt/tug@v0.38.0`. An
app that takes its mailer from `mail.FromEnv`, as one made by `tug new
-auth` does, keeps its mail in the mailbox under `tug dev` from then on,
with no change of its own, and its log has a line for each mail where it
had the mail. A deploy, the app's tests, and anything run without
`TUG_DEV` write mail out as before, and `MAIL_HOST` still sends it, under
`tug dev` too, as to a Mailpit.

### v0.37.0

Nothing an app calls changed: `go get github.com/cuonggt/tug@v0.37.0`, and
an app that gives no head from Go and has no `inertia.BigInt` goes as it
did, with the same types, and its tests pass as they did. To give its
pages a head from Go, an app made before puts `{{ .InertiaHead }}` in
`app.html`, before its `<title>`, as an app made with `-ssr` has it, and
takes that `<title>`'s `data-inertia` off, which Svelte needs; turns on
`serverHead: true` in `createInertiaApp`, on Inertia 3.5.0 or later, or in
Svelte, a function that leaves the title out, as a new app's
`resources/js/inertia.ts` has it; and gives `inertia.Config` a `Title`
that says a title as its `title` callback does. An `inertia.BigInt` needs
Inertia 3.8.0, as a new app's `package.json` has it.

### v0.36.0

Nothing an app calls changed: `go get github.com/cuonggt/tug@v0.36.0`, and
`go install github.com/cuonggt/tug/cmd/tug@v0.36.0` for the CLI. The app
lists its routes itself, in the run `tug gen` makes, so `tug routes` needs
the app on v0.36.0: one on an older tug says "the app has no routes". `tug
gen` writes the same types as before, and an app's tests pass as they did.

### v0.35.0

Nothing an app calls changed: `go get github.com/cuonggt/tug@v0.35.0`, and
an app with `APP_DEBUG` on shows the page from then on, one made before
too, with no change of its own, and its tests pass as they did. With it
off, as a deployed app has it, its errors are answered as before.
`APP_EDITOR=vscode` in the `.env` makes the frames links to the editor. A
client of a debugging app that read a server error's JSON gets `errors`,
`stack` and `route` beside its `message`, and the `message` without a
panic's stack after it.

### v0.34.0

Nothing an app calls changed: `go get github.com/cuonggt/tug@v0.34.0`, and
a queue without `Carry` runs as it did. An app made with `tug new -auth`
before keeps its jobs table and its Store as they are, and its tests pass,
as `TestStore` checks what's carried only of a `CarryStore`. To carry the
request's ID, its queue's `Carry` lists `middleware.CarryRequestID`, and
its jobs table keeps what's carried: a migration adds the column, `ALTER
TABLE jobs ADD COLUMN carried TEXT;`, and the Store writes it as it
pushes, reads it back as it claims and lists the failed, and says so with
`KeepsCarried`, as a new app's `jobs.go` and `jobs_db.go` do. `Carry`
without the Store's part carries nothing.

### v0.33.1

`go get github.com/cuonggt/tug@v0.33.1` mends an app made before with no
change of its own. A poll, a deferred prop's fetch, and `router.reload({
only: [...] })` no longer show a flash that was waiting: the next visit
does.

### v0.33.0

Nothing an app calls changed: `go get github.com/cuonggt/tug@v0.33.0`, and
`c.Flash` flashes as it did. tug gen writes `flashDataType` only once the
app declares a key, so an app made before keeps its
`resources/js/types.ts` as it is until then. To type its flash, it
declares its keys with `tug.Flash`, as the starters' `flash.go` does, sets
them through the declarations, runs tug gen, and deletes its own
`flashDataType`: one with the same keys of the same types is let through
beside tug gen's, and any other is `error TS2717: Subsequent property
declarations must have the same type`.

### v0.32.0

Nothing an app calls changed, and a route that takes nothing is as it was:
`go get github.com/cuonggt/tug@v0.32.0`. tug gen writes `Inputs` and
`form()` into `routes.ts` from then on, which the app commits as before.
To type a form, its route `Takes` what its handler binds, after its name,
and the form's action is `form()`'s, in place of `route()` and a `method`;
in React, `<Form<Inputs['the.route']>>`. A route at a time does, and the
starters' forms show each kind. A struct whose json and form tags name a
field two ways is the one change tug gen can ask of an app, once the
struct is taken.

### v0.31.1

Tug's Go is as it was, so `go get github.com/cuonggt/tug@v0.31.1` changes
nothing in an app. The fix is in the auth starter's frontend, the same
file in React, Vue and Svelte: an app made with `tug new -auth` from
v0.31.1, `go install github.com/cuonggt/tug/cmd/tug@v0.31.1`, has it, and
one made before takes [its
`resources/js/lib/broadcasts.ts`](https://github.com/cuonggt/tug/blob/v0.31.1/cmd/tug/starter-auth/resources/js/lib/broadcasts.ts)
as it is.

### v0.31.0

Nothing an app calls changed: `go get github.com/cuonggt/tug@v0.31.0`. The
starters' route of the build, `app.Get("/build/{path...}",
tug.WrapHandler(assets))`, is the same, so an app made before sends its
build gzipped once it's built with the new tug. A proxy or a CDN in front
that compresses already passes the app's gzip on. An app that wants brotli
adds a compression plugin to its Vite that writes `.br` files beside the
build's, and tug sends them.

### v0.30.1

`go get github.com/cuonggt/tug@v0.30.1`. Nothing else changed.

### v0.30.0

Nothing an app calls changed: `go get github.com/cuonggt/tug@v0.30.1`,
which names a route's action right when tug answers for its handler, as it
does a form's errors, where v0.30.0 could name middleware that was only on
the way. An app that reads its Config with `tug.ConfigFromEnv`, as the
starters' `main.go` does, keeps the entries under `tug dev` with no
change, and one that makes its own sets `DevTools` itself. Install the
extension, from the Chrome Web Store or Firefox Add-ons, and open the
Inertia panel of the browser's developer tools; its client side is in
Inertia 3.6 and later, and the starters require 3.7.1. The entries go in
`.tug/`, which every app `tug new` has made leaves out of git. Leave
`TUG_DEV` unset where the app is deployed: under it, the app keeps what
every request sent.

### v0.29.0

Nothing an app calls changed: `go get github.com/cuonggt/tug@v0.29.0`. The
`Logger`'s lines have a `route` beside the path, for a request a route
answered, which a collector that keys on their attributes sees as one
more; and `Route.String()` says a path ending in a slash without its
`{$}`. An app made by the starters needs nothing else: they keep no
metrics, and their log lines have the route. To count requests by route
and jobs by kind, add a client of your monitor's to the app's own
`go.mod`, as Prometheus's, `go get github.com/prometheus/client_golang`,
and give it the two hooks:
[docs/deployment.md](https://github.com/cuonggt/tug/blob/v0.29.0/docs/deployment.md#metrics)
has it in full,
[docs/routing.md](https://github.com/cuonggt/tug/blob/v0.29.0/docs/routing.md#the-route-that-answered)
`RouteOf`, and
[docs/jobs.md](https://github.com/cuonggt/tug/blob/v0.29.0/docs/jobs.md#how-each-run-went)
`Observe`.

### v0.28.0

Nothing an app calls changed: `go get github.com/cuonggt/tug@v0.28.0`. An
app made from the auth starter before this version made its tables by a
list of steps in `db.go`, counted in SQLite's `user_version`, or a
`schema_version` table. To move them to migrations, each step the app
added to the list goes in a file of its own first, by `tug migrate new`,
with the step's SQL, named after the starter's, in their order; then take
`migrations/`, `migrations.go`, `migrations_db.go` and
`migrations_test.go` from an app `tug new -auth` makes on the same
database, with its `db.go` and `db_test.go`, and in `main.go`, `upToDate`
once the database is open, unless the binary is `migrating`, and the
`migrate` command. The first start of the new version takes the count
over, and drops it: the first files by name, as many as it counted, are
recorded as run, without running, and the rest run. So those files must be
the steps the app ran: a step of the starter's it changed in place changes
the same way in its file, and an app that has steps of its own but not
every step of v0.27.0's, or took a step of the starter's without one
before it, leaves the files of the steps it hasn't out of `migrations/`
for that start, and puts them back after, when they run as any new
migration does. More steps counted than there are files stops the app,
saying so. Once the new version has started, a build from before can't
start on the database, as it finds no count and runs its first step again,
which fails, its table being there: a deploy rolled back past it puts the
count back first.
[docs/migrations.md](https://github.com/cuonggt/tug/blob/v0.28.0/docs/migrations.md)
has migrations,
[Accounts](https://github.com/cuonggt/tug/blob/v0.28.0/docs/auth.md#the-database)
the starter's database, and
[docs/cli.md](https://github.com/cuonggt/tug/blob/v0.28.0/docs/cli.md#tug-migrate)
`tug migrate new`.

### v0.27.0

`go get github.com/cuonggt/tug@v0.27.0`, and in `app.html`, pass the
page's nonce first to Vite's tags, `{{ vite .Nonce "resources/js/app.tsx"
... }}` and `{{ viteReactRefresh .Nonce }}`, as a template's function
can't see the request. Until it does, every page a browser loads in full
is a 500, whose error says so: `vite` takes the first entry for the nonce,
and refuses it, and `viteReactRefresh` takes one argument. An app without
a policy passes `.Nonce` all the same, which is `""` for it, and its tags
are as they were. `vite.Vite`'s `Tags` and `ReactRefresh` take the nonce
first too, for Go that calls them. Then, for the headers and the policy,
add `Headers` and `CSP` to the app's middleware, after `Recover`, as the
starters have them, with `nonce="{{ .Nonce }}"` on each script of the root
template's own, as the auth starter's script for light or dark; an app
whose pages have scripts that don't carry it yet starts with `ReportOnly:
true`, and reads the log. An app made from the auth starter with its
photos on S3 adds its bucket's `Origin()` to `img-src`.
[docs/deployment.md](https://github.com/cuonggt/tug/blob/v0.27.0/docs/deployment.md#security-headers)
has the headers and the policy,
[docs/pages.md](https://github.com/cuonggt/tug/blob/v0.27.0/docs/pages.md#setting-up)
the nonce in the root template, and
[docs/routing.md](https://github.com/cuonggt/tug/blob/v0.27.0/docs/routing.md#package-middleware)
the two middlewares.

### v0.26.0

Nothing an app calls changed: `go get github.com/cuonggt/tug@v0.26.0`. To
have notifications in an app made from the auth starter, take
`notifications.go`, `notifications_db.go` and `notifications_test.go` from
an app `tug new -auth` makes on the same database and frontend, with the
`Notifications` page, `resources/js/lib/broadcasts.ts`, the layout's bell,
and the tokens page and the two-factor settings, which keep their flash;
add its `CREATE TABLE notifications` at the end of the app's own
`migrations`, as a step of its own; in `main.go`, the app's
`notifications` and `notificationMail`, the `notification-mail` job, the
`prune-notifications` schedule, `pages.Share("bell", Bell{})` and the
`/notifications` route; `shareAuth`'s `bell`; and `a.notify` in the
transaction of each change it tells of, with `apiTokens.in` in
`tokens.go`. `PasskeyMail` and its kind, `passkey-mail`, go: a passkey's
mail still waiting as the new version starts has no handler, and fails for
good, which `./blog jobs` lists. The starter's tests read mail by address,
with `NextTo`, where two mails may come at once, and the admins' test of
v0.25.0 stops the app's queue first, with `stopQueue` in `main_test.go`,
as on MySQL it now and then lost its job to the queue.
[Accounts](https://github.com/cuonggt/tug/blob/v0.26.0/docs/auth.md#notifications)
has the notifications, and
[docs/testing.md](https://github.com/cuonggt/tug/blob/v0.26.0/docs/testing.md#mail)
`NextTo`.

### v0.25.0

Nothing an app calls changed: `go get github.com/cuonggt/tug@v0.25.0`. An
error with a `StatusCode() int` method, which was a 500, is now answered
with its status, and under 500 in its own words: another package's error
that has one, returned as it is, is answered by its status now. To have
the admins and their page in an app made from the auth starter, take
`abilities.go`, `admin.go` and `admin_test.go` from an app `tug new -auth`
makes on the same database and frontend, with the `Admin/FailedJobs` page
and the layout's nav for admins; add its `admin` column's step at the end
of the app's own `migrations`, and the column to `User`, `userColumns` and
`scanUser` in `users.go`, with `setAdmin` and `admins` from its
`users_db.go`; and take `shareAuth`'s `can`, and in `main.go`,
`pages.Share("can", Can{})`, the `admins` command and the page's three
routes.
[docs/authorization.md](https://github.com/cuonggt/tug/blob/v0.25.0/docs/authorization.md)
has abilities and the gate,
[Accounts](https://github.com/cuonggt/tug/blob/v0.25.0/docs/auth.md#admins)
the admins, and
[docs/routing.md](https://github.com/cuonggt/tug/blob/v0.25.0/docs/routing.md#errors)
the errors that say their status.

### v0.24.0

Nothing an app calls changed: `go get github.com/cuonggt/tug@v0.24.0`. A
kind with `AtOnce` or `Rate` needs a Store that's an `AtOnceStore` or a
`HoldBackStore`, and `Handle` panics without one, as for `Unique`'s
options; an app's own Store keeps working for every other kind. To have
the limits in an app made from the auth starter, take `jobs_db.go`, and
`KeepsAtOnce` from `jobs.go`, from an app `tug new -auth` makes on the
same database, and its migration step, the `at_once` column, the
`jobs_held` index and the `held_kinds` table, three steps on MySQL, at the
end of the app's own `migrations`.
[docs/jobs.md](https://github.com/cuonggt/tug/blob/v0.24.0/docs/jobs.md#limits-on-a-kind)
has the limits, [a job that fails for
good](https://github.com/cuonggt/tug/blob/v0.24.0/docs/jobs.md#when-a-job-fails-for-good)
`OnFail`, and [the
Stores](https://github.com/cuonggt/tug/blob/v0.24.0/docs/jobs.md#stores)
their new extras.

### v0.23.0

Nothing an app calls changed: `go get github.com/cuonggt/tug@v0.23.0`. To
have the starter's hub in an app made from it, take `broadcasts.go`,
`broadcasts_db.go` and `broadcasts_test.go` from an app `tug new -auth`
makes on the same database; in SQLite or MySQL, add its `CREATE TABLE
broadcasts` at the end of the app's own `migrations`, as a step of its
own; and in `main.go`, the hub `main` makes and runs with `app.Go`,
`env`'s `Hub`, the app's `hub` and `broadcasts`, the `/broadcasts` route,
and in SQLite or MySQL the `prune-broadcasts` schedule, with the hub run
in the tests' `startApp` too. The verify page's listener and
`verifyEmail`'s event are a few lines each. An auth app made since v0.19.0
has a `main_test.go` that `gofmt` would change, a struct's comments, which
`gofmt -w .` puts right; the starter's is right now.
[docs/broadcasting.md](https://github.com/cuonggt/tug/blob/v0.23.0/docs/broadcasting.md)
has the rest,
[Accounts](https://github.com/cuonggt/tug/blob/v0.23.0/docs/auth.md#broadcasting)
the starter's, and
[docs/testing.md](https://github.com/cuonggt/tug/blob/v0.23.0/docs/testing.md#events)
testing what a handler publishes.

### v0.22.0

Nothing an app calls changed: `go get github.com/cuonggt/tug@v0.22.0`.
`CSRF` takes the origins it did, and paths, which it panicked on before.
To have the starter's API tokens in an app made from it, take `tokens.go`,
`tokens_db.go` and `tokens_test.go` from an app `tug new -auth` makes on
the same database and frontend, with the `Settings/Tokens` page, its link
in the settings layout, and the flash's `token` in
`resources/js/types.ts`; add its `CREATE TABLE access_tokens` at the end
of the app's own `migrations`, as a step of its own; and in `main.go`, the
tokens' three fields, the settings page's routes, the `/api` group, and
`CORS` and `CSRF("/api/")` in the app's middleware.
[docs/auth.md](https://github.com/cuonggt/tug/blob/v0.22.0/docs/auth.md#api-tokens)
has the starter's API, [access
tokens](https://github.com/cuonggt/tug/blob/v0.22.0/docs/auth.md#access-tokens)
package `auth`'s, and
[docs/routing.md](https://github.com/cuonggt/tug/blob/v0.22.0/docs/routing.md#package-middleware)
`CORS` and `CSRF`.

### v0.21.0

`go get github.com/cuonggt/tug@v0.21.0`, and `go install
github.com/cuonggt/tug/cmd/tug@v0.21.0` for `tug key`. Nothing an app
calls changed, and two-factor secrets need nothing done. `auth.TwoFactor`
now panics for a key that isn't 32 bytes, where it took any length:
`session.KeysFromEnv`'s are 32, as `session.New` has always asked.
`session.ErrNoKey` says to run `tug key`, where it said `head -c 32
/dev/urandom | base64`; `errors.Is` finds it as before.
[docs/encryption.md](https://github.com/cuonggt/tug/blob/v0.21.0/docs/encryption.md)
has package `crypt` and [rotating the
key](https://github.com/cuonggt/tug/blob/v0.21.0/docs/encryption.md#rotating-the-key).

### v0.20.0

Nothing an app calls changed: `go get github.com/cuonggt/tug@v0.20.0`. A
download is a plain link, `<a href>`, not Inertia's `<Link>`, whose visit
asks for a page.
[docs/routing.md](https://github.com/cuonggt/tug/blob/v0.20.0/docs/routing.md#files-and-downloads)
has files, downloads,
[streams](https://github.com/cuonggt/tug/blob/v0.20.0/docs/routing.md#streams)
and
[events](https://github.com/cuonggt/tug/blob/v0.20.0/docs/routing.md#events),
and
[docs/pages.md](https://github.com/cuonggt/tug/blob/v0.20.0/docs/pages.md#downloads-and-events)
a page's side of them.

### v0.19.0

Nothing an app calls changed: `go get github.com/cuonggt/tug@v0.19.0`. An
app made from the auth starter keeps its own `outbox` in `main_test.go`,
working as it does; to use `mailtest.Outbox` instead, drop the type, give
the app `&mailtest.Outbox{}`, and write `app.outbox.Next(t)` where the
tests had `app.mail()`, `app.outbox.None(t)` for `app.noMail()`, and
`app.outbox.Down(1)` for `app.outbox.down.Store(1)`.
[docs/auth.md](https://github.com/cuonggt/tug/blob/v0.19.0/docs/auth.md#copies-replies-and-files)
has copies and files,
[unsubscribing](https://github.com/cuonggt/tug/blob/v0.19.0/docs/auth.md#unsubscribing-in-one-click)
its route, and
[docs/testing.md](https://github.com/cuonggt/tug/blob/v0.19.0/docs/testing.md#mail)
the outbox.

### v0.18.0

Nothing an app calls changed: `go get github.com/cuonggt/tug@v0.18.0`. To
have the starter's cache in an app made from it, take `cache_db.go` from
an app `tug new -auth` makes on the same database, add its `CREATE TABLE
cache` at the end of the app's own `migrations`, as a step of its own, and
in `newApp` the `cache` field, the `cacheTable` it's given, and the
`prune-cache` schedule.
[docs/cache.md](https://github.com/cuonggt/tug/blob/v0.18.0/docs/cache.md)
has the rest.

### v0.17.0

Nothing an app calls changed: `go get github.com/cuonggt/tug@v0.17.0`, and
an app made from the auth starter keeps its own `command`, in `main`,
working as it does. To have `jobs` on `app.Command`, take the new
starter's `main`, without its check of `os.Args` and `command`, and the
`app.Command("jobs", ...)` in its `newApp`. The one difference that
brings: a command runs in the app as `main` makes it, so `./blog jobs`
needs the environment the server does, `APP_KEY` and `APP_URL` among it,
where it needed the database alone; in a container, `docker exec` has it.
[docs/routing.md](https://github.com/cuonggt/tug/blob/v0.17.0/docs/routing.md#commands)
has commands.

### v0.16.0

Nothing an app calls changed: `go get github.com/cuonggt/tug@v0.16.0`. An
app that pages a list itself can hand its count and its query to
`tug.Paginate`, and a `Scroll` its `Paging()`:
[docs/pages.md](https://github.com/cuonggt/tug/blob/v0.16.0/docs/pages.md#pagination)
has them.

### v0.15.0

`go get github.com/cuonggt/tug@v0.15.0`. Nothing an app calls was removed,
and without `Config.Lang`, everything is in English, as it was. Two
English messages read differently, for tests that compare them: a field is
named in words, so "password_confirmation must match password" is now
"password confirmation must match password"; and a value `Bind` can't
parse under `author.age` is "age must be a whole number", where it was
"author.age must be a whole number", with its error under `author.age`
still. For an app's languages, take the new starter's `//go:embed
all:lang`, the `lang.Load` in `newApp` and an empty `lang/`, then run `tug
lang vi`:
[docs/languages.md](https://github.com/cuonggt/tug/blob/v0.15.0/docs/languages.md)
has the rest.

### v0.14.0

Nothing an app calls changed, so `go get github.com/cuonggt/tug@v0.14.0`
is enough, and an app not behind a proxy needs nothing more. Behind one,
put `middleware.TrustProxies` first in `app.Use`, with the proxies in
`TRUSTED_PROXIES`, and count by `c.IP()`: in an app made from the auth
starter, `clientIP(c.Request())` in `auth.go` and `passkeys.go` becomes
`c.IP()`, and the proxy's lines of the new starter's `main.go` go into
`newApp`. Its `base` and `link`, in `mail.go`, can give way to
`a.routes.AbsoluteURL`, with the `Base` its mail jobs carry; jobs already
queued with one still run. An app that takes the new starter's `newApp`
stops without `APP_URL` in development too, where it took the request's
`Host`: `tug dev` sets it.
[docs/deployment.md](https://github.com/cuonggt/tug/blob/v0.14.0/docs/deployment.md#behind-a-proxy)
has proxies, and
[docs/routing.md](https://github.com/cuonggt/tug/blob/v0.14.0/docs/routing.md#whole-links-and-signed-ones)
signed links.

### v0.13.0

This is the first release that changes calls an app makes. `Try`, `Wait`
and `Clear` take a context and return an error: a store can fail, as a
database can, and a try that can't be counted is the handler's to answer,
rather than the throttle's to let in, stopping nothing while the database
is down, or to turn away, locking everyone out when it blinks.

```go
// v0.12.0
if wait := a.logins.Try(key); wait > 0 {
	return validate.Errors{"email": tooMany(wait)}
}
a.logins.Clear(key)

// v0.13.0
wait, err := a.logins.Try(c.Context(), key)
if err != nil {
	return err
}
if wait > 0 {
	return validate.Errors{"email": tooMany(wait)}
}
if err := a.logins.Clear(c.Context(), key); err != nil {
	return err
}
```

After `go get github.com/cuonggt/tug@v0.13.0`, the compiler names each
call to change. In an app made from the auth starter, they're in
`auth.go`, `passkeys.go`, `twofactor.go` and `verify.go`, and
`checkPassword` takes the request's context for them, which `settings.go`
passes. Nothing else changed: the throttles count in memory, as they did,
until they're given a store. For the starter's table, take the new
starter's `throttles_db.go` and `throttles_test.go`, the migration step
that makes `throttles`, at the end of `db.go`'s, each throttle's `Name`
and `Store` in `newApp`, and the `prune-throttles` job:
[docs/auth.md](https://github.com/cuonggt/tug/blob/v0.13.0/docs/auth.md#throttles)
has them.

### v0.12.0

Nothing was removed, so an app made with v0.11.0 needs only `go get
github.com/cuonggt/tug@v0.12.0`. A new SQLite app has the same SQL and
migrations as before, moved into `db.go` and the `_db.go` files from
`users.go`, `passkeys.go` and `jobs.go`. To move an app to Postgres or
MySQL, take those files, and `compose.yaml`, from an app `tug new` makes
on it, and move the data;
[docs/auth.md](https://github.com/cuonggt/tug/blob/v0.12.0/docs/auth.md#the-database)
has what differs between the databases.

### v0.11.0

Nothing was removed, so an app made with v0.10.0 needs only `go get
github.com/cuonggt/tug@v0.11.0`. One thing reads differently: `validate`
names a field with a `form` tag and no `json` one by its `form` tag, where
it had its Go name, so its error goes under the name the form's input has.
For the photo, the new starter's `photos.go`, the migration step that adds
`photo_key`, and its frontend's profile page and user menu are the parts
to take;
[docs/files.md](https://github.com/cuonggt/tug/blob/v0.11.0/docs/files.md)
goes through each.

### v0.10.0

Nothing was removed, and `queue.Unique()` is as it was, so an app made
with v0.9.0 needs only `go get github.com/cuonggt/tug@v0.10.0`, and its
code, its own `jobs.go` and its tests work as they are. For the new
options and the command, its Store needs the extras, as the new starter's
`jobs.go` has them, with the migration step that adds `alone_key`; for
pushing in a transaction, its stores go through one, as the starter's
`in(tx)` and `a.inTx` do.
[docs/jobs.md](https://github.com/cuonggt/tug/blob/v0.10.0/docs/jobs.md)
goes through each.

### v0.9.0

Nothing in tug's packages changed, so an app made with v0.8.0 needs only
`go get github.com/cuonggt/tug@v0.9.0`, and works as it did. What's new is
in the starters, which an app starts from once: a Vue or Svelte app starts
with `tug new`.

### v0.8.0

Nothing was removed or renamed, so an app made with v0.7.0 needs only `go
get github.com/cuonggt/tug@v0.8.0`, and its code and tests work as they
are. For passkeys, it takes what the new starter has: the migration step
in `users.go`, `passkeys.go`, and on the frontend, `lib/passkeys.ts`,
`components/passkey-settings.tsx`, and the login and confirmation pages.
[docs/auth.md](https://github.com/cuonggt/tug/blob/v0.8.0/docs/auth.md)
goes through them.

### v0.7.0

Nothing was removed or renamed, so an app made with v0.6.0 needs only `go
get github.com/cuonggt/tug@v0.7.0`, and its code and tests work as they
are. To use `Unique`, its Store needs `PushUnique` and a claim that clears
the key, as the new starter's `jobs.go` and migration have. An app whose
instances run schedules in a zone on different machines imports
`time/tzdata`, so they all work out the same times.
[docs/jobs.md](https://github.com/cuonggt/tug/blob/v0.7.0/docs/jobs.md)
has the rules.

### v0.6.0

Nothing was removed or renamed, so an app made with v0.5.0 needs only `go
get github.com/cuonggt/tug@v0.6.0`, and its code and tests work as they
are. Its tests' own client can give way to `tugtest` when it likes:
[docs/testing.md](https://github.com/cuonggt/tug/blob/v0.6.0/docs/testing.md)
shows how.

### v0.5.0

Nothing was removed or renamed, so an app made with v0.4.0 needs only `go
get github.com/cuonggt/tug@v0.5.0`, and its code and tests work as they
are. [docs/ssr.md](https://github.com/cuonggt/tug/blob/v0.5.0/docs/ssr.md)
shows how to turn SSR on in an app of your own; new apps get it with
`-ssr`.

### v0.4.0

Nothing was removed or renamed, so an app made with v0.3.0 needs only `go
get github.com/cuonggt/tug@v0.4.0`, and its code and tests work as they
are. To use `Schedule`, its store in `jobs.go` needs `PushScheduled` and a
`schedules` table, as the starter now makes them:
[docs/jobs.md](https://github.com/cuonggt/tug/blob/v0.4.0/docs/jobs.md)
has both.

### v0.3.0

Nothing was removed or renamed, so an app made with v0.2.0 needs only `go
get github.com/cuonggt/tug@v0.3.0`, and its code and tests work as they
are. Its mail goes as it did: the jobs come with new apps, and
[docs/jobs.md](https://github.com/cuonggt/tug/blob/v0.3.0/docs/jobs.md)
shows how to add a queue to an app of your own.

### v0.2.0

Nothing was removed or renamed, so an app made with v0.1.0 needs only `go
get github.com/cuonggt/tug@v0.2.0`, and its code and tests work as they
are. The starter's new pages and handlers come with new apps; an app made
with v0.1.0 keeps its own.
