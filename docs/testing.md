# Testing

An app's pages are tested the way Inertia's client uses them, without a
browser or a frontend build. Package `tugtest` is that client, for Go's
tests: a `Client` stands in for a browser with the app open. Its visits
get each page as JSON, it keeps the cookies the app sets, and its
responses read their props into the app's own types. Both starters'
tests use it.

```go
func TestANewPostIsInTheList(t *testing.T) {
	c := tugtest.New(t, newTestApp(t))
	c.Get("/posts")
	r := c.Post("/posts", map[string]any{"title": "Hello, tug"})
	if r.Location() != "/posts" {
		t.Fatalf("got %v, want back to the list", r)
	}
	r = r.Follow()
	if posts := tugtest.Props(r, PostsIndex).Posts; len(posts) != 1 || r.Page.Flash["success"] != "Post created" {
		t.Fatalf("posts %v, flash %v", posts, r.Page.Flash)
	}
}
```

Everything but the browser runs as it does in production: the handlers,
the database, the session. The client sends what Inertia's client sends,
and the test checks the page object the frontend would be handed.

## A client

`tugtest.New(t, handler)` takes the app, a `*tug.App` or any
`http.Handler`, as the test makes it: the starters' tests have `newApp`
make it with an empty build, a key of their own, and in the auth starter,
a database of the test's own. On SQLite, that's a file in a temporary
directory. On Postgres or MySQL, it's made on a server, the one `DB_URL`
names, or else the one the app's `compose.yaml` runs, and dropped after:

```sh
docker compose up -d
go test ./...
```

With no server, the tests fail, saying so, rather than skip and pass
with nothing tested. `go test` doesn't read `.env`: a `DB_URL` for the
tests is set where they run, as a CI job sets it to a database service
of its own. A test's database has its throttles' counts too, so each
test starts with no tries counted against it, and two instances of the
app on one database, as a test can start, count each other's.

