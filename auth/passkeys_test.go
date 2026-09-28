package auth

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cuonggt/tug/auth/passkeytest"
	"github.com/cuonggt/tug/session"
)

// site is a site that takes passkeys, as example.com.
func site() *Passkeys {
	return &Passkeys{RPID: "example.com", Origin: "https://example.com", Name: "Example"}
}

var ann = PasskeyUser{Handle: []byte("a handle of Ann's"), Name: "ann@example.com", DisplayName: "Ann"}

// register adds a passkey from key, as a browser does in two requests,
// and returns what FinishRegistration returned.
func register(t *testing.T, b *browser, p *Passkeys, key *passkeytest.Authenticator, exclude ...[]byte) (Passkey, error) {
	t.Helper()
	var options []byte
	b.request("POST", "/passkeys/options", func(s *session.Session, _ *http.Request) {
		var err error
		if options, err = p.StartRegistration(s, ann, exclude); err != nil {
			t.Fatal(err)
		}
	})
	answer, err := key.Create(options)
	if err != nil {
		return Passkey{}, err
	}
	return finishRegistration(b, p, answer)
}

func finishRegistration(b *browser, p *Passkeys, answer []byte) (pk Passkey, err error) {
	b.request("POST", "/passkeys", func(s *session.Session, _ *http.Request) {
		pk, err = p.FinishRegistration(s, answer)
	})
	return pk, err
}

// startLogin returns StartLogin's options, for a login allowing allow.
func startLogin(t *testing.T, b *browser, p *Passkeys, allow ...Passkey) []byte {
	t.Helper()
	var options []byte
	b.request("POST", "/login/passkey/options", func(s *session.Session, _ *http.Request) {
		var err error
		if options, err = p.StartLogin(s, allow); err != nil {
			t.Fatal(err)
		}
	})
	return options
}

func finishLogin(b *browser, p *Passkeys, answer []byte, kept ...Passkey) (pk Passkey, err error) {
	b.request("POST", "/login/passkey", func(s *session.Session, _ *http.Request) {
		pk, err = p.FinishLogin(s, answer, func(id []byte) (Passkey, error) {
			for _, k := range kept {
				if bytes.Equal(k.ID, id) {
					return k, nil
				}
			}
			return Passkey{}, errors.New("no such passkey")
		})
	})
	return pk, err
}

// login logs in with a passkey from key, with the passkeys kept.
func login(t *testing.T, b *browser, p *Passkeys, key *passkeytest.Authenticator, kept ...Passkey) (Passkey, error) {
	t.Helper()
	answer, err := key.Get(startLogin(t, b, p))
	if err != nil {
		t.Fatal(err)
	}
	return finishLogin(b, p, answer, kept...)
}

func TestAPasskeyIsMadeAndLogsIn(t *testing.T) {
	for name, alg := range map[string]int{"ES256": -7, "Ed25519": -8, "RSA": -257} {
		t.Run(name, func(t *testing.T) {
			p, b := site(), newBrowser(t)
			key := passkeytest.New("https://example.com")
			key.Alg = alg
			pk, err := register(t, b, p, key)
			if err != nil {
				t.Fatal(err)
			}
			if len(pk.ID) == 0 || len(pk.PublicKey) == 0 || !bytes.Equal(pk.UserHandle, ann.Handle) || pk.SignCount != 0 || pk.BackedUp || !slices.Equal(pk.Transports, []string{"internal", "hybrid"}) {
				t.Errorf("made %+v", pk)
			}
			for want := uint32(1); want <= 2; want++ {
				after, err := login(t, b, p, key, pk)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(after.ID, pk.ID) || after.SignCount != want {
					t.Errorf("login %d: %+v, want its count at %d", want, after, want)
				}
				pk = after
			}
		})
	}
}

func TestAPasskeyThatSyncsCountsNothing(t *testing.T) {
	p, b := site(), newBrowser(t)
	key := passkeytest.New("https://example.com")
	key.Synced = true
	pk, err := register(t, b, p, key)
	if err != nil || !pk.BackedUp {
		t.Fatalf("made %+v, %v", pk, err)
	}
	for range 2 {
		if pk, err = login(t, b, p, key, pk); err != nil || pk.SignCount != 0 {
			t.Fatalf("logged in as %+v, %v: want a count of 0, and in", pk, err)
		}
	}
}

