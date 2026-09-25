# Getting started

This page makes a new app and runs it, adds a page and a form to it, tests
them, and builds the app into one binary.

## What you need

- Go 1.26 or later.
- Node.js 22.12 or later, or 20.19 or later in Node 20: Vite 8, which
  builds the frontend, needs one of them. The app's Dockerfile builds with
  Node 24.
- npm, which comes with Node. `tug new` installs the app's packages with it.

## Install tug

```sh
go install github.com/cuonggt/tug/cmd/tug@latest
tug version
```

`go install` puts `tug` in `$(go env GOPATH)/bin`, or in `$GOBIN` when
that's set, and that directory needs to be on your `PATH`. `tug version`
prints the version installed, such as `tug v0.1.0`. The apps `tug new`
makes require that version.

## Make an app

```sh
tug new blog
cd blog
```

`tug new` makes the directory `blog`, which mustn't exist yet or must be
empty, and puts an app in it that's ready to run:

- the starter's files, with a Go module named after the directory, which
  `-module github.com/you/blog` changes;
- a `.env` with a fresh `APP_KEY`, the 32 random bytes that encrypt the
  session cookie, and `APP_DEBUG=true`, which shows an error's details in
  its 500 response;
- the Go modules, with `go mod tidy`, and the frontend's packages, with
  `npm install`;
- the TypeScript of the app's pages and routes, in `resources/js/tug`,
  which it builds the app once to write.

For an app with accounts, add `-auth`:

```sh
tug new -auth blog
```

That lays a second starter over the first. People register, log in and
out, and reset a forgotten password with a link sent by email, and a
dashboard is for those who've logged in. The users are in SQLite, in
`app.db`. The code is the app's own, the handlers in `auth.go` and the
database in `users.go`, to change as the app needs. Until `MAIL_HOST` is
set, mail isn't sent: it's written out with the app's output in `tug dev`,
reset links and all. [Accounts](auth.md) has the rest. This page goes on
with the plain starter.

## Run it

```sh
tug dev
```

```
tug  │ serving http://127.0.0.1:8080, built in 268ms
```

Open http://127.0.0.1:8080. `tug dev` runs two servers and shows their
output with its own, each line labeled `tug`, `app` or `vite`:

- **Vite's dev server**, `npm run dev`, on `localhost:5173`, which serves
  the frontend's modules to the browser.
- **The app**, the Go server the browser visits, built into `.tug/app`.
  When port 8080 is taken, `tug dev` takes the next free one and says so.
  `ADDR` or `PORT`, in `.env` or the environment, picks one instead.

The page comes from the Go server, and its scripts from Vite: while Vite
runs, the plugin in `vite.config.ts` writes its address to `public/hot`,
which the Go server reads each time it renders a page.

As you work:

- **Go files, `go.mod` and `go.sum`, and templates** (`.html`, `.tmpl`,
  `.gohtml`): tug builds the app again, writes the TypeScript types if
  they've changed, restarts the app, and reloads the browser.
- **A build that fails**: its errors show under `app`, and the app from the
  last build that worked keeps running until a save fixes it.
- **The frontend**, under `resources/`: Vite sends the change to the
  browser, and a React component updates in place, keeping its state.
- **`.env`**: it's read when `tug dev` starts. Stop it with Ctrl-C and
  start it again.

