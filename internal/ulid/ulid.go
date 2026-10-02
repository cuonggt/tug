// Package ulid makes IDs that sort as they were made: ULIDs, 48 bits of the
// millisecond and 80 random ones, in 26 characters of Crockford's base32.
// DevTools names its entries by them, and the mailbox its mail.
package ulid

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

// A Maker makes ULIDs, which sort as they were made: two made in one
// millisecond are one apart, so they sort too. Its zero value is ready.
type Maker struct {
	mu   sync.Mutex
	ms   uint64
	high uint16 // the random part's top 16 bits
	low  uint64 // and the rest
}

// Next is a new ID, made at t.
func (u *Maker) Next(t time.Time) string {
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

// Valid reports whether s is a ULID, as a file is named by one, and an
// endpoint takes one.
func Valid(s string) bool {
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
