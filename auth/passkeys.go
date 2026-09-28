package auth

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/cuonggt/tug/session"
)

// Passkeys logs users in with passkeys: WebAuthn's credentials, which a
// phone, a laptop or a password manager keeps, and unlocks with the user's
// PIN, fingerprint or face. A passkey is two factors on its own, the
// device and what unlocks it, and nothing the site keeps logs anyone in:
// the site has a public key, and the device the private one, which never
// leaves it.
//
// Adding a passkey, and logging in with one, each takes two requests. The
// first's Start returns the options of the browser's navigator.credentials,
// as JSON, and keeps their challenge in the session; the second's Finish
// checks the browser's answer, as JSON, against it. A challenge is
// answered once, within five minutes.
//
// Which passkeys are whose is the app's to keep, as its users are: the new
// passkey FinishRegistration returns, and after each login, the passkey as
// FinishLogin returns it, with its new count.
type Passkeys struct {
	// RPID is the site the passkeys are for: its domain, as example.com. A
	// passkey works there and on the domains under it, and nowhere else.
	// Browsers make passkeys for a domain, never an IP address, and in
	// development localhost is one.
	RPID string

	// Origin is the site's origin, as https://example.com, which the
	// browser's answer has to name exactly.
	Origin string

	// Name is the site's name, as the browser shows it when asking to make
	// a passkey.
	Name string
}

// Passkey is one of a user's passkeys, as the app keeps it.
type Passkey struct {
	// ID is the passkey's credential ID, which a login's answer names it by.
	ID []byte

	// PublicKey is its key, as the authenticator made it: a COSE key.
	PublicKey []byte

	// SignCount is how many times the authenticator counts it has signed
	// with the passkey. One that syncs, as a password manager's does,
	// counts nothing, and keeps 0.
	SignCount uint32

	// UserHandle is the handle of the user it was made for.
	UserHandle []byte

	// Transports are how the browser reached the authenticator, such as
	// "internal" or "hybrid", which it can try again.
	Transports []string

	// BackedUp says the passkey syncs to the user's other devices.
	BackedUp bool
}

// PasskeyUser is who a new passkey is for.
type PasskeyUser struct {
	// Handle names the user to the authenticator, which keeps it with the
	// passkey: NewPasskeyHandle's random bytes, the same for each of the
	// user's passkeys, and never their ID or their email, which it would
	// give away to anyone who reads the authenticator.
	Handle []byte

	// Name is the user's account as the authenticator lists the passkey,
	// their email, say, and DisplayName their name.
	Name, DisplayName string
}

// NewPasskeyHandle returns a user handle, for a user's first passkey: 32
// random bytes.
func NewPasskeyHandle() []byte {
	b := make([]byte, 32)
	rand.Read(b)
	return b
}

const (
	// passkeyKey is the session key of the passkey the session asked for.
	passkeyKey = "tug.auth.passkey"

	// challengeFor is how long a challenge waits for its answer.
	challengeFor = 5 * time.Minute
)

// The flags of an authenticator's data.
const (
	flagUP = 0x01 // someone was there, and touched it
	flagUV = 0x04 // and unlocked it: a PIN, a fingerprint, a face
	flagBE = 0x08 // the passkey can sync
	flagBS = 0x10 // and does
	flagAT = 0x40 // a new passkey comes with the data
	flagED = 0x80 // extensions come after it
)

