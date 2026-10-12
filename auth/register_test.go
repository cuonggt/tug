package auth

import (
	"bytes"
	"testing"
	"time"
)

func newRegistrations(key byte) *Registrations {
	return &Registrations{Keys: [][]byte{bytes.Repeat([]byte{key}, 32)}}
}

func TestARegistrationTokenWorksForTheEmailAndNameItWasMadeFor(t *testing.T) {
	rs := newRegistrations(1)
	token := rs.Token("ann@example.com", "Ann Lee")
	if !rs.Check(token, "ann@example.com", "Ann Lee") || !rs.Check(token, "ann@example.com", "Ann Lee") {
		t.Fatal("the token doesn't work, or works only once")
	}
	if rs.Check(token, "bob@example.com", "Ann Lee") {
		t.Error("the token works for another email")
	}
	if rs.Check(token, "ann@example.com", "Mallory") {
		t.Error("the token works for another name")
	}
	if rs.Check(token, "ann@example.comAnn", " Lee") {
		t.Error("the token works for the same letters split another way")
	}
}

func TestARegistrationTokenExpiresAfterADay(t *testing.T) {
	now := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	rs := newRegistrations(1)
	rs.now = func() time.Time { return now }
	token := rs.Token("ann@example.com", "Ann Lee")
	now = now.Add(23 * time.Hour)
	if !rs.Check(token, "ann@example.com", "Ann Lee") {
		t.Error("the token expired early")
	}
	now = now.Add(time.Hour)
	if rs.Check(token, "ann@example.com", "Ann Lee") {
		t.Error("the token works after a day")
	}
}

func TestARegistrationTokenIsNoVerificationTokenNorTheOtherWayRound(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	rs, vs := &Registrations{Keys: [][]byte{key}}, &Verifications{Keys: [][]byte{key}}
	if vs.Check(rs.Token("42", "ann@example.com"), "42", "ann@example.com") {
		t.Error("a registration token verifies an email")
	}
	if rs.Check(vs.Token("ann@example.com", "Ann Lee"), "ann@example.com", "Ann Lee") {
		t.Error("a verification token finishes a registration")
	}
}

func TestARegistrationTokenFromAKeyBeingRotatedOutStillWorks(t *testing.T) {
	old := newRegistrations(1).Token("ann@example.com", "Ann Lee")
	rotated := &Registrations{Keys: [][]byte{bytes.Repeat([]byte{2}, 32), bytes.Repeat([]byte{1}, 32)}}
	if !rotated.Check(old, "ann@example.com", "Ann Lee") {
		t.Error("a token from the previous key doesn't work")
	}
	if newRegistrations(3).Check(old, "ann@example.com", "Ann Lee") {
		t.Error("another app's token works")
	}
}
