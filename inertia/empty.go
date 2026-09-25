package inertia

import (
	"encoding"
	"encoding/json"
	"reflect"
	"sync"
)

// emptyNils returns v with each nil slice and map in it, at any depth, made
// empty, so they go out as [] and {} rather than null. A page's TypeScript
// types say Post[], and posts.map() on a null is a crash in the browser, so
// the Go habit of a nil slice for "none" has to stop at the page object.
//
// v itself is left alone: what has to change is copied, and a value with
// nothing to change is returned as it is.
func emptyNils(v any) any {
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
	if !mayHoldNil(rv.Type()) {
		return v
	}
	fixed, changed := fixNils(rv, 0)
	if !changed {
		return v
	}
	return fixed.Interface()
}

// maxDepth stops fixNils going round a cycle of pointers forever.
const maxDepth = 64

var (
	jsonMarshalerType = reflect.TypeFor[json.Marshaler]()
	textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()
)

// marshalsItself reports whether t encodes itself, whatever it holds.
func marshalsItself(t reflect.Type) bool {
	return t.Implements(jsonMarshalerType) || t.Implements(textMarshalerType) ||
		reflect.PointerTo(t).Implements(jsonMarshalerType) || reflect.PointerTo(t).Implements(textMarshalerType)
}

func fixNils(v reflect.Value, depth int) (reflect.Value, bool) {
	t := v.Type()
	if depth > maxDepth || !mayHoldNil(t) {
		return v, false
	}
	switch t.Kind() {
	case reflect.Slice:
		if v.IsNil() {
			return reflect.MakeSlice(t, 0, 0), true
		}
		var out reflect.Value
		for i := range v.Len() {
			e, changed := fixNils(v.Index(i), depth+1)
			if !changed {
				continue
			}
			if !out.IsValid() {
				out = reflect.MakeSlice(t, v.Len(), v.Len())
				reflect.Copy(out, v)
			}
			out.Index(i).Set(e)
		}
		return out, out.IsValid()
	case reflect.Array:
		var out reflect.Value
		for i := range v.Len() {
			e, changed := fixNils(v.Index(i), depth+1)
			if !changed {
				continue
			}
			if !out.IsValid() {
				out = reflect.New(t).Elem()
				out.Set(v)
			}
			out.Index(i).Set(e)
		}
		return out, out.IsValid()
	case reflect.Map:
		if v.IsNil() {
			return reflect.MakeMap(t), true
		}
		var out reflect.Value
		for iter := v.MapRange(); iter.Next(); {
			e, changed := fixNils(iter.Value(), depth+1)
			if !changed {
				continue
			}
			if !out.IsValid() {
				out = reflect.MakeMapWithSize(t, v.Len())
				for it := v.MapRange(); it.Next(); {
					out.SetMapIndex(it.Key(), it.Value())
				}
			}
			out.SetMapIndex(iter.Key(), e)
		}
		return out, out.IsValid()
	case reflect.Pointer:
		if v.IsNil() {
			return v, false // a nil pointer means no value, and null says so
		}
		e, changed := fixNils(v.Elem(), depth+1)
		if !changed {
			return v, false
		}
		p := reflect.New(t.Elem())
		p.Elem().Set(e)
		return p, true
	case reflect.Interface:
		if v.IsNil() {
			return v, false
		}
		e, changed := fixNils(v.Elem(), depth+1)
		if !changed {
			return v, false
		}
		out := reflect.New(t).Elem()
		out.Set(e)
		return out, true
	case reflect.Struct:
		var out reflect.Value
		for i := range t.NumField() {
			sf := t.Field(i)
			if !sf.IsExported() || sf.Tag.Get("json") == "-" {
				continue
			}
			f, changed := fixNils(v.Field(i), depth+1)
			if !changed {
				continue
			}
			if !out.IsValid() {
				out = reflect.New(t).Elem()
				out.Set(v)
			}
			out.Field(i).Set(f)
		}
		return out, out.IsValid()
	}
	return v, false
}

// mayHoldNil reports whether a value of type t can hold a nil slice or map
// that encoding/json would write as null. Most props are plain data that
// can't, and this lets emptyNils skip them without a look.
func mayHoldNil(t reflect.Type) bool {
	if known, ok := nilTypes.Load(t); ok {
		return known.(bool)
	}
	may := typeMayHoldNil(t, map[reflect.Type]bool{})
	nilTypes.Store(t, may)
	return may
}

var nilTypes sync.Map // reflect.Type → bool

func typeMayHoldNil(t reflect.Type, visiting map[reflect.Type]bool) bool {
	if marshalsItself(t) {
		return false
	}
	switch t.Kind() {
	case reflect.Slice, reflect.Map, reflect.Interface:
		return true
	case reflect.Pointer, reflect.Array:
		return typeMayHoldNil(t.Elem(), visiting)
	case reflect.Struct:
		if visiting[t] {
			return false // the first visit decides
		}
		visiting[t] = true
		for i := range t.NumField() {
			sf := t.Field(i)
			if sf.IsExported() && sf.Tag.Get("json") != "-" && typeMayHoldNil(sf.Type, visiting) {
				return true
			}
		}
	}
	return false
}
