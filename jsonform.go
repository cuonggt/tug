package tug

import (
	"bytes"
	"cmp"
	"encoding/json"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Inertia's <Form> sends a form as JSON, with every value a string, as
// FormData has it: "on" for a ticked checkbox, "42" from a number input,
// "2026-09-25" from a date input, and "" from an input left empty.
// encoding/json takes none of those for a bool, a number or a time, where a
// form body gives them. So a JSON body that doesn't decode is read again,
// with those strings read the way a form's values are (jsonAsForm).

// jsonAsForm rewrites data, a JSON body for type t, so that each string
// where t has a bool, a number or a time.Time is one, read as setScalar
// reads a form value, and an empty one is left out, as an empty input is.
// A string that doesn't parse stays as it is, for encoding/json to report
// as it would have. It reports whether it changed anything.
//
// It's only for a body that has already failed to decode: data is known to
// be well-formed, and a body that decodes as it is never gets here.
func jsonAsForm(data []byte, t reflect.Type) ([]byte, bool) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber() // numbers as they're written, as encoding/json reads them
	v, err := readJSON(dec)
	if err != nil {
		return nil, false
	}
	changed := false
	v, _ = asForm(v, t, &changed)
	if !changed {
		return nil, false
	}
	var b bytes.Buffer
	writeJSON(&b, v)
	return b.Bytes(), true
}

// asForm returns v, a JSON value bound for type t, with the strings in it
// for bools, numbers and times read as a form's values. keep is false for
// an empty string there, which the caller leaves out where it can, so that
// its field is left alone.
func asForm(v any, t reflect.Type, changed *bool) (_ any, keep bool) {
	// A pointer is its value's, as encoding/json fills it: a pointer type
	// has no methods of its own, so what decodes itself is the base type.
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == timeType {
		return timeAsForm(v, changed)
	}
	if decodesItself(t) {
		return v, true
	}
	switch v := v.(type) {
	case string:
		return scalarAsForm(v, t, changed)
	case jsonObject:
		var fields []jsonField
		switch t.Kind() {
		case reflect.Struct:
			fields = jsonFieldsOf(t)
		case reflect.Map:
		default:
			return v, true
		}
		kept := v[:0]
		for _, m := range v {
			// A key that's no field is left as it came, and so is one for a
			// field with a ",string" tag, which reads its number from the text.
			var mt reflect.Type
			if t.Kind() == reflect.Map {
				mt = t.Elem()
			} else if f, ok := fieldFor(fields, m.key); ok && !f.quoted {
				mt = f.typ
			}
			if mt != nil {
				var keep bool
				if m.value, keep = asForm(m.value, mt, changed); !keep {
					*changed = true
					continue
				}
			}
			kept = append(kept, m)
		}
		return kept, true
	case jsonArray:
		if t.Kind() != reflect.Slice && t.Kind() != reflect.Array {
			return v, true
		}
		kept := v[:0]
		for _, e := range v {
			value, keep := asForm(e, t.Elem(), changed)
			switch {
			case keep:
				kept = append(kept, value)
			case t.Kind() == reflect.Array:
				// An array's places are fixed: the empty string stays, and
				// fails as it did.
				kept = append(kept, e)
			default:
				*changed = true // an empty entry in a list is left out, as a form's is
			}
		}
		return kept, true
	}
	return v, true
}

