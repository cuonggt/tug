# Roadmap

tug is built in milestones, each ending in something that runs. It targets
the Inertia.js v3 protocol (v3.0.0, March 2026), written against
[the spec](https://inertiajs.com/the-protocol).

|    | Milestone             | Status |
|----|-----------------------|--------|
| M1 | HTTP core             | done   |
| M2 | Inertia core + Vite   | next   |
| M3 | Forms and validation  |        |
| M4 | The full v3 protocol  |        |
| M5 | CLI                   |        |
| M6 | v0.1.0                |        |

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

## M2 · Inertia core + Vite — next

- A first visit gets HTML with the page object in
  `<script type="application/json" data-page="app">`, with Go's HTML
  escaping left on so the data can't close the tag. A request with
  `X-Inertia` gets the page object as JSON, with `Vary: X-Inertia`.
- The asset version is a hash of the Vite manifest. A GET with a stale
  version gets a 409 with `X-Inertia-Location`.
- Partial reloads (`X-Inertia-Partial-Component`, `-Data`, `-Except`),
  shared props, and always, optional and deferred props, with prop closures
  resolved concurrently.
- 302 becomes 303 after PUT, PATCH and DELETE for plain handlers too
  (`Ctx.Redirect` already does it); redirects to other sites go through a
  409; `encryptHistory` and `clearHistory`.
- Vite: the dev server with hot reload and the React refresh preamble; the
  production manifest with CSS and preload tags; built files embedded in
  the binary.
- Typed pages: `tug.Page[PostsIndexProps]("Posts/Index")`.
- Nil slices go out as `[]` rather than `null`, so the generated TypeScript
  types hold.
- Done when the example React app navigates between pages, deferred props
  load after first paint, and a Playwright smoke test passes.

## M3 · Forms and validation

- Encrypted cookie sessions; flash messages through v3's `flash` field.
- Validation, go-playground/validator tags behind tug's own interface,
  fills the `errors` prop, with error bags, and redirects back. Precognition
  requests get 204 or 422. A `BindError` becomes a field error.
- The CSRF 403, and other refusals by middleware, answer through the
  ErrorHandler, so Inertia shows them as pages rather than plain text.
- Done when a page in the example app creates, edits and deletes with live
  validation and flash messages.

## M4 · The full v3 protocol

Merge, prepend and deep-merge props (with `matchPropsOn` and resets), once
props, infinite scroll with a pagination helper, rescued deferred props,
redirects that keep the URL fragment (`X-Inertia-Redirect`,
`preserveFragment`), `sharedProps`, and Inertia error pages. Done when tests
cover every header and page field in the spec.

## M5 · CLI

`tug new` creates a project; `tug dev` runs Vite, rebuilds and restarts Go
on changes, regenerates the TypeScript types and reloads the browser;
`tug build` makes the single binary and a distroless Dockerfile; `tug gen`
writes the TypeScript types and routes. Done when
`tug new blog && cd blog && tug dev` gives a running app in under a minute.

## M6 · v0.1.0

An auth starter (login, register, logout, password reset), docs, and the
first release.

After v0.1: SSR through a Node or Bun process beside the binary, Vue and
Svelte starters, background job queues.

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
- Go 1.25 at least, for `http.CrossOriginProtection`.
