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

// tokens makes and checks the tokens that links in mail carry, for one
// purpose: reset links' or verification links'. A token is the time it
// expires and a signature over that and what it stands for, with a key for
// that purpose alone, so it's kept nowhere and a token for one purpose is
// no good for another.
type tokens struct {
	purpose  string // the HKDF info the purpose's key is derived with
	keys     [][]byte
	lifetime time.Duration
	now      func() time.Time
}

func (tk tokens) make(parts ...string) string {
	expires := strconv.FormatInt(tk.clock().Add(tk.lifetime).Unix(), 36)
	return expires + "." + base64.RawURLEncoding.EncodeToString(tk.sign(tk.keys[0], expires, parts))
}

func (tk tokens) check(token string, parts ...string) bool {
	expires, sig, ok := strings.Cut(token, ".")
	if !ok {
		return false
	}
	unix, err := strconv.ParseInt(expires, 36, 64)
	if err != nil || tk.clock().Unix() >= unix {
		return false
	}
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return false
	}
	for _, key := range tk.keys {
		if hmac.Equal(got, tk.sign(key, expires, parts)) {
			return true
		}
	}
	return false
}

func (tk tokens) clock() time.Time {
	if tk.now != nil {
		return tk.now()
	}
	return time.Now()
}

// sign is a token's signature: HMAC-SHA256 with a key for the purpose
// alone, derived from the app's, over the expiry and the parts, each
// preceded by its length so that no two sets of parts read the same.
func (tk tokens) sign(appKey []byte, expires string, parts []string) []byte {
	key, err := hkdf.Key(sha256.New, appKey, nil, tk.purpose, 32)
	if err != nil {
		panic(err) // only for a length SHA-256 can't make
	}
	mac := hmac.New(sha256.New, key)
	for _, part := range append([]string{expires}, parts...) {
		mac.Write([]byte(strconv.Itoa(len(part)) + ":" + part))
	}
	return mac.Sum(nil)[:16]
}

// lifetimeOr is d, or def when d isn't set.
func lifetimeOr(d, def time.Duration) time.Duration {
	if d <= 0 {
		return def
	}
	return d
}
