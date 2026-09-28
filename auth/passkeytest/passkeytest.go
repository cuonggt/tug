// Package passkeytest is a passkey authenticator in software, for tests of
// an app's passkeys. It answers the options an app's auth.Passkeys gives,
// as JSON, as a browser does with a phone or a password manager: it makes
// passkeys, and logs in with them. Its keys are real ES256, Ed25519 or RSA
// keys, and its answers carry real signatures.
//
//	key := passkeytest.New("https://example.com")
//	answer, err := key.Create(options) // StartRegistration's options
//	...
//	answer, err = key.Get(options) // StartLogin's
//
// An Authenticator is one device, for one goroutine at a time.
package passkeytest

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"slices"
	"strings"
)

// Authenticator makes passkeys and logs in with them, as a phone, a laptop
// or a password manager does. New makes one.
type Authenticator struct {
	// Origin is the page's origin, which the browser tells the site in each
	// answer: another site's is what a phishing page would send.
	Origin string

	// Alg is the COSE algorithm of the passkeys it makes: -7 for ES256, as
	// it is to begin with, -8 for Ed25519, or -257 for RSA.
	Alg int

	// Synced has it make passkeys as a password manager does, which sync to
	// the user's other devices, and count nothing.
	Synced bool

	// Unverified has it answer with the user there, but without their PIN,
	// fingerprint or face.
	Unverified bool

	passkeys []*passkey
}

type passkey struct {
	id     []byte
	rpID   string
	user   []byte
	signer crypto.Signer
	alg    int
	cose   []byte // the public key, as COSE
	count  uint32
	synced bool
}

// New returns an authenticator for pages of origin, with no passkeys yet.
func New(origin string) *Authenticator {
	return &Authenticator{Origin: origin, Alg: -7}
}

// Create answers the options of navigator.credentials.create, as JSON, as
// a browser does: it makes a passkey for the site and the user they name,
// in place of one it has for them already, and returns the browser's
// answer, as JSON. It fails, as a browser does, when it has a passkey the
// options exclude, or the site takes none of the algorithms it makes.
func (a *Authenticator) Create(options []byte) ([]byte, error) {
	var o struct {
		RP struct {
			ID string `json:"id"`
		} `json:"rp"`
		User struct {
			ID string `json:"id"`
		} `json:"user"`
		Challenge        string `json:"challenge"`
		PubKeyCredParams []struct {
			Alg int `json:"alg"`
		} `json:"pubKeyCredParams"`
		ExcludeCredentials []struct {
			ID string `json:"id"`
		} `json:"excludeCredentials"`
	}
	if err := json.Unmarshal(options, &o); err != nil {
		return nil, fmt.Errorf("passkeytest: the options aren't navigator.credentials.create's: %w", err)
	}
	rpID, err := a.site(o.RP.ID)
	if err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(o.PubKeyCredParams, func(p struct {
		Alg int `json:"alg"`
	}) bool {
		return p.Alg == a.Alg
	}) {
		return nil, fmt.Errorf("passkeytest: the site takes none of the algorithms it makes, %d", a.Alg)
	}
	for _, ex := range o.ExcludeCredentials {
		id, _ := unb64(ex.ID)
		if slices.ContainsFunc(a.passkeys, func(pk *passkey) bool { return string(pk.id) == string(id) }) {
			return nil, errors.New("passkeytest: it has a passkey for this account already, which the options exclude")
		}
	}
	user, err := unb64(o.User.ID)
	if err != nil || len(user) == 0 {
		return nil, errors.New("passkeytest: the options' user has no ID")
	}
	pk, err := newPasskey(a.Alg)
	if err != nil {
		return nil, err
	}
	pk.id, pk.rpID, pk.user, pk.synced = random(32), rpID, user, a.Synced
	// An authenticator keeps one passkey for each site and account.
	a.passkeys = slices.DeleteFunc(a.passkeys, func(old *passkey) bool {
		return old.rpID == rpID && string(old.user) == string(user)
	})
	a.passkeys = append(a.passkeys, pk)

	data := a.authData(pk, 0x40) // a new passkey comes with it
	data = append(data, make([]byte, 16)...)
	data = binary.BigEndian.AppendUint16(data, uint16(len(pk.id)))
	data = append(data, pk.id...)
	data = append(data, pk.cose...)
	// The attestation of a device that was asked for none.
	attestation := cborAppend(nil, cborMap{{"fmt", "none"}, {"attStmt", cborMap{}}, {"authData", data}})
	return json.Marshal(map[string]any{
		"id":    b64(pk.id),
		"rawId": b64(pk.id),
		"type":  "public-key",
		"response": map[string]any{
			"clientDataJSON":    b64(a.clientData("webauthn.create", o.Challenge)),
			"attestationObject": b64(attestation),
			"transports":        []string{"internal", "hybrid"},
		},
		"clientExtensionResults":  map[string]any{},
		"authenticatorAttachment": "platform",
	})
}

