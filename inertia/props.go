package inertia

import (
	"cmp"
	"fmt"
	"maps"
	"reflect"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"
)

// firstLoad is how a prop takes part in a page's first load.
type firstLoad int

const (
	sent     firstLoad = iota // goes out with the page
	optional                  // goes out only when a partial reload asks for it
	deferred                  // left out, and named for the client to fetch after
)

// behavior is what a prop does besides hold a value. Each prop type sets
// the parts that apply to it.
type behavior struct {
	firstLoad firstLoad
	always    bool
	group     string // a deferred prop's
	rescue    bool

	merge     bool
	deep      bool
	prepend   bool     // at the root, rather than append
	appendAt  []string // paths inside the prop to merge at, rather than the root
	prependAt []string
	matchOn   []string

	once    bool
	onceKey string
	expires time.Time
	fresh   bool

	scroll bool
}

// prop is a value that isn't plain data: one of the types below.
type prop interface {
	behave() behavior
	resolve() (any, error)
	missing() bool // a zero value, left out as if it weren't there
}

var propType = reflect.TypeFor[prop]()

func call[T any](fn func() (T, error)) (any, error) {
	v, err := fn()
	return v, err
}

// LazyProp is a prop worked out only when it goes out. Make one with Lazy.
type LazyProp[T any] struct{ fn func() (T, error) }

// Lazy is a prop worked out only when it goes out: on a full visit, and on
// a partial reload that asks for it, but not on one that doesn't.
func Lazy[T any](fn func() (T, error)) LazyProp[T] { return LazyProp[T]{fn} }

// Merge makes the prop one that a partial reload adds to what the client
// has, rather than replace: see MergeProp.
func (p LazyProp[T]) Merge(opts ...MergeOption) MergeProp[T] {
	return newMerge(p.fn, opts)
}

func (p LazyProp[T]) behave() behavior      { return behavior{} }
func (p LazyProp[T]) resolve() (any, error) { return call(p.fn) }
func (p LazyProp[T]) missing() bool         { return p.fn == nil }

// OptionalProp is a prop that goes out only when asked for. Make one with
// Optional.
type OptionalProp[T any] struct {
	fn func() (T, error)
	b  behavior
}

// Optional is a prop that goes out only when a partial reload asks for it
// by name, and is never worked out otherwise.
func Optional[T any](fn func() (T, error)) OptionalProp[T] {
	return OptionalProp[T]{fn: fn, b: behavior{firstLoad: optional}}
}

// Once makes the prop one the client keeps; see OnceProp.
func (p OptionalProp[T]) Once(opts ...OnceOption) OptionalProp[T] {
	p.b = withOnce(p.b, opts)
	return p
}

func (p OptionalProp[T]) behave() behavior      { return p.b }
func (p OptionalProp[T]) resolve() (any, error) { return call(p.fn) }
func (p OptionalProp[T]) missing() bool         { return p.fn == nil }

// AlwaysProp is a prop that goes out with every response. Make one with
// Always.
type AlwaysProp[T any] struct{ v T }

// Always is a prop that goes out with every response, including partial
// reloads that don't ask for it, as the errors prop does.
func Always[T any](v T) AlwaysProp[T] { return AlwaysProp[T]{v} }

func (p AlwaysProp[T]) behave() behavior      { return behavior{always: true} }
func (p AlwaysProp[T]) resolve() (any, error) { return p.v, nil }
func (p AlwaysProp[T]) missing() bool         { return false }

// DeferProp is a prop the client fetches after the page shows. Make one with
// Defer.
type DeferProp[T any] struct {
	fn func() (T, error)
	b  behavior
}

// Defer is a prop left out of the first load, so the page shows without
// waiting for it; the client then fetches it with a partial reload. Props
// in the same group, "default" unless one is given, come in one request.
func Defer[T any](fn func() (T, error), group ...string) DeferProp[T] {
	b := behavior{firstLoad: deferred, group: "default"}
	if len(group) > 0 && group[0] != "" {
		b.group = group[0]
	}
	return DeferProp[T]{fn: fn, b: b}
}

