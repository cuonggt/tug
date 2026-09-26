# The CLI

`tug` makes and runs tug apps:

```
tug new <dir>    make a new app in dir, ready to run
tug dev          run the app, rebuilding and reloading it as it changes
tug gen          write the TypeScript of the app's pages and routes
tug build        build the app into one binary, with its frontend in it
tug version      print tug's version
```

`tug <command> -h` prints a command's flags. A command that fails says why
after `tug:`, and exits with status 1.

`tug dev`, `tug gen` and `tug build` run in the app's directory, the one
with its `main` package and its `package.json`, and stop, saying which is
missing, anywhere else. The directory needn't have a `go.mod` of its own,
as an app inside a bigger Go module doesn't.

## What the commands share

### `.env` and the environment

`tug dev`, `tug gen` and `tug build` read the app's `.env`, and run `go`,
npm and the app with what it sets. A variable set in tug's own environment
wins over the file's, so `ADDR=127.0.0.1:9000 tug dev` runs the app there
whatever `.env` says. The file is lines of `KEY=value`:

```sh
# A comment is a line that starts with #.
APP_KEY=base64:...
export MAIL_FROM_NAME="The blog"
```

A leading `export ` and the quotes around a value are dropped, and blank
lines are skipped. A `#` after a value is part of the value. A line that
isn't `KEY=value` stops the command, which names the line. A missing
`.env` is the same as an empty one.

The app itself never reads `.env`. Built and deployed, it takes its
settings from its environment.

The variables tug itself uses:

