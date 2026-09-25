# Accounts

`tug new -auth` makes an app with accounts: people register, log in and
out, and reset a forgotten password with a link sent by email. The handlers
are the app's own code, in `auth.go`, to change as the app does. They stand
on package `auth`, which has the parts where a slip is a security hole, and
package `mail`, which sends the links.

## The auth starter

```sh
tug new -auth blog
cd blog
tug dev
```

The app is the plain starter ([Getting started](getting-started.md))
with accounts: registering, logging in and out, a forgotten password reset
by a mailed link, a dashboard for users only, and `auth.user` on every
page, error pages included: the user who's logged in, or `null`. The users
are in SQLite, through `database/sql`. The driver is modernc.org/sqlite,
which is pure Go, so `tug build` still makes a static binary.

| Route                          | Name                                 | For      |
|--------------------------------|--------------------------------------|----------|
| `GET /`                        | `home`                               | everyone |
| `GET /dashboard`               | `dashboard`                          | users    |
| `GET`, `POST /register`        | `register`, `register.store`         | guests   |
| `GET`, `POST /login`           | `login`, `login.store`               | guests   |
| `POST /logout`                 | `logout`                             | users    |
| `GET`, `POST /forgot-password` | `password.request`, `password.email` | guests   |
| `GET /reset-password/{token}`  | `password.reset`                     | guests   |
| `POST /reset-password`         | `password.store`                     | guests   |

A guest on a route for users is sent to log in, and comes back after;
someone logged in on a route for guests goes to the dashboard.

`-auth` lays the auth starter over the plain one: its files replace the
plain starter's of the same name, and add the rest.

- `main.go`: the environment, the database, and `newApp`, with the routes.
- `auth.go`: the handlers, with `usersOnly` and `guestsOnly`.
- `users.go`: `User`, the `users` table, made at the first start, and its
  queries. An email is unique whatever its case.
- `main_test.go`: a test of each flow, with the mail kept in memory.
- `resources/js/pages`: `Home`, `Dashboard`, and the four pages in `Auth/`.
- `.env.example`: `APP_KEY`, `APP_URL`, `DB_PATH` and the mail's
  variables, with what each is for. The `Dockerfile` keeps the database in
  a `/data` volume.

### Trying it

`tug new` writes a `.env` with `APP_KEY` and `APP_DEBUG=true`, and nothing
else. With no `APP_URL`, links start with the address the request came to;
with no `MAIL_HOST`, mail is written out rather than sent. Register, log
out, follow "Forgotten your password?", and the mail is in `tug dev`'s
output, with a link to click:

```
app  │ mail, not sent (MAIL_HOST isn't set):
app  │   From: (nobody)
app  │   To: ann@example.com
app  │   Subject: Reset your blog password
app  │   ...
app  │   http://127.0.0.1:8080/reset-password/tlwzi8.pIFKYhVaE1CzWqcFH-bjXw?email=ann%40example.com
```

The database is `app.db`, or wherever `DB_PATH` says, and `.gitignore`
leaves it out.

## How it works

### Users only, guests only

```go
app.Get("/dashboard", a.usersOnly(dashboard)).Name("dashboard")
```

`dashboard` takes the user as well as the `Ctx`:
`func dashboard(c *tug.Ctx, user *User) error`. `usersOnly` finds the
request's user and hands it over, or sends a guest to log in, keeping the
page with `auth.SetIntended` for after. Routes take a `tug.HandlerFunc`, so
a handler that takes a user can't be routed without `usersOnly`: forgetting
it doesn't compile. That's why it's a wrapper rather than middleware, and
why the user is an argument rather than something to dig out of the
request's context. `guestsOnly` wraps the pages for guests, and sends
someone who's logged in to the dashboard instead, even from a reset link.

### Finding the request's user

```go
func (a *app) user(r *http.Request) (*User, error) {
	s := session.From(r.Context())
	id, ok := auth.UserID(s)
	if !ok {
		return nil, nil
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		auth.Logout(s)
		return nil, nil
	}
	u, err := a.users.byID(r.Context(), n)
	if errors.Is(err, errNoUser) || err == nil && !auth.Current(s, u.PasswordHash) {
		auth.Logout(s)
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}
```