// Merge makes the prop one the client adds to what it has when it comes,
// rather than replace: see MergeProp.
func (p DeferProp[T]) Merge(opts ...MergeOption) DeferProp[T] {
	p.b = withMerge(p.b, opts)
	return p
}

// Once makes the prop one the client keeps; see OnceProp.
func (p DeferProp[T]) Once(opts ...OnceOption) DeferProp[T] {
	p.b = withOnce(p.b, opts)
	return p
}

// Rescue keeps the prop's failure from failing the response: the prop is
// left out and named in rescuedProps, which the client's <Deferred>
// component shows its rescue for, and the error is logged through
// slog.Default().
func (p DeferProp[T]) Rescue() DeferProp[T] {
	p.b.rescue = true
	return p
}

func (p DeferProp[T]) behave() behavior      { return p.b }
func (p DeferProp[T]) resolve() (any, error) { return call(p.fn) }
func (p DeferProp[T]) missing() bool         { return p.fn == nil }

// MergeProp is a prop that a partial reload adds to what the client has,
// rather than replace. A list is appended to, or prepended to, and items
// with the same key, named by MatchOn, are replaced where they are; an
// object gets the new keys. Make one with Merge, or Lazy(...).Merge().
//
// A full visit replaces it all the same: merging is for partial reloads,
// such as fetching the next page of a list.
type MergeProp[T any] struct {
	fn func() (T, error)
	b  behavior
}

// Merge is v as a prop that partial reloads add to rather than replace.
func Merge[T any](v T, opts ...MergeOption) MergeProp[T] {
	return newMerge(func() (T, error) { return v, nil }, opts)
}

func newMerge[T any](fn func() (T, error), opts []MergeOption) MergeProp[T] {
	return MergeProp[T]{fn: fn, b: withMerge(behavior{}, opts)}
}

// Once makes the prop one the client keeps; see OnceProp.
func (p MergeProp[T]) Once(opts ...OnceOption) MergeProp[T] {
	p.b = withOnce(p.b, opts)
	return p
}

func (p MergeProp[T]) behave() behavior      { return p.b }
func (p MergeProp[T]) resolve() (any, error) { return call(p.fn) }
func (p MergeProp[T]) missing() bool         { return p.fn == nil }

// A MergeOption says how a prop is merged.
type MergeOption func(*behavior)

// Prepend puts new items before the ones the client has, rather than
// after them.
func Prepend() MergeOption { return func(b *behavior) { b.prepend = true } }

// DeepMerge merges objects at every depth, and the lists in them.
func DeepMerge() MergeOption { return func(b *behavior) { b.deep = true } }

// MatchOn names the key that makes two items of a list the same item, so a
// new copy replaces the old one where it is: "id", or "data.id" for the
// items at the prop's data.
func MatchOn(fields ...string) MergeOption {
	return func(b *behavior) { b.matchOn = append(b.matchOn, fields...) }
}

// AppendAt merges at a path inside the prop, such as the "data" of a
// paginated list, rather than at the prop itself.
func AppendAt(path string) MergeOption {
	return func(b *behavior) { b.appendAt = append(b.appendAt, path) }
}

// PrependAt is AppendAt, putting the new items first.
func PrependAt(path string) MergeOption {
	return func(b *behavior) { b.prependAt = append(b.prependAt, path) }
}

func withMerge(b behavior, opts []MergeOption) behavior {
	b.merge = true
	b.matchOn = slices.Clone(b.matchOn)
	b.appendAt = slices.Clone(b.appendAt)
	b.prependAt = slices.Clone(b.prependAt)
	for _, opt := range opts {
		opt(&b)
	}
	return b
}

