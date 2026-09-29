// Package crypt encrypts the app's own values for keeping, such as a token
// for another service, which a user connected, kept in a column: a copy
// of the database, as a leaked backup, then has the token sealed, and
// nothing to open it with. A Box seals with AES-256-GCM, under a key for
// its purpose alone, derived from each of the app's keys, which
// session.KeysFromEnv reads from APP_KEY and APP_PREVIOUS_KEYS.
//
//	tokens, err := crypt.New(keys, "github tokens")
//	...
//	id := strconv.FormatInt(u.ID, 10)
//	sealed := tokens.Seal([]byte(token), "users", id) // keep sealed in the user's row
//	...
//	token, err := tokens.Open(sealed, "users", id)
//
// It has no import of tug. tug seals its own values, its sessions and
// two-factor secrets, the same way, each with a key of its own.
package crypt

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/cuonggt/tug/internal/seal"
)

// Box seals and opens values for one purpose. Share one by pointer: its
// methods are safe to call at once.
type Box struct {
	box *seal.Box
}

// New returns a Box for purpose, such as "github tokens", with keys, the
// app's, each 32 bytes, as session.KeysFromEnv reads them: the first
// seals, and each opens. Its key is derived from each of keys for the
// purpose alone, so a value sealed for one purpose doesn't open as
// another's, and never as one of tug's own. An empty purpose, no keys, or
// a key that isn't 32 bytes is an error.
func New(keys [][]byte, purpose string) (*Box, error) {
	if purpose == "" {
		return nil, errors.New(`crypt: a Box has a purpose, such as "github tokens", which its key is for alone`)
	}
	b, err := seal.New(keys, "tug crypt "+purpose)
	if err != nil {
		return nil, fmt.Errorf("crypt: %w; session.KeysFromEnv reads the app's", err)
	}
	return &Box{b}, nil
}

// ErrOpen is Open's error for a value that doesn't open.
var ErrOpen = errors.New("crypt: the value doesn't open: it was changed, or sealed for another purpose or owner, or with a key that's no longer among the app's")

// Seal encrypts value, and returns it as text, in base64url, for a
// column, a cookie or a link. The same value sealed twice is two texts.
//
// owner is what the value belongs to, as its table and row, "users",
// "42": Open needs the same, so a value copied to another row, by whoever
// can write the table, doesn't open there as that row's. It's checked,
// not kept. Without an owner, a value opens wherever it's put.
func (b *Box) Seal(value []byte, owner ...string) string {
	return b.box.Seal(value, ad(owner))
}

// Open returns the value Seal sealed, with the same owner. It fails with
// ErrOpen for a value changed since, sealed for another purpose or owner,
// or with a key that's no longer among the Box's.
func (b *Box) Open(sealed string, owner ...string) ([]byte, error) {
	value, _, err := b.box.Open(sealed, ad(owner))
	if err != nil {
		return nil, ErrOpen
	}
	return value, nil
}

// Stale reports whether sealed was sealed with a key other than the
// first, as everything was before the app's key was rotated: Seal what
// Open returns again, and keep that in its place, while the old key is
// still among the Box's, as once it has gone, what it sealed can't be
// opened. A value that doesn't open isn't stale.
func (b *Box) Stale(sealed string, owner ...string) bool {
	_, i, err := b.box.Open(sealed, ad(owner))
	return err == nil && i > 0
}

// ad is what a value is bound to: each part of its owner after its length
// and a colon, as tug signs its links' parts, so that no two owners read
// the same, "users", "42" and "users4", "2" among them.
func ad(owner []string) []byte {
	var b []byte
	for _, part := range owner {
		b = append(b, strconv.Itoa(len(part))+":"+part...)
	}
	return b
}
