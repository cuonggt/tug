package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
)

// TwoFactor has the parts of two-factor logins where a slip is a security
// hole: the secret a user's authenticator app shares with the server, the
// codes the app makes from it, and recovery codes for a lost phone. The
// codes are TOTP's (RFC 6238), as Google Authenticator, 1Password and the
// rest make them: six digits, and a new one every 30 seconds.
//
// The app stores a user's secret, and their recovery codes, sealed:
// encrypted with a key derived from the app's own, so that a copy of the
// database, such as a leaked backup, doesn't give away the second factor
// along with the first.
//
//	tf := &auth.TwoFactor{Keys: keys, Issuer: "Blog"} // session.KeysFromEnv's
//
//	// Turning it on: show tf.URL(user.Email, secret) as a QR code, and
//	// take a code from the app to be sure it's set up.
//	secret := tf.NewSecret()
//	step, ok := tf.Check(secret, code, 0)
//	// ok: store tf.Seal(secret), and step as the last step used
//
//	// Logging in, after the password.
//	secret, err := tf.Open(user.TwoFactorSecret)
//	step, ok = tf.Check(secret, code, user.TwoFactorStep)
//	// ok: store step in place of the last
type TwoFactor struct {
	// Keys seal secrets and recovery codes. The first seals and each of
	// them opens, so the app's key can be rotated, and Stale finds what to
	// seal again with the new one. The app's own keys will do, as
	// session.KeysFromEnv reads them: a key for two-factor logins alone is
	// derived from each.
	Keys [][]byte

	// Issuer names the app in authenticator apps, beside the account: the
	// app's name, such as "Blog".
	Issuer string

	now func() time.Time
}

// b32 is the base32 that authenticator apps take secrets in: RFC 4648's,
// without padding.
var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewSecret returns a new secret, for a user turning two-factor logins on:
// 160 random bits, as RFC 4226 has them, in base32. An authenticator app
// takes it from URL's QR code, or typed in by hand.
func (tf *TwoFactor) NewSecret() string {
	b := make([]byte, 20)
	rand.Read(b)
	return b32.EncodeToString(b)
}

// URL returns the otpauth:// URL that sets up an authenticator app with
// secret, for account, such as the user's email, under the Issuer: show it
// as a QR code for the app to scan.
func (tf *TwoFactor) URL(account, secret string) string {
	label := labelEscape(account)
	q := url.Values{"secret": {secret}}
	if tf.Issuer != "" {
		label = labelEscape(tf.Issuer) + ":" + label
		q.Set("issuer", tf.Issuer)
	}
	// Apps read the query's spaces as %20; some would keep a + as it is.
	return "otpauth://totp/" + label + "?" + strings.ReplaceAll(q.Encode(), "+", "%20")
}

// labelEscape escapes one side of the label, the colon between the two
// sides included.
func labelEscape(s string) string {
	return strings.ReplaceAll(url.PathEscape(s), ":", "%3A")
}

// Check reports whether code is the one secret makes now, or in the 30
// seconds either side, for a phone whose clock is off, and returns the
// time step it belongs to. after is the step of the code that last worked
// for the user, or 0 for none: a code works once, and no code from before
// it works after it, so one seen over the user's shoulder is no use. Keep
// the step that Check returns in place of after.
func (tf *TwoFactor) Check(secret, code string, after int64) (step int64, ok bool) {
	key, err := b32.DecodeString(strings.ToUpper(strings.TrimRight(secret, "=")))
	code = strings.ReplaceAll(code, " ", "")
	if err != nil || len(key) == 0 || len(code) != 6 {
		return 0, false
	}
	now := tf.clock().Unix() / 30
	for s := now - 1; s <= now+1; s++ {
		if s > after && subtle.ConstantTimeCompare([]byte(totp(key, s)), []byte(code)) == 1 {
			return s, true
		}
	}
	return 0, false
}

// Code returns the code that secret makes at a time, as an authenticator
// app shows it then. A server has no use for it but tests: an app's own log
// in as a user with two-factor logins on, with a code for now, and for 30
// seconds on to log in again, as a code works once.
func (tf *TwoFactor) Code(secret string, at time.Time) (string, error) {
	key, err := b32.DecodeString(strings.ToUpper(strings.TrimRight(secret, "=")))
	if err != nil || len(key) == 0 {
		return "", errors.New("auth: a two-factor secret is base32, as NewSecret makes it")
	}
	return totp(key, at.Unix()/30), nil
}