// OnceProp is a prop the client keeps once it has it. Make one with Once.
type OnceProp[T any] struct {
	fn func() (T, error)
	b  behavior
}

// Once is a prop the client keeps once it has it, across pages, so later
// visits leave it out and don't work it out again: a list of countries, or
// plans. It's sent again when it expires (Until), when forced (Fresh), and
// when a partial reload asks for it.
func Once[T any](fn func() (T, error), opts ...OnceOption) OnceProp[T] {
	return OnceProp[T]{fn: fn, b: withOnce(behavior{}, opts)}
}

func (p OnceProp[T]) behave() behavior      { return p.b }
func (p OnceProp[T]) resolve() (any, error) { return call(p.fn) }
func (p OnceProp[T]) missing() bool         { return p.fn == nil }

// A OnceOption says how long the client keeps a once prop, and under what
// name.
type OnceOption func(*behavior)

// As names what the client keeps the prop as, when that isn't the prop's
// path: pages whose props differ in name can share one copy.
func As(key string) OnceOption { return func(b *behavior) { b.onceKey = key } }

// Until has the client drop its copy at t, and fetch the prop again after.
func Until(t time.Time) OnceOption { return func(b *behavior) { b.expires = t } }

// Fresh sends the prop even to a client that has it, when when is true, as
// after the data behind it has changed.
func Fresh(when bool) OnceOption { return func(b *behavior) { b.fresh = when } }

func withOnce(b behavior, opts []OnceOption) behavior {
	b.once = true
	for _, opt := range opts {
		opt(&b)
	}
	return b
}

// Paging is where a page of a list sits among the others: the query
// parameter that picks a page, and the pages around this one, numbers or
// cursors, nil where there's none.
type Paging struct {
	PageName string // default "page"
	Previous any
	Next     any
	Current  any
}

// PageNumbers is the Paging of page number page, with a next page when
// there are more.
func PageNumbers(page int, more bool) Paging {
	p := Paging{Current: page}
	if page > 1 {
		p.Previous = page - 1
	}
	if more {
		p.Next = page + 1
	}
	return p
}

// ScrollProp is a page of a list, for Inertia's InfiniteScroll. Make one with
// Scroll.
type ScrollProp[T any] struct {
	fn func() ([]T, Paging, error)
	b  behavior
}

// Scroll is a page of a list for Inertia's <InfiniteScroll> component,
// which asks for the pages after it, or before it, as the list scrolls,
// and adds them to what it shows. fn returns the page the request asks
// for, and where it sits. The prop goes out as {"data": items}.
func Scroll[T any](fn func() ([]T, Paging, error)) ScrollProp[T] {
	return ScrollProp[T]{fn: fn, b: behavior{merge: true, scroll: true}}
}

// Defer leaves the list out of the first load, as Defer does a prop.
func (p ScrollProp[T]) Defer(group ...string) ScrollProp[T] {
	p.b.firstLoad = deferred
	p.b.group = "default"
	if len(group) > 0 && group[0] != "" {
		p.b.group = group[0]
	}
	return p
}

// MatchOn names the key that makes two items the same, so a page that
// repeats one replaces it rather than add it twice: "id".
func (p ScrollProp[T]) MatchOn(fields ...string) ScrollProp[T] {
	p.b.matchOn = slices.Clone(p.b.matchOn)
	for _, f := range fields {
		p.b.matchOn = append(p.b.matchOn, "data."+f)
	}
	return p
}

func (p ScrollProp[T]) behave() behavior { return p.b }
func (p ScrollProp[T]) missing() bool    { return p.fn == nil }

func (p ScrollProp[T]) resolve() (any, error) {
	items, paging, err := p.fn()
	if err != nil {
		return nil, err
	}
	return scrolled{value: map[string]any{"data": items}, paging: paging}, nil
}

// scrolled is what a ScrollProp resolves to: the prop's value, and the
// paging that goes in scrollProps.
type scrolled struct {
	value  any
	paging Paging
}

