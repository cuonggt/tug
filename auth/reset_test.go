package auth

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func newResets(key byte) *Resets {
	return &Resets{Keys: [][]byte{bytes.Repeat([]byte{key}, 32)}}
}

func TestAResetTokenWorksUntilThePasswordChanges(t *testing.T) {
	rs := newResets(1)
	token := rs.Token("42", "hash-1")
	if !rs.Check(token, "42", "hash-1") {
		t.Fatal("the token doesn't work")
	}
	if rs.Check(token, "42", "hash-2") {
		t.Error("the token works after the password has changed")
	}
	if rs.Check(token, "43", "hash-1") {
		t.Error("the token works for another user")
	}
}

func TestAResetTokenExpires(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	rs := newResets(1)
	rs.now = func() time.Time { return now }
	token := rs.Token("42", "hash-1")
	now = now.Add(59 * time.Minute)
	if !rs.Check(token, "42", "hash-1") {
		t.Error("the token expired early")
	}
	now = now.Add(time.Minute)
	if rs.Check(token, "42", "hash-1") {
		t.Error("the token works after an hour")
	}

	rs.Lifetime = 10 * time.Minute
	token = rs.Token("42", "hash-1")
	now = now.Add(10 * time.Minute)
	if rs.Check(token, "42", "hash-1") {
		t.Error("the token outlived its Lifetime")
	}
}

func TestAResetTokenFromAKeyBeingRotatedOutStillWorks(t *testing.T) {
	old := newResets(1).Token("42", "hash-1")
	rotated := &Resets{Keys: [][]byte{bytes.Repeat([]byte{2}, 32), bytes.Repeat([]byte{1}, 32)}}
	if !rotated.Check(old, "42", "hash-1") {
		t.Error("a token from the previous key doesn't work")
	}
	if !newResets(2).Check(rotated.Token("42", "hash-1"), "42", "hash-1") {
		t.Error("a new token isn't signed with the first key")
	}
}

func TestAResetTokenOnlyWorksWithTheKeyThatMadeIt(t *testing.T) {
	if newResets(2).Check(newResets(1).Token("42", "hash-1"), "42", "hash-1") {
		t.Error("another app's token works")
	}
}

func TestATamperedResetTokenDoesntWork(t *testing.T) {
	rs := newResets(1)
	token := rs.Token("42", "hash-1")
	expires, sig, _ := strings.Cut(token, ".")
	unix, _ := strconv.ParseInt(expires, 36, 64)
	later := strconv.FormatInt(unix+3600, 36)
	flipped := []byte(sig)
	flipped[0] ^= 3 // another character, whichever it was
	for _, bad := range []string{
		"", ".", token + "x", sig, later + "." + sig,
		expires + "." + string(flipped), "!!!." + sig, expires + ".not base64",
	} {
		if rs.Check(bad, "42", "hash-1") {
			t.Errorf("%q works", bad)
		}
	}
}

func TestAResetTokenFitsInAURLPath(t *testing.T) {
	token := newResets(1).Token("42", "$argon2id$v=19$m=19456,t=2,p=1$c2FsdA$aGFzaA")
	if !regexp.MustCompile(`^[0-9a-z]+\.[A-Za-z0-9_-]{22}$`).MatchString(token) {
		t.Errorf("token %q", token)
	}
}
