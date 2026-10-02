package inertia

import "strconv"

// BigInt is a whole number that goes to the page as a JavaScript BigInt,
// for one past the safe range of a JavaScript number, which ends at
// 2^53 - 1: a snowflake ID, 900719925474099988, arrives as a number
// rounded to 900719925474100000. A BigInt goes out as the protocol's
// {"$bigint": "900719925474099988"}, a small one too, which Inertia's
// client, from 3.8.0, reads as a BigInt on a page that says it has them,
// as a page with one in it does. tug gen types it bigint.
//
// It reads as an int64 does: the client sends one back as its digits,
// which Bind reads, and tugtest reads one from a page as the number it is.
// It has no UnmarshalJSON of its own, as encoding/json, from Go 1.27,
// gives the error of one no field, which Bind would name.
type BigInt int64

// bigIntMarker is how a BigInt begins as it goes out, which nothing else
// in a page's JSON does: a string's quotes are escaped.
var bigIntMarker = []byte(`{"$bigint":`)

// MarshalJSON writes n as the protocol's marker.
func (n BigInt) MarshalJSON() ([]byte, error) {
	b := make([]byte, 0, 40)
	b = append(b, `{"$bigint":"`...)
	b = strconv.AppendInt(b, int64(n), 10)
	return append(b, `"}`...), nil
}
