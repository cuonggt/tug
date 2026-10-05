# Benchmarks

tug is measured two ways, each beside other frameworks. Over HTTP, an app
of each framework serves the same Inertia page under load: tug's, and
Laravel's, Rails', Django's and AdonisJS's. In one process, Go's own
benchmarks show what tug adds to a request beside ServeMux, Gin, Echo and
Chi, and its Inertia beside gonertia's. The code is in
[bench/](../bench), a module of its own, so tug's `go.mod` needs none of
theirs. The last run's numbers are in
[bench/results.json](../bench/results.json), which the tables here are
written from, and the website's home page shows.

## Over HTTP

### The page

Every app serves one page, `GET /posts/{id}`: Inertia's `Posts/Show`, with
a post and its ten comments as its props. They're in
[bench/page/page.json](../bench/page/page.json), which each app reads once,
as it starts, and sets the post's ID from the route. Two requests for it
are measured:

- **A visit**, as Inertia's client makes one from another page of the
  app: `X-Inertia`, the build's version, `X-Requested-With`, and the
  cookies the first visit set, as a browser sends them. It's answered with
  the page object as JSON, about 2.3 KB.
- **A first visit**, as a browser makes one from a link: no cookies, and
  answered with the root template's HTML, the page object in it. The
  template is the same in every app, with no scripts or styles, so it's
  each adapter's own markup and nothing more.

There's no database: a query takes the same time from any framework, and
what's measured is what the framework itself costs a request, which is
what it takes from the time an app has for its own work.

### The apps

Each app is made by its framework's own generator, at its latest
releases, pinned by its lock file, and keeps the middleware a new app
has, with its Inertia adapter's added. Each runs in production mode,
with its sessions in a cookie, so none needs a database or Redis, and
writes no log line per request. Each is served the way its framework's
docs say to serve one in production, on every core:

- **tug**: as `tug new` makes an app, with its session, `TrustProxies`,
  `RequestID`, `Logger`, `Recover`, `Headers`, `CSP`, which makes a nonce
  for each response, and `CSRF`, in one process, which uses every core.
- **Laravel**: Octane on Swoole, Laravel's own server for an app kept in
  memory between requests, with a worker a core, as Octane starts it, and
  its other defaults, a worker started again after 500 requests among
  them. OPcache is on, which takes two settings under Octane: PHP's
  command line leaves it off, and Octane's `clear_opcache`, on by
  default, would keep it from holding the app's code. The config, routes
  and views are cached, as `php artisan optimize` caches them for a deploy.
- **Rails**: Puma in cluster mode, with a worker a core, as Puma's docs
  recommend, three threads each, as Rails' `puma.rb` has, the app loaded
  before the workers start, and YJIT on, as Rails turns it on. The
  `allow_browser` line `rails new` writes is left out: it reads each
  request's User-Agent, for a frontend the page doesn't have.
- **Django**: gunicorn, with two workers a core, and one, as gunicorn's
  docs recommend. They're its default kind, sync, which close a connection
  after each answer, as gunicorn expects a proxy such as nginx in front of
  it, so each of Django's requests opens a connection.
- **AdonisJS**: its own server, a process a core, through Node's cluster
  module, as its docs for v6 recommend PM2's cluster mode for; v7's start
  one process, which would leave every core but one idle.

Every app's `setup.sh`, `start.sh` and settings are in
[bench/apps](../bench/apps).

### The load

`go run . http` starts each app in turn, alone, and checks that its answer
to each request is the page, props and all, before any load. Then it
sends each request on 64 connections kept open, each sending the next as
the answer to its last comes in, as wrk does: for 10 seconds first,
uncounted, for each runtime's JIT, caches and pools, then for 3 rounds of
10 seconds each. The table has the round with the median requests a
second, and its 99th percentile: the time from sending a request to
reading the end of its answer, which 99 in 100 took no longer than.

64 connections keep every app's workers busy. tug answers more still with
more of them, as its one process takes on as many as come, but Django's
gunicorn refuses connections past its listen queue, which macOS holds to
128. An app that closes each connection after its answer, as gunicorn's
workers do, gets the next request on a new one, and the load resets its
own end of the old one, so that its ports aren't held waiting out
TIME_WAIT, which on macOS would use them all up in seconds.

<!-- bench:http -->

| | Language | Visits a second | p99 | First visits a second | p99 |
|---|---|--:|--:|--:|--:|
| **tug** | Go | 62,919 | 5.22 ms | 57,715 | 5.92 ms |
| AdonisJS | JavaScript | 27,740 (tug 2.3×) | 16.3 ms | 28,781 (tug 2.0×) | 21.6 ms |
| Rails | Ruby | 10,752 (tug 5.9×) | 16.3 ms | 9,920 (tug 5.8×) | 14.3 ms |
| Django | Python | 7,567 (tug 8.3×) | 28.0 ms | 6,471 (tug 8.9×) | 41.2 ms |
| Laravel | PHP | 6,611 (tug 9.5×) | 31.4 ms | 6,267 (tug 9.2×) | 35.1 ms |

<!-- /bench:http -->

<!-- bench:http-setup -->

Measured 2026-10-05 on Apple M1 Max, 10 cores, 32 GB, macOS 27.0.1: 64 connections, 10 seconds of warmup, then 3 rounds of 10 seconds, the median round's. Each app's versions and server:

