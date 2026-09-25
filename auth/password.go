package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"log/slog"
	"runtime"
	"strings"

	"golang.org/x/crypto/argon2"
)

// The argon2id settings HashPassword uses, OWASP's: 19 MiB of memory and
// two passes, about 30 ms on a server's core.
const (
	memory  = 19 * 1024 // KiB
	passes  = 2
	threads = 1
	saltLen = 16
	keyLen  = 32
)

// hashing holds the hashes running at once to one per CPU. Each takes
// 19 MiB, and a burst of logins could otherwise take all the memory there
// is; more at once wouldn't finish any sooner.
var hashing = make(chan struct{}, runtime.GOMAXPROCS(0))

// b64 is the base64 of the PHC string format: standard, unpadded.
var b64 = base64.RawStdEncoding

// HashPassword hashes password with argon2id, and a salt of its own, for
// storing in its place. The hash is in the PHC string format that other
// argon2id libraries read and write, such as PHP's:
//
//	$argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>
func HashPassword(password string) string {
	salt := make([]byte, saltLen)
	rand.Read(salt)
	key := derive(password, argonParams{memory: memory, passes: passes, threads: threads, salt: salt, keyLen: keyLen})
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, memory, passes, threads, b64.EncodeToString(salt), b64.EncodeToString(key))
}

// CheckPassword reports whether password is the one hash was made from.
//
// When no user has the email that was tried, check the password against
// an empty hash all the same: it takes as long as a real check, and fails,
// so a login form's speed doesn't tell which emails have accounts.
func CheckPassword(hash, password string) bool {
	p, ok := parseHash(hash)
	if !ok {
		if hash != "" {
			slog.Warn("auth: a stored password hash isn't argon2id's, so no password matches it")
		}
		p = decoy
	}
	key := derive(password, p)
	return ok && subtle.ConstantTimeCompare(key, p.key) == 1
}

// NeedsRehash reports whether hash was made with other settings than
// HashPassword uses now, as it will be once they're raised. Hash the
// password again when the user next logs in, while the app has it.
func NeedsRehash(hash string) bool {
	p, ok := parseHash(hash)
	return !ok || p.memory != memory || p.passes != passes || p.threads != threads ||
		len(p.salt) != saltLen || len(p.key) != keyLen
}

type argonParams struct {
	memory, passes uint32
	threads        uint8
	salt, key      []byte
	keyLen         uint32
}

// decoy is what CheckPassword checks against when there's no hash: the
// settings HashPassword uses, so the check costs what a real one does.
var decoy = argonParams{memory: memory, passes: passes, threads: threads, salt: make([]byte, saltLen), keyLen: keyLen}

func derive(password string, p argonParams) []byte {
	hashing <- struct{}{}
	defer func() { <-hashing }()
	return argon2.IDKey([]byte(password), p.salt, p.passes, p.memory, p.threads, p.keyLen)
}

// parseHash reads a PHC string. The bounds keep a hash from asking for
// more memory or time than any sensible setting does.
func parseHash(hash string) (argonParams, bool) {
	var p argonParams
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return p, false
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.passes, &p.threads); err != nil ||
		parts[3] != fmt.Sprintf("m=%d,t=%d,p=%d", p.memory, p.passes, p.threads) {
		return p, false
	}
	var err error
	if p.salt, err = b64.DecodeString(parts[4]); err != nil {
		return p, false
	}
	if p.key, err = b64.DecodeString(parts[5]); err != nil {
		return p, false
	}
	p.keyLen = uint32(len(p.key))
	ok := p.memory >= 8*uint32(p.threads) && p.memory <= 4<<20 && p.passes >= 1 && p.passes <= 64 && p.threads >= 1 &&
		len(p.salt) >= 8 && len(p.salt) <= 64 && len(p.key) >= 16 && len(p.key) <= 64
	return p, ok
}
