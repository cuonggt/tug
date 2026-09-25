package auth

import (
	"strings"
	"testing"
)

func TestAPasswordChecksAgainstItsHash(t *testing.T) {
	hash := HashPassword("correct horse battery staple")
	if !CheckPassword(hash, "correct horse battery staple") {
		t.Error("the password doesn't check against its own hash")
	}
	for _, wrong := range []string{"", "correct horse battery", "Correct horse battery staple"} {
		if CheckPassword(hash, wrong) {
			t.Errorf("%q checks", wrong)
		}
	}
}

func TestAHashIsArgon2idWithASaltOfItsOwn(t *testing.T) {
	a, b := HashPassword("secret"), HashPassword("secret")
	if a == b {
		t.Error("the same password hashed the same twice: the salt isn't random")
	}
	if !strings.HasPrefix(a, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Errorf("hash %s", a)
	}
	if NeedsRehash(a) {
		t.Error("a new hash needs rehashing")
	}
}

// The reference implementation's test vector (phc-winner-argon2, test.c):
// a hash made elsewhere, as by PHP's password_hash, checks here too.
const referenceHash = "$argon2id$v=19$m=65536,t=2,p=1$c29tZXNhbHQ$CTFhFdXPJO1aFaMaO6Mm5c8y7cJHAph8ArZWb2GRPPc"

func TestAHashFromAnotherArgon2idLibraryChecks(t *testing.T) {
	if !CheckPassword(referenceHash, "password") {
		t.Error("the reference hash doesn't check")
	}
	if CheckPassword(referenceHash, "passwore") {
		t.Error("the reference hash takes the wrong password")
	}
	if !NeedsRehash(referenceHash) {
		t.Error("a hash with other settings doesn't need rehashing")
	}
}

func TestNoHashOrABrokenOneMatchesNothing(t *testing.T) {
	for _, hash := range []string{
		"",
		"$2y$12$R9h/cIPz0gi.URNNX3kh2OPST9/PgBkqquzi.Ss7KIUgO2t0jWMUW", // bcrypt
		"$argon2i$v=19$m=65536,t=2,p=1$c29tZXNhbHQ$RdescudvJCsgt3ub+b+dWRWJTmaaJObG",
		"$argon2id$v=16$m=65536,t=2,p=1$c29tZXNhbHQ$CTFhFdXPJO1aFaMaO6Mm5c8y7cJHAph8ArZWb2GRPPc",
		"$argon2id$v=19$m=65536,t=2,p=1,x=1$c29tZXNhbHQ$CTFhFdXPJO1aFaMaO6Mm5c8y7cJHAph8ArZWb2GRPPc",
		"$argon2id$v=19$m=999999999,t=2,p=1$c29tZXNhbHQ$CTFhFdXPJO1aFaMaO6Mm5c8y7cJHAph8ArZWb2GRPPc", // 1 TB
		"$argon2id$v=19$m=65536,t=2,p=1$c29tZXNhbHQ$not base64!",
		"$argon2id$v=19$m=65536,t=2,p=1$$CTFhFdXPJO1aFaMaO6Mm5c8y7cJHAph8ArZWb2GRPPc",
		"password",
	} {
		if CheckPassword(hash, "password") {
			t.Errorf("%q matched", hash)
		}
		if !NeedsRehash(hash) {
			t.Errorf("%q doesn't need rehashing", hash)
		}
	}
}