// scalarAsForm reads s for a field of type t, when that's a bool or a
// number, as setScalar reads a form value for one.
func scalarAsForm(s string, t reflect.Type, changed *bool) (any, bool) {
	if !boolOrNumber(t.Kind()) {
		return s, true
	}
	if s == "" {
		return s, false
	}
	var out any
	var err error
	switch t.Kind() {
	case reflect.Bool:
		out, err = parseBool(s)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var n int64
		n, err = strconv.ParseInt(s, 10, t.Bits())
		out = json.Number(strconv.FormatInt(n, 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		var n uint64
		n, err = strconv.ParseUint(s, 10, t.Bits())
		out = json.Number(strconv.FormatUint(n, 10))
	default: // a float
		var f float64
		f, err = strconv.ParseFloat(s, t.Bits())
		if math.IsInf(f, 0) || math.IsNaN(f) {
			return s, true // JSON has no way to write these
		}
		out = json.Number(strconv.FormatFloat(f, 'g', -1, t.Bits()))
	}
	if err != nil {
		return s, true
	}
	*changed = true
	return out, true
}

// boolOrNumber reports whether k is a kind a JSON bool or number decodes
// into.
func boolOrNumber(k reflect.Kind) bool {
	switch k {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

// timeAsForm reads v for a time.Time as a form value is, in what an HTML
// date or datetime-local input sends as well as RFC 3339, and writes it in
// RFC 3339, the one time.Time reads from JSON.
func timeAsForm(v any, changed *bool) (any, bool) {
	s, ok := v.(string)
	if !ok {
		return v, true
	}
	if s == "" {
		return s, false
	}
	var tm time.Time
	if tm.UnmarshalText([]byte(s)) == nil {
		return s, true // RFC 3339, which time.Time reads from JSON as it is
	}
	tm, err := parseTime(s)
	if err != nil {
		return s, true
	}
	*changed = true
	return tm.Format(time.RFC3339Nano), true
}

var jsonUnmarshalerType = reflect.TypeFor[json.Unmarshaler]()

// decodesItself reports whether encoding/json hands the value for a t to t
// to read, as it does for a json.Unmarshaler, and a string for an
// encoding.TextUnmarshaler: what it's sent is t's business.
func decodesItself(t reflect.Type) bool {
	pt := reflect.PointerTo(t)
	return t.Implements(jsonUnmarshalerType) || pt.Implements(jsonUnmarshalerType) ||
		t.Implements(textUnmarshalerType) || pt.Implements(textUnmarshalerType)
}

// jsonObject and jsonArray are a JSON body as jsonAsForm reads it. An
// object keeps its members in the order they came, repeats and all, so that
// what encoding/json reads back is the body as it was sent, but for the
// strings rewritten, and a bad value is still the first error it was.
type (
	jsonObject []jsonMember
	jsonArray  []any
)

type jsonMember struct {
	key   string
	value any
}

// readJSON reads the value dec is at: a jsonObject, a jsonArray, a string,
// a json.Number, a bool, or nil.
func readJSON(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch tok {
	case json.Delim('{'):
		obj := jsonObject{}
		for dec.More() {
			key, err := dec.Token()
			if err != nil {
				return nil, err
			}
			value, err := readJSON(dec)
			if err != nil {
				return nil, err
			}
			k, _ := key.(string)
			obj = append(obj, jsonMember{key: k, value: value})
		}
		_, err := dec.Token() // }
		return obj, err
	case json.Delim('['):
		arr := jsonArray{}
		for dec.More() {
			value, err := readJSON(dec)
			if err != nil {
				return nil, err
			}
			arr = append(arr, value)
		}
		_, err := dec.Token() // ]
		return arr, err
	}
	return tok, nil
}

// writeJSON writes v, as readJSON reads a value, as JSON.
func writeJSON(b *bytes.Buffer, v any) {
	switch v := v.(type) {
	case jsonObject:
		b.WriteByte('{')
		for i, m := range v {
			if i > 0 {
				b.WriteByte(',')
			}
			writeJSON(b, m.key)
			b.WriteByte(':')
			writeJSON(b, m.value)
		}
		b.WriteByte('}')
	case jsonArray:
		b.WriteByte('[')
		for i, e := range v {
			if i > 0 {
				b.WriteByte(',')
			}
			writeJSON(b, e)
		}
		b.WriteByte(']')
	case json.Number:
		b.WriteString(string(v))
	case nil:
		b.WriteString("null")
	default: // a string or a bool
		out, _ := json.Marshal(v)
		b.Write(out)
	}
}

// jsonField is a struct field as encoding/json decodes objects into it:
// the key it takes, and its type.
type jsonField struct {
	name   string
	tagged bool  // the name is its json tag's
	index  []int // where it is, through the structs embedded on the way
	typ    reflect.Type
	quoted bool // a ",string" tag: the value comes as text, which the field reads
}

var jsonFieldCache sync.Map // reflect.Type → []jsonField

// jsonFieldsOf lists the fields that encoding/json decodes objects of
// struct type t into, by its rules: json tags and "-"; the fields of
// embedded structs, promoted as Go promotes them, except that a tagged
// field wins among fields at the same depth; and none for a name that two
// fields at the shallowest depth share. jsonAsForm must find the field
// encoding/json fills, or it could make a number of a string for a field
// that takes text.
func jsonFieldsOf(t reflect.Type) []jsonField {
	if fs, ok := jsonFieldCache.Load(t); ok {
		return fs.([]jsonField)
	}
	type embedded struct {
		typ   reflect.Type
		index []int
	}
	var fields []jsonField
	next := []embedded{{typ: t}}
	visited := map[reflect.Type]bool{}
	var count, nextCount map[reflect.Type]int
	for len(next) > 0 {
		current := next
		next = nil
		count, nextCount = nextCount, map[reflect.Type]int{}
		for _, s := range current {
			if visited[s.typ] {
				continue
			}
			visited[s.typ] = true
			for i := range s.typ.NumField() {
				sf := s.typ.Field(i)
				if sf.Anonymous {
					et := sf.Type
					if et.Kind() == reflect.Pointer {
						et = et.Elem()
					}
					if !sf.IsExported() && et.Kind() != reflect.Struct {
						continue // an unexported struct can still have exported fields
					}
				} else if !sf.IsExported() {
					continue
				}
				tag := sf.Tag.Get("json")
				if tag == "-" {
					continue
				}
				name, opts, _ := strings.Cut(tag, ",")
				if !validJSONName(name) {
					name = ""
				}
				index := append(slices.Clone(s.index), i)
				ft := sf.Type
				if ft.Name() == "" && ft.Kind() == reflect.Pointer {
					ft = ft.Elem()
				}
				if name == "" && sf.Anonymous && ft.Kind() == reflect.Struct {
					// An embedded struct's fields come up a level, once each
					// type, however many times it's embedded.
					nextCount[ft]++
					if nextCount[ft] == 1 {
						next = append(next, embedded{typ: ft, index: index})
					}
					continue
				}
				f := jsonField{
					name:   cmp.Or(name, sf.Name),
					tagged: name != "",
					index:  index,
					typ:    sf.Type,
					quoted: slices.Contains(strings.Split(opts, ","), "string") && (boolOrNumber(ft.Kind()) || ft.Kind() == reflect.String),
				}
				fields = append(fields, f)
				if count[s.typ] > 1 {
					// Embedded more than once at one depth: its fields
					// cancel out, which a second copy shows below.
					fields = append(fields, f)
				}
			}
		}
	}

	slices.SortFunc(fields, func(a, b jsonField) int {
		if c := strings.Compare(a.name, b.name); c != 0 {
			return c
		}
		if c := len(a.index) - len(b.index); c != 0 {
			return c
		}
		if a.tagged != b.tagged {
			if a.tagged {
				return -1
			}
			return 1
		}
		return slices.Compare(a.index, b.index)
	})
	var kept []jsonField
	for i := 0; i < len(fields); {
		j := i + 1
		for j < len(fields) && fields[j].name == fields[i].name {
			j++
		}
		// The first of the fields with a name is the shallowest, and the
		// tagged one of those, unless another is as shallow and as tagged.
		same := fields[i:j]
		if len(same) == 1 || len(same[0].index) != len(same[1].index) || same[0].tagged != same[1].tagged {
			kept = append(kept, same[0])
		}
		i = j
	}
	slices.SortFunc(kept, func(a, b jsonField) int { return slices.Compare(a.index, b.index) })
	jsonFieldCache.Store(t, kept)
	return kept
}

// fieldFor returns the field encoding/json decodes an object's key into:
// the one named key, or else the first whose name is key in another case.
func fieldFor(fields []jsonField, key string) (jsonField, bool) {
	for _, f := range fields {
		if f.name == key {
			return f, true
		}
	}
	for _, f := range fields {
		if strings.EqualFold(f.name, key) {
			return f, true
		}
	}
	return jsonField{}, false
}

// validJSONName reports whether encoding/json takes name, from a json tag,
// as a field's name: letters, digits, and punctuation other than quotes
// and backslashes.
func validJSONName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if !strings.ContainsRune("!#$%&()*+-./:;<=>?@[]^_{|}~ ", c) && !unicode.IsLetter(c) && !unicode.IsDigit(c) {
			return false
		}
	}
	return true
}
