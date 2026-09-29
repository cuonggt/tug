package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"net/http"
	"slices"
	"strings"
)

// AccessTokens makes the tokens an app's API takes in place of a login:
// its users make them, for a script, their phone's app or another
// service, and send them in Authorization: Bearer. A token is a password
// for the API: the app shows it to its user once, as it's made, and keeps
// its hash in its place, with whose it is, its name, what it may do, and
// when it expires.
//
//	tokens := &auth.AccessTokens{Prefix: "blog"}
//	token, hash := tokens.New() // show token once; keep hash
//	...
//	sent, ok := auth.BearerToken(r)
//	row, err := table.byHash(ctx, tokens.Hash(sent))
//
// A token is 32 random bytes, which no one guesses, so the hash is a
// SHA-256, fast, where a password's is argon2id, for a secret people
// choose: one lookup by it finds the token, and a copy of the database
// has none that works.
type AccessTokens struct {
	// Prefix starts each token, before an underscore, as blog_..., so a
	// token pasted where it shouldn't be, in a repository or a log, says
	// whose it is, and a scanner of secrets can look for it. The app's
	// name will do: letters and digits.
	Prefix string
}

// New returns a new token, the Prefix, an underscore and 32 random bytes
// in base32, 52 letters and digits, for its user, once, and its hash, for
// the app to keep in its place. It panics on a Prefix of anything but
// letters and digits.
func (at *AccessTokens) New() (token string, hash []byte) {
	b := make([]byte, 32)
	rand.Read(b)
	// Two-factor secrets' base32, in lower case: letters and digits, which
	// a double click selects the whole of.
	token = at.prefix() + strings.ToLower(b32.EncodeToString(b))
	return token, at.Hash(token)
}

// tokenLen is how long the random part of a token is.
const tokenLen = 52

// Hash returns the hash of token, a token a request sent, to find it by
// among the ones the app keeps: the SHA-256 of the whole of it. It's nil
// for one that can't be a token of the app's, not the Prefix, an
// underscore and 52 letters and digits, which no lookup needs to find.
func (at *AccessTokens) Hash(token string) []byte {
	secret, ok := strings.CutPrefix(token, at.prefix())
	if !ok || len(secret) != tokenLen || strings.Trim(secret, "abcdefghijklmnopqrstuvwxyz234567") != "" {
		return nil
	}
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func (at *AccessTokens) prefix() string {
	if at.Prefix == "" || strings.Trim(at.Prefix, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789") != "" {
		panic(fmt.Sprintf("auth: AccessTokens.Prefix is letters and digits, as the app's name, not %q", at.Prefix))
	}
	return at.Prefix + "_"
}

// BearerToken returns the token r sends in its Authorization header, as
// "Bearer" and the token, or false for none.
func BearerToken(r *http.Request) (string, bool) {
	scheme, token, ok := strings.Cut(strings.TrimSpace(r.Header.Get("Authorization")), " ")
	token = strings.TrimSpace(token)
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return "", false
	}
	return token, true
}

// Abilities are what an access token may do, each a word of the app's,
// as "posts:write", or "*" for everything, which the app keeps with the
// token, and a route checks.
type Abilities []string

// Can reports whether the abilities have ability, or "*".
func (a Abilities) Can(ability string) bool {
	return slices.Contains(a, ability) || slices.Contains(a, "*")
}