// StartRegistration returns the options of navigator.credentials.create,
// as JSON, for user to make a passkey for the site, and keeps their
// challenge in s. exclude are the IDs of the user's passkeys already: an
// authenticator that has one of them makes no other.
func (p *Passkeys) StartRegistration(s *session.Session, user PasskeyUser, exclude [][]byte) ([]byte, error) {
	if s == nil {
		panic("auth: StartRegistration needs a session; set tug's Config.Session")
	}
	if len(user.Handle) == 0 || len(user.Handle) > 64 {
		return nil, errors.New("auth: a passkey's user handle is 1 to 64 bytes, such as NewPasskeyHandle's")
	}
	challenge := newChallenge(s, map[string]any{"ceremony": "create", "user": b64url(user.Handle)})
	opts := creationOptions{
		RP:        rpEntity{ID: p.RPID, Name: p.Name},
		User:      userEntity{ID: b64url(user.Handle), Name: user.Name, DisplayName: user.DisplayName},
		Challenge: b64url(challenge),
		// Ed25519 first, which authenticators that have it prefer; every
		// authenticator has ES256.
		PubKeyCredParams:   []credParam{{"public-key", algEdDSA}, {"public-key", algES256}, {"public-key", algRS256}},
		Timeout:            challengeFor.Milliseconds(),
		ExcludeCredentials: []credDescriptor{},
		// A passkey the authenticator keeps, so a login needs no email,
		// unlocked each time, so it's two factors on its own.
		AuthenticatorSelection: authenticatorSelection{ResidentKey: "required", RequireResidentKey: true, UserVerification: "required"},
		Attestation:            "none",
	}
	for _, id := range exclude {
		opts.ExcludeCredentials = append(opts.ExcludeCredentials, credDescriptor{Type: "public-key", ID: b64url(id)})
	}
	return json.Marshal(opts)
}

// FinishRegistration checks the browser's answer to StartRegistration's
// options, as JSON, and returns the passkey it made, for the app to keep
// with the user. A passkey whose ID the app has for anyone already is one
// to refuse. The answer's attestation, which says what made the passkey,
// isn't checked, as the options ask for none: any make of authenticator
// will do.
func (p *Passkeys) FinishRegistration(s *session.Session, answer []byte) (Passkey, error) {
	c, err := takeCeremony(s, "create")
	if err != nil {
		return Passkey{}, err
	}
	cred, err := parseCredential(answer)
	if err != nil {
		return Passkey{}, err
	}
	if err := p.checkClientData(cred.clientData, "webauthn.create", c.challenge); err != nil {
		return Passkey{}, err
	}
	att, rest, err := decodeCBOR(cred.attestationObject)
	m, ok := att.(map[any]any)
	if err != nil || !ok || len(rest) > 0 {
		return Passkey{}, errors.New("auth: a new passkey's attestation object isn't one")
	}
	raw, _ := m["authData"].([]byte)
	ad, err := parseAuthData(raw)
	if err != nil {
		return Passkey{}, err
	}
	if err := p.checkAuthData(ad); err != nil {
		return Passkey{}, err
	}
	if ad.flags&flagAT == 0 || !bytes.Equal(ad.credentialID, cred.rawID) {
		return Passkey{}, errors.New("auth: a new passkey's answer doesn't have the passkey it names")
	}
	if _, _, err := parseCOSEKey(ad.publicKey); err != nil {
		return Passkey{}, fmt.Errorf("auth: a new passkey's key: %w", err)
	}
	return Passkey{
		ID:         bytes.Clone(ad.credentialID),
		PublicKey:  bytes.Clone(ad.publicKey),
		SignCount:  ad.signCount,
		UserHandle: c.user,
		Transports: cred.transports,
		BackedUp:   ad.flags&flagBS != 0,
	}, nil
}

// StartLogin returns the options of navigator.credentials.get, as JSON, to
// log in with a passkey, and keeps their challenge in s. With no allow,
// any of the site's passkeys the browser finds will do, whoever's it is,
// for a login that asks for no email. With allow, a user's passkeys, only
// they will, as for someone logged in already confirming that it's them.
func (p *Passkeys) StartLogin(s *session.Session, allow []Passkey) ([]byte, error) {
	if s == nil {
		panic("auth: StartLogin needs a session; set tug's Config.Session")
	}
	kept := map[string]any{"ceremony": "get"}
	opts := requestOptions{RPID: p.RPID, Timeout: challengeFor.Milliseconds(), AllowCredentials: []credDescriptor{}, UserVerification: "required"}
	if len(allow) > 0 {
		ids := make([]string, 0, len(allow))
		for _, pk := range allow {
			ids = append(ids, b64url(pk.ID))
			opts.AllowCredentials = append(opts.AllowCredentials, credDescriptor{Type: "public-key", ID: b64url(pk.ID), Transports: pk.Transports})
		}
		kept["allow"] = ids
	}
	opts.Challenge = b64url(newChallenge(s, kept))
	return json.Marshal(opts)
}

