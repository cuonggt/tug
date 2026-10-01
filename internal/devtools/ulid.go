package devtools

import (
	"crypto/rand"
	"encoding/binary"
	"strings"
	"sync"
	"time"
)

// crockford is the alphabet of a ULID: Crockford's base32, without the
// letters that read as others, I, L, O and U.
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// ulids makes the entries' IDs, ULIDs, as the protocol's are: 48 bits of
// the millisecond and 80 random ones, in 26 characters, which sort as they
// were made. Two made in one millisecond are one apart, so they sort too.
type ulids struct {
	mu   sync.Mutex
	ms   uint64
	high uint16 // the random part's top 16 bits
	low  uint64 // and the rest
}

// next is a new ID, made at t.
func (u *ulids) next(t time.Time) string {
	u.mu.Lock()
	defer u.mu.Unlock()
	ms := uint64(t.UnixMilli())
	if ms <= u.ms {
		u.low++
		if u.low == 0 {
			u.high++
		}
	} else {
		var r [10]byte
		rand.Read(r[:])
		u.ms, u.high, u.low = ms, binary.BigEndian.Uint16(r[:2]), binary.BigEndian.Uint64(r[2:])
	}
	// 128 bits, the millisecond first, in 26 digits of 5 bits each, the
	// first of which has the top 3, under two that are 0.
	hi, lo := u.ms<<16|uint64(u.high), u.low
	var id [26]byte
	for i := 25; i >= 0; i-- {
		id[i] = crockford[lo&31]
		lo = lo>>5 | hi<<59
		hi >>= 5
	}
	return string(id[:])
}

// isULID reports whether s is a ULID: what an entry's file is named, and
// what the endpoint of one takes.
func isULID(s string) bool {
	if len(s) != 26 || s[0] > '7' {
		return false
	}
	for i := range len(s) {
		if strings.IndexByte(crockford, s[i]) < 0 {
			return false
		}
	}
	return true
}