// ScrollMeta is where a scroll prop's page sits, in the page object's
// scrollProps.
type ScrollMeta struct {
	PageName     string `json:"pageName"`
	PreviousPage any    `json:"previousPage"`
	NextPage     any    `json:"nextPage"`
	CurrentPage  any    `json:"currentPage"`
	Reset        bool   `json:"reset"`
}

// OnceMeta is a once prop's entry in the page object's onceProps.
type OnceMeta struct {
	Prop      string `json:"prop"`
	ExpiresAt *int64 `json:"expiresAt"` // Unix milliseconds
}

// entry is a prop and its name.
type entry struct {
	key   string
	value any
}

// entriesOf lists the props in a struct or a map with string keys.
func entriesOf(props any) ([]entry, error) {
	if props == nil {
		return nil, nil
	}
	if es, ok := childrenOf(reflect.ValueOf(props), true); ok {
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

// childrenOf lists the props inside v when it's something props nest in: a
// map with string keys, or a struct whose type holds props. Other values
// are data, and go out as encoding/json writes them. top takes any struct,
// as the props of a page are.
func childrenOf(v reflect.Value, top bool) ([]entry, bool) {
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil, top
		}
		v = v.Elem()
	}
	if v.Type().Implements(propType) || (!top && !holdsProps(v.Type())) {
		return nil, false
	}
	switch v.Kind() {
	case reflect.Map:
		if v.Type().Key().Kind() != reflect.String {
			return nil, false
		}
		if m, ok := v.Interface().(map[string]any); ok {
			return sortedEntries(m), true
		}
		if m, ok := v.Interface().(Props); ok {
			return sortedEntries(m), true
		}
		es := make([]entry, 0, v.Len())
		for _, k := range v.MapKeys() {
			es = append(es, entry{k.String(), v.MapIndex(k).Interface()})
		}
		slices.SortFunc(es, func(a, b entry) int { return cmp.Compare(a.key, b.key) })
		return es, true
	case reflect.Struct:
		return structEntries(v), true
	}
	return nil, false
}

// holdsProps reports whether a value of type t can hold a prop: a map or
// struct with a prop somewhere in it, or an interface, which can hold
// anything. Lists aren't looked inside.
func holdsProps(t reflect.Type) bool {
	if known, ok := propTypes.Load(t); ok {
		return known.(bool)
	}
	holds := typeHoldsProps(t, map[reflect.Type]bool{})
	propTypes.Store(t, holds)
	return holds
}

var propTypes sync.Map // reflect.Type → bool

