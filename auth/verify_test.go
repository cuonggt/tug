package auth

import (
	"bytes"
	"testing"
	"time"
)

func newVerifications(key byte) *Verifications {
	return &Verifications{Keys: [][]byte{bytes.Repeat([]byte{key}, 32)}}
}

func TestAVerificationTokenWorksForTheEmailItWasMadeFor(t *testing.T) {
	vs := newVerifications(1)
	token := vs.Token("42", "ann@example.com")
	if !vs.Check(token, "42", "ann@example.com") || !vs.Check(token, "42", "ann@example.com") {
		t.Fatal("the token doesn't work, or works only once")
	}
	if vs.Check(token, "42", "ann@elsewhere.example") {
		t.Error("the token works after the email has changed")
	}
	if vs.Check(token, "43", "ann@example.com") {
		t.Error("the token works for another user")
	}
}

func TestAVerificationTokenExpiresAfterADay(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	vs := newVerifications(1)
	vs.now = func() time.Time { return now }
	token := vs.Token("42", "ann@example.com")
	now = now.Add(23 * time.Hour)
	if !vs.Check(token, "42", "ann@example.com") {
		t.Error("the token expired early")
	}
	now = now.Add(time.Hour)
	if vs.Check(token, "42", "ann@example.com") {
		t.Error("the token works after a day")
	}
}

func TestAVerificationTokenIsNoResetTokenNorTheOtherWayRound(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	vs, rs := &Verifications{Keys: [][]byte{key}}, &Resets{Keys: [][]byte{key}}
	if rs.Check(vs.Token("42", "same"), "42", "same") {
		t.Error("a verification token resets a password")
	}
	if vs.Check(rs.Token("42", "same"), "42", "same") {
		t.Error("a reset token verifies an email")
	}
}

func TestAVerificationTokenFromAKeyBeingRotatedOutStillWorks(t *testing.T) {
	old := newVerifications(1).Token("42", "ann@example.com")
	rotated := &Verifications{Keys: [][]byte{bytes.Repeat([]byte{2}, 32), bytes.Repeat([]byte{1}, 32)}}
	if !rotated.Check(old, "42", "ann@example.com") {
		t.Error("a token from the previous key doesn't work")
	}
	if newVerifications(3).Check(old, "42", "ann@example.com") {
		t.Error("another app's token works")
	}
}
