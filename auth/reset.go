package auth

import "time"

// Resets makes and checks the tokens that password reset links carry, as
// in /reset-password/<token>?email=ann@example.com. A token is kept
// nowhere: it's signed with the app's key, for one user and the password
// hash they have when it's made. So it stops working once the password
// changes, which is to say once it's been used, and it expires besides.
//
//	resets := &auth.Resets{Keys: keys} // session.KeysFromEnv's
//	link := "/reset-password/" + resets.Token(user.ID, user.PasswordHash)
//	...
//	if !resets.Check(token, user.ID, user.PasswordHash) {
//		// expired, used, or not one of ours
//	}
type Resets struct {
	// Keys sign the tokens. The first signs and each of them checks, so a
	// new key can go first while links made with the old one still work.
	// The app's own keys will do, as session.KeysFromEnv reads them: a key
	// for tokens alone is derived from each.
	Keys [][]byte

	// Lifetime is how long a token works for. Default 1 hour.
	Lifetime time.Duration

	now func() time.Time
}

// Token returns a token that resets the password of the user with this ID,
// whose password hash is passwordHash, until it expires or the password
// changes. It's safe in a URL's path.
func (rs *Resets) Token(id, passwordHash string) string {
	if len(rs.Keys) == 0 {
		panic("auth: Resets.Keys is empty; session.KeysFromEnv reads the app's")
	}
	return rs.tokens().make(id, passwordHash)
}

// Check reports whether token resets the password of the user with this
// ID, whose password hash is passwordHash now: that one of the keys made
// it for them, since their last change of password, and that it hasn't
// expired.
func (rs *Resets) Check(token, id, passwordHash string) bool {
	return rs.tokens().check(token, id, passwordHash)
}

func (rs *Resets) tokens() tokens {
	return tokens{purpose: "tug password reset", keys: rs.Keys, lifetime: lifetimeOr(rs.Lifetime, time.Hour), now: rs.now}
}
