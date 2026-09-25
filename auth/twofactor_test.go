package auth

import (
	"bytes"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cuonggt/tug/session"
)

func newTwoFactor(key byte) *TwoFactor {
	return &TwoFactor{Keys: [][]byte{bytes.Repeat([]byte{key}, 32)}, Issuer: "Blog"}
}

// The secret of RFC 6238's test vectors, "12345678901234567890", in base32.
const rfcSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

func TestACodeIsTheOneRFC6238Makes(t *testing.T) {
	// RFC 6238's SHA-1 vectors, whose eight digits end in these six.
	for unix, code := range map[int64]string{
		59:          "287082",
		1111111109:  "081804",
		1111111111:  "050471",
		1234567890:  "005924",
		2000000000:  "279037",
		20000000000: "353130",
	} {
		tf := newTwoFactor(1)
		tf.now = func() time.Time { return time.Unix(unix, 0) }
		if step, ok := tf.Check(rfcSecret, code, 0); !ok || step != unix/30 {
			t.Errorf("at %d, %s: step %d, %v", unix, code, step, ok)
		}
		if _, ok := tf.Check(rfcSecret, "000000", 0); ok && code != "000000" {
			t.Errorf("at %d, a wrong code checks", unix)
		}
	}
}

func TestACodeWorksOnceAndNoneFromBeforeIt(t *testing.T) {
	now := time.Unix(1234567890, 0)
	tf := newTwoFactor(1)
	tf.now = func() time.Time { return now }
	step, ok := tf.Check(rfcSecret, "005924", 0)
	if !ok {
		t.Fatal("the code doesn't check")
	}
	if _, ok := tf.Check(rfcSecret, "005924", step); ok {
		t.Error("the code worked twice")
	}
	earlier := totp(mustDecode(t, rfcSecret), step-1)
	if _, ok := tf.Check(rfcSecret, earlier, step); ok {
		t.Error("a code from before the last one worked")
	}
	now = now.Add(30 * time.Second)
	if _, ok := tf.Check(rfcSecret, totp(mustDecode(t, rfcSecret), step+1), step); !ok {
		t.Error("the next code doesn't work")
	}
}

func TestACodeFromAPhoneAStepOffWorksAndTwoStepsOffDoesnt(t *testing.T) {
	now := time.Unix(1234567890, 0)
	tf := newTwoFactor(1)
	tf.now = func() time.Time { return now }
	key, step := mustDecode(t, rfcSecret), now.Unix()/30
	for off, want := range map[int64]bool{-2: false, -1: true, 0: true, 1: true, 2: false} {
		if _, ok := tf.Check(rfcSecret, totp(key, step+off), 0); ok != want {
			t.Errorf("a code %d steps off: %v", off, ok)
		}
	}
	if _, ok := tf.Check(rfcSecret, "005 924", 0); !ok {
		t.Error("a code typed with a space doesn't check")
	}
	for _, bad := range []string{"", "5924", "0059245", "abcdef"} {
		if _, ok := tf.Check(rfcSecret, bad, 0); ok {
			t.Errorf("%q checks", bad)
		}
	}
	if _, ok := tf.Check("not base32!", "005924", 0); ok {
		t.Error("a code checks against a secret that isn't one")
	}
}

func TestCodeMakesWhatTheAppShows(t *testing.T) {
	tf := newTwoFactor(1)
	if code, err := tf.Code(rfcSecret, time.Unix(1234567890, 0)); err != nil || code != "005924" {
		t.Errorf("got %q, %v", code, err)
	}
	now := time.Now()
	code, _ := tf.Code(rfcSecret, now)
	step, ok := tf.Check(rfcSecret, code, 0)
	if next, _ := tf.Code(rfcSecret, now.Add(30*time.Second)); !ok {
		t.Error("a code for now doesn't check")
	} else if _, ok := tf.Check(rfcSecret, next, step); !ok {
		t.Error("the code for 30 seconds on doesn't check after it")
	}
	if _, err := tf.Code("not base32!", now); err == nil {
		t.Error("a secret that isn't one made a code")
	}
}

func TestANewSecretIs160RandomBitsInBase32(t *testing.T) {
	tf := newTwoFactor(1)
	a, b := tf.NewSecret(), tf.NewSecret()
	if a == b || len(mustDecode(t, a)) != 20 || !regexp.MustCompile(`^[A-Z2-7]{32}$`).MatchString(a) {
		t.Errorf("secrets %q and %q", a, b)
	}
}

func TestTheURLIsTheOneAuthenticatorAppsScan(t *testing.T) {
	tf := &TwoFactor{Keys: [][]byte{bytes.Repeat([]byte{1}, 32)}, Issuer: "Ann's Blog: Notes"}
	want := "otpauth://totp/Ann%27s%20Blog%3A%20Notes:ann@example.com?issuer=Ann%27s%20Blog%3A%20Notes&secret=" + rfcSecret
	if got := tf.URL("ann@example.com", rfcSecret); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	tf.Issuer = ""
	if got := tf.URL("ann@example.com", rfcSecret); got != "otpauth://totp/ann@example.com?secret="+rfcSecret {
		t.Errorf("without an issuer: %s", got)
	}
}

