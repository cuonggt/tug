# Encryption

An app has one key, `APP_KEY`, and tug makes a key of its own from it for
each thing it encrypts or signs: the session's cookie, two-factor
secrets, the links in mail and the signed links, a private disk's links,
and the app's own values, sealed with package `crypt`. A key made for one
of them opens or checks nothing of another's, and one that leaked gives
away no other.

## The app's keys

`APP_KEY` is `base64:` and 32 random bytes in base64, as Laravel writes
it. `tug new` puts one in `.env` for development, and `tug key` prints a
new one, for a deployed app, which keeps its own, secret, and the same in
every copy of the app that runs:

```sh
tug key
```

`APP_PREVIOUS_KEYS` has the keys being rotated out, comma separated.
`session.KeysFromEnv` reads both, `APP_KEY` first, and each part of tug
that takes `Keys` takes that list: the first key encrypts or signs, and
each of them opens or checks, so what an old key made still works while
it's among them.

| What | Package | How | Its key's purpose |
|---|---|---|---|
| The session's cookie | `session` | AES-256-GCM, with the cookie's name bound to it | `tug session` |
| Two-factor secrets and recovery codes | `auth`, `TwoFactor` | AES-256-GCM | `tug two-factor` |
| Links to reset a password, and to verify an email | `auth`, `Resets` and `Verifications` | HMAC-SHA256 | `tug password reset`, `tug email verification` |
| Signed links | `tug`, `SignedURL` | HMAC-SHA256 | `tug signed link` |
| A private disk's links | `storage`, `Local` | HMAC-SHA256 | `tug storage link` |
| The app's own values | `crypt`, `Box` | AES-256-GCM | `tug crypt` and the Box's purpose |

A key's purpose is HKDF's `info`: each is derived from each of the app's
keys, with SHA-256.

## The app's own values

Package `crypt` seals what the app keeps and mustn't give away with a copy
of its database, such as a leaked backup: a token for another service that
a user connected, say, which it calls that service with later.

```go
tokens, err := crypt.New(keys, "github tokens") // keys: session.KeysFromEnv's
if err != nil {
	return err
}

id := strconv.FormatInt(u.ID, 10)
sealed := tokens.Seal([]byte(token), "users", id)
// keep sealed in the user's row, and later:
token, err := tokens.Open(sealed, "users", id)
```

- `crypt.New(keys, purpose)` is a `Box` for one purpose: its key is
  derived from each of the app's for that purpose alone, so a value sealed
  for one purpose doesn't open as another's, and none opens as one of
  tug's own.
- `Seal` encrypts a value with AES-256-GCM, under the first key and a
  random nonce, so the same value sealed twice is two texts. It returns
  base64url, for a column, a cookie or a link.
- What a value belongs to, its owner, as its table and row, is bound to
  it: `Open` needs the same, so a value copied to another row, by whoever
  can write the table, doesn't open there as that row's. It's checked, not
  kept. A value sealed with no owner opens with none.
- `Open` returns the value, or `crypt.ErrOpen` for one changed since, one
  sealed for another purpose or owner, or with a key that's no longer
  among the app's.
- `Stale` says whether a value was sealed with a key other than the first,
  as everything was before the key was rotated: seal what `Open` returns
  again, and keep that in its place.

A value is bytes, and a struct is JSON first. What's only compared, and
never read back, as a password, or a token the app hands out and checks,
is hashed, not sealed: `auth.HashPassword`, or a SHA-256 for a long random
token. Sealed values aren't Laravel's: its payload is JSON of an IV, an
AES-CBC text and an HMAC, so a value from a Laravel app is opened there,
and sealed again with a `Box`.

## Rotating the key

A key that may have leaked, or one that has served its time, is rotated:

1. `tug key` prints the new key.
2. The app is deployed with the new key in `APP_KEY`, and the old one
   first in `APP_PREVIOUS_KEYS`. Everything the old key made still works:
   sessions, two-factor secrets, links, and the app's sealed values.
3. What the old key sealed moves to the new one. The sessions move as
   they're used, as each response seals its session again. The auth
   starter seals every two-factor secret again as it starts, with
   `TwoFactor.Stale`. The app's own sealed values move by `Box.Stale`, in
   a command or as the app starts, as the starter moves its secrets:

   ```go
   if tokens.Stale(u.GitHubToken, "users", id) {
       token, err := tokens.Open(u.GitHubToken, "users", id)
       ...
       err = users.setGitHubToken(ctx, u.ID, tokens.Seal(token, "users", id))
   }
   ```

4. The old key is dropped once nothing needs it: after a month, as long as
   a remembered login lasts, and as long as the links the app sent last.
   A signed link made to last years, as one to unsubscribe, works only
   while the key that signed it is among the app's.

A new `APP_KEY` with the old one dropped at once ends every session, and
every link sent, and leaves every value the old key sealed, two-factor
secrets among them, unopened for good.

## What's not here yet

- **Cookies of the app's, encrypted**: what a visitor's browser keeps for
  the app is in its session, which is. A cookie of the app's own, as a
  theme a script reads, is `http.SetCookie`'s, as a script can't read a
  sealed one.
- **Jobs whose payload is sealed**, as Laravel's `ShouldBeEncrypted`: a
  job carries IDs, and reads what's secret, sealed, from where the app
  keeps it ([Background jobs](jobs.md#a-kind-of-job)).
- **A key kept outside the environment**, in a service for secrets: the
  app reads it itself, and gives its bytes to each part that takes `Keys`,
  as `session.ParseKey` reads one.
