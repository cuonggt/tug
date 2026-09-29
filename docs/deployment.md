# Deployment

A tug app ships as one binary: the Go server, with the frontend's build
inside it. `tug build` makes that binary, and every app that `tug new` makes
has a Dockerfile that builds it into an image. The binary takes its
settings from the environment. This page covers each of those, then
running behind a proxy, shutting down, health checks, rotating the key,
and logs.

## `tug build`

`tug build`, run in the app's directory, says what it's doing, among the
output of npm and Vite:

```
tug build: the types
tug build: type-checking the frontend
tug build: the frontend
tug build: the binary
tug build: blog, 9.6 MB, frontend included
```

In order, it:

1. writes the TypeScript types, as `tug gen` does, by building the app and
   running it: see [typescript.md](typescript.md);
2. runs the `typecheck` script when `package.json` has one, as the
   starters' does (`tsc --noEmit`), so a frontend that no longer fits the
   Go stops the build;
3. runs the `build` script, `vite build`, which writes the frontend to
   `public/build`;
4. builds the Go with `CGO_ENABLED=0 go build -trimpath -ldflags="-s -w"`:
   a static binary without its symbol table and debug information, named
   after the app's directory, or as `-o` says.

The scripts run with the package manager that the app's lockfile is for:
bun, pnpm or yarn, and npm when there's none of theirs. Like `tug dev`,
`tug build` reads the app's `.env` for these steps.

The binary is the whole app:

```sh
APP_KEY=base64:... ./blog
```

