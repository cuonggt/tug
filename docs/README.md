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
   files, downloads, streams and events, binding requests, and errors.
3. [Pages](pages.md): Inertia pages and their props, the props worked out
   later, shared props, redirects, downloads and events on a page, error
   pages, and Vite.
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
   and packages `auth` and `mail`, with copies, files and a link to
   unsubscribe in one click.
9. [Background jobs](jobs.md): package `queue`, for work that outlasts the
   request, and runs again when it fails, runs on a schedule, in a time
   zone too, or waits once however often it's asked for.
10. [Cache](cache.md): package `cache`, for what's slow to work out, kept
    where every instance of the app finds it, and locks for what mustn't
    run twice at once.
11. [TypeScript](typescript.md): the types `tug gen` writes from the Go.
12. [Testing](testing.md): package `tugtest`, Inertia's client for Go's
    tests of an app's pages, forms, uploads and logins, and `mailtest`,
    for its mail.
13. [Deployment](deployment.md): one binary, the Dockerfile, and the
    environment.
14. [The CLI](cli.md): `tug new`, `tug dev`, `tug gen`, `tug lang` and
    `tug build`, in full.

[The roadmap](roadmap.md) has how tug was built, the decisions behind it,
and what comes next.
