# Roadmap

tug is built in milestones, each ending in something that runs. It targets
the Inertia.js v3 protocol (v3.0.0, March 2026), written against
[the spec](https://inertiajs.com/the-protocol).

|    | Milestone             | Status |
|----|-----------------------|--------|
| M1 | HTTP core             | done   |
| M2 | Inertia core + Vite   | done   |
| M3 | Forms and validation  | next   |
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

## M2 · Inertia core + Vite — done

- Package `inertia`, usable with any router: `Render`, `Middleware`,
  `Location`. A first visit gets HTML with the page object in
  `<script data-page="app" type="application/json">`; a visit from the
  client gets it as JSON. `Vary: X-Inertia` goes on every response.
- A GET from another build gets a 409 with `X-Inertia-Location` and
  `X-Inertia-Version`, before the handler runs.
- Partial reloads, shared props (`Share`, `ShareFunc`, and `WithProps` for
  middleware, listed in `sharedProps`), and `Lazy`, `Optional`, `Always`
  and `Defer` props. The props that go out in one response are worked out
  concurrently, and a panic in one becomes an error.
- A 302 after PUT, PATCH or DELETE becomes a 303; `encryptHistory` and
  `clearHistory`, sent only when true.
- Nil slices and maps, at any depth, go out as `[]` and `{}`.
- Package `vite`: tags from the dev server (with the React refresh preamble)
  while `public/hot` exists, and from the manifest otherwise; the version is
  a hash of the manifest; `ServeHTTP` serves the build.
- `tug.Page[P]`, `Ctx.Inertia`, `Ctx.Location`; `Config.Inertia` puts the
  middleware around every route.
- `examples/inertia`: React pages, a deferred prop, a partial reload, a
  DELETE link, and Playwright tests in Chrome, which CI runs.

Choices made on the way:

- `X-Inertia-Location` is relative, where the spec's example is absolute:
  behind a proxy that ends TLS, an absolute URL would have to guess the
  scheme.
- The dev server announces itself in a file, as laravel-vite-plugin does,
  so `npm run dev` switches the Go server to it without a restart. The
  example's `vite.config.ts` has the ten-line plugin that writes it.
- The example keeps Laravel's layout: the build in `public/build`, served
  under `/build/`, and embedded with the rest of `public/`.
- The root template loads the page's own component chunk with the app's, so
  a first visit doesn't wait for one before fetching the other.
- The example resolves pages with `import.meta.glob` rather than
  `@inertiajs/vite`, which adds little without SSR. SSR will bring it in:
  its dev server answers at `/__inertia_ssr`.

## M3 · Forms and validation — next

- Encrypted cookie sessions; flash messages through v3's `flash` field, and
  `clearHistory` carried across a redirect.
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
`preserveFragment`), prop types nested inside other props, dot paths in
partial reloads, and Inertia error pages. Done when tests cover every header
and page field in the spec.

## M5 · CLI

`tug new` creates a project; `tug dev` runs Vite, rebuilds and restarts Go
on changes, regenerates the TypeScript types and reloads the browser;
`tug build` makes the single binary and a distroless Dockerfile; `tug gen`
writes the TypeScript types and routes, which `examples/inertia` writes by
hand in `resources/js/types.ts` until then. Done when
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
