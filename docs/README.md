# The tug guide

tug is a web framework for Go apps whose frontend is
[Inertia.js](https://inertiajs.com): Go handlers render React pages with
props, with no API in between, and the whole app ships as one binary. This
guide goes from a new app to a deployed one. Each package's own
documentation, on [pkg.go.dev](https://pkg.go.dev/github.com/cuonggt/tug),
has the rest of the detail.

1. [Getting started](getting-started.md): install tug, make an app, run
   it, and add a page and a form.
2. [Routing and handlers](routing.md): the App, routes, groups,
   middleware, `Ctx`, binding requests, and errors.
3. [Pages](pages.md): Inertia pages and their props, the props worked out
   later, shared props, redirects, error pages, and Vite.
4. [Forms and sessions](forms.md): validation, forms that check each field
   as it's left, flash messages, sessions, and CSRF.
5. [Accounts](auth.md): the auth starter, and packages `auth` and `mail`.
6. [Background jobs](jobs.md): package `queue`, for work that outlasts the
   request, and runs again when it fails, or runs on a schedule.
7. [TypeScript](typescript.md): the types `tug gen` writes from the Go.
8. [Deployment](deployment.md): one binary, the Dockerfile, and the
   environment.
9. [The CLI](cli.md): `tug new`, `tug dev`, `tug gen` and `tug build`, in
   full.

[The roadmap](roadmap.md) has how tug was built, the decisions behind it,
and what comes next.