`Get`, `Post`, `Put`, `Patch` and `Delete` are visits, as Inertia's client
makes them: with `X-Inertia`, the version of the build the client runs,
the cookies the app has set, and the page the client is on as the
`Referer`. A body goes as JSON: a map or a struct, encoded with
encoding/json, or a string or `[]byte` as it is. Inertia's `<Form>` sends
each of its values as a string, `"on"` for a ticked box and `"42"` from a
number input, and `Bind` reads them as it reads a form's values, so a
test's body can be either. A map with a file in it goes as a multipart
form, as the client sends an upload: see [Uploads](#uploads).

Each returns a `*tugtest.Response`: its `Code`, `Header` and `Body`, and
`Page`, the page it shows, an `inertia.Page` with the component, props,
URL, flash data and the rest of the page object; the zero `Page` for a
response that shows none, such as a redirect. A Response prints as the
request and what it got, which is what a test's message wants:

```
POST /register: 303 to /dashboard
GET /dashboard: 200 Dashboard
GET /posts/9: 404 Error
```

A client is one browser: two logins are two clients, and each test makes
its own.

## Props

`tugtest.Props` reads a page's props into the struct its `tug.Page`
declares, and fails the test when the response shows another page, or
none:

```go
dash := tugtest.Props(r, Dashboard) // a DashboardProps
if dash.User.EmailVerifiedAt == nil {
	...
}
```

A prop of one of package `inertia`'s types, such as a `DeferProp`, holds a
function, which JSON can't bring back: `Props` leaves it as it is.
`tugtest.Prop` reads any one prop, by name, or by a dotted path to a prop
inside another, into the type it's given, and fails the test when the page
has no prop there:

```go
user := tugtest.Prop[*User](r, "auth.user") // nil for a guest
stats := tugtest.Prop[Stats](r, "stats")
first := tugtest.Prop[string](r, "posts.data.0.title") // a number picks an item of a list
```

`r.Has(path)` tells whether a prop is there, for a test that one isn't.
`tugtest.Flash` reads flash data as `Prop` reads a prop,
`tugtest.Flash[[]string](r, "recoveryCodes")`; `r.Page.Flash` has it all,
for a message: `r.Page.Flash["success"] != "Post created"`.

## Forms and redirects

A form sent from a page goes back to it when it doesn't validate, as
`BindValid` sends it back to the `Referer`, and the client sends the page
it's on as the `Referer`, as a browser does. `r.Location()` is where a
response sends the client, and `r.Follow()` follows it there, and on
through the redirects after, as the browser does, returning the response
at the end:

```go
c.Get("/posts/create")
r := c.Post("/posts", map[string]any{"title": ""})
if r.Code != http.StatusSeeOther || r.Location() != "/posts/create" {
	t.Fatalf("got %v, want back to the form", r)
}
if errs := r.Follow().Errors(); errs["title"] != "title is required" {
	t.Errorf("errors %v", errs)
}
```

`r.Errors()` is the validation errors the response carries, by field: a
page's `errors` prop, or those a 422 lists. A form with an error bag sends
it with `tugtest.ErrorBag("createPost")`, and following its redirect
sends it again, as the client does, so `Errors` finds the form's errors
under it.

`Follow` follows a 303, 301 or 302 with a GET, and makes a 307 or 308's
request again, body and all. A 409 is how the protocol has the client go
somewhere itself: `X-Inertia-Location`, as `c.Location` sends and a new
build does, is loaded whole, with `FirstVisit`, and `X-Inertia-Redirect`,
for a `#fragment`, is visited. A redirect to another site fails the test,
as do ten redirects that lead on to more.

A form that checks its fields as they're left sends a Precognition
request. `tugtest.Validate` makes one, which the app answers without
running the rest of the handler:

```go
r := c.Post("/posts", map[string]any{"title": "Taken"}, tugtest.Validate("title"))
if r.Code != http.StatusUnprocessableEntity || r.Errors()["title"] != "title is taken" {
	t.Errorf("got %v", r)
}
```

## Uploads

```go
r := c.Post("/settings/profile/photo", map[string]any{
	"photo": tugtest.File{Name: "ann.png", Type: "image/png", Content: png},
})
```

A `tugtest.File` in a body, a map's value, makes the visit a
`multipart/form-data` one, as Inertia's client sends a form with a file
in it: the file as a part of its own, with its `Name` and its `Type`, or
`application/octet-stream`, which is what the browser would say, and the
map's other values as the form's fields, written as the client writes
them. A string goes as it is, a number as its digits, `true` and `false`
as `1` and `0`, `nil` as empty, a list's items each under `tags[]`, and a
nested map's values under `user[name]`. A `File` with no `Name` is a file
input left empty, which `Bind` takes as none.

What a file is, the app tells from its bytes, so a test of `file_type`
sends the bytes it means: a PNG's first eight, `"\x89PNG\r\n\x1a\n"`, are
enough to be one, and a page named `ann.png` is still a page. The auth
starter's tests upload a photo so, and read the disk, a `storage.Local`
in the test's temporary directory, to see that a replaced one is gone.
See [Files](files.md#tests).

## Partial reloads

A partial reload is of the page the client is on: `c.Reload` visits it
again, and `tugtest.Only`, `Except` and `Reset` name the props, as the
client's `router.reload` does. A deferred prop comes this way, after its
page:

```go
r := c.Get("/")
if !slices.Equal(r.Page.DeferredProps["default"], []string{"stats"}) {
	t.Fatalf("deferred %v", r.Page.DeferredProps)
}
stats := tugtest.Prop[Stats](c.Reload(tugtest.Only("stats")), "stats")
```

`Only` with `Get` is a partial reload to another URL of the same page, as
`<InfiniteScroll>` asks for the next page of a list; `tugtest.Header` sets
any other header of a visit:

```go
r := c.Get("/?page=3", tugtest.Only("posts"), tugtest.Header("X-Inertia-Infinite-Scroll-Merge-Intent", "append"))
```

A partial reload before the client is on a page fails the test: its
component is the page's.

## Sessions and cookies

The client keeps the cookies the app sets, and drops the ones it expires,
so a login lasts from one visit to the next, and ends with logging out.
`r.Cookie(name)` is a cookie a response set, for a test of how long it
lasts.

`c.Session` changes the client's session as a request to the app would,
for a test that starts somewhere a person takes several pages to reach:

```go
// oldLogin is a browser logged in so long ago that the app asks for the
// password again: the session has the login, and no password typed since.
func (a *testApp) oldLogin(u *User) *tugtest.Client {
	sessions, err := session.New(session.Config{Keys: a.keys})
	if err != nil {
		a.t.Fatal(err)
	}
	c := a.client()
	c.Session(sessions, func(s *session.Session) { auth.Login(s, u.authID(), u.PasswordHash) })
	return c
}
```

It takes the app's `session.Store`, or one made with the same
`session.Config`, as here, and keeps what the request before flashed for
the next visit.

## First visits and builds

`c.FirstVisit` loads a page whole, as a browser does for an address typed
in or a link from another site: a GET without `X-Inertia`, which gets the
HTML, with the page object in it, which the Response reads. It's how to
test the root template, and the Vite tags in it:

```go
r := c.FirstVisit("/")
if !strings.Contains(r.Body, `<script type="module" src="/build/assets/app-1.js"></script>`) {
	...
}
```

`c.Version` is the build the client runs, which each visit sends: the
version of the last page it was shown, or, before it's been shown one, the
app's, taken at its first visit. A browser still running an old build gets
a 409 that loads the page again, which a test sets up with another:

```go
c.Version = "an old build"
if r := c.Get("/posts/1"); r.Code != http.StatusConflict || r.Location() != "/posts/1" {
	t.Errorf("got %v", r)
}
```

`c.Do` sends any other request with the client's cookies: a health check,
a file from the build, an upload, or an API's JSON.

```go
r := c.Do(httptest.NewRequest("GET", "/up", nil))
```

## Mail

Package `mailtest` has a `Mailer` for tests: `Outbox`, which keeps what
the app sends, for the test to read mail by mail.

```go
func TestANewAccountIsMailedALinkToVerifyItsEmail(t *testing.T) {
	out := &mailtest.Outbox{}
	c := tugtest.New(t, newTestApp(t, out)) // the app, sending its mail to out
	c.Post("/register", map[string]any{"name": "Ann", "email": "ann@example.com", ...})
	if m := out.Next(t); m.To[0] != "ann@example.com" || !strings.Contains(m.Text, "/verify-email/") {
		t.Errorf("the mail to %v:\n%s", m.To, m.Text)
	}
}
```

- `Next(t)` returns the next mail the test hasn't had, in the order they
  were sent, waiting up to five seconds for it, as a job may send it once
  the request that pushed it has been answered. It fails the test when
  none comes.
- `None(t)` checks that no more comes, now or in the next 100
  milliseconds, as after a form that mustn't mail anyone.
- `Down(n)` has the next n mails fail, with `mailtest.ErrDown`, as they
  would with the mail server down, for a test of a mail that goes again.
- `Sent()` is every mail so far, read or not.

An `Outbox` refuses a message an SMTP server wouldn't be sent, as
`mail.Log` does, so a mail that couldn't go in production fails its test.
The auth starter's tests use one ([Accounts](auth.md#testing)).

## When a test fails

The client fails the test, with `t.Fatalf`, when the test asks it for
what isn't there: a prop the page doesn't have, the props of a page it
isn't, a redirect to follow from a response that isn't one. Its message
says what the response was instead, so

```go
tugtest.Props(c.Get("/dashboard"), Dashboard)
```

fails with `GET /dashboard: 302 to /verify-email, not the page Dashboard`.
What the app answered, its statuses, props and errors, is the test's to
check.

## What's not here yet

- The client doesn't keep once props, or merge a partial reload into the
  page it has: each Response is what the app sent. A test of a once prop
  sends `X-Inertia-Except-Once-Props` itself, with `tugtest.Header`.
- None of the frontend runs: what the pages do with their props is for
  the browser tests, as `examples/inertia`'s Playwright tests are.
