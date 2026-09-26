# Server-side rendering

Without it, a first visit gets the page object and an empty
`<div id="app">`, and the page shows once the browser has loaded the
scripts and rendered it. With server-side rendering, the page's HTML is in
the response already: search engines and link previews read it, and the
page shows before its scripts run. The browser then hydrates it, as React
calls taking over HTML it rendered elsewhere, and from there the app is as
it always is.

Inertia renders pages on the server with the same React components, in
JavaScript, so it takes Node: tug runs it beside the app, as a process of
its own. It's optional, and a page that isn't rendered on the server, as
when Node isn't there, renders in the browser as it would without it.

## Turning it on

```sh
tug new -ssr blog          # or: tug new -auth -ssr blog
```

An app made with `-ssr` has, beyond the rest:

- `resources/js/ssr.tsx`: the app on the server, Inertia's SSR server.
  Like `app.tsx`, the app in the browser, it makes the app with
  `createApp` from `resources/js/inertia.tsx`, which every app has.
- `@inertiajs/vite` in `vite.config.ts`, which renders pages through the
  dev server, and builds `ssr.tsx` with `vite build --ssr`: `npm run build`
  runs both builds.
- In `main.go`, the SSR build embedded from `ssr/build`, a
  `ssr.Gateway` as `inertia.Config.SSR`, and `app.Go(server.Run)`, which
  runs Node.
- `{{ .InertiaHead }}` in `app.html`, and a `Dockerfile` whose image has
  Node.

```tsx
// resources/js/ssr.tsx
const render = await createApp()
...
createServer((page) => render(page as Parameters<typeof render>[0], renderToString), {
  host: '127.0.0.1',
  port: Number(process.env.SSR_PORT) || 13714,
})
```

```go
// main.go
server := &ssr.Server{Bundle: bundle, Node: os.Getenv("SSR_NODE")}
pages, err := inertia.New(inertia.Config{
	...
	SSR: &ssr.Gateway{DevServer: assets.DevServer, Server: server, URL: os.Getenv("SSR_URL")},
})
...
if os.Getenv("TUG_DEV") == "" && os.Getenv("SSR_URL") == "" {
	app.Go(server.Run)
}
```

## How a page renders

A first visit renders the page object as always, and hands it, as JSON, to
`Config.SSR`, whose answer is the page's head and body. The body, the page
object and the `<div id="app" data-server-rendered="true">` with the
page's HTML, goes where `{{ .Inertia }}` is, and the head, the page's
`<title>` and the tags its `<Head>` has, where `{{ .InertiaHead }}` is. A
visit from Inertia's client, as after a link is clicked, gets JSON, as
without SSR: only first visits render on the server.

The gateway renders a page:

- through the Vite dev server, while it runs, as under `tug dev`:
  `@inertiajs/vite` answers at `/__inertia_ssr`, from the sources, so a
  page follows them as they change;
- otherwise through Node, which `ssr.Server` runs beside the app from the
  SSR build;
- or through `SSR_URL`, an SSR server run apart from the app, as
  `node ssr/build/ssr.mjs` with its own `SSR_PORT`, in which case the app
  runs no Node of its own.

A page renders in the browser instead, as it would without SSR, when
there's nothing to render it: no build yet, no Node, or a Node that's
starting again. That's said once, as the app starts, not for each page. A
page that fails on the server, or takes longer than two seconds, renders
in the browser too, and the failure is logged, with Inertia's hint:

```
2026/09/26 16:37:20 WARN inertia: a page wasn't rendered on the server, so it renders in the browser component=Dashboard err="ssr: window is not defined, at resources/js/pages/Dashboard.tsx:12:3. The global window object doesn't exist in Node.js. Wrap browser-specific code in a onMounted/useEffect/onMount lifecycle hook, or check \"typeof window !== 'undefined'\" before using it."
```

`inertia.WithoutSSR(ctx)` renders a request's page in the browser, for
pages that gain nothing from SSR: those behind a login, say, which no
search engine sees.

## Node beside the app