// FinishLogin checks the browser's answer to StartLogin's options, as
// JSON. find returns the app's passkey with the answer's ID, or an error
// when it has none. FinishLogin returns the passkey as it is after the
// login, with its new count, for the app to keep, and log its user in.
//
// A passkey whose count goes back, rather than on, has been copied, and
// logs no one in; one that syncs counts nothing, and keeps 0.
func (p *Passkeys) FinishLogin(s *session.Session, answer []byte, find func(id []byte) (Passkey, error)) (Passkey, error) {
	c, err := takeCeremony(s, "get")
	if err != nil {
		return Passkey{}, err
	}
	cred, err := parseCredential(answer)
	if err != nil {
		return Passkey{}, err
	}
	if len(c.allow) > 0 && !slices.ContainsFunc(c.allow, func(id []byte) bool { return bytes.Equal(id, cred.rawID) }) {
		return Passkey{}, errors.New("auth: a passkey the login didn't allow")
	}
	pk, err := find(cred.rawID)
	if err != nil {
		return Passkey{}, fmt.Errorf("auth: finding a passkey: %w", err)
	}
	switch {
	case cred.userHandle == nil && len(c.allow) == 0:
		// Without an email, the handle is what says whose login it is.
		return Passkey{}, errors.New("auth: a passkey's answer names no user")
	case cred.userHandle != nil && !bytes.Equal(cred.userHandle, pk.UserHandle):
		return Passkey{}, errors.New("auth: a passkey's answer names another user than the passkey's")
	}
	if err := p.checkClientData(cred.clientData, "webauthn.get", c.challenge); err != nil {
		return Passkey{}, err
	}
	ad, err := parseAuthData(cred.authenticatorData)
	if err != nil {
		return Passkey{}, err
	}
	if err := p.checkAuthData(ad); err != nil {
		return Passkey{}, err
	}
	key, _, err := parseCOSEKey(pk.PublicKey)
	if err != nil {
		return Passkey{}, fmt.Errorf("auth: a passkey's key, as the app keeps it: %w", err)
	}
	sum := sha256.Sum256(cred.clientData)
	if !key.verify(append(bytes.Clone(cred.authenticatorData), sum[:]...), cred.signature) {
		return Passkey{}, errors.New("auth: a passkey's signature doesn't check out")
	}
	if (ad.signCount != 0 || pk.SignCount != 0) && ad.signCount <= pk.SignCount {
		return Passkey{}, fmt.Errorf("auth: a passkey's count went from %d back to %d: it may have been copied", pk.SignCount, ad.signCount)
	}
	pk.SignCount, pk.BackedUp = ad.signCount, ad.flags&flagBS != 0
	return pk, nil
}

// checkClientData checks the browser's own part of an answer: that it's
// to the challenge kept, for the ceremony, from the site itself.
func (p *Passkeys) checkClientData(raw []byte, ceremony string, challenge []byte) error {
	var c struct {
		Type        string `json:"type"`
		Challenge   string `json:"challenge"`
		Origin      string `json:"origin"`
		CrossOrigin bool   `json:"crossOrigin"`
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return errors.New("auth: a passkey's client data isn't JSON")
	}
	got, err := unb64url(c.Challenge)
	switch {
	case c.Type != ceremony:
		return fmt.Errorf("auth: a passkey's answer is for %q, not %q", c.Type, ceremony)
	case err != nil || subtle.ConstantTimeCompare(got, challenge) != 1:
		return errors.New("auth: a passkey's answer is to another challenge")
	case c.Origin != p.Origin:
		return fmt.Errorf("auth: a passkey's answer is from %s, not %s", c.Origin, p.Origin)
	case c.CrossOrigin:
		return errors.New("auth: a passkey's answer is from a frame inside another site")
	}
	return nil
}

