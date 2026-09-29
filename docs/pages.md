# Pages

A page is a React component, or a Vue or Svelte one, that a Go handler
renders with props; this guide's are React's. A first visit gets HTML with
the page in it; after that, Inertia's client makes each visit itself and
gets the next page as JSON. Package `inertia` is the server side of that
protocol, Inertia v3, and package `vite` loads the frontend.
[`examples/inertia`](../examples/inertia/main.go) uses most of what's
here.

## Setting up

An app made by `tug new` sets its pages up in `main.go`:

```go
//go:embed app.html
var rootTemplate string

func newApp(cfg tug.Config, build fs.FS, keys [][]byte) (*tug.App, error) {
	assets, err := vite.New(vite.Config{Build: build, HotFile: "public/hot"})
	if err != nil {
		return nil, err
	}
	pages, err := inertia.New(inertia.Config{
		Template: rootTemplate,
		Funcs:    assets.Funcs(),
		Version:  assets.Version(),
	})
	if err != nil {
		return nil, err
	}
	pages.Share("appName", "blog")
	sessions, err := session.New(session.Config{Keys: keys})
	if err != nil {
		return nil, err
	}

	cfg.Inertia, cfg.Session, cfg.ErrorPage = pages, sessions, "Error"
	app := tug.New(cfg)
	app.Get("/build/{path...}", tug.WrapHandler(assets))
	app.Get("/", home).Name("home")
	return app, nil
}
```