- `ADDR` and `PORT`: where `tug dev` runs the app.
- `TUG_GEN`: set by tug when it runs the app for its types; see
  [`tug gen`](#tug-gen).
- `NO_COLOR`: when it's set, `tug dev`'s labels aren't colored. They're
  only colored on a terminal.
- `CGO_ENABLED`: set to `0` by `tug build` for the binary.

### The package manager

`tug dev` and `tug build` run the frontend's scripts with the package
manager whose lockfile the app has: Bun for `bun.lock` or `bun.lockb`, pnpm
for `pnpm-lock.yaml`, Yarn for `yarn.lock`, and npm otherwise. `tug new`
installs with npm, and the starter's Dockerfile builds with it.

### `.tug`

`.tug/` is where tug works in the app. `.tug/app` is the app's binary, as
`tug gen` and `tug dev` build it; `.tug/gen.json` is the TypeScript the app
writes for `tug gen`; and `.tug/reload` is the file `tug dev` writes to
reload the browser. The starter's `.gitignore` and `.dockerignore` leave
it out.

## `tug new`

```
tug new [flags] <dir>
```

- `-auth`: with accounts: registering and verifying an email, logging in
  with a second factor if the user likes, resetting a password by email,
  and settings, with the users in SQLite and a frontend of Tailwind and
  shadcn/ui.
- `-ssr`: with server-side rendering: a first visit's page is rendered on
  the server as well as in the browser, by Node, which runs beside the app.
- `-module path`: the app's Go module path. Default: the directory's name.
- `-no-install`: don't install the app's packages or write its types.
- `-tug-dir dir`: a checkout of tug to build the app against, rather than
  a release.

The flags and the directory can come in any order: `tug new blog -auth`
is `tug new -auth blog`.

`tug new` stops when `dir` is there and isn't empty. Otherwise it:

1. works out which tug the app requires (below);
2. writes the starter, `cmd/tug/starter` in tug's source, and with `-auth`
   `cmd/tug/starter-auth` over it, whose files replace the plain starter's
   of the same name and add the rest. The directory's name is the app's:
   in `package.json`, the page titles, and the `appName` prop;
3. writes `.env`, readable by you alone, with `APP_KEY=base64:` and 32
   random bytes in base64, and `APP_DEBUG=true`;
4. unless `-no-install` says not to, runs `go mod tidy` and
   `npm install`, then builds the app and writes its types, as `tug gen`
   does.

After `-no-install`, run `go mod tidy` before `tug dev`, as `tug new` says:
the app doesn't build until that has filled in its `go.mod` and `go.sum`.
`tug dev` installs the npm packages itself when `node_modules` isn't
there, and writes the types.

With `-auth`, the Go files for accounts are added, `auth.go`,
`verify.go`, `twofactor.go`, `settings.go`, `mail.go` and `users.go`, with
their tests, and so is a frontend of Tailwind and shadcn/ui: its layouts,
components and hooks, the pages in `resources/js/pages/Auth` and
`resources/js/pages/Settings`, and shadcn's `components.json`. The plain
starter's `Layout.tsx` is left out, and most of its other files are the
auth starter's own: `main.go`, `main_test.go`, `app.html`, `package.json`,
`vite.config.ts`, `tsconfig.json`, `app.tsx`, `app.css`, the pages it has,
`Dockerfile`, `.env.example`, `.gitignore`, `.dockerignore` and
`README.md`. [Accounts](auth.md) goes through them.

With `-ssr`, either starter gets `resources/js/ssr.tsx` and `ssr/.gitkeep`,
and its `main.go`, `main_test.go`, `app.html`, `package.json`,
`vite.config.ts`, `Dockerfile`, `.env.example`, `.gitignore`,
`.dockerignore` and `README.md` render pages on the server too.
[Server-side rendering](ssr.md) goes through them.

### Which tug the app requires

The app's `go.mod` requires the tug that `tug new` is:

- **A release**, installed with `go install
  github.com/cuonggt/tug/cmd/tug@latest` or `@v0.4.0`, makes apps that
  require that version.
- **A commit the go command fetched**, as `go install
  github.com/cuonggt/tug/cmd/tug@<commit>` does, makes apps that require
  its pseudo-version, which the go command can fetch for them too. The
  checksum in tug's build info says it came that way.
- **A build from a checkout** of tug, whose version is `(devel)` or a
  pseudo-version, makes apps that build against that checkout:

  ```
  require github.com/cuonggt/tug v0.0.0

  replace github.com/cuonggt/tug => /home/you/src/tug
  ```

  tug finds the checkout by its own source path, which the compiler
  records unless tug was built with `-trimpath`. When it can't, `tug new`
  stops: "this tug isn't a release: pass -tug-dir with the checkout of tug
  to build the app against".

`-tug-dir` builds the app against the checkout it names, whichever tug
runs it. The directory's `go.mod` must be `module github.com/cuonggt/tug`,
or `tug new` stops, saying it isn't a checkout of tug.

## `tug dev`

```
tug dev
```

Runs the app for development. It has no flags.

### What it runs

1. It reads `.env`, and installs the frontend's packages when
   `node_modules` isn't there.
2. It starts Vite's dev server: `npm run dev`.
3. It picks the app's address (below), and passes it to the app as
   `ADDR`, with `TUG_DEV=1`, which tells an app with server-side rendering
   that the dev server renders its pages, so it runs no Node of its own.
4. It builds the app into `.tug/app`, writes its types as `tug gen` does,
   starts it, and waits for it to take connections. Then it says where the
   app is, and reloads the browser:

   ```
   tug  │ serving http://127.0.0.1:8080, built in 268ms
   ```

5. It waits for a change, and goes back to 4.

Its output, and that of what it runs, comes a line at a time, labeled
`tug`, `app` (the build's and the app's) or `vite`. Ctrl-C or SIGTERM stops
Vite and the app, and `tug dev` with them. Each runs in a process group of
its own, so that what it starts stops too: npm runs Vite as a child. On
Windows, which has no process groups, the process is killed.

### The address

With `ADDR` or `PORT` set, in `.env` or the environment, the app runs
where they say, as `tug.ConfigFromEnv` reads them. A port alone,
`ADDR=:9000` or `PORT=9000`, is `127.0.0.1:9000` under `tug dev`. When
something else listens there, `tug dev` stops and says so.

Without either, it's `127.0.0.1:8080`, or when that's taken, the next free
port up to 8099, as `php artisan serve` does:

```
tug  │ 127.0.0.1:8080 is taken, so the app is on 127.0.0.1:8081
```

### What it watches

`tug dev` looks at the app's files every 300 milliseconds, rather than
depend on a library that watches them, for the ones the Go server needs a
rebuild for:

- `.go` files, `go.mod` and `go.sum`;
- templates Go embeds: `.html`, `.tmpl` and `.gohtml` files.

It looks in every directory but `node_modules`, `vendor`, `testdata` and
`public`, and those whose names start with `.` or `_`. After a change, it
waits 100 milliseconds for the rest of the save, such as an editor saving
several files, or a formatter after it, and builds once:

```
tug  │ main.go and 2 more changed
```

The frontend isn't among them: Vite watches it (below). Nor is `.env`,
which is read once, when `tug dev` starts: restart `tug dev` after changing
it.

### After a change

- **The build fails.** The compiler's errors show under `app`, `tug dev`
  says "the build failed: fix it, and tug builds again when it's saved",
  and the app from the last build that worked keeps running, so the
  browser still has a server to talk to.
- **The build works.** The types are written again, but only the files
  whose content changed, so that one that hasn't doesn't set off Vite, or
  an editor. The old app gets SIGTERM, which lets it finish the requests
  it's answering, and is killed if it's still there 5 seconds later. The
  new one starts, and once it takes connections, the browser reloads. An
  app that isn't listening within 10 seconds keeps running, but the
  browser isn't reloaded. When the types can't be written, `tug dev` says
  why and starts the app anyway.
- **The app stops by itself**, as when `main` calls `log.Fatal`: "the app
  stopped (exit status 1): tug starts it again when a file is saved".
- **Vite stops**: `tug dev` stops too.

### How the browser follows

The browser loads the page from the Go server, and its scripts from Vite.
The starter's `vite.config.ts` has a plugin called `tug` that ties the two
together while the dev server runs:

- **`public/hot`.** Once Vite is listening, the plugin writes its address,
  `http://localhost:5173`, to `public/hot`, and removes the file when Vite
  exits. The Go server's `vite` package reads that file each time it
  renders a page. While it's there, pages load their scripts from the dev
  server, with React's refresh; otherwise, from the build in
  `public/build`. So the Go server needn't restart when Vite starts or
  stops.
- **`.tug/reload`.** The plugin watches this file, which `tug dev` writes
  after each restart of the app, and has Vite tell the browser to reload
  the page.

A change to the frontend doesn't go through `tug dev` at all: Vite sends
the changed module to the browser, and a React component updates in place,
keeping its state. When `tug dev` writes `resources/js/tug` again, Vite
sees those files change too, and updates what imports them.

A `public/hot` left behind by a Vite that didn't exit cleanly points the
Go server at a dev server that isn't there, and the pages' scripts don't
load: delete it.

Vite's port is 5173, and fixed (`strictPort`), because `public/hot` and
`server.origin` name it: when another app's Vite has it, this one exits,
and `tug dev` with it. To run two apps at once, give one of them another
port in its `vite.config.ts`, in both `devServer` and `server.port`.

## `tug gen`

```
tug gen
```

Writes the TypeScript of the app's pages and named routes into
`resources/js/tug`, and says which files it wrote, or "the types are up to
date". It has no flags.

- `pages.ts` has the props of each page declared with `tug.Page`, and of
  the error page, `Config.ErrorPage`; the props shared with `Share`; and
  `PageProps<'Component'>`, a page's props and the shared ones together.
- `routes.ts` has the named routes, and `route()`, which builds a route's
  path from its name and values, as `app.URL` does in Go. A route without
  a name isn't in it.

[TypeScript](typescript.md) has what each Go type becomes.

tug learns the pages and routes by running the app, rather than by reading
its source: route groups and prefixes only exist at run time. `tug gen`
builds the app into `.tug/app`, with `.env` read as `tug dev` reads it, and
runs it with `TUG_GEN` set to a file, `.tug/gen.json`. `app.Run` sees
`TUG_GEN`, and writes the TypeScript there and returns, instead of serving.
Then `tug gen` writes each of the two files whose content has changed.

So whatever `main` does before `app.Run`, it does for `tug gen` too. The
starter's `main` reads `APP_KEY`, and stops without it:

```
tug: the app stopped before writing its types (exit status 1):
2026/09/25 16:33:11 session: APP_KEY isn't set; make one with `head -c 32 /dev/urandom | base64` and set APP_KEY=base64:<that>
```

The auth starter's `main` also opens the SQLite database, which makes
`app.db` when it isn't there, and its `newApp` stops without `APP_URL`
unless `APP_DEBUG` is on. Where there's no `.env`, as in CI, set what
`main` needs:

```sh
APP_KEY=base64:$(head -c 32 /dev/urandom | base64) tug gen
```

The app has 30 seconds to write its types. One that runs longer, or exits
without calling `app.Run`, stops `tug gen` with a question: "does main
call app.Run?".

`tug new`, `tug dev` and `tug build` write the types too, so `tug gen` by
hand is for changes made while `tug dev` isn't running. The starter's
`.gitignore` leaves `resources/js/tug` in, to commit with the Go it's
written from. A CI step can check that it's current, as tug's own CI does
for its example:

```sh
tug gen && git diff --exit-code resources/js/tug
```

## `tug build`

```
tug build [-o binary]
```

- `-o binary`: the binary to write, such as `-o bin/blog`. Default: the
  directory's name.

Builds the app into one binary with its frontend in it. It says each step
as it starts it:

1. `tug build: the types`: it builds the app and writes its types, as
   `tug gen` does, so it needs what `main` needs too.
2. `tug build: type-checking the frontend`: `npm run typecheck`, when
   `package.json` has a `typecheck` script. The starter's is
   `tsc --noEmit`.
3. `tug build: the frontend`: `npm run build`. Vite writes the build, with
   its manifest, to `public/build`, which the Go server serves under
   `/build/`. With server-side rendering, the script runs
   `vite build --ssr` too, which writes the SSR build to `ssr/build`.
4. `tug build: the binary`: `go build -trimpath -ldflags="-s -w"`, with
   `CGO_ENABLED=0`. That makes it static, so it runs with nothing else
   installed, and leaves out the debug information and the paths of the
   machine it was built on.

Then it says what it made:

```
tug build: blog, 9.6 MB, frontend included
```

A step that fails stops it, and names the command. The frontend is inside
because the starter's `main.go` embeds `public/`, with
`//go:embed all:public`, and `app.html`, and with server-side rendering
`ssr/` too, which Node runs beside the app.

`tug build` makes a binary for the system it runs on. With `GOOS` or
`GOARCH` set for another, the first step can't run the app it has built,
and stops. The starter's `Dockerfile` builds on Linux, with the same
`go build` as step 4: [Deployment](deployment.md) covers it.

## `tug version`

```
tug version
```

Prints tug's version: `tug v0.4.0` for that release. A tug installed at a
commit prints its pseudo-version, and one built from a checkout prints
`(devel)` or a pseudo-version, with `+dirty` when the checkout had changes.
It's the version `tug new` goes by.

`tug help`, `tug -h` and `tug --help` print the list of commands.
