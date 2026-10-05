# The tug guide

tug is a web framework for Go apps whose frontend is
[Inertia.js](https://inertiajs.com): Go handlers render React, Vue or
Svelte pages with props, with no API in between, and the whole app ships
as one binary. This guide goes from a new app to a deployed one. Each
package's own documentation, on
[pkg.go.dev](https://pkg.go.dev/github.com/cuonggt/tug), has the rest of
the detail.

1. [Getting started](getting-started.md): install tug, make an app, run
   it, and add a page and a form.
2. [Routing and handlers](routing.md): the App, routes, whole and signed
   links, groups, middleware, security headers among them, the route that
   answered, the client's address behind a proxy, `Ctx`, files, downloads,
   streams and events, binding requests, and errors.
3. [Pages](pages.md): Inertia pages and their props, the root template
   and its nonce, the props worked out later, shared props, redirects,
   downloads and events on a page, error pages, Vite, and Inertia's
   DevTools.
4. [Server-side rendering](ssr.md): pages rendered on the server for a
   first visit, by Node beside the app, and package `ssr`.
5. [Forms and sessions](forms.md): validation, rules of the app's own,
   forms that check each field as it's left, flash messages, sessions, and
   CSRF.
6. [Languages](languages.md): what tug and the app say, in the request's
   language, from a file for each, package `lang`, and `tug lang`.
7. [Files](files.md): uploads, checked by their size and what they are,
   kept on the app's disk or in S3 with package `storage`, and links to
   them, public or signed.
8. [Accounts](auth.md): the auth starter, in SQLite, Postgres or MySQL,
   its API tokens, admins and notifications, and packages `auth` and
   `mail`, with copies, files and a link to unsubscribe in one click.
9. [Migrations](migrations.md): the database's tables, made and changed
   by files of SQL, each run once, in order, as the app starts, and by its
   `migrate` command, with `tug migrate new` for the next, and package
   `migrate`.
10. [Authorization](authorization.md): what a user may do with a thing,
    package `auth`'s abilities, a no as a 403 that says why, and what a
    page's user may do in its props.
11. [Encryption](encryption.md): the app's key, what tug encrypts and
    signs with it, package `crypt`, for the app's own values, and rotating
    the key.
12. [Background jobs](jobs.md): package `queue`, for work that outlasts
    the request, and runs again when it fails, runs on a schedule, in a
    time zone too, waits once however often it's asked for, or so many at
    once or a second, on every instance, and says when it has failed for
    good, and how each run went.
13. [Cache](cache.md): package `cache`, for what's slow to work out, kept
    where every instance of the app finds it, and locks for what mustn't
    run twice at once.
14. [Broadcasting](broadcasting.md): package `broadcast`, events on
    channels, carried by the app's database to the pages open on every
    instance, which reload what changed.
15. [TypeScript](typescript.md): the types `tug gen` writes from the Go.
16. [Testing](testing.md): package `tugtest`, Inertia's client for Go's
    tests of an app's pages, forms, uploads and logins, `mailtest`, for
    its mail, and its events.
17. [Deployment](deployment.md): one binary, the Dockerfile, the
    environment, the headers that say what a browser may do with the app's
    pages, a Content-Security-Policy among them, the logs, and metrics.
18. [The CLI](cli.md): `tug new`, `tug dev`, `tug gen`, `tug lang`,
    `tug migrate`, `tug build` and `tug key`, in full.

[The roadmap](roadmap.md) has how tug was built, the decisions behind it,
and what comes next, and [the benchmarks](benchmarks.md) how fast it
serves a page beside other frameworks, and how that was measured.