func TestASealedSecretOpensWithTheKeyThatSealedIt(t *testing.T) {
	tf := newTwoFactor(1)
	sealed := tf.Seal(rfcSecret)
	if strings.Contains(sealed, rfcSecret) || sealed == tf.Seal(rfcSecret) {
		t.Errorf("sealed %q: readable, or the same twice", sealed)
	}
	if got, err := tf.Open(sealed); err != nil || got != rfcSecret {
		t.Fatalf("opened %q, %v", got, err)
	}
	if _, err := newTwoFactor(2).Open(sealed); err == nil {
		t.Error("another key opened it")
	}
	b := []byte(sealed)
	b[len(b)-1] ^= 1
	for _, bad := range []string{string(b), "", "short", "not base64!"} {
		if _, err := tf.Open(bad); err == nil {
			t.Errorf("%q opened", bad)
		}
	}
}

func TestASecretSealedBeforeTheKeyWasRotatedIsStaleUntilSealedAgain(t *testing.T) {
	sealed := newTwoFactor(1).Seal(rfcSecret)
	rotated := &TwoFactor{Keys: [][]byte{bytes.Repeat([]byte{2}, 32), bytes.Repeat([]byte{1}, 32)}}
	secret, err := rotated.Open(sealed)
	if err != nil || secret != rfcSecret || !rotated.Stale(sealed) {
		t.Fatalf("opened %q, %v; stale %v", secret, err, rotated.Stale(sealed))
	}
	again := rotated.Seal(secret)
	if rotated.Stale(again) {
		t.Error("sealed again with the new key, it's still stale")
	}
	if got, err := newTwoFactor(2).Open(again); err != nil || got != rfcSecret {
		t.Errorf("with the old key gone: %q, %v", got, err)
	}
}

func TestARecoveryCodeWorksOnce(t *testing.T) {
	codes := NewRecoveryCodes()
	if len(codes) != 8 || len(slices.Compact(slices.Sorted(slices.Values(codes)))) != 8 {
		t.Fatalf("codes %q", codes)
	}
	for _, c := range codes {
		if !regexp.MustCompile(`^[a-z2-7]{5}-[a-z2-7]{5}$`).MatchString(c) {
			t.Errorf("code %q", c)
		}
	}
	typed := strings.ToUpper(strings.ReplaceAll(codes[3], "-", " "))
	rest, ok := UseRecoveryCode(codes, typed)
	if !ok || len(rest) != 7 || slices.Contains(rest, codes[3]) || len(codes) != 8 {
		t.Fatalf("using %q: %v, left %q", typed, ok, rest)
	}
	if _, ok := UseRecoveryCode(rest, codes[3]); ok {
		t.Error("a code worked twice")
	}
	for _, bad := range []string{"", "-", "aaaaa-aaaaa", codes[0][:5]} {
		if _, ok := UseRecoveryCode(codes, bad); ok {
			t.Errorf("%q worked", bad)
		}
	}
}

func TestALoginHeldBackForASecondFactorIsntALogin(t *testing.T) {
	b := newBrowser(t)
	b.request("POST", "/login", func(s *session.Session, _ *http.Request) {
		StartTwoFactor(s, "42")
	})
	b.request("GET", "/two-factor-challenge", func(s *session.Session, _ *http.Request) {
		if _, ok := UserID(s); ok {
			t.Error("the session is logged in before the second factor")
		}
		if id, ok := TwoFactorPending(s); !ok || id != "42" {
			t.Errorf("pending %q, %v", id, ok)
		}
		Login(s, "42", "hash-1")
		if _, ok := TwoFactorPending(s); ok {
			t.Error("the login is still pending after Login")
		}
	})
}

func TestALoginHeldBackForASecondFactorWaitsTenMinutes(t *testing.T) {
	start := time.Now()
	defer func() { now = time.Now }()
	b := newBrowser(t)
	b.request("POST", "/login", func(s *session.Session, _ *http.Request) {
		Login(s, "7", "hash-7") // someone else, who's logged out by it
		StartTwoFactor(s, "42")
	})
	b.request("GET", "/two-factor-challenge", func(s *session.Session, _ *http.Request) {
		if _, ok := UserID(s); ok {
			t.Error("the login before is still there")
		}
		now = func() time.Time { return start.Add(9 * time.Minute) }
		if _, ok := TwoFactorPending(s); !ok {
			t.Error("the login stopped waiting early")
		}
		now = func() time.Time { return start.Add(10 * time.Minute) }
		if _, ok := TwoFactorPending(s); ok {
			t.Error("the login still waits after ten minutes")
		}
	})
}

func TestAConfirmedPasswordLastsAsLongAsTheAppAllows(t *testing.T) {
	start := time.Now()
	defer func() { now = time.Now }()
	b := newBrowser(t)
	b.request("POST", "/confirm-password", func(s *session.Session, _ *http.Request) {
		Login(s, "42", "hash-1")
		if PasswordConfirmed(s, time.Hour) {
			t.Error("confirmed before it was")
		}
		SetPasswordConfirmed(s)
	})
	b.request("GET", "/settings", func(s *session.Session, _ *http.Request) {
		now = func() time.Time { return start.Add(59 * time.Minute) }
		if !PasswordConfirmed(s, time.Hour) {
			t.Error("the confirmation didn't last from one request to the next")
		}
		now = func() time.Time { return start.Add(61 * time.Minute) }
		if PasswordConfirmed(s, time.Hour) {
			t.Error("the confirmation lasted past its hour")
		}
		now = time.Now
		Login(s, "42", "hash-2")
		if PasswordConfirmed(s, time.Hour) {
			t.Error("a new login kept the old one's confirmation")
		}
	})
	if PasswordConfirmed(nil, time.Hour) {
		t.Error("no session is confirmed")
	}
}

func mustDecode(t *testing.T, secret string) []byte {
	t.Helper()
	key, err := b32.DecodeString(secret)
	if err != nil {
		t.Fatal(err)
	}
	return key
}