// totp is the six-digit code that key makes at step: HOTP (RFC 4226), with
// HMAC-SHA1, of the number of 30-second steps since 1970.
func totp(key []byte, step int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	n := binary.BigEndian.Uint32(sum[offset:]) & 0x7fffffff
	return fmt.Sprintf("%06d", n%1_000_000)
}

func (tf *TwoFactor) clock() time.Time {
	if tf.now != nil {
		return tf.now()
	}
	return time.Now()
}

// Seal encrypts value, a user's secret or their recovery codes, for
// storing, with the first of Keys. The result is base64, safe in any text
// column.
func (tf *TwoFactor) Seal(value string) string {
	aead := sealer(tf.keys()[0])
	nonce := make([]byte, aead.NonceSize(), aead.NonceSize()+len(value)+aead.Overhead())
	rand.Read(nonce)
	return base64.RawURLEncoding.EncodeToString(aead.Seal(nonce, nonce, []byte(value), nil))
}

// Open returns the value that Seal sealed. It fails when sealed has been
// changed since, or was sealed with a key that's no longer among Keys.
func (tf *TwoFactor) Open(sealed string) (string, error) {
	value, _, err := tf.open(sealed)
	return value, err
}

// Stale reports whether sealed was sealed with a key other than the first
// of Keys, as everything was before the app's key was rotated. Seal what
// Open returns again, while the old key is still among Keys: once it has
// gone, what it sealed can't be opened.
func (tf *TwoFactor) Stale(sealed string) bool {
	_, i, err := tf.open(sealed)
	return err == nil && i > 0
}

var errSealed = errors.New("auth: a sealed two-factor value doesn't open: it has been changed, or was sealed with a key that's no longer among TwoFactor.Keys")

// open returns the value sealed, and the index of the key that sealed it.
func (tf *TwoFactor) open(sealed string) (string, int, error) {
	b, err := base64.RawURLEncoding.DecodeString(sealed)
	if err != nil {
		return "", 0, errSealed
	}
	for i, key := range tf.keys() {
		aead := sealer(key)
		n := aead.NonceSize()
		if len(b) < n {
			return "", 0, errSealed
		}
		if value, err := aead.Open(nil, b[:n], b[n:], nil); err == nil {
			return string(value), i, nil
		}
	}
	return "", 0, errSealed
}

func (tf *TwoFactor) keys() [][]byte {
	if len(tf.Keys) == 0 {
		panic("auth: TwoFactor.Keys is empty; session.KeysFromEnv reads the app's")
	}
	return tf.Keys
}

// sealer is AES-256-GCM with a key for two-factor logins alone, derived
// from the app's.
func sealer(appKey []byte) cipher.AEAD {
	key, err := hkdf.Key(sha256.New, appKey, nil, "tug two-factor", 32)
	if err != nil {
		panic(err) // only for a length SHA-256 can't make
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err) // only for a key that isn't 32 bytes, which HKDF made
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		panic(err)
	}
	return aead
}

// NewRecoveryCodes returns eight new recovery codes, each good for one
// login in place of a code from the app, for a user who has lost their
// phone: ten random letters and digits apiece, as "k3p9x-m2w7q", for the
// user to keep somewhere safe. Store them sealed, and show them to no one
// but the user, once they've confirmed their password.
func NewRecoveryCodes() []string {
	codes := make([]string, 8)
	for i := range codes {
		t := strings.ToLower(rand.Text()) // 26 characters of base32
		codes[i] = t[:5] + "-" + t[5:10]
	}
	return codes
}

// UseRecoveryCode reports whether code is one of codes, in any case and
// with or without its dash, and returns the codes without it. Store those
// in their place: a recovery code works once.
func UseRecoveryCode(codes []string, code string) (rest []string, ok bool) {
	want := plainCode(code)
	found := -1
	for i, c := range codes {
		if subtle.ConstantTimeCompare([]byte(plainCode(c)), []byte(want)) == 1 && found < 0 {
			found = i
		}
	}
	if want == "" || found < 0 {
		return codes, false
	}
	return slices.Delete(slices.Clone(codes), found, found+1), true
}

// plainCode is a recovery code as it's compared: lower case, without the
// dash or the spaces someone may type.
func plainCode(code string) string {
	return strings.ToLower(strings.NewReplacer("-", "", " ", "").Replace(code))
}