// checkAuthData checks the authenticator's part of an answer: that it's
// for the site, and that someone unlocked it.
func (p *Passkeys) checkAuthData(ad authData) error {
	site := sha256.Sum256([]byte(p.RPID))
	switch {
	case !bytes.Equal(ad.rpIDHash, site[:]):
		return fmt.Errorf("auth: a passkey's answer is for another site than %s", p.RPID)
	case ad.flags&flagUP == 0:
		return errors.New("auth: a passkey was used with no one there")
	case ad.flags&flagUV == 0:
		return errors.New("auth: a passkey was used without its PIN, fingerprint or face")
	case ad.flags&flagBS != 0 && ad.flags&flagBE == 0:
		return errors.New("auth: a passkey says it syncs, and that it can't")
	}
	return nil
}

// authData is an authenticator's data (WebAuthn §6.1): the site it's for,
// its flags, its count, and with a new passkey, the passkey.
type authData struct {
	rpIDHash     []byte
	flags        byte
	signCount    uint32
	credentialID []byte // a new passkey's
	publicKey    []byte // its COSE key, as the bytes it came as
}

func parseAuthData(b []byte) (authData, error) {
	bad := func(what string) (authData, error) {
		return authData{}, fmt.Errorf("auth: a passkey's authenticator data %s", what)
	}
	if len(b) < 37 {
		return bad("is too short")
	}
	ad := authData{rpIDHash: b[:32], flags: b[32], signCount: binary.BigEndian.Uint32(b[33:37])}
	rest := b[37:]
	if ad.flags&flagAT != 0 {
		// The authenticator's make, 16 bytes, the ID's length and the ID,
		// and the key, whose length is the CBOR's own.
		if len(rest) < 18 {
			return bad("ends in its passkey")
		}
		n := int(binary.BigEndian.Uint16(rest[16:18]))
		rest = rest[18:]
		if n == 0 || n > 1023 || len(rest) < n {
			return bad("has a passkey ID that isn't 1 to 1023 bytes")
		}
		ad.credentialID, rest = rest[:n], rest[n:]
		_, after, err := decodeCBOR(rest)
		if err != nil {
			return bad("has a key that isn't CBOR")
		}
		ad.publicKey, rest = rest[:len(rest)-len(after)], after
	}
	if ad.flags&flagED != 0 {
		ext, after, err := decodeCBOR(rest)
		if _, ok := ext.(map[any]any); err != nil || !ok {
			return bad("has extensions that aren't a map")
		}
		rest = after
	}
	if len(rest) > 0 {
		return bad("has bytes left over")
	}
	return ad, nil
}

// credential is a browser's answer, as its JSON has it.
type credential struct {
	rawID             []byte
	clientData        []byte
	attestationObject []byte // a new passkey's
	authenticatorData []byte // a login's
	signature         []byte
	userHandle        []byte // nil when the answer has none
	transports        []string
}

// knownTransports are the transports WebAuthn names; the answer's others
// are dropped.
var knownTransports = []string{"ble", "hybrid", "internal", "nfc", "smart-card", "usb"}

func parseCredential(answer []byte) (credential, error) {
	var j struct {
		ID       string `json:"id"`
		RawID    string `json:"rawId"`
		Type     string `json:"type"`
		Response struct {
			ClientDataJSON    string   `json:"clientDataJSON"`
			AttestationObject string   `json:"attestationObject"`
			AuthenticatorData string   `json:"authenticatorData"`
			Signature         string   `json:"signature"`
			UserHandle        *string  `json:"userHandle"`
			Transports        []string `json:"transports"`
		} `json:"response"`
	}
	if err := json.Unmarshal(answer, &j); err != nil || j.Type != "public-key" {
		return credential{}, errors.New("auth: a passkey's answer isn't the JSON of a public-key credential")
	}
	var c credential
	var err error
	for _, f := range []struct {
		from string
		to   *[]byte
	}{
		{j.RawID, &c.rawID},
		{j.Response.ClientDataJSON, &c.clientData},
		{j.Response.AttestationObject, &c.attestationObject},
		{j.Response.AuthenticatorData, &c.authenticatorData},
		{j.Response.Signature, &c.signature},
	} {
		if *f.to, err = unb64url(f.from); err != nil {
			return credential{}, errors.New("auth: a passkey's answer has a field that isn't base64url")
		}
	}
	if id, err := unb64url(j.ID); err != nil || len(c.rawID) == 0 || !bytes.Equal(id, c.rawID) {
		return credential{}, errors.New("auth: a passkey's answer has an ID that isn't its raw ID")
	}
	if h := j.Response.UserHandle; h != nil && *h != "" {
		if c.userHandle, err = unb64url(*h); err != nil {
			return credential{}, errors.New("auth: a passkey's answer has a user handle that isn't base64url")
		}
	}
	for _, t := range j.Response.Transports {
		if slices.Contains(knownTransports, t) && !slices.Contains(c.transports, t) {
			c.transports = append(c.transports, t)
		}
	}
	return c, nil
}

