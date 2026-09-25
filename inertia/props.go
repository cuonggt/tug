package inertia

import (
	"cmp"
	"fmt"
	"maps"
	"net/http"
	"reflect"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
)

// kind is how a prop takes part in partial reloads.
type kind int

const (
	regular  kind = iota
	lazy          // worked out only when it goes out
	optional      // goes out only when a partial reload asks for it
	always        // goes out even when a partial reload doesn't ask for it
	deferred      // left out of the first load, which names it for the client to fetch
)

// prop is a value that isn't plain data: one of the types below.
type prop interface {
	kind() kind
	resolve() (any, error)
	deferGroup() string
	missing() bool // a zero value, which is left out as if it weren't there
}

// LazyProp is a prop worked out only when it goes out. Make one with Lazy.
type LazyProp[T any] struct{ fn func() (T, error) }

// Lazy is a prop worked out only when it goes out: on a full visit, and on
// a partial reload that asks for it, but not on one that doesn't.
func Lazy[T any](fn func() (T, error)) LazyProp[T] { return LazyProp[T]{fn} }

func (p LazyProp[T]) kind() kind            { return lazy }
func (p LazyProp[T]) resolve() (any, error) { return call(p.fn) }
func (p LazyProp[T]) deferGroup() string    { return "" }
func (p LazyProp[T]) missing() bool         { return p.fn == nil }

// OptionalProp is a prop that goes out only when asked for. Make one with
// Optional.
type OptionalProp[T any] struct{ fn func() (T, error) }

// Optional is a prop that goes out only when a partial reload asks for it
// by name, and is never worked out otherwise.
func Optional[T any](fn func() (T, error)) OptionalProp[T] { return OptionalProp[T]{fn} }

func (p OptionalProp[T]) kind() kind            { return optional }
func (p OptionalProp[T]) resolve() (any, error) { return call(p.fn) }
func (p OptionalProp[T]) deferGroup() string    { return "" }
func (p OptionalProp[T]) missing() bool         { return p.fn == nil }

// AlwaysProp is a prop that goes out with every response. Make one with
// Always.
type AlwaysProp[T any] struct{ v T }

// Always is a prop that goes out with every response, including partial
// reloads that don't ask for it, as the errors prop does.
func Always[T any](v T) AlwaysProp[T] { return AlwaysProp[T]{v} }

func (p AlwaysProp[T]) kind() kind            { return always }
func (p AlwaysProp[T]) resolve() (any, error) { return p.v, nil }
func (p AlwaysProp[T]) deferGroup() string    { return "" }
func (p AlwaysProp[T]) missing() bool         { return false }

// DeferProp is a prop the client fetches after the page shows. Make one with
// Defer.
type DeferProp[T any] struct {
	fn    func() (T, error)
	group string
}

// Defer is a prop left out of the first load, so the page shows without
// waiting for it; the client then fetches it with a partial reload. Props
// in the same group, "default" unless one is given, come in one request.
func Defer[T any](fn func() (T, error), group ...string) DeferProp[T] {
	p := DeferProp[T]{fn: fn, group: "default"}
	if len(group) > 0 && group[0] != "" {
		p.group = group[0]
	}
	return p
}

func (p DeferProp[T]) kind() kind            { return deferred }
func (p DeferProp[T]) resolve() (any, error) { return call(p.fn) }
func (p DeferProp[T]) deferGroup() string    { return p.group }
func (p DeferProp[T]) missing() bool         { return p.fn == nil }

func call[T any](fn func() (T, error)) (any, error) {
	v, err := fn()
	return v, err
}

// entry is a prop and its name.
type entry struct {
	key   string
	value any
}

func kindOf(v any) kind {
	if p, ok := v.(prop); ok {
		return p.kind()
	}
	return regular
}

// entriesOf lists the props in a struct or a map with string keys.
func entriesOf(props any) ([]entry, error) {
	switch p := props.(type) {
	case nil:
		return nil, nil
	case Props:
		return sortedEntries(p), nil
	case map[string]any:
		return sortedEntries(p), nil
	}
	v := reflect.ValueOf(props)
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, nil
		}
		v = v.Elem()
	}
	switch {
	case v.Kind() == reflect.Struct:
		return structEntries(v), nil
	case v.Kind() == reflect.Map && v.Type().Key().Kind() == reflect.String:
		var es []entry
		for _, k := range v.MapKeys() {
			es = append(es, entry{k.String(), v.MapIndex(k).Interface()})
		}
		slices.SortFunc(es, func(a, b entry) int { return cmp.Compare(a.key, b.key) })
		return es, nil
	}
	return nil, fmt.Errorf("inertia: props are a struct or a map with string keys, not %T", props)
}

