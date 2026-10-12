package auth

import "time"

// Registrations makes and checks the tokens that the links finishing a
// registration carry, as in /register/passkey/<token>?email=…&name=…:
// following one shows that the email reaches whoever is registering, before
// their account is made, as one made with a passkey is. Like a
// verification token, a token is kept nowhere: it's signed with the app's
// key, for the email and the name it was asked for with. It can be used
// again until it expires, which does no harm: once the email has an
// account, the app makes no other.
//
//	registrations := &auth.Registrations{Keys: keys} // session.KeysFromEnv's
//	link := "/register/passkey/" + registrations.Token(email, name) + "?email=" + url.QueryEscape(email) + "&name=" + url.QueryEscape(name)
//	...
//	if !registrations.Check(token, email, name) {
//		// expired, for another email or name, or not one of ours
//	}
type Registrations struct {
	// Keys sign the tokens. The first signs and each of them checks, as
	// Resets' do. The app's own keys will do: a key for registration tokens
	// alone is derived from each, so a verification token is no good here,
	// nor a registration token there.
	Keys [][]byte

	// Lifetime is how long a token works for. Default 24 hours.
	Lifetime time.Duration

	now func() time.Time
}

// Token returns a token that finishes registering the email, for someone
// of this name, until it expires. It's safe in a URL's path.
func (rs *Registrations) Token(email, name string) string {
	if len(rs.Keys) == 0 {
		panic("auth: Registrations.Keys is empty; session.KeysFromEnv reads the app's")
	}
	return rs.tokens().make(email, name)
}

// Check reports whether token finishes registering email for someone of
// this name: that one of the keys made it for them, and that it hasn't
// expired.
func (rs *Registrations) Check(token, email, name string) bool {
	return rs.tokens().check(token, email, name)
}

func (rs *Registrations) tokens() tokens {
	return tokens{purpose: "tug registration", keys: rs.Keys, lifetime: lifetimeOr(rs.Lifetime, 24*time.Hour), now: rs.now}
}