// The options of navigator.credentials, as WebAuthn's JSON writes them.

type creationOptions struct {
	RP                     rpEntity               `json:"rp"`
	User                   userEntity             `json:"user"`
	Challenge              string                 `json:"challenge"`
	PubKeyCredParams       []credParam            `json:"pubKeyCredParams"`
	Timeout                int64                  `json:"timeout"`
	ExcludeCredentials     []credDescriptor       `json:"excludeCredentials"`
	AuthenticatorSelection authenticatorSelection `json:"authenticatorSelection"`
	Attestation            string                 `json:"attestation"`
}

type requestOptions struct {
	Challenge        string           `json:"challenge"`
	Timeout          int64            `json:"timeout"`
	RPID             string           `json:"rpId"`
	AllowCredentials []credDescriptor `json:"allowCredentials"`
	UserVerification string           `json:"userVerification"`
}

type rpEntity struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type userEntity struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
}

type credParam struct {
	Type string `json:"type"`
	Alg  int64  `json:"alg"`
}

type credDescriptor struct {
	Type       string   `json:"type"`
	ID         string   `json:"id"`
	Transports []string `json:"transports,omitempty"`
}

type authenticatorSelection struct {
	ResidentKey        string `json:"residentKey"`
	RequireResidentKey bool   `json:"requireResidentKey"`
	UserVerification   string `json:"userVerification"`
}

// ceremony is what a Start kept in the session for its Finish.
type ceremony struct {
	challenge []byte
	user      []byte   // a new passkey's user handle
	allow     [][]byte // the passkeys a login allows, when it's one user's
}

var errNoChallenge = errors.New("auth: no passkey was asked for in this session, or it was over five minutes ago")

// newChallenge makes a challenge, and keeps it in s with the rest of what
// the ceremony's Finish needs.
func newChallenge(s *session.Session, kept map[string]any) []byte {
	challenge := make([]byte, 32)
	rand.Read(challenge)
	kept["challenge"] = b64url(challenge)
	kept["at"] = now().Unix()
	s.Set(passkeyKey, kept)
	return challenge
}

// takeCeremony returns what the Start of the ceremony kept in s, and
// forgets it: a challenge is answered once, rightly or not.
func takeCeremony(s *session.Session, kind string) (ceremony, error) {
	if s == nil {
		return ceremony{}, errNoChallenge
	}
	kept, _ := s.Get(passkeyKey).(map[string]any)
	s.Delete(passkeyKey)
	at, ok := unixTime(kept["at"])
	if kept["ceremony"] != kind || !ok || now().Sub(at) >= challengeFor {
		return ceremony{}, errNoChallenge
	}
	var c ceremony
	challenge, _ := kept["challenge"].(string)
	user, _ := kept["user"].(string)
	var err error
	if c.challenge, err = unb64url(challenge); err != nil || len(c.challenge) == 0 {
		return ceremony{}, errNoChallenge
	}
	if c.user, err = unb64url(user); err != nil {
		return ceremony{}, errNoChallenge
	}
	// A list comes back from the session's JSON as []any.
	var allow []any
	switch ids := kept["allow"].(type) {
	case []string:
		for _, id := range ids {
			allow = append(allow, id)
		}
	case []any:
		allow = ids
	}
	for _, id := range allow {
		str, _ := id.(string)
		b, err := unb64url(str)
		if err != nil || len(b) == 0 {
			return ceremony{}, errNoChallenge
		}
		c.allow = append(c.allow, b)
	}
	return c, nil
}

// b64url is WebAuthn's base64: URL-safe, without padding.
func b64url(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func unb64url(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
}
