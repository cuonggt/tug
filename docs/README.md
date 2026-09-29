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
   links, groups, middleware, the client's address behind a proxy, `Ctx`,
   binding requests, and errors.
3. [Pages](pages.md): Inertia pages and their props, the props worked out
   later, shared props, redirects, error pages, and Vite.
4. [Server-side rendering](ssr.md): pages rendered on the server for a
   first visit, by Node beside the app, and package `ssr`.
5. [Forms and sessions](forms.md): validation, forms that check each field
   as it's left, flash messages, sessions, and CSRF.
6. [Files](files.md): uploads, checked by their size and what they are,
   kept on the app's disk or in S3 with package `storage`, and links to
   them, public or signed.
7. [Accounts](auth.md): the auth starter, in SQLite, Postgres or MySQL,
   and packages `auth` and `mail`.
8. [Background jobs](jobs.md): package `queue`, for work that outlasts the
   request, and runs again when it fails, runs on a schedule, in a time
   zone too, or waits once however often it's asked for.
9. [TypeScript](typescript.md): the types `tug gen` writes from the Go.
10. [Testing](testing.md): package `tugtest`, Inertia's client for Go's
    tests of an app's pages, forms, uploads and logins.
11. [Deployment](deployment.md): one binary, the Dockerfile, and the
    environment.
12. [The CLI](cli.md): `tug new`, `tug dev`, `tug gen` and `tug build`, in
    full.

[The roadmap](roadmap.md) has how tug was built, the decisions behind it,
and what comes next.