It doesn't read `.env`, though: only `tug dev`, `tug gen` and `tug build`
do. Set its variables in the environment it runs in, which the
[configuration](#configuration) lists.

`tug build` makes a binary for the machine it runs on. Setting `GOOS` for
it fails, as it runs the app it builds to write the types:

```
tug: the app stopped before writing its types (fork/exec .../.tug/app: exec format error):
```

For a Linux server, from a Mac, let `tug build` build the frontend, then
build the Go again for Linux, which embeds the same `public/build`:

```sh
tug build
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o blog-linux .
```

The [Dockerfile](#the-dockerfile) is the other way: it builds on Linux.

### The frontend in the binary

The starter's `main.go` embeds `public/`, and serves the build from it:

```go
// public is what the frontend's build writes to public/build, and anything
// else public/ holds. The .gitkeep lets it compile before the first build.
//
//go:embed all:public
var public embed.FS

// in main and newApp
build, err := fs.Sub(public, "public/build")
assets, err := vite.New(vite.Config{Build: build, HotFile: "public/hot"})
app.Get("/build/{path...}", tug.WrapHandler(assets))
```

- `all:` embeds names that start with `.` or `_`, which `go:embed` leaves
  out otherwise, and Vite writes its manifest to `.vite/manifest.json`.
- `go build` embeds what `public/build` holds when it runs, which is why
  `tug build` builds the frontend first. A binary built before any
  frontend build answers every page with a 500, and the log says there's
  no build to load.
- The build is served under `/build/`. The files in `assets/` have a hash
  of their content in their names, so they're cached for a year:
  `Cache-Control: public, max-age=31536000, immutable`. The manifest, and
  anything else whose name starts with a dot, isn't served.
- The manifest's hash is the frontend's version, which Inertia's client
  sends with each visit. After a deploy with a new frontend, a browser
  still running the old one gets a 409 on its next visit and loads the page
  afresh, so open tabs pick up the deploy.

Run in the app's directory, the binary finds the `public/hot` that Vite's
dev server writes, and loads the frontend from the dev server rather than
from its own build. A dev server that didn't exit cleanly leaves the file
behind: delete it.

## Configuration

| Variable            | What it does | Default | Read by |
|---------------------|--------------|---------|---------|
| `ADDR`              | Where the app listens, as `host:port`. | `:8080`, every interface | `tug.ConfigFromEnv` |
| `PORT`              | The port, when `ADDR` isn't set, as Cloud Run and Fly.io set it. | none | `tug.ConfigFromEnv` |
| `APP_DEBUG`         | `true` or `1` puts a 500's error, and a panic's stack, in the response. Leave it off in production. | off | `tug.ConfigFromEnv` |
| `APP_URL`           | The app's address, such as `https://example.com`, which the links that leave it start with: `AbsoluteURL` and `SignedURL`'s, and the auth starter's mail, whose passkeys are for it too. With `https://`, the starters' session cookie is for HTTPS only. `tug dev` sets it to the address it shows. | none: the auth starter stops without it | `tug.ConfigFromEnv` |
| `APP_KEY`           | Encrypts the session cookies. The auth starter also encrypts two-factor secrets with it, and signs the links in its mail and to its photos, as `SignedURL` signs. | none: the starters stop without it | `session.KeysFromEnv` |
| `APP_PREVIOUS_KEYS` | Keys being rotated out, comma separated. They still decrypt sessions and check links. | none | `session.KeysFromEnv` |
| `TRUSTED_PROXIES`   | The proxies in front of the app, such as a load balancer, whose `X-Forwarded-For` says whose each request is: addresses or ranges, comma separated, as `10.0.0.0/8`, or `*` for whatever connects. See [Behind a proxy](#behind-a-proxy). | none: a request is from whatever connected | the starters' `main.go`, for `middleware.TrustProxies` |

The auth starter reads these as well:

| Variable            | What it does | Default | Read by |
|---------------------|--------------|---------|---------|
| `DB_PATH`           | The SQLite database. | `app.db`, and `/data/app.db` in its image | its `db.go` |
| `DB_URL`            | On Postgres or MySQL, the database, as `postgres://user:password@host:5432/blog?sslmode=require` or `mysql://user:password@host:3306/blog?tls=true`, with the driver's settings in its query. | none: needed, unless `DB_HOST` is set | its `db.go` |
| `DB_HOST`, `DB_PORT`, `DB_DATABASE`, `DB_USERNAME`, `DB_PASSWORD` | The database's parts, as Laravel names them, where there's no `DB_URL`. | the port is `5432` or `3306` | its `db.go` |
| `FILES_PATH`        | The directory the photos people upload are kept in, unless `FILESYSTEM_DISK` is `s3`. | `files`, and `/data/files` in its image | its `main.go` |
| `FILESYSTEM_DISK`   | `local`, the directory, or `s3`, a bucket, which instances on more than one machine share. | `local` | `storage.FromEnv` |
| `AWS_BUCKET`, `AWS_DEFAULT_REGION`, `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_ENDPOINT`, `AWS_USE_PATH_STYLE_ENDPOINT` | The bucket, and the keys to it, for `FILESYSTEM_DISK=s3`: in S3, or a service that speaks its API, as [files.md](files.md#from-the-environment) has. | none, and `us-east-1` for the region | `storage.FromEnv` |
| `QUEUE_WORKERS`     | How many background jobs, such as mail, run at once. With `0`, this instance runs none and leaves its jobs to the others on the database. | `4` | its `main.go` |
| `MAIL_HOST`         | The SMTP server. Without it, mail is written to the standard error instead of sent. | none | `mail.FromEnv` |
| `MAIL_PORT`         | The server's port. On 465 the connection is TLS from the start; on another, it uses STARTTLS when the server offers it. | `587` | `mail.FromEnv` |
| `MAIL_USERNAME`, `MAIL_PASSWORD` | The login, for a server that wants one. | none | `mail.FromEnv` |
| `MAIL_FROM_ADDRESS` | Who mail is from. It's needed with `MAIL_HOST`. | none | `mail.FromEnv` |
| `MAIL_FROM_NAME`    | The name that goes with it. | none | `mail.FromEnv` |

A key is 32 random bytes in base64, after `base64:`:

```sh
echo "APP_KEY=base64:$(head -c 32 /dev/urandom | base64)"
```

The `.env` that `tug new` writes has a key for development. A deployed app
needs a key of its own, kept secret, and the same one in every copy of the
app that runs: a session one copy writes, another reads.

`ADDR` wins over `PORT`. The Dockerfiles set neither, so the app listens on
`:8080`, or on the `PORT` that a platform such as Cloud Run sets.

An app with server-side rendering, `tug new -ssr`, reads these as well
([ssr.md](ssr.md)):

| Variable   | What it does | Default | Read by |
|------------|--------------|---------|---------|
| `SSR_NODE` | The Node that renders pages on the server, beside the app. | the `node` on `PATH`, and `/nodejs/bin/node` in its image | its `main.go` |
| `SSR_URL`  | An SSR server run apart, such as `http://127.0.0.1:13714`, to render pages with, in place of a Node of the app's own. | none | its `main.go` |

## The Dockerfile

The plain starter's:

```dockerfile
# The app as one static binary, frontend included, on an image with nothing
# else in it.
#
#   docker build -t app . && docker run -p 8080:8080 -e APP_KEY=... app

FROM node:24-alpine AS frontend
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM golang:1.26-alpine AS server
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/public/build public/build
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /server .

FROM gcr.io/distroless/static-debian12
COPY --from=server /server /server
# The app listens on :8080, or on the PORT a platform sets.
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/server"]
```

- `frontend` installs the npm packages and runs `vite build`. It uses npm
  and `package-lock.json`, as `tug new` sets an app up. It has no Go, so
  it neither writes the types nor checks them: it builds with the files in
  `resources/js/tug` as they are, which is why they're committed (see
  [typescript.md](typescript.md#keeping-them-current)).
- `server` downloads the Go modules in a layer of their own, which Docker
  keeps until go.mod or go.sum changes, puts the frontend's build in
  `public/build`, and builds the binary as `tug build` does.
- The image the app runs in is distroless: the binary and not much else,
  with no shell and no package manager. It has CA certificates, which mail
  over TLS needs, and time zone data. The app runs as its `nonroot` user.

`.dockerignore` keeps `node_modules`, `public/build`, `public/hot`,
`.tug`, `.env` and `.git` out of the build, and the auth starter's on
SQLite keeps a development `app.db` out too. So the frontend is built
afresh, and the development `.env` never reaches the image: the
variables come with `docker run -e`, as the Dockerfile's comment shows.

The auth starter's image, on SQLite, keeps its database, and the photos
people upload, in `/data`, a volume, so that they outlive the container:

```dockerfile
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /server . && mkdir /data

FROM gcr.io/distroless/static-debian12
COPY --from=server /server /server
# 65532 is the image's nonroot user, who has to be able to write the database.
COPY --from=server --chown=65532:65532 /data /data
ENV DB_PATH=/data/app.db FILES_PATH=/data/files
# The app listens on :8080, or on the PORT a platform sets.
EXPOSE 8080
VOLUME /data
USER nonroot:nonroot
ENTRYPOINT ["/server"]
```

The app runs as `nonroot`, whose ID is 65532, so `/data` has to belong to
65532. Owning the file isn't enough: SQLite makes the database in that
directory when it isn't there, and, with the write-ahead log the starter
turns on, its `-wal` and `-shm` files beside it, and the app makes
`/data/files` as the first photo goes in. With `FILESYSTEM_DISK=s3`, the
photos are in the bucket instead, and the volume has the database alone.
The distroless image has no shell to make the directory in, so the
`server` stage makes it and the copy sets its owner. A new named volume
starts with the image's `/data`, owner and all. A directory of the
host's, mounted with `-v /srv/blog:/data`, keeps the host's owner: make
it writable by 65532.

The SQLite driver is modernc.org/sqlite, in pure Go, so the binary is
still static.

### On Postgres or MySQL

An app made with `-postgres` or `-mysql` has the same image, with `/data`
for the photos alone: the database is wherever `DB_URL` says, with the
driver's settings in its query, such as Postgres's `sslmode`:

```sh
docker run -p 8080:8080 -v blog-data:/data \
  -e APP_KEY=base64:... -e APP_URL=https://example.com \
  -e DB_URL='postgres://blog:...@db.example.com:5432/blog?sslmode=require' \
  -e MAIL_HOST=smtp.example.com -e MAIL_FROM_ADDRESS=hello@example.com blog
```

or `DB_HOST`, `DB_PORT`, `DB_DATABASE`, `DB_USERNAME` and `DB_PASSWORD`,
as Laravel has them. The drivers, pgx and go-sql-driver/mysql, are pure
Go too. The app brings the tables up to date as it starts, and
instances starting at once take turns ([Accounts](auth.md#the-database)).
With the photos in a bucket as well, `FILESYSTEM_DISK=s3`, an instance
keeps nothing of its own, and as many as the database takes can run
side by side, anywhere that reaches it: the jobs, the throttles' counts
of the tries at logging in, the cache and its locks, and the sessions, in
their cookies, are the same whichever instance a request reaches. The
starter is tested on Postgres 18 and MySQL 8.4.

The app's `compose.yaml` is for development: it runs the database on
`127.0.0.1`, with a password everyone knows. A deployed app's database is
a managed one's, or one run apart from the app.

With server-side rendering, the frontend stage's `npm run build` writes the
SSR build to `ssr/build` too, which the Go stage copies in, to embed, and
the image is `gcr.io/distroless/nodejs24-debian12`, which has Node, at
`/nodejs/bin/node`, as `SSR_NODE` says. The binary is still static; Node is
there for it to run. Running the image:

```sh
docker run -p 8080:8080 -v blog-data:/data \
  -e APP_KEY=base64:... -e APP_URL=https://example.com \
  -e MAIL_HOST=smtp.example.com -e MAIL_USERNAME=... -e MAIL_PASSWORD=... \
  -e MAIL_FROM_ADDRESS=hello@example.com blog
```

## Behind a proxy

`Run` serves plain HTTP. TLS ends in front of the app: at a proxy, a load
balancer, or the platform.

The session cookie is marked `Secure`, for HTTPS only, when the request
came over TLS, and a request from a proxy that ended TLS didn't. Set
`Secure` in the session's `Config` for an app served over HTTPS this way:

```go
sessions, err := session.New(session.Config{Keys: keys, Secure: true})
```

The starters set it from `APP_URL`, which starts with `https://` for an
app served over HTTPS.

`c.RedirectRoute`, a form sent back with its errors, and the 409 that
reloads a page for a new build all carry a path rather than a whole URL,
so the app needn't know the scheme or host the proxy answers on. A link
that leaves the app, as in the auth starter's mail, does need them, which
is what `APP_URL` is for: a link made from the request's `Host` could
point to any site the request names.

The proxy should pass on the `Host` the request came with. A form that
doesn't validate goes back to the page in its `Referer` only when that's
on the request's host, and otherwise to `/`. And `middleware.CSRF`
compares the `Origin` with the `Host`, for a browser that doesn't send
`Sec-Fetch-Site`.

Behind a proxy, a request comes from the proxy's address. The auth
starter counts failed logins by email and address, `c.IP()`, so there
every client would count as one: five wrong passwords for an email, from
anyone, would make everyone wait out the minute for it, and five asks for
a reset link a minute would be the whole site's. A limit on a route by
address, with `tug.Limit`, has the same trouble. So name the proxies in
`TRUSTED_PROXIES`, which the starters hand `middleware.TrustProxies`:

```sh
TRUSTED_PROXIES=10.0.0.0/8          # a load balancer in the app's network
TRUSTED_PROXIES=10.0.0.0/8,172.16.0.0/12
TRUSTED_PROXIES='*'                 # a platform whose proxy alone reaches the app
```

A request from one of them then comes from the client in the proxies'
`X-Forwarded-For`, read from its end, where each proxy adds the address it
saw, so what a client writes in the header itself changes nothing
([Routing](routing.md#the-clients-address) has how). `*` believes
whatever connects: it's for a platform whose proxy's addresses the app
can't name, where nothing else can reach the app, as anything else that
can would pick its own address. Behind a CDN and a load balancer, name
both: the CDN's ranges, which it publishes, and the load balancer's.

`middleware.RequestID` keeps an `X-Request-ID` that the proxy sets, when
it's short and plain, so the proxy's log and the app's share the ID.

The server `Run` starts has a `ReadHeaderTimeout` of 10 seconds, so a
client that sends its headers slowly can't hold a connection, and an
`IdleTimeout` of 2 minutes, after which a connection with no request is
closed. There's no timeout on reading a body or writing a response: a
slow upload or a long response is limited by the proxy's timeouts. Keep
the proxy's idle timeout, for its connections to the app, under the app's
two minutes, so that it never sends a request on a connection the app is
closing. A body that `Bind` reads is limited to `Config.BodyLimit`,
32 MiB, and the proxy may have a smaller limit of its own.

## Shutting down

On SIGTERM or SIGINT, `Run` logs "shutting down" and stops taking
connections. The requests in flight run to their end, for up to
`Config.ShutdownTimeout`, 10 seconds unless the app sets another. Then
`Run` returns nil and the starters' `main` returns. Requests still running
after the timeout have their connections closed, and `Run` returns an
error, "tug: requests still running after 10s", which the starters'
`main` passes to `log.Fatal`.

A platform gives the process some time between SIGTERM and killing it.
Docker's `docker stop` gives 10 seconds, the same as tug's default, so
choose a timeout under the platform's. In the plain starter's `main`:

```go
cfg := tug.ConfigFromEnv()
cfg.ShutdownTimeout = 8 * time.Second
app, err := newApp(cfg, build, keys)
```

The Dockerfile's `ENTRYPOINT` is the binary itself, with no shell in
between, so the signal reaches it. Work that a handler starts in a
goroutine of its own isn't a request, and `Run` doesn't wait for it; what
`app.Go` runs, it does. The auth starter's job queue, which sends its
mail, runs that way: as the app shuts down, it takes no more jobs, and
gives the ones running 10 seconds, its `Grace`, alongside the requests'
`ShutdownTimeout`. A job still running after that is put back, and runs at
the next start. So shutting down takes as long as the longer of the two,
and a moment more to put jobs back: keep it under the platform's grace
period. [jobs.md](jobs.md) has the rest. The Node of an app with
server-side rendering stops the same way, with the renders it has, and
five seconds to.

An app's [commands](routing.md#commands), as the auth starter's `jobs`,
which lists the jobs that failed for good and runs them again, run in
place of the server, on the same database, and exit. In a container,
one runs beside the server, with its environment: `docker exec
<container> /server jobs`. A command runs in the app as `main` makes it,
so it needs what the server needs, `APP_KEY` among it, which the
container has.

## Health checks

The auth starter answers `GET /up` with 200 and `up` while the app is
serving and its database answers a ping, and a 503 when it doesn't, for a
load balancer's or a platform's health check to ask. Like any request,
each check is a line in the request log. The plain starter has no such
route; a handler that answers 200 does.

## Rotating `APP_KEY`

Put the new key in `APP_KEY`, and the old one in `APP_PREVIOUS_KEYS`:

```sh
APP_KEY=base64:<the new key>
APP_PREVIOUS_KEYS=base64:<the old key>
```

The first key encrypts, and every key decrypts. Each response writes a
visitor's session back to its cookie, with the new key, so sessions move
over as people use the app. A session that goes unused for its lifetime,
2 hours unless `session.Config.Lifetime` says otherwise, has ended anyway,
so once that long has passed, drop the old key. In the auth starter, a
login that ticked "Remember me" lasts a month without a visit, so keep the
old key for a month there. Its reset links, which last an hour, its
verification links, a day, and the links to the photos on its own disk,
two days at most, are checked against every key too. Its
two-factor secrets are sealed with the key: as the app starts with more
than one key, it seals every user's again with the new one, so none of
them is lost when the old key goes.

A new `APP_KEY` without the old one in `APP_PREVIOUS_KEYS` logs everyone
out: no cookie decrypts, so every visitor starts a new session. That's
what to do when the old key has leaked: don't keep it.

## Logs

tug logs through `slog.Default()` and never sets it. Unless the app sets
it, that's a line of text on the standard error:

```
2026/09/25 16:44:09 INFO listening addr=[::]:8080
2026/09/25 16:44:10 INFO request method=GET path=/ status=200 size=806 duration=1.057625ms request_id=S53TAMBAGDIAG5OAIFBGPRG6KN
2026/09/25 16:44:11 WARN request method=GET path=/build/.vite/manifest.json status=404 size=19 duration=14.833µs request_id=5N5DHQOALQZIZMXK3HVCPXNPD3
2026/09/25 16:44:12 INFO shutting down timeout=10s
```

`middleware.Logger` logs the "request" lines: the method, the path
without its query string, which can carry tokens, the status, the size,
the time taken, and the request's ID. A 5xx is logged at Error, a 4xx at
Warn, and the rest at Info. A server error's details go to the log, in a
"request failed" line with the error and a panic's stack, and not to the
response while `APP_DEBUG` is off.

For JSON lines, as most log collectors want, set the default at the start
of `main`:

```go
func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	// ...
}
```

Set it before `Run`, which gives the `http.Server`'s own error log the
handler that's the default when it starts. From then on the `log`
package's output goes through the handler too, `log.Fatal`'s included.
`slog.HandlerOptions` sets the level: `Level: slog.LevelWarn` leaves out
the Info lines. [routing.md](routing.md#logging) lists what tug logs.