`ssr.Server.Run` writes the SSR build to a directory of its own, and runs
Node on it, with `SSR_PORT` set to a free port on 127.0.0.1. Once its
server answers `/health`, pages render there. If Node stops, it starts
again, after a second, then two, and so on to a minute while it keeps
stopping. As the app shuts down, `Run` stops Node, and the directory goes.
On Linux, Node stops even when the app is killed, and can't stop it.

- `SSR_NODE` is the Node to run, such as `/nodejs/bin/node`. Default the
  `node` on `PATH`.
- `SSR_URL` is an SSR server run apart, such as `http://127.0.0.1:13714`:
  the app sends pages there, and runs none of its own.
- `TUG_DEV`, which `tug dev` sets, leaves Node out: the dev server renders.

Without a build, as before the first `tug build`, or without Node, `Run`
says why, and returns: the app serves its pages all the same.

## Writing pages that render on the server

A component renders first in Node, which has no browser:

- **No `window`, `document` or `localStorage` while rendering**, or at the
  top of a module the app imports: they belong in effects and event
  handlers, which the server doesn't run. The auth starter's `app.tsx`
  keeps its browser-only code, the toasts for flash messages and the
  appearance following the system's, out of `inertia.tsx`, which the
  server imports.
- **The same on both sides.** The browser's first render has to match the
  server's HTML, or React throws it away and renders the page again, with
  error #418 in the console. A date formatted in the browser's own language
  or zone differs from the server's, and so does anything random, or read
  from the browser. The auth starter's dashboard formats its date in
  `en-US` and UTC, and its appearance hook reads `localStorage` through
  `useSyncExternalStore`, whose server snapshot is `'system'`, so the page
  hydrates as the server rendered it and then shows the browser's choice.
- **The head.** A page's `<Head>` tags come in `{{ .InertiaHead }}`, its
  `<title>` first. The root template's own `<title>` comes after, for a
  page without one.

## Development

Under `tug dev`, the Vite dev server renders pages, from the sources, with
the stylesheets' links in the head, so a page doesn't show unstyled while
its scripts load. A page that fails to render says so in Vite's output,
with a hint, and renders in the browser. `tug dev` gives the app
`TUG_DEV=1`, so it starts no Node of its own.

## Building and deploying

`npm run build`, which `tug build` runs, is `vite build && vite build
--ssr`: the second builds `ssr.tsx` into `ssr/build`, with its packages in
it, so it runs without `node_modules`, and without source maps, which would
make the binary megabytes bigger. The binary embeds it, as it embeds
`public/build`.

The binary is static, as without SSR, but Node has to be where it runs.
The `Dockerfile` of an app with SSR is on
`gcr.io/distroless/nodejs24-debian12`, with `SSR_NODE=/nodejs/bin/node`:

```dockerfile
FROM gcr.io/distroless/nodejs24-debian12
ENV SSR_NODE=/nodejs/bin/node
COPY --from=server /server /server
...
ENTRYPOINT ["/server"]
```

A `docker stop` stops the app, which stops Node. Node takes about 45 MB of
memory beside the app's own, rendering the auth starter's pages.

## Package ssr

```go
type Gateway struct {
	DevServer func() string // vite.Vite's DevServer: the dev server, while it runs
	URL       string        // an SSR server run apart
	Server    *Server       // the SSR server the app runs itself
	Timeout   time.Duration // for a page to render; default 2 seconds
}

type Server struct {
	Bundle fs.FS  // what `vite build --ssr` built
	Entry  string // the file in it that starts the server; default "ssr.mjs"
	Node   string // default "node", on PATH
}
```

`Gateway` is an `inertia.Renderer`, which is all `inertia.Config.SSR`
takes:

```go
type Renderer interface {
	Render(ctx context.Context, page []byte) (inertia.Rendered, error)
}
```

A `Rendered` with no body and no error is a page it didn't render, which
renders in the browser without a word; an error is logged, and the page
renders in the browser too. Package `inertia` has no idea it's Node.

## What's not here yet

- **Vue and Svelte**: the starters are React's, though Inertia's SSR, and
  `@inertiajs/vite`, render Vue and Svelte too.
- **Streaming**: Inertia renders a page to a string, and sends it whole.
- **More than one Node**: Inertia's SSR server can run a process per CPU,
  its cluster mode, where tug runs one.