func TestAPasskeyWorksOnlyForTheSiteItWasMadeFor(t *testing.T) {
	p := site()
	// A phishing page's answer names its own origin.
	phished := passkeytest.New("https://examp1e.com")
	if _, err := register(t, newBrowser(t), p, phished); err == nil || !strings.Contains(err.Error(), "https://examp1e.com") {
		t.Errorf("a passkey made from another origin: %v", err)
	}

	// A passkey made for another site answers for that site.
	b, key := newBrowser(t), passkeytest.New("https://example.com")
	other := &Passkeys{RPID: "other.example", Origin: "https://example.com", Name: "Other"}
	var options []byte
	b.request("POST", "/passkeys/options", func(s *session.Session, _ *http.Request) {
		options, _ = other.StartRegistration(s, ann, nil)
	})
	answer, err := key.Create(options)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := finishRegistration(b, p, answer); err == nil || !strings.Contains(err.Error(), "another site") {
		t.Errorf("a passkey made for another site: %v", err)
	}

	pk, err := register(t, b, p, key)
	if err != nil {
		t.Fatal(err)
	}
	key.Origin = "https://examp1e.com"
	if _, err := login(t, b, p, key, pk); err == nil {
		t.Error("logged in from another origin")
	}
}

func TestAPasskeyIsUnlockedEachTime(t *testing.T) {
	p, b := site(), newBrowser(t)
	key := passkeytest.New("https://example.com")
	key.Unverified = true
	if _, err := register(t, b, p, key); err == nil || !strings.Contains(err.Error(), "PIN") {
		t.Errorf("made a passkey with no PIN, fingerprint or face: %v", err)
	}
	key.Unverified = false
	pk, err := register(t, b, p, key)
	if err != nil {
		t.Fatal(err)
	}
	key.Unverified = true
	if _, err := login(t, b, p, key, pk); err == nil || !strings.Contains(err.Error(), "PIN") {
		t.Errorf("logged in with no PIN, fingerprint or face: %v", err)
	}
}

func TestAChallengeIsAnsweredOnceWithinFiveMinutes(t *testing.T) {
	start := time.Now()
	defer func() { now = time.Now }()
	p, b := site(), newBrowser(t)
	key := passkeytest.New("https://example.com")
	pk, err := register(t, b, p, key)
	if err != nil {
		t.Fatal(err)
	}

	answer, _ := key.Get(startLogin(t, b, p))
	if _, err := finishLogin(b, p, answer, pk); err != nil {
		t.Fatal(err)
	}
	if _, err := finishLogin(b, p, answer, pk); !errors.Is(err, errNoChallenge) {
		t.Errorf("the same answer twice: %v", err)
	}

	// An answer to options that others have replaced.
	first := startLogin(t, b, p)
	startLogin(t, b, p)
	answer, _ = key.Get(first)
	if _, err := finishLogin(b, p, answer, pk); err == nil || !strings.Contains(err.Error(), "another challenge") {
		t.Errorf("an answer to options replaced since: %v", err)
	}

	now = func() time.Time { return start }
	answer, _ = key.Get(startLogin(t, b, p))
	now = func() time.Time { return start.Add(5 * time.Minute) }
	if _, err := finishLogin(b, p, answer, pk); !errors.Is(err, errNoChallenge) {
		t.Errorf("an answer five minutes on: %v", err)
	}
	now = time.Now

	// A login's answer where a new passkey's was asked for.
	answer, _ = key.Get(startLogin(t, b, p))
	b.request("POST", "/passkeys/options", func(s *session.Session, _ *http.Request) {
		p.StartRegistration(s, ann, nil)
	})
	if _, err := finishRegistration(b, p, answer); err == nil || !strings.Contains(err.Error(), `"webauthn.get"`) {
		t.Errorf("a login's answer made a passkey: %v", err)
	}
}