// Get answers the options of navigator.credentials.get, as JSON, as a
// browser does: it logs in with its passkey for the site, the newest it
// made, or one of those the options allow, and returns the browser's
// answer, as JSON. It fails when it has none.
func (a *Authenticator) Get(options []byte) ([]byte, error) {
	var o struct {
		Challenge        string `json:"challenge"`
		RPID             string `json:"rpId"`
		AllowCredentials []struct {
			ID string `json:"id"`
		} `json:"allowCredentials"`
	}
	if err := json.Unmarshal(options, &o); err != nil {
		return nil, fmt.Errorf("passkeytest: the options aren't navigator.credentials.get's: %w", err)
	}
	rpID, err := a.site(o.RPID)
	if err != nil {
		return nil, err
	}
	var pk *passkey
	for _, k := range slices.Backward(a.passkeys) {
		allowed := len(o.AllowCredentials) == 0 || slices.ContainsFunc(o.AllowCredentials, func(c struct {
			ID string `json:"id"`
		}) bool {
			return c.ID == b64(k.id)
		})
		if k.rpID == rpID && allowed {
			pk = k
			break
		}
	}
	if pk == nil {
		return nil, fmt.Errorf("passkeytest: it has no passkey for %s that the options allow", rpID)
	}
	if !pk.synced {
		pk.count++
	}
	data := a.authData(pk, 0)
	clientData := a.clientData("webauthn.get", o.Challenge)
	sum := sha256.Sum256(clientData)
	sig, err := pk.sign(append(data, sum[:]...))
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{
		"id":    b64(pk.id),
		"rawId": b64(pk.id),
		"type":  "public-key",
		"response": map[string]any{
			"clientDataJSON":    b64(clientData),
			"authenticatorData": b64(data),
			"signature":         b64(sig),
			"userHandle":        b64(pk.user),
		},
		"clientExtensionResults":  map[string]any{},
		"authenticatorAttachment": "platform",
	})
}

// Clone returns an authenticator with copies of a's passkeys, as a copy of
// its keys would have them: the same keys, counting on from the same
// numbers, which a site that checks the counts can tell from the original.
func (a *Authenticator) Clone() *Authenticator {
	b := *a
	b.passkeys = nil
	for _, pk := range a.passkeys {
		c := *pk
		b.passkeys = append(b.passkeys, &c)
	}
	return &b
}

// site is the site of options with rpID, which is the page's host when
// they don't name one, as a browser has it.
func (a *Authenticator) site(rpID string) (string, error) {
	if rpID != "" {
		return rpID, nil
	}
	u, err := url.Parse(a.Origin)
	if err != nil || u.Hostname() == "" {
		return "", fmt.Errorf("passkeytest: the Origin %q isn't one", a.Origin)
	}
	return u.Hostname(), nil
}

// authData is the authenticator's data for pk, with the flags that say
// what it did, and more.
func (a *Authenticator) authData(pk *passkey, more byte) []byte {
	site := sha256.Sum256([]byte(pk.rpID))
	flags := byte(0x01) | more // someone was there
	if !a.Unverified {
		flags |= 0x04
	}
	if pk.synced {
		flags |= 0x08 | 0x10
	}
	data := append(site[:], flags)
	return binary.BigEndian.AppendUint32(data, pk.count)
}

// clientData is the browser's own part of an answer.
func (a *Authenticator) clientData(ceremony, challenge string) []byte {
	data, _ := json.Marshal(map[string]any{"type": ceremony, "challenge": challenge, "origin": a.Origin, "crossOrigin": false})
	return data
}

// newPasskey makes a key for alg, and its COSE form.
func newPasskey(alg int) (*passkey, error) {
	pk := &passkey{alg: alg}
	switch alg {
	case -7:
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}
		point, err := key.PublicKey.Bytes()
		if err != nil {
			return nil, err
		}
		pk.signer = key
		pk.cose = cborAppend(nil, cborMap{{1, 2}, {3, -7}, {-1, 1}, {-2, point[1:33]}, {-3, point[33:]}})
	case -8:
		pub, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
		pk.signer = key
		pk.cose = cborAppend(nil, cborMap{{1, 1}, {3, -8}, {-1, 6}, {-2, []byte(pub)}})
	case -257:
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, err
		}
		pk.signer = key
		pk.cose = cborAppend(nil, cborMap{{1, 3}, {3, -257}, {-1, key.N.Bytes()}, {-2, big.NewInt(int64(key.E)).Bytes()}})
	default:
		return nil, fmt.Errorf("passkeytest: it makes passkeys for algorithms -7, -8 and -257, not %d", alg)
	}
	return pk, nil
}

// sign signs data with the passkey, as its algorithm does.
func (pk *passkey) sign(data []byte) ([]byte, error) {
	switch pk.alg {
	case -8:
		return pk.signer.Sign(nil, data, crypto.Hash(0))
	default:
		sum := sha256.Sum256(data)
		return pk.signer.Sign(rand.Reader, sum[:], crypto.SHA256)
	}
}

func random(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func unb64(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
}

// cborMap is a CBOR map, written in the order of its pairs.
type cborMap []struct{ k, v any }

// cborAppend appends v to b as CBOR, as an authenticator writes it: ints,
// byte and text strings, and maps.
func cborAppend(b []byte, v any) []byte {
	switch v := v.(type) {
	case int:
		if v >= 0 {
			return cborHead(b, 0, uint64(v))
		}
		return cborHead(b, 1, uint64(-1-v))
	case []byte:
		return append(cborHead(b, 2, uint64(len(v))), v...)
	case string:
		return append(cborHead(b, 3, uint64(len(v))), v...)
	case cborMap:
		b = cborHead(b, 5, uint64(len(v)))
		for _, kv := range v {
			b = cborAppend(cborAppend(b, kv.k), kv.v)
		}
		return b
	}
	panic(fmt.Sprintf("passkeytest: no CBOR for %T", v))
}

func cborHead(b []byte, major byte, n uint64) []byte {
	switch {
	case n < 24:
		return append(b, major<<5|byte(n))
	case n <= 0xff:
		return append(b, major<<5|24, byte(n))
	case n <= 0xffff:
		return binary.BigEndian.AppendUint16(append(b, major<<5|25), uint16(n))
	case n <= 0xffffffff:
		return binary.BigEndian.AppendUint32(append(b, major<<5|26), uint32(n))
	}
	return binary.BigEndian.AppendUint64(append(b, major<<5|27), n)
}
