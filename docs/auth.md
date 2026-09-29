# Accounts

`tug new -auth` makes an app with accounts, complete enough to ship: people
register and verify their email, log in, with a code from an authenticator
app too once they turn that on, reset a forgotten password with a link sent
by email, and change their profile, email, photo, password and appearance
in their settings, or delete their account. The handlers are the app's own code, to
change as the app does. They stand on package `auth`, which has the parts
where a slip is a security hole, and package `mail`, which sends the links.
The frontend is React with Tailwind and shadcn/ui, as Laravel's React
starter kit has it, or with `-vue` or `-svelte`, Vue or Svelte with
shadcn's components for them, as Laravel's Vue and Svelte kits have them.

## The auth starter

```sh
tug new -auth blog
cd blog
tug dev
```

The app is the plain starter ([Getting started](getting-started.md))
with accounts, and `auth.user` on every page, error pages included: the
user who's logged in, or `null`. The users are in SQLite, through
`database/sql`, or with `-postgres` or `-mysql`, in Postgres or MySQL,
which the app's `compose.yaml` runs in development:

```sh
tug new -auth -postgres blog
cd blog
docker compose up -d
tug dev
```

The database is chosen as the app is made, as `laravel new` asks, and
the app's SQL is written for it: there's no layer between them, and no
variable to switch it later. The drivers are pure Go, modernc.org/sqlite,
pgx and go-sql-driver/mysql, so `tug build` still makes a static binary.
[The database](#the-database) has what differs between them.

| Route                                     | Name                                          | For |
|-------------------------------------------|-----------------------------------------------|-----|
| `GET /`                                   | `home`                                        | everyone |
| `GET /up`                                 |                                               | health checks |
| `GET /files/{key...}`                     |                                               | anyone with a signed link |
| `GET /dashboard`                          | `dashboard`                                   | verified users |
| `GET`, `POST /register`                   | `register`, `register.store`                  | guests |
| `GET`, `POST /login`                      | `login`, `login.store`                        | guests |
| `GET`, `POST /two-factor-challenge`       | `two-factor.login`, `two-factor.login.store`  | a login waiting for its code |
| `POST /logout`                            | `logout`                                      | users |
| `GET`, `POST /forgot-password`            | `password.request`, `password.email`          | guests |
| `GET /reset-password/{token}`             | `password.reset`                              | guests |
| `POST /reset-password`                    | `password.store`                              | guests |
| `GET`, `POST /confirm-password`           | `password.confirm`, `password.confirm.store`  | users |
| `GET`, `POST /verify-email`               | `verification.notice`, `verification.send`    | users |
| `GET /verify-email/{id}/{token}`          | `verification.verify`                         | anyone with the link |
| `GET /settings`                           | `settings`                                    | goes to the profile |
| `GET`, `PATCH`, `DELETE /settings/profile` | `profile.edit`, `profile.update`, `profile.destroy` | users, password confirmed |
| `POST`, `DELETE /settings/profile/photo`  | `profile.photo.update`, `profile.photo.destroy` | users, password confirmed |
| `GET /settings/security`                  | `security.edit`                               | verified users, password confirmed |
| `PUT /settings/password`                  | `user-password.update`                        | verified users, password confirmed |
| `POST`, `DELETE /settings/two-factor`     | `two-factor.enable`, `two-factor.disable`     | verified users, password confirmed |
| `POST /settings/two-factor/confirm`       | `two-factor.confirm`                          | verified users, password confirmed |
| `POST /settings/two-factor/recovery-codes` | `two-factor.recovery-codes`                  | verified users, password confirmed |
| `GET /settings/appearance`                | `appearance.edit`                             | users |

The names are Laravel's. A guest on a route for users is sent to log in,
and comes back after; someone logged in on a route for guests goes to the
dashboard.

`-auth` lays the auth starter over the plain one: its files replace the
plain starter's of the same name and add the rest, and the plain
`Layout.tsx` (`Layout.vue`, `Layout.svelte`) is left out. Then its
database's files: its SQL, which is all the Go that differs between
them.

- `main.go`: the environment, the database, and `newApp`, with the routes
  and the health check.
- `auth.go`: who's logged in, the wrappers that guard routes, registering,
  logging in and out, a forgotten password, and asking for the password
  again.
- `verify.go`: verifying an email. `twofactor.go`: two-factor logins.
  `settings.go`: the settings pages. `photos.go`: a user's photo, on the
  app's disk. `mail.go`: the mail the app sends.
- `jobs.go`: the background jobs, in tables of their own, for package
  `queue`, which runs them beside the server, and the `jobs` command,
  which lists the ones that failed for good and runs them again. The mail
  goes by jobs, and a job every night deletes the ones that failed over a
  month ago.
- `users.go`: `User`, and the `users` table. An email is unique whatever
  its case.
- The database's: `db.go`, which opens it, from the environment, and has
  the migrations that make its tables; `users_db.go`, `passkeys_db.go`,
  `jobs_db.go` and `throttles_db.go`, the SQL of each table; `db_test.go`,
  the tests' databases and the migrations' tests; and on Postgres or
  MySQL, `compose.yaml`, which runs the database in development.
- `main_test.go`, `auth_test.go`, `settings_test.go`, `twofactor_test.go`,
  `passkeys_test.go`, `photos_test.go`, `jobs_test.go`,
  `throttles_test.go`: a test of each flow, in browsers of
  package `tugtest`, with the mail kept in memory, the photos in a
  temporary directory, and a database of each test's own.
- `resources/js`: `inertia.tsx`, which makes the app and picks each
  page's layout; `layouts/`, the app's, the login card's, and the
  settings'; `components/`, the app's own and shadcn's in
  `components/ui`; and the pages, `Home`, `Dashboard` and `Error`, and
  those in `Auth/` and `Settings/`.
- `.env.example`: `APP_KEY`, `APP_URL`, `TRUSTED_PROXIES`, the
  database's `DB_PATH`, or `DB_URL`, `QUEUE_WORKERS`, the photos'
  `FILESYSTEM_DISK`, `FILES_PATH` and S3's variables, and the mail's,
  with what each is for. The `Dockerfile` keeps the photos in a `/data`
  volume, and SQLite's database with them.

### Trying it

`tug new` writes a `.env` with `APP_KEY` and `APP_DEBUG=true`, and on
Postgres or MySQL, `DB_URL`: the database `compose.yaml` runs, which
`docker compose up -d` starts. `tug dev` gives the app `APP_URL`, the
address it shows it at, so its links lead there; with no `MAIL_HOST`,
mail is written out rather than sent. Register, and the link that
verifies the email is in `tug dev`'s output, to click:

```
app  │ mail, not sent (MAIL_HOST isn't set):
app  │   From: (nobody)
app  │   To: ann@example.com
app  │   Subject: Verify your email for blog
app  │   ...
app  │   http://localhost:8080/verify-email/1/tlz8g9.TVcaDFHYjwXK12bVYHJQCw
```

A reset link comes the same way. To turn two-factor logins on, go to
Settings, then Security, and scan the QR code with an authenticator app on
a phone; to add a passkey, "Add a passkey" there, and the browser offers
the laptop's, the phone's, or a password manager. Passkeys work at
`http://localhost:8080`, where `tug dev` shows the app: browsers make them
for a domain, not an IP address such as `127.0.0.1`. SQLite's database is
`app.db`, or wherever `DB_PATH` says, and `.gitignore` leaves it out.

## How it works

### Who can see a page

```go
app.Get("/dashboard", a.usersOnly(verified(dashboard))).Name("dashboard")
app.Get("/settings/security", a.usersOnly(verified(passwordConfirmed(a.securityPage)))).Name("security.edit")
```

`dashboard` takes the user as well as the `Ctx`:
`func dashboard(c *tug.Ctx, user *User) error`, a `userHandler`.
`usersOnly` finds the request's user and hands it over, or sends a guest to
log in, keeping the page with `auth.SetIntended` for after. Routes take a
`tug.HandlerFunc`, and only `usersOnly` makes one of a `userHandler`, so a
handler that takes a user can't be routed without it: forgetting it
doesn't compile. That's why it's a wrapper rather than middleware, and why
the user is an argument rather than something to dig out of the request's
context.

Between them, a `userHandler` can be wrapped in more:

- `verified` sends a user whose email isn't verified to the page that asks
  them to verify it. The dashboard, and the security settings, have it.
  The profile doesn't, so that someone who typed their email wrong can put
  it right.
- `passwordConfirmed` sends a user who last typed their password over
  three hours ago to type it again, and back to the page after. It guards
  the settings that do the most harm in the hands of whoever finds a
  browser left logged in: the email, since who has it can reset the
  password; the password; two-factor logins; and deleting the account.
  Logging in counts as typing it, so right after, the settings open
  straight away.

`guestsOnly` wraps the pages for guests, and sends someone who's logged in
to the dashboard instead, even from a reset link.

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
its layout shows who's logged in too. `Share("auth", Auth{})` is a guest's
value, which `ShareFunc`'s replaces, and it's how `tug gen` learns the
prop's type, since it can't see what a function returns. In
`resources/js/tug/pages.ts`, `SharedProps` has `auth: Auth`, and `Auth` is
`{ user: User | null }`, so `usePage().props.auth.user` is typed in every
component, as in `layouts/app-layout.tsx`.

`User` has `emailVerifiedAt`, null until the email is verified, and
`twoFactor`, whether logging in takes a code. Its password hash, two-factor
secret and recovery codes are tagged `json:"-"`, so they never reach a
page. [Pages](pages.md#shared-props) has more on shared props, and
[TypeScript](typescript.md) on the types.

### Registering

`register` binds `RegisterInput` with `BindValid`. Its tags ask for a
name, an email, and a password of 8 to 200 characters, and two checks of
the handler's own follow: `confirmed`, that the password was typed the
same twice, and one that the email has no account. The register page
checks each field with the server as it's left, so a taken email shows
before the form is sent ([Forms](forms.md)). `users.add` stores the user
with `auth.HashPassword(in.Password)`. Its insert does nothing when the
email is taken, so two people registering the same email at once make one
account, and the second gets the same error.

The new user is logged in, with `auth.Login`, and has just typed their
password, so `auth.SetPasswordConfirmed` records that. A link to verify
the email is mailed to them, and they go to the dashboard, which, for a
user whose email isn't verified, is the page that asks them to follow the
link.

### Verifying an email

```go
// in a request, at registering, or at a new email
a.verifyMail.Push(c.Context(), VerifyMail{User: u.ID, Email: u.Email})

// in the job, as the mail goes
link, err := a.routes.AbsoluteURL("verification.verify", u.ID, a.verifications.Token(u.authID(), u.Email))
```

The mail goes by a job, which makes the link as it sends it, unless the
user has verified or changed their email since. The link is `APP_URL`,
then `/verify-email/{id}/{token}`, with a token from
`auth.Verifications`: signed for the user's ID and their email as it is,
and good for a day. Following it proves the email reaches its owner, so it
works in any browser, logged in or not, such as the phone the mail was read
on: `verifyEmail` checks the token against the user's email now, stores
`email_verified_at`, and goes to the dashboard, by way of the login page
for a guest, where "your email is verified" waits.

- A link for an email the user has changed since, or one tampered with,
  verifies nothing: the user goes to the page that asks them to verify,
  with a message saying the link has expired and to send another.
- "Send another link", on that page and on the profile, mails a new one,
  once a minute at most, and goes back to the page it was pressed on
  (`c.RedirectBack()`).
- A new email, from the profile, isn't verified until its own link is
  followed; the same email in another case stays verified. A password
  reset verifies the email too: the link came to it.

### Logging in and out

```go
key := strings.ToLower(in.Email) + "|" + c.IP()
wait, err := a.logins.Try(c.Context(), key)
if err != nil {
    return err
}
if wait > 0 {
    return validate.Errors{"email": tooMany(wait)}
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
if err := a.logins.Clear(c.Context(), key); err != nil {
    return err
}
```

- Five tries a minute for an email from one address, and the next waits
  for the minute to be up. `a.logins` is an `auth.Throttle`, whose `Try`
  counts a try before the password is checked, so tries sent at the same
  moment count too; a login that works clears them. The tries are counted
  in the database, in the `throttles` table, so every instance of the
  app counts them together, and a restart forgets none: see
  [Throttles](#throttles).
- With no user, the password is checked against no hash, which takes as
  long as a real check, and the answer is a wrong password's: neither it
  nor its timing says which emails have accounts. A handler can return
  `validate.Errors` of its own, as here, and the form goes back with them.
- After the right password, a hash made with older settings is made again
  and stored (`auth.NeedsRehash`).
- "Remember me" makes the session last a month without a visit, with
  `s.SetLifetime(rememberFor)`, where the rest last two hours. Each visit
  starts the month again. The box sends `"on"` when it's ticked, which
  `Bind` reads as `true` for `LoginInput.Remember`, a `bool`, as it reads a
  form's value ([Routing](routing.md#binding)).
- A user with two-factor logins on goes on to give a code (below).
  Everyone else is logged in with `auth.Login`, and goes to the page
  `auth.Intended` kept, or the dashboard.

`logout` calls `auth.Logout`, which empties the whole session, not only
the login, and `c.ClearHistory()`, which has the client throw away the
pages it kept for Back (see [Before going live](#before-going-live)), and
goes home with a flash message. It's a POST, from a
`<Link method="post" as="button">` in the user menu, so
`middleware.CSRF()` turns it away from another site.

### Two-factor logins

Once a user turns them on, logging in takes a code from an authenticator
app on their phone as well as the password. After the right password,
`login` holds the login back, with `auth.StartTwoFactor`, and sends them to
the challenge page. Meanwhile the session isn't logged in as anyone: it
keeps the user's ID for ten minutes, and the challenge takes either a code
or one of the user's recovery codes.

```go
secret, err := a.twoFactor.Open(u.TwoFactorSecret)
if err != nil {
    return false, err
}
step, ok := a.twoFactor.Check(secret, code, u.TwoFactorStep)
if !ok {
    return false, nil
}
return a.users.setTwoFactorStep(ctx, u.ID, step)
```

- A code works once. `Check` takes the time step of the code that last
  worked, `two_factor_step`, and takes no code from then or before; the
  update stores the new step only when it's later than the one stored, so
  of two logins with the same code at the same moment, one gets in. A code
  seen over the user's shoulder is no use.
- Five tries a minute for a login, counted before the code is checked, as
  the password's are: a million codes can't be tried one by one.
- A recovery code works once too, whatever its case, and with or without
  its dash. Using one stores the codes without it, only if no one else has
  changed them since.
- A password reset doesn't let a user with two-factor logins on past the
  code: the link in their mail is one factor, not two, so after the reset
  they go to the challenge, and log in once they give a code.

Turning them on is in the security settings. "Turn on" makes a secret,
which the session keeps while the user sets their app up, and the page
shows it as a QR code of its `otpauth://` URL, and as text to type in. The
user types the code the app shows, which shows it's set up, and only then
does the database get the secret, sealed, and eight new recovery codes,
sealed too. The page shows the codes once, from the flash, for the user
to keep somewhere safe; "Show my recovery codes" fetches them again with a
partial reload of `recoveryCodes`, an `inertia.Optional` prop that no other
visit carries, and "Make new codes" replaces them. "Turn off" clears the
secret and the codes. All of it is behind `passwordConfirmed`.

The secret and the codes are sealed with `auth.TwoFactor`: encrypted with a
key derived from `APP_KEY`, so a copy of the database, such as a leaked
backup, doesn't give away the second factor along with the first. After
`APP_KEY` changes, with the old key in `APP_PREVIOUS_KEYS`, `newApp` seals
every user's again with the new one as the app starts (`resealTwoFactor`),
so the old key can go without taking anyone's second factor with it.

### Passkeys

A passkey logs in with no email and no password: the user's phone, laptop
or password manager keeps it, and unlocks it with their PIN, fingerprint
or face. That's two factors, the device and what unlocks it, so a login
with a passkey asks for no code from the phone, and counts as the
password typed for `passwordConfirmed`. The handlers are in `passkeys.go`,
on `auth.Passkeys`, which checks what the browser answers.

- **Adding one** is on the security page, behind `passwordConfirmed`: a
  name for it, then the browser asks where to keep it. The page fetches
  the options, `POST /settings/passkeys/options`, which the user's first
  passkey makes a handle for, a random one the authenticator keeps, never
  their ID or email; the browser makes the passkey, and the page posts its
  answer to `POST /settings/passkeys`, which keeps it in the `passkeys`
  table, and mails the user, "A passkey was added to your account", as a
  new way in is one its owner should hear of. An account has ten at most:
  confirming it's the user with one keeps the IDs of all of theirs in the
  session, which has to fit in its cookie.
- **Logging in with one** is the login page's "Log in with a passkey", and
  the email field's autofill, which offers the user's passkeys as it's
  focused, in the browsers that do: `autocomplete="username webauthn"`,
  and a request that waits in the background. The passkey says whose
  login it is. Five tries a minute from an address, as with passwords;
  "Remember me" counts too.
- **Confirming it's them** takes a passkey as well as the password: the
  page that asks has "Use a passkey", for a user who has one.
- **Removing one** is its bin on the security page. Deleting the account
  deletes its passkeys.

The browser's side is `resources/js/lib/passkeys.ts`: the options, as
JSON, into what `navigator.credentials` takes, and its answer back into
JSON. A passkey is for the site's domain, from `APP_URL`, as the links in
mail are: under `tug dev`, `localhost`.

### Asking for the password again

`passwordConfirmed` asks for the password when the user last typed it more
than `confirmFor`, three hours, ago: at logging in, registering, a reset,
a change of password, or on the page that asks for it,
`Auth/ConfirmPassword`. That page's form checks the password, five tries a
minute, with `auth.SetPasswordConfirmed` after the right one, and goes back
to the page the user was on their way to. A form sent after the three
hours ran out, such as the profile's left open all afternoon, can't be
sent again for them: they go back to the page it was on, which
`auth.SetIntended` keeps from the form's `Referer`, and send it again.

### Settings

- **Profile**: the name and email. A new email is mailed a link, and isn't
  verified until it's followed; one another account has is turned away.
- **Photo**: a PNG, JPEG or WebP of 2 MB at most, told by its bytes, which
  the user menu shows in place of the initials, uploaded with its progress
  shown. `photos.go` keeps it on the app's disk with package `storage`:
  `files/`, or `FILES_PATH`, or with `FILESYSTEM_DISK=s3`, a bucket. The
  photos are private: `a.user` gives each page's user a link to theirs,
  signed until the end of the next day, which is the same link all day so
  the browser keeps it cached, and served at `/files` by that link alone.
  A new photo replaces the old, and removing it brings the initials back;
  the file that nothing names any more is deleted by a job, `delete-file`,
  pushed in the transaction that drops it. See [Files](files.md).
- **Delete your account**: a dialog that takes the password once more,
  since it can't be undone. The row goes, and its photo with it, this
  browser is logged out and its history cleared, and the user's other
  sessions end at their next request, which finds no user.
- **Security**: a new password, which takes the current one. The new hash
  ends every login made with the old one, which is how to end a login on a
  lost laptop, and this browser logs in again with it. Then two-factor
  logins, above.
- **Appearance**: light, dark, or as the system is. It's the browser's
  choice, kept in its `localStorage`, rather than the account's: the
  root template's script applies it before the page paints, so there's no
  flash of the wrong one, and `hooks/use-appearance.ts` keeps it from then
  on, as the system's changes too.

Password checks in the settings, as in the page that asks for it again,
count five tries a minute for a user (`a.passwords`), so whoever has
someone's browser can guess no faster than at the login page.

### A forgotten password

```go
wait, err := a.resetAsks.Try(c.Context(), c.IP())
if err == nil && wait == 0 {
    wait, err = a.mails.Try(c.Context(), "reset|"+strings.ToLower(in.Email))
}
if err != nil {
    return err
}
if wait == 0 {
    if err := a.resetMail.Push(c.Context(), ResetMail{Email: in.Email}); err != nil {
        return err
    }
}
c.Flash("success", "If that email has an account, a link to reset its password is on its way.")
return c.RedirectRoute("password.request")
```

- The answer is the same for any email, and so is the work behind it: the
  request pushes a `ResetMail` job whatever the email, and the job looks it
  up. How long the form takes to answer can't say who has an account.
- An email gets one mail a minute at most, so no one's inbox can be
  flooded from the form: `a.mails` is a `Throttle` with a `Max` of 1, for
  reset links by email and verification links by user. Each ask is a job
  in the database, so an address has five a minute, in `a.resetAsks`.
- The job, `mailReset`, finds the account, makes the token from its
  password hash as it is then, and sends the mail, so no token waits in
  the database. A mail that doesn't go runs again later.
- The link is `APP_URL`, then the path of `password.reset` with a token
  from `auth.Resets`, then the email in the query.

The links are never made from the request's `Host`, which is whatever
the request says: someone could ask for a reset of your email with their
own site in it, and the mail you get would link there, with a working
token. So `newApp` won't start without `APP_URL`, which `tug dev` sets in
development.

The link opens `Auth/ResetPassword` with the token and the email as props,
and the form sends them back with the new password. The token is checked
against the user's password hash as it is now. The new hash ends the
token, which was made for the old one, and every other session logged in
with the old password. The email is verified, as the link came to it, and
this browser logs in with the new password, or, with two-factor logins on,
goes on to give a code.

### Mail

`mail.go` has the app's mail: a `message` with a greeting, what it's
about, a link, and what to do if it wasn't the user. `message.mail()`
writes it as plain text, and as HTML with the link as a button, from an
`html/template` with its style inline, as mail programs take it, and with
what the user typed, such as their name, escaped.

The mail goes by jobs, which `jobs.go` keeps in the database: a
`VerifyMail` or a `ResetMail` names the user, or the email, and the job
makes the mail, link and token, as it runs, the link from `APP_URL` with
`a.routes.AbsoluteURL`. So a mail server that's down, or the app
restarting, loses no mail: a job that fails runs again, 10 times over
about four hours, and then stays in the table as failed, with its error,
for a month: a scheduled job, `prune-jobs`, deletes older ones every
night. `main` runs the jobs beside the server with `app.Go`.
[jobs.md](jobs.md) has the queue.

A mail about what a handler writes is pushed in the same transaction:
the account and the mail that verifies its email, a passkey and the mail
that tells its owner of it, a new email and its link. `a.inTx` runs the
handler's writes in one transaction, with the stores made from it, as
`a.users.in(tx)`, and the jobs pushed with `Kind.In(a.jobs.in(tx))`,
commits it, and wakes the queue: the two are kept together, or neither
is, so no account waits for a link that was never pushed, and no passkey
is added that its owner isn't told of. A transaction holds what it
writes until it ends, and in SQLite, which has one writer, the lock for
writing from its start, so what the handler writes goes through `tx`,
and slow work, such as hashing a password, comes before it.

The binary has a command, which `newApp` adds with `app.Command`, and
which runs in place of the server, on the same database, and exits:
`./blog jobs` lists the jobs that failed for good, with their errors, and
`./blog jobs retry 42`, or `retry all`, runs them again. In the image,
it's `docker exec <container> /server jobs`. `./blog help` lists the
commands, for an app that adds more.

### The database

`db.go` opens the database the environment names, `DB_PATH` for SQLite
and `DB_URL` for the others, and runs the migrations it hasn't had, in
order, before the app serves:

```go
var migrations = []string{
    `CREATE TABLE users (...)`,
    `CREATE TABLE jobs (...); CREATE INDEX jobs_due ...`,
}
```

To change the tables, add a step at the end, such as an `ALTER TABLE`,
and never change one that has run somewhere, since it won't run there
again. A database from a newer build of the app, as after a deploy is
rolled back, is left as it is. Instances starting at once take turns,
each database in its own way:

- **SQLite** counts the steps in `PRAGMA user_version`, and runs each in a
  transaction with its count, so one that fails leaves no trace. The
  connection's `_txlock=immediate` takes the lock for writing as a
  transaction begins, so two starts at once take turns rather than fail.
- **Postgres** counts them in a table, `schema_version`, and runs each in
  a transaction with its count too, which locks the count's row as it
  reads it: of instances starting at once, one runs the step, and the
  others wait, then find it counted.
- **MySQL** commits a statement that changes a table as it runs, not with
  the rest of a transaction, so each step is one statement, which MySQL 8
  does whole or not at all, and `schema_version` counts it after. An
  instance holds `GET_LOCK` on one connection while it runs the steps. A
  crash in the instant between a step and its count leaves the step run
  and uncounted, which the next start can't tell from one that never ran:
  `schema_version`'s `running` names it, and the app stops, saying which,
  for someone to see whether it ran, rather than guess. Laravel's
  migrations have the same gap on MySQL.

The SQL of each table is in a file of its own, `users_db.go`,
`passkeys_db.go`, `jobs_db.go`, `throttles_db.go` and `cache_db.go`,
beside the Go that's the same on any database, and it's written for its database, where
they differ:

| | SQLite | Postgres | MySQL |
|---|---|---|---|
| An email, whatever its case | a column `COLLATE NOCASE` | a unique index on `lower(email)`, which the queries compare with | a column `COLLATE utf8mb4_0900_as_ci` |
| The row a write made | `RETURNING` | `RETURNING` | read back, by the result's `LastInsertId` |
| A write a unique index turns away | `ON CONFLICT DO NOTHING`, `UPDATE OR IGNORE` | `ON CONFLICT DO NOTHING`, or the error, which ends the transaction | the error, 1062, after which a transaction goes on |
| Times | `DATETIME`, in UTC | `timestamptz` | `DATETIME(6)`, in UTC, the connection's time zone |
| Placeholders | `?` | `$1` | `?` |

MySQL's email compares with `utf8mb4_0900_as_ci`: whatever its case, as
SQLite's `NOCASE`, but not whatever its accents, as MySQL's default,
`utf8mb4_0900_ai_ci`, would, which makes `ann@example.com` and
`ánn@example.com` one account. Its other strings compare as they are,
with `utf8mb4_0900_bin`. Its connections read what's committed at each
statement, as Postgres's do, rather than what was as the transaction
began: `READ COMMITTED`, which the driver sets as each connects, with
the time zone. And MySQL sets an `UPDATE`'s columns in order, each from
the row as the ones before it left it, so the one that reads the old
email comes first.

The starter is tested on Postgres 18 and MySQL 8.4, the older of MySQL's
two long-term releases, 9.7 being the newer. MariaDB isn't: its
collations aren't MySQL's, and an app on it picks its email column's.

An app that moves to another database takes that database's files from
an app `tug new` makes on it: `db.go`, the `_db.go` files,
`db_test.go` and `compose.yaml`. `go mod tidy` adds the driver, the app
is given `DB_URL`, or `DB_PATH`, where it runs, and the data it moves
itself.

### Throttles

The starter's five throttles count in the database, in a `throttles`
table, whose SQL is in `throttles_db.go`: every instance of the app
counts the tries made at any of them, and a restart, as each deploy is,
forgets none.

```go
counts := &throttles{db: e.DB}
a := &app{
    logins: &auth.Throttle{Name: "logins", Max: 5, Window: time.Minute, Store: counts},
    ...
}
```

| Throttle     | Counts                                                                    | By |
|--------------|---------------------------------------------------------------------------|----|
| `logins`     | logins with a password, and with a passkey, five a minute                 | the email and the address, or the address |
| `passwords`  | the password of someone logged in, as they confirm, change it or delete their account, five a minute | the user |
| `codes`      | the codes of a login waiting for its second factor, five a minute         | the user |
| `mails`      | reset links and verification links, one a minute                          | the email, or the user |
| `reset-asks` | asks for a reset link, five a minute                                      | the address |

A row is a key's tries, and when their window ends. The key is the
SHA-256 package `auth` makes of the throttle's name and what it counts
by, so the table keeps no email or address, and the `passwords` and
`codes` of one user count apart. A try is one statement, an upsert that
counts one more, or starts a new window where the last is over, and reads
back the count: of tries at once, from any of the instances, each gets a
count of its own, and five get in. MySQL has no `RETURNING`, so there
it's a transaction that reads back what the upsert wrote. Every hour, a
scheduled job, `prune-throttles`, deletes the rows whose window is over.

A throttle that can't count, as when the database is down, has an error
for the handler to return, as any of the database's: the login is a 500,
rather than let in unthrottled, or turned away as though it had tried too
often.

### The cache

The starter gives the app a cache, `a.cache`, for what's slow to work
out, and for locks that hold across its instances: package `cache`
([Cache](cache.md)). It's in the database too, in a `cache` table, whose
SQL is in `cache_db.go`, and every hour a scheduled job, `prune-cache`,
deletes what has expired. The starter keeps nothing in it itself.

### Health checks

`GET /up` answers 200 `up` while the app is serving and its database
answers a ping, and a 503 when it doesn't, for a load balancer or a
platform's health check. Like every request, it's in the request log.

### The frontend

`resources/js` is React with Tailwind and shadcn/ui's components, on
Radix, and lucide's icons. `components.json` is shadcn's, so
`npx shadcn@latest add table` adds a component to `components/ui` that
fits the rest. The theme is in `resources/css/app.css`: colours as
variables, one set for light and one for dark.

`inertia.tsx` picks each page's layout by its name:
`layouts/auth-layout.tsx`, the card around a form, for `Auth/...`;
`layouts/settings-layout.tsx` inside `layouts/app-layout.tsx` for
`Settings/...`; `app-layout` for the rest; and none for `Home`, the
landing page. A page names its card's title with a static `layout`, as
`Login.layout = { title: 'Log in', ... }`.

What a handler flashes with `c.Flash("success", ...)` or `"error"` shows
as a toast, with sonner. `app.tsx` listens for Inertia's `flash` event from
the start, so the flash that comes with the first page, as after following
a link in the app's mail, shows too. `types.ts` types the flash, with the
recovery codes as well.

With `-vue`, `resources/js` is the same app in Vue: shadcn-vue's
components, on Reka UI, `@lucide/vue`'s icons and vue-sonner's toasts,
and `npx shadcn-vue@latest add` for more. With `-svelte`, it's Svelte 5:
shadcn-svelte's components, on Bits UI, in the classic style that
shadcn/ui's have in React, `@lucide/svelte`'s icons and svelte-sonner's
toasts. shadcn-svelte's CLI adds its newer styles unless it's pointed at
the classic registry, so more are added with
`COMPONENTS_REGISTRY_URL=https://shadcn-svelte.com/registry npx shadcn-svelte@latest add`.

The pages have the same names, props and words in each, but for the
frontend's own name and files where the landing page and the dashboard
say them, so the Go and its tests are the same whichever it is, and
`lib/passkeys.ts`, `types.ts` and `app.css` are the same files. A Vue page
names its card's title with
`defineOptions({ layout: { title: 'Log in', ... } })`, and a Svelte page
with `export const layout = { title: 'Log in', ... }` in its
`<script module>`. Neither Inertia has a place for the toasts at the root
of the app, as React's `withApp` is, so `app.ts` mounts them on their own
beside it, in the browser, where they outlive each page.

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
  another and give nothing away should a cookie ever be read. A login
  starts afresh: a password confirmed before it, or a login waiting for
  its second factor, is forgotten. It panics without a session: set tug's
  `Config.Session`.
- `UserID(s)` returns the ID, a string whatever the app's IDs are, and
  whether someone is logged in.
- `Current(s, passwordHash)` reports whether the login was made with the
  password hash the user has now. After a new password it's false for
  every session logged in with the old one, which is how a reset logs out
  the other browsers.
- `Logout(s)` empties the session, flash data and all: whoever uses the
  browser next shouldn't find anything kept for the last person.

Everything but `Login` and `StartTwoFactor` takes a nil session, as in an
app without `Config.Session`, where no one is logged in.

### Back to the page after logging in

```go
auth.SetIntended(c.Session(), c.Request())                  // before sending a guest to log in
return c.Redirect(auth.Intended(c.Session(), "/dashboard")) // once they have
```

`SetIntended` keeps the request's path and query. A form sent while logged
out, or after a confirmed password ran out, can't be sent again for them,
so for a request other than a GET it keeps the page the form was on
instead, by its `Referer`, when that's a page of this site. `Intended`
returns it, and forgets it, or the fallback when there's none. It's always
a path on this site: `//evil.example`, `/\evil.example` and full URLs give
the fallback.

### Asking for the password again

```go
if !auth.PasswordConfirmed(c.Session(), 3*time.Hour) {
    auth.SetIntended(c.Session(), c.Request())
    return c.RedirectRoute("password.confirm")
}
// ... and on the page that asks, after auth.CheckPassword:
auth.SetPasswordConfirmed(c.Session())
```

`SetPasswordConfirmed` records in the session when the user last typed
their password, and `PasswordConfirmed(s, d)` reports whether that was
within `d`. `Login` forgets it, so the app sets it after a login that took
the password too. The time is in the cookie with the rest, so an old copy
of a cookie is only as confirmed as it was when the password was typed.

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

### Verification tokens

```go
verifications := &auth.Verifications{Keys: keys}

token := verifications.Token(id, user.Email)     // for the link
ok := verifications.Check(token, id, user.Email) // false: expired, for another email, or not one of ours
```

`Verifications` makes tokens as `Resets` does, signed for the user's ID and
their email, and with a key for verification alone, so a reset token is no
good as a verification token, nor the other way round. A token stops
working once the email changes, and after `Lifetime`, a day by default. It
can be used again until then, which does no harm: the email is verified
already.

### Two-factor logins

```go
tf := &auth.TwoFactor{Keys: keys, Issuer: "Blog"}

secret := tf.NewSecret()                 // 160 random bits, in base32
url := tf.URL(user.Email, secret)        // otpauth://totp/Blog:ann@example.com?issuer=Blog&secret=...
step, ok := tf.Check(secret, code, last) // the code the app shows, and not one used before
sealed := tf.Seal(secret)                // for storing; tf.Open(sealed) gets it back

codes := auth.NewRecoveryCodes()             // eight, as "k3p9x-m2w7q"
rest, ok := auth.UseRecoveryCode(codes, typed) // store rest: a code works once
```

- The codes are TOTP's, RFC 6238, as authenticator apps make them: six
  digits of HMAC-SHA1 over the number of 30-second steps since 1970.
  `Check` takes the code of the step now, or of the one either side for a
  phone whose clock is off, but none from the step `after` or before it,
  and returns the step of the one that matched, for the app to store in
  its place. The tests check RFC 6238's own vectors.
- `URL` is what the QR code holds, which apps scan: the `Issuer`, the
  account, and the secret.
- `Seal` encrypts a secret, or a user's recovery codes, with AES-256-GCM
  and a key for two-factor logins alone derived from the first of `Keys`;
  `Open` decrypts with any of them. `Stale` reports whether a value was
  sealed with a key other than the first: seal it again while the old key
  is still among `Keys`, as the starter does as it starts.
- `Code(secret, t)` is the code an app shows at `t`, for an app's tests,
  which log in as a user with two-factor logins on.
- `NewRecoveryCodes` makes eight codes of ten random letters and digits,
  50 bits each. `UseRecoveryCode` compares in constant time, ignoring case,
  spaces and the dash.

```go
auth.StartTwoFactor(s, id) // the password was right; a code comes next
...
id, ok := auth.TwoFactorPending(s) // on the page that asks for the code
```

`StartTwoFactor` holds back the login of a user whose password was right:
the session isn't logged in, and keeps the ID for `TwoFactorPending` for
ten minutes. `Login` ends the wait, once the code checks out.

### Passkeys

```go
site := &auth.Passkeys{RPID: "example.com", Origin: "https://example.com", Name: "Blog"}

// Adding one: the options for navigator.credentials.create, as JSON...
options, err := site.StartRegistration(s, auth.PasskeyUser{Handle: handle, Name: u.Email, DisplayName: u.Name}, kept)
// ...and, from the next request, the browser's answer, as JSON.
pk, err := site.FinishRegistration(s, answer) // keep pk with the user

// Logging in: options for navigator.credentials.get, any passkey of the site's...
options, err = site.StartLogin(s, nil)
// ...and the answer, checked against the passkey the app has with its ID.
pk, err = site.FinishLogin(s, answer, func(id []byte) (auth.Passkey, error) { ... })
```

`Passkeys` makes the options a browser needs to make a passkey, or log in
with one, keeps their challenge in the session, and checks the answer,
which comes back in the next request. It knows no users: the app keeps
each passkey with its user, as `FinishRegistration` returns it, and after
each login as `FinishLogin` returns it, with its new count.

- **The options** ask for a passkey the authenticator keeps, so a login
  needs no email, and unlocks each time, so a passkey is two factors on
  its own. A new passkey's user `Handle` is `NewPasskeyHandle`'s random
  bytes, the same for each of the user's passkeys.
- **A challenge** is 32 random bytes, in the session, answered once,
  rightly or not, within five minutes. `Login` forgets one it finds.
- **An answer** has to be to the challenge, for the ceremony it was asked
  for, from the site's `Origin` exactly, for its `RPID`, with the user
  there and the device unlocked. A login's signature has to check out with
  the passkey's key, the passkey has to be the user's the answer names,
  and its count has to go on from the count kept: one that goes back has
  been copied. A passkey that syncs, as a password manager's does, counts
  nothing, and keeps 0.
- **Keys** are ES256 on P-256, Ed25519, or RSA of 2048 bits or more, as
  COSE keys; a point off its curve, or any other key, is refused. The
  attestation, which says what made a passkey, isn't checked, as the
  options ask for none: any make of authenticator will do.
- **CBOR**, which an authenticator's data is in, is read by a small, strict
  reader of what WebAuthn writes, fuzzed so that no answer can crash it.

Package `auth/passkeytest` is a passkey authenticator in software, for
tests: `passkeytest.New(origin)` makes one, whose `Create` and `Get`
answer the options as a browser does, with real keys and real
signatures. `Clone` makes a copy, which a site that checks counts turns
away; `Synced`, `Unverified`, `Alg` and `Origin` make the others a test
needs.

### Throttle

```go
logins := &auth.Throttle{Name: "logins", Max: 5, Window: time.Minute, Store: counts}

wait, err := logins.Try(ctx, key)
if err != nil {
    return err
}
if wait > 0 {
    // too many; try again in wait
}
// ... the try: a wrong password counts
err = logins.Clear(ctx, key) // after one that works
```

`Throttle` counts tries by a key, as `login` above counts them by email
and address. `Try` counts one and returns 0, until the key has had `Max`
(default 5) within `Window` (default a minute, from the first); then it
returns what's left of the window, and the try shouldn't be made. Checking
and counting are one step, so tries sent at the same moment can't all get
in under `Max`. `Wait` returns the same wait without counting a try, and
`Clear` forgets the key, as after a login that succeeds. The zero value is
ready to use; share one by pointer.

Without a `Store`, the counts are in memory, this process's own, and
start again when it restarts. A `ThrottleStore` keeps them where every
instance of the app counts, as the starter's table does:

```go
type ThrottleStore interface {
    Hit(ctx context.Context, key []byte, now time.Time, window time.Duration) (tries int, ends time.Time, err error)
    Tries(ctx context.Context, key []byte, now time.Time) (tries int, ends time.Time, err error)
    Clear(ctx context.Context, key []byte) error
}
```

- `Hit` counts a try and returns the tries in the key's window, this one
  included, and when the window ends, starting one at `now` when the
  key's has ended, all in one step: of tries at once, each gets a count of
  its own. A try turned away counts too, and changes nothing that shows,
  as the window's end doesn't move with it.
- `Tries` reads a key's count without adding to it, for `Wait`, and
  `Clear` forgets it.
- A key is 32 bytes, the SHA-256 of the throttle's `Name` and the key it
  counts by: a store keeps no email or address, and two throttles on one
  store count apart, as the starter's `passwords` and `codes` do, which
  both count by user. The times are the app's clock, which the `Throttle`
  passes in.
- A store's error is `Try`'s, `Wait`'s and `Clear`'s: a try that can't be
  counted is the caller's to answer, rather than the throttle's to let in,
  and stop nothing while its store is down, or turn away, and lock
  everyone out when a database blinks.

Package `throttletest` checks that a store keeps these promises: a
store's tests run `throttletest.TestStore`, with a new, empty store for
each, as the starter's do on its table in each database.

A throttle on a route, rather than in a handler, is `tug.Limit`
([Routing](routing.md#a-limit-for-a-route)).

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

### Testing

The starter's tests hand the app a `Mailer` that keeps what it's given,
and can be down for a number of mails, as a mail server can:

```go
type outbox struct {
    sent chan mail.Message
    down atomic.Int32
}

func (o *outbox) Send(ctx context.Context, m mail.Message) error {
    if o.down.Add(-1) >= 0 {
        return errors.New("dial tcp: connection refused")
    }
    ...
}
```

A test reads the mail from the channel, and gives up after five seconds:
a job sends it, so it may not be there yet when the request is answered.
Each test app runs its own queue on its own database, asking for jobs
every 10 milliseconds, so a mail that failed goes again as soon as its
wait is over. `jobs_test.go` runs `queuetest.TestStore` on the `jobs`
table.

On SQLite, a test's database is a file in its temporary directory. On
Postgres or MySQL, `db_test.go` makes one for each test on the server
`DB_URL` names, or else on `compose.yaml`'s, and drops it after:

```sh
docker compose up -d
go test ./...
```

With no server to make them on, every test fails, saying so, where
skipping would pass with nothing tested. The tests don't read `.env`: a
`DB_URL` for them is set where they run, as CI sets it for a database of
its own.

The tests log in as a user with two-factor logins on with
`auth.TwoFactor.Code`, a code for now and one for 30 seconds on, since a
code works once.

Each browser in a test is a `tugtest.Client` ([Testing](testing.md)), so
two logins to one account are two clients. `verifiedUser` registers a
user, follows the link mailed to them, and returns the browser they did
it in. `oldLogin` returns one logged in long ago, with no password typed
since, made with `Client.Session` and a `session.Store` with the test's
key. The shared `auth.user` reads into the app's own `User`, with
`tugtest.Prop[*User](r, "auth.user")`, and a page's props into its own
struct, `tugtest.Props(r, Security)`. `passkeys_test.go` adds passkeys and
logs in with them from a `passkeytest.Authenticator`, as a phone would.

## Before going live

- **`APP_URL` and the mail variables**: the app won't start without
  `APP_URL`, its address such as `https://example.com`. It starts without
  `MAIL_HOST`, and its mail goes to the log rather than to people,
  verification links included, so no one can verify their email. [Deployment](deployment.md) has how to set them.
- **A secure cookie**: a login lives in the session's cookie. An `APP_URL`
  of `https://` marks it for HTTPS only, which behind a proxy that ends
  TLS, where requests reach the app over plain HTTP, it otherwise wouldn't
  be.
- **The client's address**: behind a proxy or load balancer, a request
  comes from the proxy's address, the same for everyone, so without
  `TRUSTED_PROXIES` the login throttle counts everyone's wrong passwords
  for an email together: five from anyone, and its owner waits too. Name
  the proxies in it, and `c.IP()` is the client's, read from their
  `X-Forwarded-For`. [Deployment](deployment.md#behind-a-proxy) has how.
- **Logins live in the cookie**, and the server keeps no list of them.
  Logging out rewrites that browser's cookie, but a copy taken before still
  works until the user sets a new password or the session expires: two
  hours from its last request, or a month with "Remember me", and a copy
  in use keeps itself alive. Changing the password, in the security
  settings, ends every other login. A new `APP_KEY`, without the old one in
  `APP_PREVIOUS_KEYS`, ends every session at once, and every link mailed.
- **Rotating `APP_KEY`**: keep the old key in `APP_PREVIOUS_KEYS` for a
  month, as long as a remembered login lasts. The two-factor secrets move
  to the new key as the app starts, and the sessions as they're used.
- **The browser's history**: Inertia keeps each page's props in it for
  Back. The starter has it encrypted, with `EncryptHistory: true` in
  `newApp`'s `inertia.Config`, and `logout` clears it, so after logging out
  Back fetches the page again, and gets the login page. The browser only
  encrypts for a page served over HTTPS, or from `localhost`: over plain
  HTTP elsewhere, Inertia's client keeps the history unencrypted, and says
  so in the console. See [History encryption](pages.md#history-encryption).
- **The login's throttle counts by email and address**, so it slows
  guessing one account's password, not trying one password on many
  accounts, which takes a limit by address alone, as `tug.Limit` puts on a
  route ([Routing](routing.md#a-limit-for-a-route)).
- **Registering says who has an account**: the register form turns away an
  email that has one, as the field is left, with no throttle, and so does
  the profile. Logging in and a forgotten password don't say.
- **Passwords**: 8 to 200 characters, and nothing else is asked of them.
  Laravel's starter also asks, in production, for mixed case, numbers and
  symbols, and checks the password against those leaked in breaches.
- **Guessing a code**: five tries a minute, as Laravel's starter allows,
  is 7,200 a day, and each has about three in a million to be right, so
  someone with the password could get past the code in a few weeks of
  trying. Counting failed codes over a longer window as well, in
  `twoFactorChallenge`, stops that, at the price of letting whoever has
  the password lock its owner out for as long. Codes that keep failing
  after the right password mean someone else knows it: mailing the user
  then is worth doing.
- **Mail that keeps failing**: a mail job runs 10 times over about four
  hours, then stays in the `jobs` table as failed for a month, with an
  error in the log. `./blog jobs` lists the jobs that failed, and runs them
  again: see [jobs.md](jobs.md). And now and then a mail goes twice: a job
  runs at least once, and again when the app is killed as it sends.
- **Where the photos are kept**: in `files/` beside the binary, or
  `FILES_PATH`, which the image sets to `/data/files`, on its volume, so
  they outlive the container. Instances of the app on more than one
  machine don't share a directory: give them `FILESYSTEM_DISK=s3` and a
  bucket, which all of them reach. A photo's links are signed with
  `APP_KEY`, and checked with `APP_PREVIOUS_KEYS` too, so the ones on
  pages already open still work after a new key.
- **Where the database is**: SQLite is a file, on the image's volume, and
  its instances share one machine. On Postgres or MySQL, `DB_URL` names
  the database, with the driver's settings in its query, as Postgres's
  `sslmode=require` or MySQL's `tls=true`, and instances anywhere that
  reach it share it, and its jobs. `compose.yaml` is for development: it
  runs the database on `127.0.0.1` with a password everyone knows.

### What the starter leaves out

- **Which make of authenticator** a passkey is on, which checking its
  attestation would say, for a site that takes only some.
- **Accounts with a passkey and no password**: registering takes one, and
  passkeys come after.
- **A list of the sessions logged in**, to end one: logins live in
  cookies, so there's nothing to list. A new password ends them all but
  this browser's.
- **Telling the old email** when the email changes, or the password, as
  some apps do so that the owner hears of a change they didn't make.
- **Logging in with another site**, such as GitHub or Google.