func TestALoginChecksTheSignatureAndTheCount(t *testing.T) {
	p, b := site(), newBrowser(t)
	key := passkeytest.New("https://example.com")
	pk, err := register(t, b, p, key)
	if err != nil {
		t.Fatal(err)
	}
	copied := key.Clone()

	// A signature changed on its way.
	answer, _ := key.Get(startLogin(t, b, p))
	var j map[string]any
	json.Unmarshal(answer, &j)
	response := j["response"].(map[string]any)
	sig, _ := unb64url(response["signature"].(string))
	sig[len(sig)-1] ^= 1
	response["signature"] = b64url(sig)
	answer, _ = json.Marshal(j)
	if _, err := finishLogin(b, p, answer, pk); err == nil || !strings.Contains(err.Error(), "signature") {
		t.Errorf("a signature changed: %v", err)
	}

	// The original and its copy both count on from 1.
	if pk, err = login(t, b, p, key, pk); err != nil || pk.SignCount != 2 {
		t.Fatalf("the original: %+v, %v", pk, err)
	}
	if _, err := login(t, b, p, copied, pk); err == nil || !strings.Contains(err.Error(), "copied") {
		t.Errorf("a copy whose count went back: %v", err)
	}
}

func TestALoginTakesOnlyAPasskeyTheAppHasForTheUserItNames(t *testing.T) {
	p, b := site(), newBrowser(t)
	key := passkeytest.New("https://example.com")
	pk, err := register(t, b, p, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := login(t, b, p, key); err == nil || !strings.Contains(err.Error(), "no such passkey") {
		t.Errorf("a passkey the app doesn't have: %v", err)
	}
	someoneElses := pk
	someoneElses.UserHandle = []byte("a handle of Bob's")
	if _, err := login(t, b, p, key, someoneElses); err == nil || !strings.Contains(err.Error(), "another user") {
		t.Errorf("a passkey kept as another user's: %v", err)
	}

	// Confirming, the login allows the user's own passkeys alone.
	other := passkeytest.New("https://example.com")
	otherPK, err := register(t, b, p, other)
	if err != nil {
		t.Fatal(err)
	}
	answer, err := other.Get(startLogin(t, b, p, pk))
	if err == nil {
		t.Fatalf("an authenticator answered with a passkey the options don't allow: %s", answer)
	}
	// As a browser that ignored the options would.
	answer, _ = other.Get(startLogin(t, b, p))
	startLogin(t, b, p, pk)
	if _, err := finishLogin(b, p, answer, pk, otherPK); err == nil || !strings.Contains(err.Error(), "didn't allow") {
		t.Errorf("a passkey the login didn't allow: %v", err)
	}
}

func TestTheOptionsAskForAPasskeyKeptOnTheDeviceAndUnlocked(t *testing.T) {
	p, b := site(), newBrowser(t)
	var created []byte
	b.request("POST", "/passkeys/options", func(s *session.Session, _ *http.Request) {
		created, _ = p.StartRegistration(s, ann, [][]byte{[]byte("kept already")})
	})
	var c struct {
		RP                     map[string]string
		User                   map[string]string
		Challenge              string
		PubKeyCredParams       []map[string]any
		ExcludeCredentials     []map[string]string
		AuthenticatorSelection map[string]any
		Attestation            string
	}
	if err := json.Unmarshal(created, &c); err != nil {
		t.Fatal(err)
	}
	challenge, _ := unb64url(c.Challenge)
	if c.RP["id"] != "example.com" || c.RP["name"] != "Example" || c.User["id"] != b64url(ann.Handle) || c.User["name"] != "ann@example.com" ||
		len(challenge) != 32 || len(c.PubKeyCredParams) != 3 || len(c.ExcludeCredentials) != 1 || c.ExcludeCredentials[0]["id"] != b64url([]byte("kept already")) ||
		c.AuthenticatorSelection["residentKey"] != "required" || c.AuthenticatorSelection["userVerification"] != "required" || c.Attestation != "none" {
		t.Errorf("creation options %s", created)
	}

	var r struct {
		RPID             string
		AllowCredentials []map[string]any
		UserVerification string
	}
	json.Unmarshal(startLogin(t, b, p, Passkey{ID: []byte("one"), Transports: []string{"usb"}}), &r)
	if r.RPID != "example.com" || r.UserVerification != "required" || len(r.AllowCredentials) != 1 || r.AllowCredentials[0]["id"] != b64url([]byte("one")) {
		t.Errorf("request options %+v", r)
	}
}

func TestAnAuthenticatorThatHasAPasskeyForTheAccountMakesNoOther(t *testing.T) {
	p, b := site(), newBrowser(t)
	key := passkeytest.New("https://example.com")
	pk, err := register(t, b, p, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := register(t, b, p, key, pk.ID); err == nil {
		t.Error("made another passkey where the options exclude the one it has")
	}
}

func TestDecodeCBORRefusesWhatWebAuthnDoesntWrite(t *testing.T) {
	for name, b := range map[string][]byte{
		"nothing":                  {},
		"a length left open":       {0x5f, 0x41, 0x00, 0xff},
		"a tag":                    {0xc1, 0x00},
		"a float":                  {0xf9, 0x3c, 0x00},
		"undefined":                {0xf7},
		"a string past the end":    {0x45, 0x01},
		"an array longer than all": {0x9a, 0xff, 0xff, 0xff, 0xff},
		"a map longer than all":    {0xbb, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		"a key twice":              {0xa2, 0x01, 0x00, 0x01, 0x00},
		"a key that's a list":      {0xa1, 0x80, 0x00},
		"text that isn't UTF-8":    {0x61, 0xff},
		"an integer too big":       {0x1b, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		"nesting too deep":         bytes.Repeat([]byte{0x81}, 20),
	} {
		if v, _, err := decodeCBOR(b); err == nil {
			t.Errorf("%s: read %v", name, v)
		}
	}
	v, rest, err := decodeCBOR([]byte{0xa2, 0x01, 0x02, 0x20, 0x43, 1, 2, 3, 0x99})
	if m, ok := v.(map[any]any); err != nil || !ok || m[int64(1)] != int64(2) || !bytes.Equal(m[int64(-1)].([]byte), []byte{1, 2, 3}) || !bytes.Equal(rest, []byte{0x99}) {
		t.Errorf("read %v, %x, %v", v, rest, err)
	}
}

func FuzzDecodeCBOR(f *testing.F) {
	f.Add([]byte{0xa2, 0x01, 0x02, 0x20, 0x43, 1, 2, 3})
	f.Add([]byte{0x9f, 0xff})
	f.Fuzz(func(t *testing.T, b []byte) {
		decodeCBOR(b)
		parseCOSEKey(b)
		parseAuthData(b)
	})
}

func TestAPasskeysKeyIsOneItTakes(t *testing.T) {
	small, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	offCurve := make([]byte, 32)
	offCurve[31] = 7
	ec, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	point, _ := ec.PublicKey.Bytes()
	for name, key := range map[string][]byte{
		"an RSA key of 1024 bits": cose(t, map[int]any{1: 3, 3: -257, -1: small.N.Bytes(), -2: big.NewInt(int64(small.E)).Bytes()}),
		"a point off P-256":       cose(t, map[int]any{1: 2, 3: -7, -1: 1, -2: offCurve, -3: offCurve}),
		"a P-384 key":             cose(t, map[int]any{1: 2, 3: -7, -1: 2, -2: point[1:33], -3: point[33:]}),
		"ES384":                   cose(t, map[int]any{1: 2, 3: -35, -1: 1, -2: point[1:33], -3: point[33:]}),
		"no type":                 cose(t, map[int]any{3: -7}),
	} {
		if _, _, err := parseCOSEKey(key); err == nil {
			t.Errorf("%s was taken", name)
		}
	}
	if _, _, err := parseCOSEKey(cose(t, map[int]any{1: 2, 3: -7, -1: 1, -2: point[1:33], -3: point[33:]})); err != nil {
		t.Errorf("a P-256 key: %v", err)
	}
}

// cose writes a COSE key's map, with its integer labels, as CBOR.
func cose(t *testing.T, m map[int]any) []byte {
	t.Helper()
	b := []byte{0xa0 | byte(len(m))}
	for k, v := range m {
		b = appendCBORInt(b, k)
		switch v := v.(type) {
		case int:
			b = appendCBORInt(b, v)
		case []byte:
			if len(v) < 24 {
				b = append(b, 0x40|byte(len(v)))
			} else if len(v) < 256 {
				b = append(b, 0x58, byte(len(v)))
			} else {
				b = append(b, 0x59, byte(len(v)>>8), byte(len(v)))
			}
			b = append(b, v...)
		}
	}
	return b
}

func appendCBORInt(b []byte, n int) []byte {
	major, v := byte(0), n
	if n < 0 {
		major, v = 1, -1-n
	}
	switch {
	case v < 24:
		return append(b, major<<5|byte(v))
	case v < 256:
		return append(b, major<<5|24, byte(v))
	}
	return append(b, major<<5|25, byte(v>>8), byte(v))
}
