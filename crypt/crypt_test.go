package crypt_test

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/cuonggt/tug/auth"
	"github.com/cuonggt/tug/crypt"
)

func key(b byte) []byte { return bytes.Repeat([]byte{b}, 32) }

func box(t *testing.T, purpose string, keys ...[]byte) *crypt.Box {
	t.Helper()
	b, err := crypt.New(keys, purpose)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestAValueSealedOpens(t *testing.T) {
	tokens := box(t, "github tokens", key(1))
	sealed := tokens.Seal([]byte("gho_16C7e42F292c6912E7710c838347Ae178B4a"))
	value, err := tokens.Open(sealed)
	if err != nil || string(value) != "gho_16C7e42F292c6912E7710c838347Ae178B4a" {
		t.Fatalf("got %q, %v", value, err)
	}
	if strings.Contains(sealed, "gho_") || strings.ContainsAny(sealed, "+/=") {
		t.Errorf("sealed %q: the token shows, or it isn't base64url", sealed)
	}
	if again := tokens.Seal([]byte("gho_16C7e42F292c6912E7710c838347Ae178B4a")); again == sealed {
		t.Error("the same value sealed twice is the same text")
	}
}

func TestAValueSealedWithAKeySinceRotatedOpensAndIsStale(t *testing.T) {
	sealed := box(t, "github tokens", key(1)).Seal([]byte("token"))
	rotated := box(t, "github tokens", key(2), key(1)) // a new APP_KEY, the old in APP_PREVIOUS_KEYS
	if value, err := rotated.Open(sealed); err != nil || string(value) != "token" {
		t.Fatalf("with the old key among the keys: %q, %v", value, err)
	}
	if !rotated.Stale(sealed) {
		t.Error("a value sealed with the old key isn't stale")
	}
	if again := rotated.Seal([]byte("token")); rotated.Stale(again) {
		t.Error("a value sealed with the new key is stale")
	}
	if _, err := box(t, "github tokens", key(2)).Open(sealed); !errors.Is(err, crypt.ErrOpen) {
		t.Errorf("once the old key is dropped: %v", err)
	}
	if rotated.Stale("not sealed") {
		t.Error("a value that doesn't open is stale")
	}
}

func TestAValueChangedDoesntOpen(t *testing.T) {
	tokens := box(t, "github tokens", key(1))
	sealed := tokens.Seal([]byte("token"))
	raw, _ := base64.RawURLEncoding.DecodeString(sealed)
	for i := range raw {
		changed := bytes.Clone(raw)
		changed[i] ^= 1
		if _, err := tokens.Open(base64.RawURLEncoding.EncodeToString(changed)); !errors.Is(err, crypt.ErrOpen) {
			t.Fatalf("with byte %d changed: %v", i, err)
		}
	}
	for _, bad := range []string{"", "not base64!", sealed[:10], sealed + "A"} {
		if _, err := tokens.Open(bad); !errors.Is(err, crypt.ErrOpen) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestAValueSealedForAnotherPurposeDoesntOpen(t *testing.T) {
	sealed := box(t, "github tokens", key(1)).Seal([]byte("token"))
	if _, err := box(t, "slack tokens", key(1)).Open(sealed); !errors.Is(err, crypt.ErrOpen) {
		t.Errorf("got %v", err)
	}
}

func TestAValueOpensOnlyWhereItBelongs(t *testing.T) {
	tokens := box(t, "github tokens", key(1))
	sealed := tokens.Seal([]byte("token"), "users", "42")
	if value, err := tokens.Open(sealed, "users", "42"); err != nil || string(value) != "token" {
		t.Fatalf("in its own row: %q, %v", value, err)
	}
	for _, owner := range [][]string{{"users", "43"}, {}, {"users42"}, {"users4", "2"}, {"users", "42", ""}} {
		if _, err := tokens.Open(sealed, owner...); !errors.Is(err, crypt.ErrOpen) {
			t.Errorf("as %q's: %v", owner, err)
		}
	}
	// One sealed for no owner opens with none.
	if _, err := tokens.Open(tokens.Seal([]byte("token")), "users", "42"); !errors.Is(err, crypt.ErrOpen) {
		t.Errorf("a value of no owner, opened as a row's: %v", err)
	}
}

func TestNoPurposeOpensTugsOwnValues(t *testing.T) {
	// A Box's key is its purpose's, after "tug crypt ", which none of tug's
	// own is: a Box named for two-factor secrets opens none of them.
	tf := &auth.TwoFactor{Keys: [][]byte{key(1)}}
	secret := tf.Seal("JBSWY3DPEHPK3PXP")
	for _, purpose := range []string{"two-factor", "tug two-factor"} {
		if _, err := box(t, purpose, key(1)).Open(secret); !errors.Is(err, crypt.ErrOpen) {
			t.Errorf("a Box for %q opened a two-factor secret: %v", purpose, err)
		}
	}
	if _, err := tf.Open(box(t, "tug two-factor", key(1)).Seal([]byte("JBSWY3DPEHPK3PXP"))); err == nil {
		t.Error("a value a Box sealed opened as a two-factor secret")
	}
}

func TestABoxHasAPurposeAndKeys(t *testing.T) {
	for name, newBox := range map[string]func() (*crypt.Box, error){
		"no purpose":        func() (*crypt.Box, error) { return crypt.New([][]byte{key(1)}, "") },
		"no keys":           func() (*crypt.Box, error) { return crypt.New(nil, "github tokens") },
		"a key of 16 bytes": func() (*crypt.Box, error) { return crypt.New([][]byte{key(1)[:16]}, "github tokens") },
	} {
		if b, err := newBox(); err == nil || b != nil {
			t.Errorf("%s: %v, %v", name, b, err)
		}
	}
	_, err := crypt.New([][]byte{key(1)[:16]}, "github tokens")
	if err == nil || !strings.Contains(err.Error(), "a key is 32 bytes, not 16") {
		t.Errorf("got %v", err)
	}
}

func TestABoxSealsAndOpensAtOnce(t *testing.T) {
	tokens := box(t, "github tokens", key(1))
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if value, err := tokens.Open(tokens.Seal([]byte("token"))); err != nil || string(value) != "token" {
				t.Errorf("got %q, %v", value, err)
			}
		})
	}
	wg.Wait()
}