func sortedEntries(m map[string]any) []entry {
	es := make([]entry, 0, len(m))
	for _, k := range slices.Sorted(maps.Keys(m)) {
		es = append(es, entry{k, m[k]})
	}
	return es
}

// structEntries lists a struct's exported fields as props, named and left
// out as encoding/json would name and leave them out.
func structEntries(v reflect.Value) []entry {
	var es []entry
	t := v.Type()
	for i := range t.NumField() {
		sf := t.Field(i)
		tag := sf.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		fv := v.Field(i)
		if sf.Anonymous && name == "" && fv.Kind() == reflect.Struct {
			es = append(es, structEntries(fv)...)
			continue
		}
		if !sf.IsExported() {
			continue
		}
		if name == "" {
			name = sf.Name
		}
		if slices.Contains(strings.Split(opts, ","), "omitempty") && isEmpty(fv) {
			continue
		}
		es = append(es, entry{name, fv.Interface()})
	}
	return es
}

// isEmpty is encoding/json's idea of empty, for omitempty.
func isEmpty(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Interface, reflect.Pointer:
		return v.IsZero()
	}
	return false
}

// selection is which props a request asks for.
type selection struct {
	partial      bool // a partial reload of the component being rendered
	only, except map[string]bool
}

// selectionFor reads a partial reload's headers. They count only when the
// reload is of the component being rendered: a partial reload that lands on
// another component, after a redirect, gets it whole.
func selectionFor(r *http.Request, component string) selection {
	if !IsInertia(r) || r.Header.Get(headerPartialComponent) != component {
		return selection{}
	}
	return selection{
		partial: true,
		only:    headerSet(r.Header.Get(headerPartialData)),
		except:  headerSet(r.Header.Get(headerPartialExcept)),
	}
}

func headerSet(v string) map[string]bool {
	set := map[string]bool{}
	for s := range strings.SplitSeq(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			set[s] = true
		}
	}
	return set
}

// wants reports whether a prop goes out. A full visit gets every prop but
// the optional and deferred ones. A partial reload gets the props it names
// in X-Inertia-Partial-Data, or all of them when it names none, less those
// in X-Inertia-Partial-Except. Always props, and errors, go out either way.
func (s selection) wants(key string, k kind) bool {
	switch {
	case key == "errors" || k == always:
		return true
	case !s.partial:
		return k != optional && k != deferred
	case len(s.only) > 0 && !s.only[key]:
		return false
	}
	return !s.except[key]
}

// resolve works out the props that go out, and the groups of deferred
// props a full visit names for the client to fetch.
func resolve(entries []entry, sel selection) (map[string]any, map[string][]string, error) {
	props := make(map[string]any, len(entries))
	var deferredProps map[string][]string
	var later []entry
	for _, e := range entries {
		p, isProp := e.value.(prop)
		if isProp && p.missing() {
			continue
		}
		k := kindOf(e.value)
		if k == deferred && !sel.partial {
			if deferredProps == nil {
				deferredProps = map[string][]string{}
			}
			deferredProps[p.deferGroup()] = append(deferredProps[p.deferGroup()], e.key)
			continue
		}
		if !sel.wants(e.key, k) {
			continue
		}
		if isProp {
			later = append(later, e)
			continue
		}
		props[e.key] = emptyNils(e.value)
	}

	values, err := resolveAll(later)
	if err != nil {
		return nil, nil, err
	}
	for n, e := range later {
		props[e.key] = emptyNils(values[n])
	}
	return props, deferredProps, nil
}

// resolveAll works out props concurrently, since each is often a query of
// its own. A panic in one becomes its error rather than taking the program
// down, which is what a panic in a goroutine would otherwise do.
func resolveAll(entries []entry) ([]any, error) {
	values := make([]any, len(entries))
	errs := make([]error, len(entries))
	if len(entries) == 1 {
		values[0], errs[0] = resolveOne(entries[0])
	} else {
		var wg sync.WaitGroup
		for n, e := range entries {
			wg.Go(func() { values[n], errs[n] = resolveOne(e) })
		}
		wg.Wait()
	}
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return values, nil
}

func resolveOne(e entry) (v any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("inertia: prop %q panicked: %v\n\n%s", e.key, r, debug.Stack())
		}
	}()
	v, err = e.value.(prop).resolve()
	if err != nil {
		err = fmt.Errorf("inertia: prop %q: %w", e.key, err)
	}
	return v, err
}