func typeHoldsProps(t reflect.Type, visiting map[reflect.Type]bool) bool {
	if t.Implements(propType) {
		return true
	}
	if marshalsItself(t) {
		return false
	}
	switch t.Kind() {
	case reflect.Interface:
		return true
	case reflect.Pointer:
		return typeHoldsProps(t.Elem(), visiting)
	case reflect.Map:
		return t.Key().Kind() == reflect.String && typeHoldsProps(t.Elem(), visiting)
	case reflect.Struct:
		if visiting[t] {
			return false
		}
		visiting[t] = true
		for i := range t.NumField() {
			sf := t.Field(i)
			if (sf.IsExported() || sf.Anonymous) && sf.Tag.Get("json") != "-" && typeHoldsProps(sf.Type, visiting) {
				return true
			}
		}
	}
	return false
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

// resolver works out a page's props for one request, and the metadata the
// client needs about them, following the protocol's rules for partial
// reloads, first loads, merging and once props.
type resolver struct {
	partial      bool     // a partial reload of the component being rendered
	inertia      bool     // a visit from Inertia's client
	only, except []string // nil when the header isn't there
	reset        []string
	loaded       []string // the once props the client has
	prepend      bool     // infinite scroll asks for the page before

	deferred  map[string][]string
	rescued   []string
	merge     []string
	prepends  []string
	deepMerge []string
	matchOn   []string
	scroll    map[string]ScrollMeta
	once      map[string]OnceMeta
	failed    func(path string, err error) // a rescued prop's error
}

// level works out the props at one level of the tree, below prefix. A
// prop worked out by its parent, as the value a Lazy returned, isn't
// filtered by a partial reload's paths: the parent was asked for whole.
func (rs *resolver) level(entries []entry, prefix string, parentResolved bool) (map[string]any, error) {
	out := make(map[string]any, len(entries))
	type item struct {
		e    entry
		path string
		p    prop
		b    behavior
		call int // its index in calls
	}
	var kept []item
	var calls []prop
	for _, e := range entries {
		path := e.key
		if prefix != "" {
			path = prefix + "." + e.key
		}
		it := item{e: e, path: path}
		if p, ok := e.value.(prop); ok {
			if p.missing() {
				continue
			}
			it.p, it.b = p, p.behave()
		}
		if rs.partial && !parentResolved && !it.b.always && path != "errors" && !rs.pathWanted(path) {
			continue
		}
		if !rs.partial && it.p != nil && rs.leftOutOfFirstLoad(it.b, path) {
			continue
		}
		if it.p != nil {
			it.call = len(calls)
			calls = append(calls, it.p)
		}
		kept = append(kept, it)
	}

	values, errs := resolveAll(calls)
	for _, it := range kept {
		if it.p == nil {
			v, err := rs.value(it.e.value, it.path, parentResolved)
			if err != nil {
				return nil, err
			}
			out[it.e.key] = v
			continue
		}
		n := it.call
		if err := errs[n]; err != nil {
			if !it.b.rescue {
				return nil, fmt.Errorf("inertia: prop %q: %w", it.path, err)
			}
			rs.rescued = append(rs.rescued, it.path)
			if rs.failed != nil {
				rs.failed(it.path, err)
			}
			continue
		}
		v := values[n]
		if s, ok := v.(scrolled); ok {
			rs.collectScroll(s.paging, it.path)
			v = s.value
		}
		rs.collectMerge(it.b, it.path)
		rs.collectOnce(it.b, it.path)
		v, err := rs.value(v, it.path, true)
		if err != nil {
			return nil, err
		}
		out[it.e.key] = v
	}
	return out, nil
}

// value is v as it goes out: with the props nested in it worked out, and
// its nil slices and maps made empty.
func (rs *resolver) value(v any, path string, parentResolved bool) (any, error) {
	if v == nil {
		return nil, nil
	}
	if children, ok := childrenOf(reflect.ValueOf(v), false); ok {
		return rs.level(children, path, parentResolved)
	}
	return emptyNils(v), nil
}

// pathWanted reports whether a partial reload asks for the prop at path:
// it's one of the paths in X-Inertia-Partial-Data, or inside one, or on
// the way to one; and not in X-Inertia-Partial-Except, or inside one.
func (rs *resolver) pathWanted(path string) bool {
	if rs.only != nil && !within(path, rs.only) && !leadsTo(path, rs.only) {
		return false
	}
	return rs.except == nil || !within(path, rs.except)
}

// labeled reports whether a partial reload gets the metadata of the prop at
// path, which it does only for the props it asks for, not those on the way.
func (rs *resolver) labeled(path string) bool {
	if !rs.partial {
		return true
	}
	if rs.only != nil && !within(path, rs.only) {
		return false
	}
	return rs.except == nil || !within(path, rs.except)
}

// within reports whether path is one of paths, or inside one.
func within(path string, paths []string) bool {
	for _, p := range paths {
		if path == p || strings.HasPrefix(path, p+".") {
			return true
		}
	}
	return false
}

// leadsTo reports whether one of paths is inside path.
func leadsTo(path string, paths []string) bool {
	for _, p := range paths {
		if strings.HasPrefix(p, path+".") {
			return true
		}
	}
	return false
}

// leftOutOfFirstLoad reports whether a prop stays out of a full visit's
// response, noting the metadata the client needs about it all the same.
func (rs *resolver) leftOutOfFirstLoad(b behavior, path string) bool {
	switch {
	case b.firstLoad == optional || b.firstLoad == deferred:
		if b.firstLoad == deferred && !rs.clientHas(b, path) {
			if rs.deferred == nil {
				rs.deferred = map[string][]string{}
			}
			rs.deferred[b.group] = append(rs.deferred[b.group], path)
		}
		rs.collectMerge(b, path)
		rs.collectOnce(b, path)
		return true
	case rs.inertia && rs.clientHas(b, path):
		rs.collectOnce(b, path)
		return true
	}
	return false
}

// clientHas reports whether the client has a once prop already, and may
// keep it.
func (rs *resolver) clientHas(b behavior, path string) bool {
	return b.once && !b.fresh && slices.Contains(rs.loaded, cmp.Or(b.onceKey, path))
}

// collectMerge notes how the client merges the prop at path. A prop the
// client is resetting gets no label, so the client replaces it.
func (rs *resolver) collectMerge(b behavior, path string) {
	if !b.merge || slices.Contains(rs.reset, path) || !rs.labeled(path) {
		return
	}
	appendAt, prependAt := b.appendAt, b.prependAt
	if b.scroll {
		if rs.prepend {
			prependAt = []string{"data"}
		} else {
			appendAt = []string{"data"}
		}
	}
	switch {
	case b.deep:
		rs.deepMerge = append(rs.deepMerge, path)
	case len(appendAt) == 0 && len(prependAt) == 0 && !b.prepend:
		rs.merge = append(rs.merge, path)
	case len(appendAt) == 0 && len(prependAt) == 0:
		rs.prepends = append(rs.prepends, path)
	default:
		for _, at := range appendAt {
			rs.merge = append(rs.merge, path+"."+at)
		}
		for _, at := range prependAt {
			rs.prepends = append(rs.prepends, path+"."+at)
		}
	}
	for _, field := range b.matchOn {
		rs.matchOn = append(rs.matchOn, path+"."+field)
	}
}

// collectOnce notes a once prop, under the name the client keeps it by.
func (rs *resolver) collectOnce(b behavior, path string) {
	if !b.once || !rs.labeled(path) {
		return
	}
	meta := OnceMeta{Prop: path}
	if !b.expires.IsZero() {
		ms := b.expires.UnixMilli()
		meta.ExpiresAt = &ms
	}
	if rs.once == nil {
		rs.once = map[string]OnceMeta{}
	}
	rs.once[cmp.Or(b.onceKey, path)] = meta
}

// collectScroll notes where a scroll prop's page sits.
func (rs *resolver) collectScroll(p Paging, path string) {
	if rs.scroll == nil {
		rs.scroll = map[string]ScrollMeta{}
	}
	rs.scroll[path] = ScrollMeta{
		PageName:     cmp.Or(p.PageName, "page"),
		PreviousPage: p.Previous,
		NextPage:     p.Next,
		CurrentPage:  p.Current,
		Reset:        slices.Contains(rs.reset, path),
	}
}

// resolveAll works out props concurrently, since each is often a query of
// its own. A panic in one becomes its error rather than taking the program
// down, which is what a panic in a goroutine would otherwise do.
func resolveAll(props []prop) ([]any, []error) {
	values := make([]any, len(props))
	errs := make([]error, len(props))
	if len(props) == 1 {
		values[0], errs[0] = resolveOne(props[0])
		return values, errs
	}
	var wg sync.WaitGroup
	for n, p := range props {
		wg.Go(func() { values[n], errs[n] = resolveOne(p) })
	}
	wg.Wait()
	return values, errs
}

func resolveOne(p prop) (v any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panicked: %v\n\n%s", r, debug.Stack())
		}
	}()
	return p.resolve()
}
