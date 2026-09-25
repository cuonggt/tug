package auth

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
	"time"
)

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
	lifetime := rs.Lifetime
	if lifetime <= 0 {
		lifetime = time.Hour
	}
	expires := strconv.FormatInt(rs.clock().Add(lifetime).Unix(), 36)
	return expires + "." + base64.RawURLEncoding.EncodeToString(sign(rs.Keys[0], expires, id, passwordHash))
}

// Check reports whether token resets the password of the user with this
// ID, whose password hash is passwordHash now: that one of the keys made
// it for them, since their last change of password, and that it hasn't
// expired.
func (rs *Resets) Check(token, id, passwordHash string) bool {
	expires, sig, ok := strings.Cut(token, ".")
	if !ok {
		return false
	}
	unix, err := strconv.ParseInt(expires, 36, 64)
	if err != nil || rs.clock().Unix() >= unix {
		return false
	}
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return false
	}
	for _, key := range rs.Keys {
		if hmac.Equal(got, sign(key, expires, id, passwordHash)) {
			return true
		}
	}
	return false
}

func (rs *Resets) clock() time.Time {
	if rs.now != nil {
		return rs.now()
	}
	return time.Now()
}

// sign is the token's signature: HMAC-SHA256 with a key for reset tokens
// alone, over everything the token stands for, each part preceded by its
// length so that no two sets of parts read the same.
func sign(appKey []byte, expires, id, passwordHash string) []byte {
	key, err := hkdf.Key(sha256.New, appKey, nil, "tug password reset", 32)
	if err != nil {
		panic(err) // only for a length SHA-256 can't make
	}
	mac := hmac.New(sha256.New, key)
	for _, part := range []string{expires, id, passwordHash} {
		mac.Write([]byte(strconv.Itoa(len(part)) + ":" + part))
	}
	return mac.Sum(nil)[:16]
}
