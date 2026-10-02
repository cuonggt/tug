package inertia

import (
	"encoding/json"
	"reflect"
	"strconv"
)

// BigInt is a whole number that goes to the page as a JavaScript BigInt,
// for one past the safe range of a JavaScript number, which ends at
// 2^53 - 1: a snowflake ID, 900719925474099988, arrives as a number
// rounded to 900719925474100000. A BigInt goes out as the protocol's
// {"$bigint": "900719925474099988"}, a small one too, which Inertia's
// client, from 3.8.0, reads as a BigInt on a page that says it has them,
// as a page with one in it does. tug gen types it bigint.
//
// The client sends one back as its digits, which Bind reads into a BigInt
// as it reads an int64.
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

// UnmarshalJSON reads n as the client sends a BigInt back, its digits in a
// string, and as a number, or the protocol's marker, as a page's JSON has
// it, read back in a test. An empty string, as a form's empty field, and
// null leave n as it is.
func (n *BigInt) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	switch data[0] {
	case 'n':
		return nil // null
	case '"':
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		if s == "" {
			return nil
		}
		return n.parse(s, "string")
	case '{':
		var marker struct {
			Digits *string `json:"$bigint"`
		}
		if json.Unmarshal(data, &marker) != nil || marker.Digits == nil {
			return typeError("object")
		}
		return n.parse(*marker.Digits, "object")
	case '[':
		return typeError("array")
	case 't', 'f':
		return typeError("bool")
	}
	return n.parse(string(data), "number "+string(data))
}

func (n *BigInt) parse(digits, value string) error {
	v, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return typeError(value)
	}
	*n = BigInt(v)
	return nil
}

// typeError is encoding/json's error for a value that isn't a whole
// number, which it gives the field's path, and Bind its message.
func typeError(value string) error {
	return &json.UnmarshalTypeError{Value: value, Type: reflect.TypeFor[BigInt]()}
}
