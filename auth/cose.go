package auth

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"
)

// A passkey's public key comes as a COSE key (RFC 9052 and 9053): a CBOR
// map with the key's type under 1, its algorithm under 3, and its numbers
// under negative labels. These are the algorithms a passkey may use, which
// every authenticator has one of.
const (
	algES256 int64 = -7   // ECDSA on P-256, with SHA-256
	algEdDSA int64 = -8   // Ed25519
	algRS256 int64 = -257 // RSASSA-PKCS1-v1_5, with SHA-256
)

// coseKey is a passkey's public key, and the algorithm it signs with.
type coseKey struct {
	alg int64
	key crypto.PublicKey
}

var errCOSE = errors.New("not a passkey's public key")

// parseCOSEKey reads the COSE key at the front of b, and returns it with
// the bytes after it. It takes an ES256 key on P-256, an Ed25519 key, and
// an RSA key of 2048 bits or more, and refuses the rest, and a key that
// isn't one, such as a point off its curve.
func parseCOSEKey(b []byte) (coseKey, []byte, error) {
	v, rest, err := decodeCBOR(b)
	if err != nil {
		return coseKey{}, nil, fmt.Errorf("%w: %w", errCOSE, err)
	}
	m, ok := v.(map[any]any)
	if !ok {
		return coseKey{}, nil, fmt.Errorf("%w: it isn't a map", errCOSE)
	}
	kty, _ := m[int64(1)].(int64)
	alg, _ := m[int64(3)].(int64)
	k := coseKey{alg: alg}
	switch {
	case kty == 2 && alg == algES256:
		crv, _ := m[int64(-1)].(int64)
		x, _ := m[int64(-2)].([]byte)
		y, _ := m[int64(-3)].([]byte)
		if crv != 1 || len(x) != 32 || len(y) != 32 {
			return coseKey{}, nil, fmt.Errorf("%w: an ES256 key that isn't a point on P-256", errCOSE)
		}
		point := append(append([]byte{4}, x...), y...)
		pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), point)
		if err != nil {
			return coseKey{}, nil, fmt.Errorf("%w: %w", errCOSE, err)
		}
		k.key = pub
	case kty == 1 && alg == algEdDSA:
		crv, _ := m[int64(-1)].(int64)
		x, _ := m[int64(-2)].([]byte)
		if crv != 6 || len(x) != ed25519.PublicKeySize {
			return coseKey{}, nil, fmt.Errorf("%w: an EdDSA key that isn't Ed25519", errCOSE)
		}
		k.key = ed25519.PublicKey(bytes.Clone(x))
	case kty == 3 && alg == algRS256:
		n, _ := m[int64(-1)].([]byte)
		e, _ := m[int64(-2)].([]byte)
		N, E := new(big.Int).SetBytes(n), new(big.Int).SetBytes(e)
		// A key too big takes long to check, which a lie could use.
		if N.BitLen() < 2048 || N.BitLen() > 8192 || !E.IsInt64() || E.Int64() < 3 || E.Int64() > 1<<31-1 || E.Bit(0) == 0 {
			return coseKey{}, nil, fmt.Errorf("%w: an RSA key under 2048 bits, over 8192, or with an exponent that isn't one", errCOSE)
		}
		k.key = &rsa.PublicKey{N: N, E: int(E.Int64())}
	default:
		return coseKey{}, nil, fmt.Errorf("%w: a key of type %d for algorithm %d, where ES256, Ed25519 and RS256 are taken", errCOSE, kty, alg)
	}
	return k, rest, nil
}

// verify reports whether sig is the key's signature over data.
func (k coseKey) verify(data, sig []byte) bool {
	switch k.alg {
	case algES256:
		sum := sha256.Sum256(data)
		return ecdsa.VerifyASN1(k.key.(*ecdsa.PublicKey), sum[:], sig)
	case algEdDSA:
		return ed25519.Verify(k.key.(ed25519.PublicKey), data, sig)
	case algRS256:
		sum := sha256.Sum256(data)
		return rsa.VerifyPKCS1v15(k.key.(*rsa.PublicKey), crypto.SHA256, sum[:], sig) == nil
	}
	return false
}