[The CLI](cli.md#tug-dev) has the details.

## What's in the app

```
blog/
├── main.go              the server: its pages, routes and handlers
├── main_test.go         its tests
├── app.html             the HTML every page is rendered into
├── resources/
│   ├── css/app.css
│   └── js/
│       ├── app.tsx      the frontend's entry: finds each page's component
│       ├── Layout.tsx   what every page has around it, the flash message too
│       ├── pages/       a component for each page: Home.tsx and Error.tsx
│       ├── types.ts     the type of the flash data
│       └── tug/         written by tug gen, not by hand: pages.ts, routes.ts
├── public/              put into the binary; Vite builds into public/build
├── vite.config.ts       Vite's settings, and the plugin tug dev works with
├── go.mod, package.json, tsconfig.json
├── .env                 read by the tug commands, not the app; not committed
├── .env.example         the same without the key, to commit
├── Dockerfile           the app as one binary on a distroless image
└── README.md
```

`main.go` is in two parts. `main` reads the app's settings from the
environment, `APP_KEY` with `session.KeysFromEnv`, and `ADDR`, `PORT` and
`APP_DEBUG` with `tug.ConfigFromEnv`, and runs the app. `newApp` puts the
app together from them: Vite's tags, the Inertia pages with `app.html`
around them, the sessions, the error page, the middleware, the route that
serves the frontend's build under `/build/`, and the app's own routes. The
tests call `newApp` with settings of their own.

## Add a page

A page is a React component in `resources/js/pages`, and the Go that
renders it: a struct of its props, a `tug.Page` that ties them to the
component, a handler, and a route. This one lists the blog's posts. Add to
`main.go`, with `slices` and `sync` in its imports:

```go
// Post is a post on the blog.
type Post struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

// posts keeps the posts in memory, so they're gone when the app stops.
// Requests are handled concurrently, so the list has a lock.
type posts struct {
	mu   sync.Mutex
	list []Post
}

// PostsIndexProps are the props of resources/js/pages/Posts/Index.tsx.
type PostsIndexProps struct {
	Posts []Post `json:"posts"`
}

var PostsIndex = tug.Page[PostsIndexProps]("Posts/Index")

func (p *posts) index(c *tug.Ctx) error {
	p.mu.Lock()
	list := slices.Clone(p.list)
	p.mu.Unlock()
	return PostsIndex.Render(c, PostsIndexProps{Posts: list})
}
```

and in `newApp`, under the other routes:

```go
	p := &posts{}
	app.Get("/posts", p.index).Name("posts.index")
```

`tug.Page[PostsIndexProps]("Posts/Index")` declares the component
`resources/js/pages/Posts/Index.tsx` and the props it takes: `Render` takes
a `PostsIndexProps` and nothing else. The json tags name the props as the
component gets them, and a list with nothing in it goes out as `[]`, never
`null`. The route's name, `posts.index`, is how the Go and the frontend
both refer to it without writing out its path. The posts are made in
`newApp`, so each test's app starts with none.

Save, and `tug dev` builds the app and writes its types again:

```
tug  │ main.go changed
tug  │ wrote resources/js/tug/pages.ts, resources/js/tug/routes.ts
tug  │ serving http://127.0.0.1:8080, built in 844ms
```

`pages.ts` now has the page's props, in TypeScript:

```ts
export interface Post {
  id: number
  title: string
}

export interface PostsIndexProps {
  posts: Post[]
}
```

The component goes in `resources/js/pages/Posts/Index.tsx`:

```tsx
import { Head } from '@inertiajs/react'
import Layout from '../../Layout'
import type { PageProps } from '../../tug/pages'

export default function Index({ posts }: PageProps<'Posts/Index'>) {
  return (
    <Layout>
      <Head title="Posts" />
      <h1>Posts</h1>
      <ul>
        {posts.map((post) => (
          <li key={post.id}>{post.title}</li>
        ))}
      </ul>
    </Layout>
  )
}
```

`PageProps<'Posts/Index'>` is the page's own props and those every page
shares, such as `appName`, which `newApp` shares with `pages.Share`. To
link to the page from the home page, add `Link` to `Home.tsx`'s import from
`@inertiajs/react`, and this under the greeting:

```tsx
<Link href={route('posts.index')}>Posts</Link>
```

`route()` knows the app's named routes. A name that isn't one, such as
`route('post.index')`, doesn't type-check, and neither does a route with a
wildcard called without its value. Values go by name:
`route('posts.show', { id: 42 })` is `/posts/42` for a `/posts/{id}` route.
Vite doesn't check types as it serves; your editor does, and so do
`npm run typecheck` and `tug build`.

## Add a form

The form adds a post. In Go, it's a struct of what the form sends, with
the rules for each field in its `validate` tag, and a handler:

```go
// PostInput is what the new post form sends.
type PostInput struct {
	Title string `json:"title" validate:"required,max=80"`
}

func (p *posts) store(c *tug.Ctx) error {
	var in PostInput
	if err := c.BindValid(&in); err != nil {
		return err // back to the form, with the errors
	}
	p.mu.Lock()
	p.list = append(p.list, Post{ID: len(p.list) + 1, Title: in.Title})
	p.mu.Unlock()
	c.Flash("success", "Post created")
	return c.RedirectRoute("posts.index")
}
```

and its route, in `newApp`:

```go
	app.Post("/posts", p.store).Name("posts.store")
```

`c.BindValid` fills `in` from the request and checks it against the
`validate` tags, which are go-playground/validator's rules. When something
is wrong, it returns the errors, and the handler returns them: the browser
goes back to the form, and the page gets them in its `errors` prop, such as
"title is required" or "title must be at most 80 characters". `c.Flash`
leaves a message for the next page shown, which the starter's `Layout`
shows. `c.RedirectRoute` sends the browser to a named route, with a 303
after a POST, so that it follows with a GET.

In `Index.tsx`, import `Form` beside `Head`, and `route`:

```tsx
import { Form, Head } from '@inertiajs/react'
import { route } from '../../tug/routes'
```

and add the form under the list:

```tsx
<Form action={route('posts.store')} method="post" resetOnSuccess className="form">
  {({ errors, processing }) => (
    <>
      <label>
        Title
        <input name="title" />
      </label>
      {errors.title && <p className="error">{errors.title}</p>}
      <button type="submit" disabled={processing}>
        Add post
      </button>
    </>
  )}
</Form>
```

Inertia's `<Form>` sends its inputs by their `name`, which are the json
names of the Go struct's fields, and hands its children the `errors` under
the same names. `resetOnSuccess` empties the input once the post is added.
The `form` and `error` classes are the starter's CSS.

The starter's home page also checks its field as it's left, before the
form is sent: its input calls `validate('name')` when it loses focus, and
`BindValid` answers that request, a Precognition request, with the field's
errors, and the handler stops there. The same works here with `validate`
from the form's children, and `onBlur={() => validate('title')}` on the
input. [Forms and sessions](forms.md) has the rest: checks of the
handler's own, error bags, flash data, and the session.

## Test it

```sh
go test ./...
```

The starter's `main_test.go` tests the app the way Inertia's client uses
it, without a browser or a frontend build. `newVisitor` makes the app with
`newApp`, an empty build and a key of its own. Its `visit` sends a request
with `X-Inertia: true`, so the app answers with the page as JSON, its
component and props, and keeps the session cookie from one visit to the
next, which is what carries a form's errors and flash message to the page
after it. Two tests for the posts, in `main_test.go`:

```go
func TestANewPostIsInTheList(t *testing.T) {
	v := newVisitor(t)
	if code, _ := v.visit("POST", "/posts", `{"title":"Hello, tug"}`); code != http.StatusSeeOther {
		t.Fatalf("got %d, want a redirect", code)
	}
	_, p := v.visit("GET", "/posts", "")
	posts, _ := p.Props["posts"].([]any)
	if len(posts) != 1 || p.Flash["success"] != "Post created" {
		t.Fatalf("posts %v, flash %v", p.Props["posts"], p.Flash)
	}
}

func TestAPostWithoutATitleComesBackWithWhatToFix(t *testing.T) {
	v := newVisitor(t)
	v.visit("POST", "/posts", `{"title":""}`)
	_, p := v.visit("GET", "/posts", "")
	if p.Props["errors"].(map[string]any)["title"] != "title is required" {
		t.Fatalf("errors %v", p.Props["errors"])
	}
}
```

`npm run typecheck` checks the frontend against the types `tug gen` wrote.

## Build it

```sh
tug build
```

`tug build` writes the types, type-checks the frontend, builds it with
Vite into `public/build`, and builds the app into one binary, `./blog`,
with the frontend inside: `main.go` embeds `public/`. The starter's binary
is about 10 MB, and static, so it runs with nothing else installed.

The binary takes its settings from its environment, not from `.env`:

```sh
APP_KEY=base64:... ADDR=127.0.0.1:8080 ./blog
```

Without an `APP_KEY`, it stops and says so. A deployed app needs a key of
its own: make one with `head -c 32 /dev/urandom | base64`, and set it as
`base64:` followed by that. The app listens on `ADDR`, or on `PORT` as
platforms such as Cloud Run and Fly.io set it, or else on `:8080`. Leave
`APP_DEBUG` unset, so that a 500's details stay in the log.

The app's `Dockerfile` builds the frontend with Node and the binary with
Go, and puts the binary alone on a distroless image:

```sh
docker build -t blog .
docker run -p 8080:8080 -e APP_KEY=base64:... blog
```

The image's build doesn't run `tug`: its Vite build uses the
`resources/js/tug` in the directory, which the starter's `.gitignore`
leaves in, to be committed with the Go it's written from.
[Deployment](deployment.md) has the rest: the image, the environment, and
proxies.

## Where next

- [Routing and handlers](routing.md): routes, groups, middleware, `Ctx`,
  binding, and errors.
- [Pages](pages.md): props and prop types, shared props, redirects, error
  pages, and Vite.
- [Forms and sessions](forms.md): validation, Precognition, flash, sessions
  and CSRF.
- [Accounts](auth.md): the auth starter, and packages `auth` and `mail`.
- [TypeScript](typescript.md): what `tug gen` writes from the Go types.
- [Deployment](deployment.md): `tug build`, the Dockerfile, and the
  environment.
- [The CLI](cli.md): each command in full.

[`examples/inertia`](../examples/inertia/main.go) is a bigger app: posts
created, edited and deleted, a deferred prop, and a list that loads more as
it scrolls.