`auth.UserID` reads the ID the session keeps, the user is loaded, and
`auth.Current` checks that the login was made with the password the user
has now. A user who's gone, or who has set a new password since, is logged
out there and then, and the request goes on as a guest's. When the
database fails, there's no user and an error: `usersOnly` answers it with a
500, and `shareAuth` logs it and shows the page as a guest's.

### `auth.user` on every page

```go
pages.Share("auth", Auth{})
pages.ShareFunc(a.shareAuth)
```

`Auth` is a struct with one field, `User *User`, tagged `json:"user"`, and
`shareAuth` returns `inertia.Props{"auth": Auth{User: u}}` with `u` from
`user`. `ShareFunc` works it out for each page as it's rendered, error
pages included: the ErrorHandler renders a 404 outside any `usersOnly`, and
its `Layout` shows who's logged in too. `Share("auth", Auth{})` is a
guest's value, which `ShareFunc`'s replaces, and it's how `tug gen` learns
the prop's type, since it can't see what a function returns. In
`resources/js/tug/pages.ts`, `SharedProps` has `auth: Auth`, and `Auth` is
`{ user: User | null }`, so `usePage().props.auth.user` is typed in every
component, as in `Layout.tsx`. `User`'s password hash is tagged
`json:"-"`, so it never reaches a page. [Pages](pages.md#shared-props) has
more on shared props, and [TypeScript](typescript.md) on the types.

### Registering

`register` binds `RegisterInput` with `BindValid`. Its tags ask for a
name, an email, and a password of 8 to 200 characters, and two checks of
the handler's own follow: `confirmed`, that the password was typed the
same twice, and one that the email has no account. The register page
checks each field with the server as it's left, so a taken email shows
before the form is sent ([Forms](forms.md)). `users.add` stores the
user with `auth.HashPassword(in.Password)`. Its insert does nothing when
the email is taken, so two people registering the same email at once make
one account, and the second gets the same error. `auth.Login` logs the new
user in.

### Logging in and out

```go
key := strings.ToLower(in.Email) + "|" + clientIP(c.Request())
if wait := a.logins.Try(key); wait > 0 {
	return validate.Errors{"email": fmt.Sprintf("too many tries: wait %d seconds, and try again", int(math.Ceil(wait.Seconds())))}
}
u, err := a.users.byEmail(c.Context(), in.Email)
if err != nil && !errors.Is(err, errNoUser) {
	return err
}
hash := ""
if u != nil {
	hash = u.PasswordHash
}
if !auth.CheckPassword(hash, in.Password) {
	return validate.Errors{"email": "the email and password don't match an account"}
}
a.logins.Clear(key)
```

- Five tries a minute for an email from one address, and the next waits
  for the minute to be up. `a.logins` is an `auth.Throttle`, whose `Try`
  counts a try before the password is checked, so tries sent at the same
  moment count too; a login that works clears them.
- With no user, the password is checked against no hash, which takes as
  long as a real check, and the answer is a wrong password's: neither it
  nor its timing says which emails have accounts. A handler can return
  `validate.Errors` of its own, as here, and the form goes back with them.
- After the right password, a hash made with older settings is made again
  and stored (`auth.NeedsRehash`). `auth.Login` logs the session in, and
  the user goes to the page `auth.Intended` kept, or the dashboard.

`logout` calls `auth.Logout`, which empties the whole session, not only
the login, and `c.ClearHistory()`, which has the client throw away the
pages it kept for Back (see [Before going live](#before-going-live)), and
goes home with a flash message. It's a POST, from a
`<Link method="post" as="button">` in `Layout.tsx`, so `middleware.CSRF()`
turns it away from another site.

### A forgotten password

```go
if u != nil && a.resetMails.Try(u.Email) == 0 {
	link, err := a.resetLink(c, u)
	if err != nil {
		return err
	}
	a.sendLater(c.Context(), resetMail(u, link))
}
c.Flash("success", "If that email has an account, a link to reset its password is on its way.")
return c.RedirectRoute("password.request")
```

- The answer is the same for any email, so the form can't be asked who has
  an account.
- An account gets one mail a minute at most, so no one's inbox can be
  flooded from the form: `a.resetMails` is a `Throttle` with a `Max` of 1.
- `sendLater` sends from a goroutine, with a minute to do it, and logs a
  failure. The request doesn't wait: how long a mail server takes would
  say whether the email has an account.
- The link is `APP_URL`, then the path of `password.reset` with a token
  from `auth.Resets`, then the email in the query.

Only development, with `APP_DEBUG` on, can leave `APP_URL` out and take the
request's `Host` instead. `Host` is whatever the request says: someone
could ask for a reset of your email with their own site in it, and the mail
you get would link there, with a working token. So without `APP_DEBUG`,
`newApp` won't start without `APP_URL`.

### Resetting the password

```go
if u == nil || !a.resets.Check(in.Token, u.authID(), u.PasswordHash) {
	return validate.Errors{"email": "this link has expired, or been used"}
}
u.PasswordHash = auth.HashPassword(in.Password)
if err := a.users.setPassword(c.Context(), u.ID, u.PasswordHash); err != nil {
	return err
}
auth.Login(c.Session(), u.authID(), u.PasswordHash)
```

The link opens `Auth/ResetPassword` with the token and the email as props,
and the form sends them back with the new password. The token is checked
against the user's password hash as it is now. The new hash ends the
token, which was made for the old one, and every other session logged in
with the old password; this browser logs in again with the new one.

## Package auth

Package `auth` has no idea what a user is: the app finds its users, by ID
or by email, wherever it keeps them. It doesn't import tug. A login lives
in a `*session.Session`: `c.Session()` in a handler, and
`session.From(r.Context())` anywhere else.

### Passwords

```go
hash := auth.HashPassword(in.Password) // stored in place of the password

if !auth.CheckPassword(user.PasswordHash, in.Password) {
	// the wrong password
}
if auth.NeedsRehash(user.PasswordHash) {
	user.PasswordHash = auth.HashPassword(in.Password) // and stored
}
```

`HashPassword` uses argon2id at OWASP's settings, 19 MiB of memory, two
passes and one thread, about 30 ms on a server's core, with a salt of its
own. The hash is a PHC string, the format other argon2id libraries read
and write, such as PHP's `password_hash`:
`$argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>`. `CheckPassword` reads one
made with other settings too, and the tests check the reference
implementation's own test vector: users can move over from another app
with their hashes. `NeedsRehash` is true for a hash made with other
settings than `HashPassword`'s: hash the password again at the next login,
while the app has it. The new hash logs the user out of their other
sessions, as any new hash does.

Given an empty hash, as when no user has the email, `CheckPassword` checks
against a decoy at `HashPassword`'s settings, which takes as long, and
returns false. A hash that isn't argon2id's, such as bcrypt's, matches no
password, and logs a warning; so does one asking for more than a sensible
setting would, such as over 4 GiB of memory. At most one hash runs per CPU
at once, checks included, and the rest wait their turn: each takes 19 MiB,
and a burst of logins could otherwise take all the memory there is.

### Logins

```go
auth.Login(c.Session(), strconv.FormatInt(user.ID, 10), user.PasswordHash)

id, ok := auth.UserID(s) // "42", true, on the requests after
if !auth.Current(s, user.PasswordHash) {
	auth.Logout(s) // the password has changed since
}
```

- `Login(s, id, passwordHash)` keeps the ID in the session, under
  `tug.auth.id`, and under `tug.auth.check` a fingerprint of the password
  hash as it's stored: 12 bytes of SHA-256, which tell one hash from
  another and give nothing away should a cookie ever be read. It panics
  without a session: set tug's `Config.Session`.
- `UserID(s)` returns the ID, a string whatever the app's IDs are, and
  whether someone is logged in.
- `Current(s, passwordHash)` reports whether the login was made with the
  password hash the user has now. After a new password it's false for
  every session logged in with the old one, which is how a reset logs out
  the other browsers.
- `Logout(s)` empties the session, flash data and all: whoever uses the
  browser next shouldn't find anything kept for the last person.

Everything but `Login` takes a nil session, as in an app without
`Config.Session`, where no one is logged in.

### Back to the page after logging in

```go
auth.SetIntended(c.Session(), c.Request())                  // before sending a guest to log in
return c.Redirect(auth.Intended(c.Session(), "/dashboard")) // once they have
```

`SetIntended` keeps the request's path and query, for a GET only: a form
sent while logged out can't be sent again for them. `Intended` returns it,
and forgets it, or the fallback when there's none. It's always a path on
this site: `//evil.example`, `/\evil.example` and full URLs give the
fallback.

### Reset tokens

```go
keys, err := session.KeysFromEnv() // APP_KEY, then APP_PREVIOUS_KEYS
if err != nil {
	log.Fatal(err)
}
resets := &auth.Resets{Keys: keys}

token := resets.Token(id, user.PasswordHash)     // for the link
ok := resets.Check(token, id, user.PasswordHash) // false: expired, used, or not one of ours
```

A token is kept nowhere. It's the time it expires, and an HMAC-SHA256 over
that, the user's ID and their password hash, with a key for reset tokens
alone derived from the app's: there's no table, and nothing to clean up.
It works once, because resetting the password changes the hash it was made
for, and any other new password ends it too. It expires after `Lifetime`,
an hour by default. It's safe in a URL's path:
`tlwzi8.pIFKYhVaE1CzWqcFH-bjXw` is the expiry in base 36, and 16 bytes of
the MAC. The first of `Keys` signs, and each of them checks: with a new
`APP_KEY`, and the old one in `APP_PREVIOUS_KEYS`, links mailed before the
change still work. `Token` panics when `Keys` is empty.

### Throttle

```go
logins := &auth.Throttle{Max: 5, Window: time.Minute}

if wait := logins.Try(key); wait > 0 {
	// too many; try again in wait
}
// ... the try: a wrong password counts
logins.Clear(key) // after one that works
```

`Throttle` counts tries by a key, as `login` above counts them by email
and address. `Try` counts one and returns 0, until the key has had `Max`
(default 5) within `Window` (default a minute, from the first); then it
returns what's left of the window, and the try, which shouldn't be made,
doesn't count. Checking and counting are one step, so tries sent at the
same moment can't all get in under `Max`. `Wait` returns the same wait
without counting a try, and `Clear` forgets the key, as after a login that
succeeds. The zero value is ready to use; share one by pointer. The counts
are in memory, this process's own, and start again when it restarts.

## Package mail

```go
mailer, err := mail.FromEnv()
if err != nil {
	log.Fatal(err)
}
err = mailer.Send(ctx, mail.Message{
	To:      []string{user.Email},
	Subject: "Welcome to the blog",
	Text:    "Hi " + user.Name + ",\n\nThanks for registering.\n",
})
```

A `Message` has `From`, `To`, `Subject`, `Text`, and an optional `HTML`,
sent alongside `Text` for the mail programs that show it. An address is
`Ann Lee <ann@example.com>` or the address alone, and `From` defaults to
the `Mailer`'s. `Send` refuses a message to nobody, an address that
doesn't parse, and a subject with a line break in it, so a value from a
form can't add a header such as `Bcc`. A `Mailer` is anything with
`Send(ctx context.Context, m Message) error`.

`SMTP` sends through the server at its `Host` and `Port`, logging in with
its `Username` and `Password` when `Username` is set, and its `From` is who
mail is from when a message doesn't say.

- On port 465 the connection is TLS from the start. On any other port, 587
  by default, it's encrypted with STARTTLS when the server offers it.
- A password only goes over an encrypted connection, or to this machine
  (`localhost`, `127.0.0.1` or `::1`): to a server that doesn't offer
  STARTTLS, `Send` fails rather than send it in the clear. Without a
  `Username`, mail to such a server goes unencrypted.
- Connecting has 10 seconds, and the rest has until `ctx` is done, or a
  minute when `ctx` has no deadline, so a server that stops answering
  can't hold on to it.

`Log` writes mail out instead, for development: to its `W`, or the
standard error, which `tug dev` shows. It writes `Text` with each line
whole, so a link can be clicked, and leaves `HTML` out. It refuses the
messages `SMTP` would, but for one with no `From`, so a bad message is
found while developing.

`FromEnv` returns an `SMTP` when `MAIL_HOST` is set and a `Log` when it
isn't, from the variables Laravel uses:

| Variable                         | What it is                                       |
|----------------------------------|--------------------------------------------------|
| `MAIL_HOST`                      | the SMTP server; without it, mail is written out |
| `MAIL_PORT`                      | default 587; 465 for TLS from the start          |
| `MAIL_USERNAME`, `MAIL_PASSWORD` | for a server that wants them                     |
| `MAIL_FROM_ADDRESS`              | who mail is from; needed with `MAIL_HOST`        |
| `MAIL_FROM_NAME`                 | the name that goes with it                       |

There's no variable for the encryption: the port and the server decide
it. A `MAIL_PORT` that isn't a port, or a `MAIL_HOST` without
`MAIL_FROM_ADDRESS`, is an error, so the app stops at startup rather than
at its first mail.

### Testing mail

The starter's tests hand the app a `Mailer` that keeps what it's given:

```go
type outbox chan mail.Message

func (o outbox) Send(_ context.Context, m mail.Message) error {
	o <- m
	return nil
}
```

A test reads the mail from the channel, and gives up after five seconds:
the reset mail is sent from a goroutine, so it may not be there yet when
the request is answered.

## Before going live

- **`APP_URL` and the mail variables**: the app won't start without
  `APP_URL`, its address such as `https://example.com`, unless `APP_DEBUG`
  is on. It starts without `MAIL_HOST`, and its mail goes to the log rather
  than to people. [Deployment](deployment.md) has how to set them.
- **A secure cookie**: a login lives in the session's cookie. Behind a
  proxy that ends TLS, requests reach the app over plain HTTP, and the
  cookie isn't marked for HTTPS only unless `newApp`'s `session.Config`
  has `Secure: true`.
- **The client's address**: behind a proxy or load balancer, `clientIP` is
  the proxy's address, the same for everyone, so the login throttle counts
  everyone's wrong passwords for an email together: five from anyone, and
  its owner waits too. Once only the proxy can reach the app, have
  `clientIP` read the header the proxy sets; before that, anyone could
  write the header. [Deployment](deployment.md) has a `clientIP` for a
  proxy that adds to `X-Forwarded-For`, and the secure cookie.
- **Logins live in the cookie**, and the server keeps no list of them.
  Logging out rewrites that browser's cookie, but a copy taken before still
  works until the user sets a new password or the session expires. A
  session lasts two hours from its last request
  (`session.Config.Lifetime`), so a copy in use keeps itself alive. A new
  `APP_KEY`, without the old one in `APP_PREVIOUS_KEYS`, ends every session
  at once, and every reset link.
- **The browser's history**: Inertia keeps each page's props in it for
  Back. The starter has it encrypted, with `EncryptHistory: true` in
  `newApp`'s `inertia.Config`, and `logout` clears it, so after logging out
  Back fetches the page again, and gets the login page. The browser only
  encrypts for a page served over HTTPS, or from `localhost`: over plain
  HTTP elsewhere, Inertia's client keeps the history unencrypted, and says
  so in the console. See [History encryption](pages.md#history-encryption).
- **The throttle counts per process**: each instance of the app counts its
  own, and a restart forgets. It counts by email and address, so it slows
  guessing one account's password, not trying one password on many.
- **Registering says who has an account**: the register form turns away an
  email that has one, as the field is left, with no throttle. Logging in
  and a forgotten password don't say.
- **Mail in flight**: `app.Run` waits for requests when the app stops, and
  `sendLater`'s goroutine isn't one: a mail being sent then is lost.

### What the starter leaves out

- **Email verification**: nothing checks that an email is its owner's. It
  would take a column in `users.go`'s table, a link mailed as the reset
  link is, and a check in `usersOnly` for the pages that need it.
- **"Remember me"**: a login lasts as long as its session, and
  `session.Config.Lifetime` sets that for everyone. There's one cookie,
  and no longer-lived one for those who ask.
- **Changing the password while logged in**: a handler behind `usersOnly`
  that checks the current password with `auth.CheckPassword`, throttled as
  logging in is, stores `auth.HashPassword`'s hash, and calls `auth.Login`
  with it, as `resetPassword` does: the other sessions end, and this one
  goes on.
- **Changing the email**: an `UPDATE` in `users.go`, which the table's
  unique email turns away when the email is taken.
- **Deleting an account**: deleting the row, and `auth.Logout` for this
  browser. The user's other sessions end at their next request, when
  `user` finds no user.
