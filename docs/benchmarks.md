# Benchmarks

What tug costs a request, and what an app made with it takes to run,
measured two ways. Over HTTP, an app as `tug new` makes one serves an
Inertia page under load, and its memory, startup and size are measured.
In one process, Go's own benchmarks show what tug adds to a request over
ServeMux, Go's own router, and what rendering the page takes. The code is
in [bench/](../bench), a module of its own. The last run's numbers are in
[bench/results.json](../bench/results.json), which the tables here are
written from, and the website's home page shows.

## Why Go

Go makes a web app one static binary, which uses every core in one
process, starts in milliseconds, and runs in tens of MB of memory:

- **Performance:** tens of thousands of requests a second on a laptop, in
  one process that uses every core, in tens of MB of RAM.
- **Startup:** in milliseconds, which keeps the cost down on a platform
  that scales to zero.
- **Deploying:** one file to copy, with no runtime or dependencies to
  install, and one command on a Mac builds it for Linux or ARM.
- **Development:** a small language that compiles fast, with an HTTP
  server in its standard library that's good enough for production.

tug keeps each of them for an Inertia app. The app is one static binary,
its frontend's build inside it, served by net/http. `tug build` makes it
for the machine it runs on, and with its frontend built, `go build` makes
it again for Linux or ARM, as [Deployment](deployment.md#tug-build) shows.
The rest of this page measures what tug adds to Go.

## Over HTTP

### The page

The app serves one page, `GET /posts/{id}`: Inertia's `Posts/Show`, with
a post and its ten comments as its props. They're in
[bench/page/page.json](../bench/page/page.json), which the app has built
in, and it sets the post's ID from the route. Two requests for it are
measured:

- **A visit**, as Inertia's client makes one from another page of the
  app: `X-Inertia`, the build's version, `X-Requested-With`, and the
  cookies the first visit set, as a browser sends them. It's answered with
  the page object as JSON, about 2.3 KB.
- **A first visit**, as a browser makes one from a link: no cookies, and
  answered with the root template's HTML, the page object in it, with no
  scripts or styles.

There's no database: what's measured is what tug itself costs a request,
which is what it takes from the time an app has for its own work.

### The app

The app, in [bench/apps/tug](../bench/apps/tug), is as `tug new` makes
one, without a frontend: its session, and `TrustProxies`, `RequestID`,
`Logger`, `Recover`, `Headers`, `CSP`, which makes a nonce for each
response, and `CSRF`. It's built as `tug build` builds an app, static,
without its paths, and stripped, writes no log line per request, and
serves in one process, which uses every core.

### The load

`go run . http` starts the app and checks that its answer to each request
is the page, props and all, before any load. Then it sends each request
on 64 connections kept open, each sending the next as the answer to its
last comes in, as wrk does: for 10 seconds first, uncounted, then for 3
rounds of 10 seconds each. The table has the round with the median
requests a second, and its 99th percentile: the time from sending a
request to reading the end of its answer, which 99 in 100 took no longer
than.

<!-- bench:http -->

| | Visits a second | p99 | First visits a second | p99 |
|---|--:|--:|--:|--:|
| **tug** | 60,061 | 4.38 ms | 56,146 | 5.09 ms |

<!-- /bench:http -->

<!-- bench:http-setup -->

Measured 2026-10-05 on Apple M1 Max, 10 cores, 32 GB, macOS 27.0.1: 64 connections, 10 seconds of warmup, then 3 rounds of 10 seconds, the median round's, with these versions and server:

- **tug**: Go 1.26.1, tug v0.39.0-10-ge286336; net/http, one process.

<!-- /bench:http-setup -->

### Reading it

- **The load runs on the same machine** as the app, and they share its
  cores. Its client reads each answer only as far as its length says, so
  it takes as little of them as it can.
- **A visit carries the cookies the first visit set:** none, for a guest.
  tug's `CSRF` needs no token, and its session sets no cookie until
  there's something in it.
- **The 99th percentile follows from the rate:** with 64 requests always
  in flight, each waits its turn at the cores.
- **A real page does more**, such as a query or two. What the table shows
  is how much of each second tug leaves the app for its own work.

## Memory, startup and size

The same app, served as above, measured by `go run . footprint`:

- **Start:** from starting the app to its first answer to the page,
  asked for every 5 ms. A start is cold, with nothing kept from one
  before; the table has the median of 5.
- **Memory:** as macOS's `footprint` counts it, which is what Activity
  Monitor shows: the memory the process has written to. It's measured once
  the app has answered its first requests, then at the most while it
  answers visits on 64 connections for 10 seconds, and then 2 seconds
  after.
- **Size:** what a deploy copies: the app's one binary, which needs
  nothing installed beside it.

<!-- bench:footprint -->

| | Start | Memory, started | Memory, under load | Memory, after | Size |
|---|--:|--:|--:|--:|--:|
| **tug** | 15 ms | 6.4 MB | 19.9 MB | 19.3 MB | 9.6 MB |

<!-- /bench:footprint -->

<!-- bench:footprint-setup -->

Measured 2026-10-05 on Apple M1 Max, 10 cores, 32 GB, macOS 27.0.1, memory as macOS's footprint, the load 10 seconds of visits on 64 connections.

<!-- /bench:footprint-setup -->

## In one process

The Go benchmarks run with `go test`, each as many times as `-count` says,
and the tables have each one's median.

### What tug adds to a request

One route, `GET /posts/{id}`, answering the ID as text, through ServeMux
alone and through tug's App, as `New` makes it, with no middleware:

<!-- bench:router -->

| | ns a request | Bytes | Allocations |
|---|--:|--:|--:|
| ServeMux | 158 | 32 | 2 |
| **tug** | 233 | 176 | 3 |

<!-- /bench:router -->

tug's App is ServeMux underneath: its routes are ServeMux's patterns.
What it adds is its `Ctx`, the error a handler returns, and the recovery
of a handler's panic: well under a microsecond, a small share of what the
page itself takes, as the next table shows.

### Inertia

The page, through tug's App and through tug's `inertia` package on
ServeMux, which needs no App, each with no session and no middleware but
Inertia's own:

<!-- bench:inertia -->

| | A visit, µs | Bytes | Allocations | A first visit, µs | Bytes | Allocations |
|---|--:|--:|--:|--:|--:|--:|
| **tug, App** | 7.4 | 5,795 | 38 | 10.3 | 12,133 | 52 |
| **tug's inertia, ServeMux** | 7.0 | 5,281 | 35 | 9.6 | 11,618 | 49 |

<!-- /bench:inertia -->

<!-- bench:go-setup -->

Measured 2026-10-05 on Apple M1 Max, 10 cores, 32 GB, macOS 27.0.1, with Go 1.26.1: the median of 10 runs of each.

<!-- /bench:go-setup -->

## Running them

In `bench/`, with nothing else running on the machine:

```sh
go run . go                  # the Go benchmarks
go run . setup               # the app's build
go run . check               # the app started, and its page checked, with no load
go run . http                # the app under load
go run . footprint           # the app's memory, startup and size
```

They need Go alone. Each run writes `results.json`, and the tables here
and in the README again; `go run . docs` writes the tables alone.
`-conns`, `-warmup`, `-duration`, `-rounds`, `-starts` and `-count`
change the load and the runs. The Go benchmarks are `go test`'s own,
which run alone too, for benchstat to compare two runs of:

```sh
go test -run '^$' -bench . -benchmem -count 10 > new.txt
```

## What's not measured

- **A database:** the page reads none.
- **TLS:** the app is reached over plain HTTP, as an app behind a load
  balancer that ends TLS is.
- **Server-side rendering:** it would render in Node.
- **A logged-in user:** the visit is a guest's.