- **tug**: Go 1.26.1, tug v0.39.0-4-g3987f06; net/http, one process.
- **Laravel**: PHP 8.5.10, Laravel 13.34.0, inertia-laravel 3.5.1, Octane 2.20.0, Swoole 6.2.2; Octane on Swoole, a worker a core.
- **Rails**: Ruby 4.0.5 +YJIT, Rails 8.1.4, inertia_rails 3.22.0, Puma 8.0.2; Puma, a worker a core, 3 threads each.
- **Django**: Python 3.14.3, Django 6.1.1, inertia-django 2.0.0, gunicorn 26.2.0; gunicorn, two workers a core, and one.
- **AdonisJS**: Node 24.14.0, AdonisJS 7.5.2, @adonisjs/http-server 9.3.1, @adonisjs/inertia 5.0.1; Node's cluster, a process a core.

<!-- /bench:http-setup -->

### Reading it

- **The load runs on the same machine** as the app, and they share its
  cores. Its client reads each answer only as far as its length says, so
  it takes as little of them as it can, and the same from every app.
- **A visit carries cookies,** the ones the first visit set. Laravel's,
  Rails' and AdonisJS's sessions keep a CSRF token, so a first visit
  starts one, and each visit after it has its session cookie read and
  written again, encrypted, with the token's: from Laravel, about 1.7 KB
  of `Set-Cookie` an answer. Django sets its CSRF cookie again at each
  visit, with no session. tug's `CSRF` needs no token, and its session
  sets no cookie until there's something in it, so a guest's visits have
  none. That's each framework as it comes, and part of the difference.
- **The page objects differ a little:** each adapter adds fields of its
  own, such as `sharedProps` and the history's flags, Laravel's and
  AdonisJS's JSON escapes its slashes, and Django's has a space after
  each comma and colon. Each is within a tenth of the others' size.
- **The 99th percentile follows from the rate:** with 64 requests always
  in flight, the fewer a second an app answers, the longer each waits for
  a worker.
- **A real page does more**, such as a query or two, which take the same
  time from any framework. Where those dominate, the gap narrows; what
  the table shows is how much of each second the framework leaves the app.

## In one process

The Go benchmarks run with `go test`, each as many times as `-count` says,
and the tables have each one's median.

### What a router adds

One route, `GET /posts/{id}`, answering the ID as text, through ServeMux
alone and through each framework as its `New` makes it, with no
middleware:

<!-- bench:router -->

| | ns a request | Bytes | Allocations |
|---|--:|--:|--:|
| Gin | 93.2 | 48 | 1 |
| Echo | 117 | 16 | 1 |
| ServeMux | 158 | 32 | 2 |
| **tug** | 233 | 176 | 3 |
| Chi | 390 | 720 | 5 |

<!-- /bench:router -->

tug's App is ServeMux underneath: its routes are ServeMux's patterns.
What it adds is its `Ctx`, the error a handler returns, and the recovery
of a handler's panic, which the others leave to a middleware. Gin and Echo
have routers of their own, and keep their contexts in a pool. Each of
them costs a request well under a microsecond, a small share of what the
page itself takes, as the next table shows.

### Inertia

The page, through tug and through
[gonertia](https://github.com/romsar/gonertia), the other Go adapter of
Inertia's protocol, each with no session and no middleware but Inertia's
own: tug's App, tug's `inertia` package on ServeMux, which needs no App,
and gonertia on ServeMux.

<!-- bench:inertia -->

| | A visit, µs | Bytes | Allocations | A first visit, µs | Bytes | Allocations |
|---|--:|--:|--:|--:|--:|--:|
| **tug, App** | 7.4 | 5,795 | 38 | 10.3 | 12,133 | 52 |
| **tug's inertia, ServeMux** | 7.0 | 5,281 | 35 | 9.6 | 11,618 | 49 |
| gonertia, ServeMux | 10.1 | 8,030 | 45 | 12.3 | 11,537 | 62 |

<!-- /bench:inertia -->

<!-- bench:go-setup -->

Measured 2026-10-05 on Apple M1 Max, 10 cores, 32 GB, macOS 27.0.1, with Go 1.26.1: the median of 10 runs of each.

<!-- /bench:go-setup -->

## Running them

In `bench/`, with nothing else running on the machine:

```sh
go run . go                  # the Go benchmarks: needs Go alone
go run . setup               # each app's dependencies and build
go run . check               # each app started, and its page checked, with no load
go run . http                # each app under load, in turn
go run . http tug laravel    # only these, keeping the others' results
```

`setup` needs each language's runtime: PHP with Swoole and Composer, Ruby
and Bundler, Python 3, and Node. Each run writes `results.json`, and the
tables here and in the README again; `go run . docs` writes the tables
alone. `-conns`, `-warmup`, `-duration`, `-rounds` and `-count` change the
load and the runs. The Go benchmarks are `go test`'s own, which run alone
too, for benchstat to compare two runs of:

```sh
go test -run '^$' -bench . -benchmem -count 10 > new.txt
```

## What's not measured

- **A database:** the page reads none.
- **TLS:** every app is reached over plain HTTP, as an app behind a load
  balancer that ends TLS is.
- **Server-side rendering:** each would render in Node, the same Node.
- **A logged-in user:** each app's visit is a guest's.
