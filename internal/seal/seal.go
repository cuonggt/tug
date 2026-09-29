// Package seal encrypts values with AES-256-GCM, under a key for one use
// alone, derived by HKDF from each of the app's keys: the first seals, and
// each of them opens, so the app's key can be rotated. Package crypt seals
// the app's own values with it, and auth's TwoFactor two-factor secrets.
package seal

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
)

// Box seals and opens values with the keys New derived. Its methods are
// safe to call at once.
type Box struct {
	aeads []cipher.AEAD
}

// New returns a Box whose keys are derived from keys, each 32 bytes, with
// info, what they're for, such as "tug two-factor", which makes a key of
// its own for each use of the app's.
func New(keys [][]byte, info string) (*Box, error) {
	if len(keys) == 0 {
		return nil, errors.New("there are no keys")
	}
	b := &Box{}
	for _, key := range keys {
		if len(key) != 32 {
			return nil, fmt.Errorf("a key is 32 bytes, not %d", len(key))
		}
		derived, err := hkdf.Key(sha256.New, key, nil, info, 32)
		if err != nil {
			return nil, err
		}
		block, err := aes.NewCipher(derived)
		if err != nil {
			return nil, err
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
		b.aeads = append(b.aeads, aead)
	}
	return b, nil
}

// Seal encrypts value with the first key, under a random nonce, which
// goes first, and returns it in base64url, without padding, for a column,
// a cookie or a link. ad is what the value is bound to, which Open needs
// the same of, and which isn't kept: nil for nothing.
func (b *Box) Seal(value, ad []byte) string {
	aead := b.aeads[0]
	nonce := make([]byte, aead.NonceSize(), aead.NonceSize()+len(value)+aead.Overhead())
	rand.Read(nonce)
	return base64.RawURLEncoding.EncodeToString(aead.Seal(nonce, nonce, value, ad))
}

// ErrOpen is Open's error for a value that doesn't open.
var ErrOpen = errors.New("the value doesn't open")

// Open returns the value Seal sealed with ad, and the index of the key
// that sealed it, 0 for the first. It fails with ErrOpen for a value
// changed since, sealed with other ad or for another use, or with a key
// that's no longer among the keys.
func (b *Box) Open(sealed string, ad []byte) ([]byte, int, error) {
	raw, err := base64.RawURLEncoding.DecodeString(sealed)
	if err != nil {
		return nil, 0, ErrOpen
	}
	for i, aead := range b.aeads {
		n := aead.NonceSize()
		if len(raw) < n {
			return nil, 0, ErrOpen
		}
		if value, err := aead.Open(nil, raw[:n], raw[n:], ad); err == nil {
			return value, i, nil
		}
	}
	return nil, 0, ErrOpen
}
