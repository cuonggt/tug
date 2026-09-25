package auth

import "time"

// Verifications makes and checks the tokens that email verification links
// carry, as in /verify-email/42/<token>: following one shows that the
// email reaches its owner. Like a reset token, a token is kept nowhere:
// it's signed with the app's key, for one user and the email they have
// when it's made. So it stops working once the email changes, and it
// expires besides. It can be used again until then, which does no harm:
// the email is verified already.
//
//	verifications := &auth.Verifications{Keys: keys} // session.KeysFromEnv's
//	link := "/verify-email/" + user.ID + "/" + verifications.Token(user.ID, user.Email)
//	...
//	if !verifications.Check(token, user.ID, user.Email) {
//		// expired, for another email, or not one of ours
//	}
type Verifications struct {
	// Keys sign the tokens. The first signs and each of them checks, as
	// Resets' do. The app's own keys will do: a key for verification tokens
	// alone is derived from each, so a reset token is no good here, nor a
	// verification token there.
	Keys [][]byte

	// Lifetime is how long a token works for. Default 24 hours.
	Lifetime time.Duration

	now func() time.Time
}

// Token returns a token that verifies the email of the user with this ID,
// until it expires or the user's email changes. It's safe in a URL's path.
func (vs *Verifications) Token(id, email string) string {
	if len(vs.Keys) == 0 {
		panic("auth: Verifications.Keys is empty; session.KeysFromEnv reads the app's")
	}
	return vs.tokens().make(id, email)
}

// Check reports whether token verifies email for the user with this ID:
// that one of the keys made it for them and that email, and that it
// hasn't expired.
func (vs *Verifications) Check(token, id, email string) bool {
	return vs.tokens().check(token, id, email)
}

func (vs *Verifications) tokens() tokens {
	return tokens{purpose: "tug email verification", keys: vs.Keys, lifetime: lifetimeOr(vs.Lifetime, 24*time.Hour), now: vs.now}
}
