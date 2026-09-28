package auth

import (
	"errors"
	"fmt"
	"math"
	"unicode/utf8"
)

// A passkey's answer carries CBOR (RFC 8949): the authenticator's data,
// and the key it made. decodeCBOR reads the part of CBOR that WebAuthn
// writes, as CTAP2 sets it out: integers, byte and text strings, arrays
// and maps, and true, false and null, all with definite lengths. Anything
// else, such as a float, a tag or a length left open, is refused, as the
// answer comes from whoever sends it.

// cborMaxDepth is how deep arrays and maps may nest: WebAuthn's go three
// deep at most.
const cborMaxDepth = 8

var errCBOR = errors.New("not CBOR as WebAuthn writes it")

// decodeCBOR reads the CBOR item at the front of b, and returns it with the
// bytes after it. An item is an int64, a []byte, a string, a []any, a
// map[any]any, whose keys are int64s and strings, a bool, or nil.
func decodeCBOR(b []byte) (v any, rest []byte, err error) {
	d := &cborDecoder{b: b}
	if v, err = d.item(0); err != nil {
		return nil, nil, err
	}
	return v, d.b, nil
}

type cborDecoder struct {
	b []byte // what's left to read
}

func (d *cborDecoder) item(depth int) (any, error) {
	if depth > cborMaxDepth {
		return nil, fmt.Errorf("%w: it nests over %d deep", errCBOR, cborMaxDepth)
	}
	if len(d.b) == 0 {
		return nil, fmt.Errorf("%w: it ends early", errCBOR)
	}
	major, info := d.b[0]>>5, d.b[0]&0x1f
	d.b = d.b[1:]
	if major == 7 {
		switch info {
		case 20:
			return false, nil
		case 21:
			return true, nil
		case 22:
			return nil, nil
		}
		return nil, fmt.Errorf("%w: a float, or a simple value other than true, false and null", errCBOR)
	}
	n, err := d.argument(info)
	if err != nil {
		return nil, err
	}
	switch major {
	case 0:
		if n > math.MaxInt64 {
			return nil, fmt.Errorf("%w: an integer too big", errCBOR)
		}
		return int64(n), nil
	case 1:
		if n > math.MaxInt64 {
			return nil, fmt.Errorf("%w: an integer too small", errCBOR)
		}
		return -1 - int64(n), nil
	case 2, 3:
		if n > uint64(len(d.b)) {
			return nil, fmt.Errorf("%w: it ends early", errCBOR)
		}
		s := d.b[:n]
		d.b = d.b[n:]
		if major == 2 {
			return s, nil
		}
		if !utf8.Valid(s) {
			return nil, fmt.Errorf("%w: text that isn't UTF-8", errCBOR)
		}
		return string(s), nil
	case 4:
		// Each item takes a byte at least, so a length past the end is a lie,
		// which mustn't get to make a slice that long.
		if n > uint64(len(d.b)) {
			return nil, fmt.Errorf("%w: it ends early", errCBOR)
		}
		list := make([]any, 0, n)
		for range n {
			v, err := d.item(depth + 1)
			if err != nil {
				return nil, err
			}
			list = append(list, v)
		}
		return list, nil
	case 5:
		if n > uint64(len(d.b))/2 {
			return nil, fmt.Errorf("%w: it ends early", errCBOR)
		}
		m := make(map[any]any, n)
		for range n {
			k, err := d.item(depth + 1)
			if err != nil {
				return nil, err
			}
			switch k.(type) {
			case int64, string:
			default:
				return nil, fmt.Errorf("%w: a map key that isn't an integer or text", errCBOR)
			}
			if _, twice := m[k]; twice {
				return nil, fmt.Errorf("%w: a map with the key %v twice", errCBOR, k)
			}
			if m[k], err = d.item(depth + 1); err != nil {
				return nil, err
			}
		}
		return m, nil
	}
	return nil, fmt.Errorf("%w: a tag", errCBOR)
}

// argument reads an item's argument, whose size info gives: in info itself
// below 24, or in the 1, 2, 4 or 8 bytes after it. Info 31, a length left
// open, and the reserved 28 to 30 are refused.
func (d *cborDecoder) argument(info byte) (uint64, error) {
	if info < 24 {
		return uint64(info), nil
	}
	if info > 27 {
		return 0, fmt.Errorf("%w: a length left open, or a reserved one", errCBOR)
	}
	size := 1 << (info - 24)
	if len(d.b) < size {
		return 0, fmt.Errorf("%w: it ends early", errCBOR)
	}
	var n uint64
	for _, c := range d.b[:size] {
		n = n<<8 | uint64(c)
	}
	d.b = d.b[size:]
	return n, nil
}
