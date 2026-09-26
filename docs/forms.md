# Forms and sessions

A form is Inertia's `<Form>` on a page, sent to a handler that binds it into
a struct and checks it. A form that doesn't validate goes back to its page
with the errors; one that does leaves a flash message for the page it
redirects to. The session carries both, in an encrypted cookie, and CSRF
protection needs no tokens.

## A form, end to end

```go
type PostInput struct {
	Title string `json:"title" validate:"required,max=80"`
	Body  string `json:"body" validate:"required,min=10"`
}

func createPost(c *tug.Ctx) error {
	var in PostInput
	if err := c.BindValid(&in); err != nil {
		return err // back to the form, with what to fix
	}
	post := posts.Add(in) // the app's own storage
	c.Flash("success", "Post created")
	return c.RedirectRoute("posts.show", post.ID)
}
```

`c.BindValid` binds the request into `in`, as `c.Bind` does (see
[Routing](routing.md)), then checks it against its `validate` tags. When
something's wrong it returns `validate.Errors`, a message for each field,
which the handler returns as it is: the app's ErrorHandler sends the form
back with them. Otherwise the handler does its work, leaves a message for
the next page, and redirects (see [Pages](pages.md#redirects-and-visits)).
With the route `app.Post("/posts", createPost).Name("posts.store")`, the
form, in `resources/js/pages/Posts/Create.tsx`, is:

```tsx
import { Form } from '@inertiajs/react'
import { route } from '../../tug/routes'

export default function Create() {
  return (
    <Form action={route('posts.store')} method="post">
      {({ errors, processing }) => (
        <>
          <label>
            Title
            <input name="title" />
          </label>
          {errors.title && <p className="error">{errors.title}</p>}

          <label>
            Body
            <textarea name="body" rows={5} />
          </label>
          {errors.body && <p className="error">{errors.body}</p>}

          <button type="submit" disabled={processing}>
            Create post
          </button>
        </>
      )}
    </Form>
  )
}
```

`<Form>` sends its inputs by their `name`s, the names in the json tags, as
JSON with every value a string. `Bind` reads a string where the struct has
a bool, a number or a `time.Time` as it reads a form's value, so a
checkbox's `"on"` fills a `bool`, a number input's `"42"` an `int`, and an
input left empty leaves its field alone ([Routing](routing.md#binding) has
the rest). When the form comes back, `errors` has a message for each field
that's wrong, such as "body must be at least 10 characters", and the inputs
keep what was typed. `route()` is written by `tug gen` from the named routes
([TypeScript](typescript.md)). Sending the form back, and the flash message
outliving the redirect, take `Config.Session`, which `tug new` sets up: see
[Sessions](#sessions).

## When a form doesn't validate

`tug.DefaultErrorHandler` answers the `validate.Errors` a handler returns in
one of two ways.

A browser's request, from Inertia's client or a plain HTML form, goes back
where it came from. The errors are flashed to the session, and the response
redirects to the `Referer` when that's a page of this app, or else to `/`,
since a `Referer` can name any site. The page shown next has the errors in
its `errors` prop, `{"body": "body is required"}`, and the one after has
`{}`, as every page without errors does. A `Referrer-Policy` that leaves
the path out, such as `no-referrer` or `origin`, sends every form that
doesn't validate to `/`.

A page with two forms can keep their errors apart with an error bag:

```tsx
<Form action={route('comments.store', { id: post.id })} method="post" errorBag="comment">
```

Inertia's client names the bag in `X-Inertia-Error-Bag`, and the errors
come back under it: `{"comment": {"body": "body is required"}}`. The
`<Form>` that names the bag reads its errors from there, so `errors.body`
works in it as before.

A request whose `Accept` header asks for JSON first, as an API client's
does, gets a 422 instead, and so does any request to an app without
`Config.Session`:

```json
{"errors":{"body":"body is required","title":"title must be at most 80 characters"},"message":"body is required"}
```

`message` is the error of the field whose name sorts first. Inertia's
client would show that 422 in its error dialog rather than on the form,
which is why an app with forms has sessions. An app with an ErrorHandler of
its own hands `validate.Errors` on to `tug.DefaultErrorHandler`: nothing
else sends them back.

## Validation rules

```go
type OrderInput struct {
	Email string      `json:"email" validate:"required,email"`
	Note  string      `json:"note" validate:"omitempty,min=3"`
	Lines []LineInput `json:"lines" validate:"min=1,dive"`
}

type LineInput struct {
	Product  string `json:"product" validate:"required"`
	Quantity int    `json:"quantity" validate:"min=1"`
}
```

The rules are the tags of
[go-playground/validator](https://pkg.go.dev/github.com/go-playground/validator/v10),
separated by commas. `omitempty` skips the rest for an empty value. A field
that's a struct is checked by its own fields' tags, and a list of structs
needs `dive` for its items to be checked. tug writes a sentence for these
tags, and "*field* is invalid" for any other. The limits count characters
for text, items for a list or map, and the value itself for a number:

| Tags | Message |
|------|---------|
| `required`, `required_if`, `required_unless`, `required_with`, `required_with_all`, `required_without`, `required_without_all` | title is required |
| `email` | email must be a valid email address |
| `url`, `http_url`, `uri` | website must be a valid URL |
| `uuid`, `uuid4`, `uuid7` | id must be a valid UUID |
| `min`, `gte`; `max`, `lte` | note must be at least 3 characters; lines must have at most 20 items |
| `gt`; `lt`; `len` | price must be more than 0; price must be less than 100; code must be exactly 6 characters |
| `eq`; `ne` | answer must be 42; username must not be admin |
| `oneof` | size must be one of: small, medium, large |
| `alpha`; `alphanum` | code must contain only letters; code must contain only letters and numbers |
| `numeric`, `number` | zip must be a number |
| `boolean` | terms must be true or false |
| `datetime` | starts_at must be a valid date |
| `eqfield`; `nefield` | password_confirmation must match password; new_password must be different from password |
| `contains`; `excludes` | password must contain "!"; username must not contain "@" |
| `startswith`; `endswith` | handle must start with "@"; domain must end with ".com" |
| `unique` | tags must not repeat a value |

Fields are named as the client named them: by the json tag, or by the Go
name when there's none. A nested field is named by its dotted path, a
list's index included: the second line's quantity is `lines.1.quantity`,
which is `errors['lines.1.quantity']` in TSX, and its message is "quantity
must be at least 1". An input type can be anonymous, as
`var in struct{...}` is, and its fields are named the same way.

A value that doesn't parse, from the body or the query, is a field error
too: "five" for an `int` is `"stars": "stars must be a whole number"`. The
fields after it are still bound and checked, so the form hears about
everything at once, and the parse error is the message its field keeps. A
path value that doesn't parse is still a 404, and a body Bind can't read,
such as JSON that isn't, still fails as [Routing](routing.md) describes.

## Checks of the handler's own

```go
err := c.BindValid(&in, func(errs validate.Errors) {
	if posts.TitleTaken(in.Title) {
		errs.Add("title", "another post has that title")
	}
})
```

Checks run after the tags, with the errors found so far. `errs.Add(field,
message)` adds one unless the field has one already: the first thing wrong
with a field is the one worth saying. A check can read `errs` too, to skip
a query when its field is wrong already, as the auth starter's registration
does before it looks the email up. A check made by a function, to share
between handlers, takes a pointer to the input, as `titleFree(&in, post.ID)`
does below: it's made before BindValid fills the input in.

`validate.Errors` is a `map[string]string`. Besides `Add`, it has `First`,
the message of the field that sorts first, and `Only(fields...)`, the
errors of the fields named and of those nested in them.

A handler can also return `validate.Errors` of its own after BindValid, for
what can only be checked when the form is sent, and they're answered as
BindValid's are. The auth starter's login returns
`validate.Errors{"email": "the email and password don't match an account"}`
for a wrong password (see [Accounts](auth.md)).

`c.Validate(v, checks...)` checks a value that doesn't come from the
request, with the same answers, such as a draft about to be published:
`c.Validate(PostInput{Title: draft.Title, Body: draft.Body})`.
`validate.Struct(v)` is the check alone, for code with no request: it
returns `validate.Errors`, or nil.

## Checking each field as it's left

```tsx
<input name="title" aria-invalid={invalid('title')} onBlur={() => validate('title')} />
```

`validate` and `invalid` come from `<Form>`'s render function, beside
`errors`. `validate('title')` asks the server about the form as it stands,
without sending it: a Precognition request, to the form's action with its
method, carrying the form's data as JSON and the headers
`Precognition: true` and `Precognition-Validate-Only`, which lists the
fields checked so far. `invalid('title')` is true while the title has an
error. `validationTimeout` on `<Form>` is how long, in milliseconds, it
waits before checking again: 1500 by default, and 300 in
`examples/inertia`.

The handler needs nothing new: `BindValid` answers the request itself. It
binds and checks the whole input, tags and checks both, then keeps the
errors of the fields named in `Precognition-Validate-Only`, and of fields
nested in them. The rest haven't been filled in yet: an empty body isn't
wrong until it's been left. With no errors it answers 204, with
`Precognition-Success: true`; with some, a 422 with `message` and `errors`.
Then it returns an error, which the handler returns as it does any other.
The rest of the handler doesn't run, and the app knows the request has been
answered, so the ErrorHandler never sees it.

```go
func updatePost(c *tug.Ctx) error {
	post, err := findPost(c) // runs for every Precognition request too
	if err != nil {
		return err
	}
	var in PostInput
	if err := c.BindValid(&in, titleFree(&in, post.ID)); err != nil {
		return err // a Precognition request is answered, and stops, here
	}
	posts.Update(post.ID, in) // only when the form is sent
	c.Flash("success", "Post updated")
	return c.RedirectRoute("posts.show", post.ID)
}
```

So what comes before BindValid has to be safe to repeat. Finding the post
an edit form is for is: a post that's gone is a 404 for the Precognition
request too. Saving, sending mail, or counting a login attempt isn't. The
checks passed to BindValid run for every Precognition request as well,
whichever fields it names, which is how a taken title shows as the field is
left. What comes after BindValid, such as the login's password check, runs
only when the form is sent.

A handler without BindValid or `c.Validate` knows nothing of Precognition,
and runs in full for one: call `validate` only in a form whose handler has
one of them.

## Flash messages

```tsx
import { usePage } from '@inertiajs/react'

export default function Flash() {
  const { flash } = usePage()
  return flash.success ? <p role="status">{flash.success}</p> : null
}
```

`c.Flash(key, value)` leaves data for the next page shown, the one this
request renders or else the one it redirects to, and for that page alone.
A page that only redirects shows nothing, so flash data goes on through a
chain of redirects, and a browser that reloads for a new build of the
frontend gets it on the reloaded page. The client reads it as
`usePage().flash`, apart from the props, and doesn't keep it in history, so
going back doesn't show it again. A value can be anything encoding/json
writes, and flashing a key twice keeps the later value.

`tug gen` can't see what handlers flash, so the type is written by hand, in
`resources/js/types.ts`, which `tug new` makes for the `success` key its
handler uses. A key that isn't there doesn't type-check:

```ts
declare module '@inertiajs/core' {
  export interface InertiaConfig {
    flashDataType: { success?: string }
  }
}

export {}
```

`c.ClearHistory()` and `c.PreserveFragment()` reach the next page the same
way, through redirects. `ClearHistory` has the page tell the client to
clear the history it has encrypted, as logging out should (see
[history encryption](pages.md#history-encryption)); `PreserveFragment` has
it keep the `#fragment` of the visit that led to it. Past a redirect, all
three need `Config.Session`: without it, a handler that flashes and
redirects fails with a 500 rather than lose the message. A handler that
empties the session with `c.Session().Clear()`, as logging out does, calls
it before any of them, since `Clear` takes back what the request has
flashed too.

## Sessions

```go
keys, err := session.KeysFromEnv() // APP_KEY, then APP_PREVIOUS_KEYS
if err != nil {
	log.Fatal(err)
}
sessions, err := session.New(session.Config{Keys: keys})
if err != nil {
	log.Fatal(err)
}
cfg := tug.ConfigFromEnv()
cfg.Session = sessions
app := tug.New(cfg)
```

The app `tug new` makes does this in its `main.go`. Every request then goes
through the session's middleware, inside the app's own, and `c.Session()`
returns its session; without `Config.Session`, it's nil.

The whole session travels in one cookie, encrypted and authenticated with
AES-256-GCM under a key derived from `APP_KEY`, so the server keeps nothing.
A cookie that was tampered with, or made with a key since dropped, starts
an empty session. `session.Config` has three more fields:

- `Cookie` names the cookie, `tug_session` by default. It's `HttpOnly` and
  `SameSite=Lax`, and isn't set while the session is empty.
- `Lifetime` is how long a session lasts without a request, two hours by
  default. Each response sets the cookie again with a new expiry, so a
  session in use stays alive; one left longer starts afresh, even if the
  browser kept the cookie, as the expiry is inside it too. One session
  can have a lifetime of its own, set with `SetLifetime`, below.
- `Secure` keeps the cookie to HTTPS. A request over TLS gets that anyway,
  but behind a proxy that ends TLS, requests arrive over plain HTTP and it
  takes `Secure: true`. See [Deployment](deployment.md#behind-a-proxy).

### Keys

`APP_KEY` is `base64:` and 32 random bytes in base64, as Laravel writes it,
and `tug new` puts a fresh one in `.env`. To make another:

```sh
echo "APP_KEY=base64:$(head -c 32 /dev/urandom | base64)"
```

Without one, `KeysFromEnv` returns `session.ErrNoKey`, whose message says
the same. `session.ParseKey` reads a key in that form, or the base64 alone,
for one kept outside the environment.

To rotate the key, the new one goes in `APP_KEY` and the old one in
`APP_PREVIOUS_KEYS`, comma separated when there are several. The first key
encrypts and each of them decrypts, so sessions made with the old key still
read, and each response encrypts its session again with the new one. Once
a `Lifetime` has passed, every session still alive has been through a
response, and the old key can go: the longest lifetime any session has,
which is a month for the auth starter's "Remember me".

### Reading and writing

```go
s := c.Session()
s.Set("theme", "dark")
theme, _ := s.Get("theme").(string)
s.Delete("theme")
```

`SetLifetime(d)` has the session last `d` without a request, in place of
the `Store`'s `Lifetime`, as a login that ticked "Remember me" does in the
auth starter: a month, where everyone else's lasts two hours. The lifetime
travels in the cookie with the rest, so each response starts it again as
it does the `Store`'s, until `Clear`, or `SetLifetime(0)`, goes back to the
`Store`'s.

`Clear` empties the session, flash data included. Values go through
encoding/json on their way into the cookie, so a later request gets JSON
types back: set `42`, and the next request gets `float64(42)`; a struct
comes back as a `map[string]any`.

The session's own `Flash(key, value)` stores a value for the next request,
which reads it with `Flashed(key)`; `Reflash` keeps what the request before
flashed for one more, and `Unflash` takes back what this one flashed.
They're for values a handler reads rather than a page: `c.Flash` is built
on them, under keys of its own.

A browser keeps about 4 KB in a cookie. A session larger than that isn't
saved: the response goes out without it, what the request changed is lost,
and the log says `session: too large for a cookie, so it wasn't saved; keep
less in it`. Keep IDs in the session, and the rest in a database.

Middleware added with `app.Use` runs outside the session and can't see it;
a group's or a route's can, with `session.From(r.Context())`.

## CSRF

```go
app.Use(middleware.RequestID(), middleware.Logger(), middleware.Recover(), middleware.CSRF())
```

`middleware.CSRF()` rejects a POST, PUT, PATCH or DELETE that a browser
sent from another site, with a 403. It's Go's `http.CrossOriginProtection`.
Every current browser says where a request comes from in `Sec-Fetch-Site`,
and one that's `cross-site`, or `same-site` from another subdomain, is
rejected. For a browser that doesn't send that header, `Origin` has to
match `Host`. A request with neither, from curl or another server, isn't a
browser acting for a visitor, and passes.

So there are no tokens: nothing to put in forms, nothing to expire, and no
need for Inertia's `XSRF-TOKEN` cookie. A real user never meets the 403;
only a forged request does, which is why it's plain text rather than the
error page.

GET, HEAD and OPTIONS always pass, so they must never change anything.
That's why logging out is a POST, from a link such as
`<Link href={route('logout')} method="post" as="button">`.

An origin the app trusts, such as its admin site's, is let through by name:
`middleware.CSRF("https://admin.example.com")`. A value that isn't an
origin, such as one without a scheme, panics when the app starts rather
than turn into 403s later.

`CSRF` goes in `app.Use`, whose middleware wraps every request: every route
is covered, in whatever group, and a rejected request stops before the
session and the handler.