`assets` loads the frontend, from `build` or the dev server: see
[Vite](#vite). `inertia.Config` has five fields:

- `Template` is the root template, in html/template syntax. `New` fails
  when it's empty or doesn't parse.
- `Funcs` are its functions, such as `vite` and `viteReactRefresh`.
- `Version` names the frontend build ([A new build](#a-new-build)).
- `EncryptHistory` is under [History encryption](#history-encryption).
- `SSR` renders first visits on the server, when it's set:
  [ssr.md](ssr.md).

With `Config.Inertia` set, `c.Inertia` and `Page.Render` render with it,
and every request goes through its middleware, inside the App's own.
`Config.Session` carries flash data and validation errors to the page after
a redirect ([forms.md](forms.md)). The root template, `app.html`:

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <title data-inertia>blog</title>
    {{ viteReactRefresh }}
    {{ vite "resources/js/app.tsx" (printf "resources/js/pages/%s.tsx" .Page.Component) }}
  </head>
  <body>
    {{ .Inertia }}
  </body>
</html>
```

`{{ .Inertia }}` goes in the body: it's the page object, in
`<script data-page="app" type="application/json">`, and the
`<div id="app">` the app mounts in. encoding/json escapes `<`, `>` and `&`,
so no prop can close the script element early. `.Page` is the
`*inertia.Page` being rendered; here `vite` loads its component's script
with the app's, so a first visit fetches both at once. With server-side
rendering, the `<div id="app">` has the page's HTML in it, and
`{{ .InertiaHead }}`, in the head, has the tags of its `<Head>`, its
`<title>` first: [ssr.md](ssr.md).

## Declaring and rendering pages

```go
type PostsIndexProps struct {
	Posts []Post `json:"posts"`
}

var PostsIndex = tug.Page[PostsIndexProps]("Posts/Index")

func index(c *tug.Ctx) error {
	return PostsIndex.Render(c, PostsIndexProps{Posts: store.List()})
}
```

`tug.Page[P]` declares a page component and the props it takes, and
returns a `tug.PageOf[P]`, whose `Render` takes those props and no others.
tug gen writes their TypeScript, so the component, which takes
`PageProps<'Posts/Index'>`, is checked against the same props
([typescript.md](typescript.md)). Declare pages as package-level variables:
tug gen reads them when `app.Run` starts. A component declared twice with
different props panics. A page without a props type goes through
`c.Inertia("About", tug.Props{"version": v})`, with a struct or a map with
string keys, and tug gen doesn't know its props.

A component's name is its file's path under `resources/js/pages`, without
`.tsx` (`.vue`, `.svelte`): `"Posts/Index"` is
`resources/js/pages/Posts/Index.tsx`, for `resolve` in the starter's
`resources/js/inertia.tsx` and for the root template. A name with no file
is an error that names the file.

A first visit, a browser loading the page whole, gets the root template's
HTML with the page object in it:

```json
{"component":"Posts/Index","props":{"errors":{},"posts":[{"id":1,"title":"Hello, tug"}]},"url":"/posts","version":"c4b1e0…"}
```

Later visits come from Inertia's client, with `X-Inertia: true`, and get
the page object alone, as JSON. `url` is the request's path and query, and
`errors` is always there ([forms.md](forms.md)). The middleware adds
`Vary: X-Inertia` to every response, so a cache never answers a request
for a page's HTML with its JSON.

## Props

Props are a struct or a map with string keys, and go out as encoding/json
writes them: a struct's fields by their json tags, with `omitempty` and
`json:"-"`. The one difference is that a nil slice or map, at any depth,
goes out as `[]` or `{}`, never `null`: the page's TypeScript says `Post[]`,
and `posts.map()` on a null is a crash in the browser. The props passed in
aren't changed.

The prop types below go anywhere in the props: in maps whose values can
hold them, such as `tug.Props`, and in structs whose type has one in it, at
any depth. The client knows each by its path, as `stats.visits`:

```go
type DashboardProps struct {
	User  User `json:"user"`
	Stats struct {
		Visits inertia.DeferProp[int]  `json:"visits"`
		Sales  inertia.DeferProp[int]  `json:"sales"`
		Total  inertia.AlwaysProp[int] `json:"total"`
	} `json:"stats"`
}
```

A struct whose type has no prop type in it, such as `User`, is data: it
goes out whole, as encoding/json writes it, so its own `MarshalJSON` keeps
working. So is a list: a prop type in a slice isn't worked out.

## Props worked out later

```go
func show(c *tug.Ctx) error {
	post, err := store.Find(c)
	if err != nil {
		return err
	}
	return c.Inertia("Posts/Show", tug.Props{
		"post":     post,
		"comments": inertia.Lazy(func() ([]Comment, error) { return store.Comments(post.ID) }),
		"edits":    inertia.Optional(func() ([]Edit, error) { return store.Edits(post.ID) }),
		"readers":  inertia.Always(store.Readers(post.ID)),
		"stats":    inertia.Defer(func() (Stats, error) { return store.Stats(post.ID) }),
	})
}
```

`Lazy`, `Optional` and `Defer` take a function that returns the value and
an error, and call it only when the prop goes out; `Always` takes the
value. A full visit is a first visit, or a client's visit that isn't a
[partial reload](#partial-reloads):

| Prop           | A full visit                   | A partial reload asking for it | One that doesn't         |
|----------------|--------------------------------|--------------------------------|--------------------------|
| a value        | sent                           | sent                           | left out                 |
| `Lazy(fn)`     | sent                           | sent                           | left out, not worked out |
| `Optional(fn)` | left out, not worked out       | sent                           | left out, not worked out |
| `Always(v)`    | sent                           | sent                           | sent                     |
| `Defer(fn)`    | left out, named for the client | sent                           | left out, not worked out |

A deferred prop lets the page show without waiting for it. The page object
names it in `deferredProps` under its group, `"default"` unless `Defer` is
given one, as in `inertia.Defer(fn, "sidebar")`, and the client fetches
each group with a partial reload of its own, while
`<Deferred data="stats" fallback={...}>` shows the fallback. tug gen types
deferred and optional props as optional: `stats?: Stats`.

Props side by side, at the top or in one map or struct, are worked out
concurrently, since each is often a query of its own; functions that share
state need to guard it. A function that returns an error or panics fails
the render, as `inertia: prop "stats": database down`, and the handler
returns that for a 500: a panic becomes an error rather than take the
program down. `.Rescue()` on a deferred prop keeps its failure to itself:
the prop is left out and named in `rescuedProps`, `<Deferred>` shows its
`rescue`, and the error is logged through `slog.Default()`.

## Merging and scrolling

```go
func feed(c *tug.Ctx) error {
	return c.Inertia("Feed", tug.Props{
		"posts":    inertia.Merge(store.PostsAfter(c.Query("after")), inertia.MatchOn("id")),
		"activity": inertia.Lazy(store.Activity).Merge(inertia.Prepend()),
		"stats":    inertia.Defer(store.FeedStats).Merge(inertia.DeepMerge()),
	})
}
```

A merge prop is one a partial reload adds to what the client has, rather
than replace: a list is appended to, and an object gets the new keys, as
when a "More" button fetches the next posts. A full visit replaces it all
the same. `Lazy(fn).Merge(...)` and `Defer(fn).Merge(...)` make merge props
of those. The options are functions rather than methods, so each prop type
takes the ones that apply to it:

- `Prepend()` puts the new items before the ones the client has.
- `DeepMerge()` merges objects at every depth, and the lists in them.
- `MatchOn("id")` names the key that makes two items the same, so a new
  copy replaces the old one where it is; `"data.id"` for items at `data`.
- `AppendAt("data")` merges at a path inside the prop, such as a paginated
  list's `data`; `PrependAt` puts the new items first.

A prop the client resets, as `router.reload({ reset: ['posts'] })` does,
goes out without its merge label, so the client replaces what it has.

`Scroll` is a page of a list for Inertia's `<InfiniteScroll>`, which asks
for the pages after it, or before it, as the list scrolls. As
`examples/inertia` pages its posts:

```go
type PostsIndexProps struct {
	Posts inertia.ScrollProp[Post] `json:"posts"`
}

func index(c *tug.Ctx) error {
	return PostsIndex.Render(c, PostsIndexProps{
		Posts: inertia.Scroll(func() ([]Post, inertia.Paging, error) {
			page, err := tug.Paginate(c, 10, store.CountPosts, store.Posts)
			return page.Data, page.Paging(), err
		}).MatchOn("id"),
	})
}
```

The function returns the page the client asks for, in the `page` query
parameter, and where it sits: [`tug.Paginate`](#pagination) works out
both, or `inertia.PageNumbers(page, more)` says where a page of numbers
sits, and an `inertia.Paging` of cursors, nil where there's none, with
`PageName` for another query parameter. The prop goes out as
`{"data": [...]}`, with its paging in `scrollProps`, for
`<InfiniteScroll data="posts">`, and merges at `data`: appended for a page
after, and prepended for a page before. `.MatchOn("id")` keeps an item two
pages both have from showing twice, `.Defer()` leaves the list out of the
first load, and a reset starts it again.

## Pagination

```go
type PostsArchiveProps struct {
	Posts tug.Paginated[Post] `json:"posts"`
}

func archive(c *tug.Ctx) error {
	posts, err := tug.Paginate(c, 20, store.CountPosts, store.Posts)
	if err != nil {
		return err
	}
	return PostsArchive.Render(c, PostsArchiveProps{Posts: posts})
}

// The app's own queries: tug has no SQL.
func (s *Store) CountPosts() (n int, err error) {
	err = s.db.QueryRow("SELECT count(*) FROM posts").Scan(&n)
	return n, err
}

func (s *Store) Posts(limit, offset int) ([]Post, error) {
	rows, err := s.db.Query("SELECT id, title FROM posts ORDER BY id LIMIT ? OFFSET ?", limit, offset)
	// ... and the rows, scanned into posts
}
```

`tug.Paginate(c, perPage, count, fetch)` is the page of a list that
`?page` asks for: `count` says how many items there are, and `fetch` gets
a page's, by the limit and offset it's given, as SQL's `LIMIT` and
`OFFSET` take them. A `tug.Paginated[T]` is the page, with where it sits,
under the keys Laravel's paginators write, so a pager written for theirs
reads it:

| Key | What it is |
|-----|------------|
| `data` | The page's items, `[]` for none. |
| `current_page`, `last_page` | The page's number, and the last's: 1 for a list with none. |
| `per_page`, `total` | How many a page has, and how many the list has. |
| `from`, `to` | The first and last items' places in the list, from 1, or `null` for a page with none. |
| `first_page_url`, `last_page_url`, `prev_page_url`, `next_page_url` | Links to those pages, the last two `null` where there's none. |
| `path` | The list's path, without the query. |
| `links` | The pager's links: `{url, label, active}` for each page around this one, and `"..."`, with no URL, for those left out. |

The links are paths, with the query the list was shown with, and the
page's number changed alone, so that its filters and order go with it:
`/posts?page=3&status=draft`. `links` has every page when there are under
14, and otherwise the first two and the last two, and three on each side
of this one, as Laravel's has; unlike Laravel's, it has no "Previous" and
"Next" in it, as HTML entities a page would have to show as HTML:
`prev_page_url` and `next_page_url` are those. A pager in React, as
`examples/inertia`'s:

```tsx
<nav aria-label="Pages">
  {posts.prev_page_url && <Link href={posts.prev_page_url}>Previous</Link>}
  {posts.links.map((link, i) =>
    link.url === null ? (
      <span key={i}>{link.label}</span>
    ) : (
      <Link key={i} href={link.url} aria-current={link.active ? 'page' : undefined}>
        {link.label}
      </Link>
    ),
  )}
  {posts.next_page_url && <Link href={posts.next_page_url}>Next</Link>}
</nav>
```

A page that isn't a number, or is under 1, is the first, as Laravel has
it. A page past the last is empty, with `last_page` for the pager, rather
than a 404, as a list can shrink between two visits; `count` runs first,
so `fetch` isn't asked for a page that has no items, however far past the
last it is. The page size is the app's alone: a client that could pick it
would pick how much the database reads.

Two more, for lists that a count doesn't suit:

- `tug.SimplePaginate(c, perPage, fetch)` is a `tug.SimplePaginated[T]`,
  for a list too long to count at every visit: `fetch` is asked for one
  more than a page, to know whether there's a next, and the page has no
  `last_page`, `total` or `links`, but `current_page_url`.
- `tug.CursorPaginate(c, perPage, fetch, cursor)` is a
  `tug.CursorPaginated[T]`, for a list whose items come and go as it's
  read, as a feed's: `cursor` is where an item sits in the query's order,
  a value of the app's, such as its time and ID, and `fetch` gets the
  items after the last one's, `nil` for the first page. The next page's
  link carries it, as JSON in base64url, under `?cursor`, with `data`,
  `next_cursor` and `next_page_url`. It isn't signed, as Laravel's
  aren't: it's only where to start, and a made-up one starts somewhere
  else within what the query lets the page see; one that doesn't decode
  is the first page. Cursors go forward: a page before one runs the query
  the other way, which is the app's to write.

```go
type PostCursor struct {
	At time.Time `json:"at"`
	ID int64     `json:"id"`
}

feed, err := tug.CursorPaginate(c, 20,
	func(after *PostCursor, limit int) ([]Post, error) { return store.PostsAfter(after, limit) },
	func(p Post) PostCursor { return PostCursor{At: p.CreatedAt, ID: p.ID} })
```

Each page's `Paging()` is where it sits for `inertia.Scroll`, as above, so
one query feeds numbered pages and `<InfiniteScroll>` alike.
`tug.PageName("comments_page")` reads and links another query parameter,
for a second list on one page, and leaves the first's as it is. tug gen
writes each kind as it writes any generic struct, as `Paginated_Post`, so
a page without a count has no `total` in its TypeScript.

## Once props

```go
func checkout(c *tug.Ctx) error {
	return c.Inertia("Checkout", tug.Props{
		"plans":     inertia.Once(store.Plans),
		"countries": inertia.Once(store.Countries, inertia.Until(time.Now().Add(24*time.Hour))),
	})
}
```

A once prop is one the client keeps once it has it, across pages, such as
a list of countries. The client names the ones it has with each visit, and
the page leaves those out, without working them out. One is sent again
when it expires, at the time `Until` gives; when `Fresh(true)` forces it,
as after the data behind it has changed; and when a partial reload asks
for it. `As("countries")` names what the client keeps it as, when that
isn't its path, so pages whose props differ in name can share one copy.
`.Once(...)` takes the same options on `Defer`, `Optional` and `Merge`
props: a deferred once prop the client has isn't fetched again.

## Partial reloads

A partial reload asks for some of a page's props again, as
`router.reload({ only: ['stats'] })` does. It names the page's component,
and the props it wants, from `only`, or doesn't, from `except`; one that
names another component gets the whole page. Paths reach into nested
props, both ways. With `DashboardProps` from [Props](#props):

- `only: ['stats']` gets all of `stats`.
- `only: ['stats.visits']` gets `stats` with `visits` and `total` in it,
  not `sales`: `stats` is on the way, so it's looked into, not sent whole.
- `except: ['stats.sales']` gets the rest of the page.
- `only: ['user.name']` gets all of `user`, since `User` is data.

`errors` and top-level `Always` props go out whether asked for or not; an
`Always` prop inside a map or struct goes out when the reload gets that
far, as `total` does above. What a prop's function returns goes out whole,
and merge and once metadata goes only with the props asked for, not with
those on the way.

## Shared props

The auth starter (`tug new -auth`) shares these with every page:

```go
// Auth is the auth prop every page shares: the user who's logged in, or
// null for a guest.
type Auth struct {
	User *User `json:"user"`
}

pages.Share("appName", appName)
pages.Share("auth", Auth{})  // a guest's value, and how tug gen learns the type
pages.ShareFunc(a.shareAuth) // inertia.Props{"auth": Auth{User: u}} for each request
```

`Share` gives every page a value. `ShareFunc` gives every page, error
pages included, props worked out for each request, such as the user who's
logged in; one that costs something can be `Lazy`, so a partial reload that
doesn't ask for it skips the work. Both are for setting up, before the app
serves. tug gen writes the TypeScript of the props shared with `Share` into
`SharedProps`, part of every page's `PageProps`. A prop worked out per
request has no type it can see, so the starter shares a first value of the
same type: `auth.user` is a `User | null` on every page
([typescript.md](typescript.md) has more).

Middleware shares props with the pages of one request through the context:

```go
// withTeam shares the team with every page under it.
func withTeam(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := inertia.WithProps(r.Context(), inertia.Props{"team": store.Team(r)})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
```

A page's own props win over shared ones of the same name. Among shared
ones, the later a source, the more it knows about the request: `WithProps`
wins over `ShareFunc`, which wins over `Share`, and a later call of each
over an earlier one. The App's own middleware runs outside the session's,
so middleware that needs the session goes on a group or a route
([routing.md](routing.md)).

## Redirects and visits

`c.Redirect(to)` answers with a 302 after a GET or HEAD, and a 303 after
anything else: a 303 has browsers and Inertia follow a PUT, PATCH or DELETE
with a GET, where a 302 may repeat the method. For Inertia's client, the
middleware also turns a 302 after a PUT, PATCH or DELETE into a 303, for
handlers that write their own. Flash data goes along ([forms.md](forms.md)).

The client's XHR follows a redirect, expecting an Inertia page at the end.
To send the browser to another site, or to a page of the app that isn't an
Inertia page, `c.Location(url)` has it load the URL whole: Inertia's
client gets a 409 with the URL in `X-Inertia-Location`, and any other
request a redirect. `inertia.Location(w, r, url)` does the same for any
net/http handler.

### A new build

After a deploy, a browser with a page open still runs the old frontend, and
sends its version with each visit, in `X-Inertia-Version`. A GET from
another build than the app's `Version` gets a 409 before the handler runs,
with its own URL in `X-Inertia-Location`, and the client loads that URL
whole, the new build with it. A form sent from the old build still gets
through; the GET after it reloads, and flash data waits for it. The URL is
a path, where the protocol's example has a full URL: behind a proxy that
ends TLS, a full URL would have to guess the scheme.

### Fragments

A redirect to a URL with a `#fragment` reaches Inertia's client as a 409,
with the URL in `X-Inertia-Redirect`, and the client makes the visit
itself: its XHR would follow the redirect and lose the fragment. A
prefetch's redirect is left alone. `c.PreserveFragment()` before a redirect
works the other way: the page after it tells the client to keep the
fragment the visit had, so a comment sent from `/posts/1#comments` comes
back to the comments. Like flash data, that needs `Config.Session`.

## Downloads and events

A download is a plain link, `<a href>`, not Inertia's `<Link>`, whose
visit asks for a page, and shows anything else in a dialog. The browser
saves the file, as `Content-Disposition` says, and stays on the page:

```tsx
<a href={route('posts.export')}>Download as CSV</a>
```

A page follows work on the server, as an import's progress, with an
`EventSource` on a route of `c.Events`
([Routing](routing.md#events)), and reloads the props the work changed
with Inertia's `router.reload`:

```tsx
useEffect(() => {
  const events = new EventSource(route('imports.progress', { id }))
  events.addEventListener('progress', (e) => setDone(JSON.parse(e.data).done))
  events.addEventListener('done', () => {
    events.close() // or it connects again, as it does when a stream ends
    router.reload({ only: ['posts'] })
  })
  return () => events.close()
}, [id])
```

A change made elsewhere, by another person, on another instance, reaches
the page the same way, through package `broadcast`
([Broadcasting](broadcasting.md)): the page follows a channel, as
`posts`, on a route of `c.Events`, and reloads what it shows of the post
an event names, and again when its `EventSource` connects again, for what
it missed.

`examples/inertia`'s archive has its posts as CSV, made a row at a time
with `c.StreamDownload`, and reloads its page of posts as one is made,
changed or deleted, in this browser or another.

## Error pages

With `Config.ErrorPage` set, as the starter sets it to `"Error"`, the
default ErrorHandler shows errors as that page, with their own status:
`tug.NewHTTPError(http.StatusNotFound, "post not found")` from a handler,
the 404 or 405 of a request no route takes, or a 500. The page gets the
shared props, and `tug.ErrorPageProps`, `status` and `message`, whose
TypeScript tug gen writes: the starter's `resources/js/pages/Error.tsx`
takes `PageProps<'Error'>`.

Browsers and Inertia's client get the page; an API client, whose `Accept`
header asks for JSON first, gets `{"message":"post not found"}`. An error
that isn't a `*tug.HTTPError` is a 500 whose message is "Internal Server
Error", with the details in the log, and with `Config.Debug` on
(`APP_DEBUG=true`) a 500 shows its details as plain text instead. Without
an error page, errors are plain text, which Inertia's client shows in a
dialog.

The page is rendered with `pages.RenderStatus(w, r, code, component,
props)`, which is `Render` with another status: the client shows an
Inertia response whatever its status. A custom `Config.ErrorHandler` can
use it too. From a handler, with `c.Response()` and `c.Request()`, it
renders without the flash data and validation errors that `c.Inertia`
hands a page, so for an error status, return a `*tug.HTTPError`.

## History encryption

The client keeps each page it shows, props and all, in the browser's
history. With `EncryptHistory: true` in the `inertia.Config`, it encrypts
them, so they can't be read back after the session ends. It does so with
`window.crypto.subtle`, which browsers only have on HTTPS and localhost;
elsewhere the client warns in the console and keeps the pages unencrypted.
`inertia.WithEncryptHistory(ctx, on)` turns it on or off for one request,
whatever the Config says: for a group of pages, in middleware like
`withTeam` above.

`c.ClearHistory()` has the next page shown tell the client to clear the
history it has encrypted, as logging out should. The client changes its
key, so the pages it encrypted can't be read, and going back to one fetches
it again. Like flash data, it reaches the page after a redirect.
`encryptHistory` and `clearHistory` are in the page object only when true.

## Vite

`vite.New` takes a `vite.Config`. `Build` is Vite's build output, with
`.vite/manifest.json`: in the starter, `public/build` from the `public/`
directory that `main.go` embeds, so the binary `tug build` makes has the
frontend in it ([deployment.md](deployment.md)). `Base` is the URL path
the build is served under, `/build/` unless it's set, which must be the
`base` that `vite.config.ts` gives `vite build`. `HotFile` is the file the
dev server writes its URL to while it runs.

While the hot file is there, `vite` loads each entry from the dev server,
after its client, which does the hot reloading, and `viteReactRefresh`
adds the preamble `@vitejs/plugin-react` needs, which Vue's and Svelte's
plugins don't, so their apps' root templates leave it out. A plugin in the
starter's `vite.config.ts` writes `public/hot` when the dev server starts
and deletes it when it stops; `tug dev` runs both ([cli.md](cli.md)). The
file is read at each render, so the pages switch over without a restart. A
`public/hot` left behind by a Vite that didn't exit cleanly points them at
a dev server that isn't there: delete it.

Otherwise the tags come from the build's manifest: each entry's script, its
CSS and that of the chunks it imports, and a `modulepreload` for each of
those chunks. An entry the manifest doesn't have is an error that names it,
as is having no build at all, and a first visit is then a 500.
`assets.Version()` is a hash of the manifest, `""` before a build: it's
what `inertia.Config.Version` wants, since each build has its own manifest.

`ServeHTTP` serves the build under `Base`, and the starter routes it with
`app.Get("/build/{path...}", tug.WrapHandler(assets))`. The files Vite
writes to `assets/` are named for a hash of their content, so they're
cached for a year, with `Cache-Control: public, max-age=31536000, immutable`;
the manifest, and any name starting with a dot, isn't served.

## Package inertia without tug

Package `inertia` imports nothing of tug, and neither does `vite`, so they
work with any router on net/http. `Render` renders a page, and `Middleware`
goes around the handlers that do:

```go
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/cuonggt/tug/inertia"
	"github.com/cuonggt/tug/vite"
)

const root = `<!doctype html>
<html>
  <head>{{ viteReactRefresh }}{{ vite "resources/js/app.tsx" }}</head>
  <body>{{ .Inertia }}</body>
</html>`

func main() {
	assets, err := vite.New(vite.Config{Build: os.DirFS("public/build"), HotFile: "public/hot"})
	if err != nil {
		log.Fatal(err)
	}
	pages, err := inertia.New(inertia.Config{Template: root, Funcs: assets.Funcs(), Version: assets.Version()})
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /build/", assets)
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		if err := pages.Render(w, r, "Home", inertia.Props{"greeting": "Hello"}); err != nil {
			log.Print(err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})
	log.Fatal(http.ListenAndServe("127.0.0.1:8080", pages.Middleware(mux)))
}
```

What tug adds is the glue: `c.Inertia` hands each render the session's
validation errors and flash data, through `inertia.WithErrors` and
`inertia.WithFlash`, and what `ClearHistory` and `PreserveFragment` asked
for, through `inertia.WithClearHistory` and `inertia.WithPreserveFragment`;
and the ErrorHandler shows errors as a page. Without tug, an app calls
those itself. `inertia.IsInertia(r)` tells a visit from Inertia's client.
