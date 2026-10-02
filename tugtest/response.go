package tugtest

import (
	"cmp"
	"encoding"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cuonggt/tug"
	"github.com/cuonggt/tug/inertia"
)

// Response is what the app answered a request with.
type Response struct {
	Code   int // the status
	Header http.Header
	Body   string

	// Page is the page the response shows: the page object Inertia's client
	// gets as JSON, or the one in a first visit's HTML. It's the zero Page
	// for a response that shows none, as a redirect doesn't. Props, Prop
	// and Flash read its props and flash data into Go types.
	Page inertia.Page

	t      testing.TB
	client *Client
	req    *request        // nil for a request the client couldn't make
	props  json.RawMessage // the page's props, as they came
	flash  json.RawMessage
}

// pageScript is the element a first visit's HTML carries the page object
// in.
var pageScript = regexp.MustCompile(`(?s)<script\b[^>]*\bdata-page="[^"]*"[^>]*>(.*?)</script>`)

// readPage reads the page the response shows, if it shows one, and reports
// whether it could: a page that doesn't decode fails the test.
func (r *Response) readPage() bool {
	r.t.Helper()
	var data string
	switch {
	case r.Header.Get("X-Inertia") == "true":
		data = r.Body
	case strings.HasPrefix(r.Header.Get("Content-Type"), "text/html"):
		m := pageScript.FindStringSubmatch(r.Body)
		if m == nil {
			return true // HTML, but not an Inertia page
		}
		data = m[1]
	default:
		return true
	}
	var raw struct {
		Props json.RawMessage `json:"props"`
		Flash json.RawMessage `json:"flash"`
	}
	if err := json.Unmarshal([]byte(data), &r.Page); err != nil {
		r.t.Fatalf("%s %s: the page object isn't one: %v\n%s", r.req.method, r.req.path, err, data)
		return false
	}
	if r.Page.PreserveBigIntegers {
		data = numbered(data)
		r.Page = inertia.Page{}
		json.Unmarshal([]byte(data), &r.Page)
	}
	json.Unmarshal([]byte(data), &raw)
	r.props, r.flash = raw.Props, raw.Flash
	return true
}

// numbered is a page's JSON with each BigInt in it written as the number
// it is, in place of the protocol's {"$bigint": "..."}, which the client
// reads as a BigInt: Props, Prop and Flash read one into an
// inertia.BigInt, an int64 or a json.Number, as it is.
func numbered(data string) string {
	d := json.NewDecoder(strings.NewReader(data))
	d.UseNumber()
	var page any
	if d.Decode(&page) != nil {
		return data
	}
	out, err := json.Marshal(revive(page))
	if err != nil {
		return data
	}
	return string(out)
}

// revive is v with each BigInt's marker in it a json.Number, as the client
// makes each a BigInt.
func revive(v any) any {
	switch v := v.(type) {
	case map[string]any:
		if digits, ok := v["$bigint"].(string); ok {
			return json.Number(digits)
		}
		for k, e := range v {
			v[k] = revive(e)
		}
	case []any:
		for n, e := range v {
			v[n] = revive(e)
		}
	}
	return v
}

// String describes the response for a test's message, with the request it
// answers: "POST /register: 303 to /dashboard", "GET /: 200 Home".
func (r *Response) String() string {
	var b strings.Builder
	if r.req != nil {
		fmt.Fprintf(&b, "%s %s: ", r.req.method, r.req.path)
	}
	fmt.Fprintf(&b, "%d", r.Code)
	switch {
	case r.Page.Component != "":
		b.WriteString(" " + r.Page.Component)
	case r.Location() != "":
		b.WriteString(" to " + r.Location())
	default:
		if line, _, _ := strings.Cut(strings.TrimSpace(r.Body), "\n"); line != "" {
			b.WriteString(" " + shortened(line, 200))
		}
	}
	return b.String()
}

// shortened is s cut to about n bytes, at a character's edge.
func shortened(s string, n int) string {
	for i := range s {
		if i >= n {
			return s[:i] + "…"
		}
	}
	return s
}

// Location is where the response sends the client: a redirect's Location,
// or for a 409, the X-Inertia-Location the client loads whole, or the
// X-Inertia-Redirect it visits to keep a #fragment. It's "" for a response
// that sends the client nowhere.
func (r *Response) Location() string {
	switch {
	case r.Code >= 300 && r.Code < 400:
		return r.Header.Get("Location")
	case r.Code == http.StatusConflict:
		return cmp.Or(r.Header.Get("X-Inertia-Location"), r.Header.Get("X-Inertia-Redirect"))
	}
	return ""
}

// Follow follows the redirect the response is, and those after it, as a
// browser does, and returns the response at the end, which is usually the
// page they lead to. A 303, 301 or 302 is followed with a GET, and a 307
// or 308 makes the request again, body and all; a visit's headers go
// again, as Inertia's client's do, the error bag among them. A 409's
// X-Inertia-Location is loaded whole, with FirstVisit, and its
// X-Inertia-Redirect visited. Follow fails the test when the response
// isn't a redirect, when a redirect leads to another site, and when ten
// lead on to more.
func (r *Response) Follow() *Response {
	r.t.Helper()
	if r.Location() == "" {
		r.t.Fatalf("%v: that isn't a redirect to follow", r)
		return r
	}
	for range 10 {
		if r = r.next(); r.Location() == "" {
			return r
		}
	}
	r.t.Fatalf("%v: ten redirects on, and still redirected", r)
	return r
}

// next makes the request the response sends the client on to.
func (r *Response) next() *Response {
	r.t.Helper()
	c := r.client
	to, err := url.Parse(r.Location())
	if err == nil {
		base, _ := url.Parse("http://" + r.req.host + r.req.path)
		to = base.ResolveReference(to)
	}
	if err != nil || to.Host != r.req.host {
		r.t.Fatalf("%v: that leads to another site", r)
		return c.none()
	}
	switch {
	case r.Code == http.StatusConflict && r.Header.Get("X-Inertia-Location") != "":
		return c.FirstVisit(to.String())
	case r.Code == http.StatusConflict:
		return c.Get(to.String())
	}
	req := &request{method: http.MethodGet, path: to.RequestURI(), host: to.Host, header: r.req.header.Clone()}
	if r.Code == http.StatusTemporaryRedirect || r.Code == http.StatusPermanentRedirect {
		req.method, req.body = r.req.method, r.req.body
	} else {
		req.header.Del("Content-Type")
	}
	return c.send(req)
}

// Errors returns the validation errors the response carries, by field: a
// page's errors prop, under the error bag the visit named if it named one,
// or the errors a 422 lists, as a Precognition request or an API client
// gets them. It's empty when there are none.
func (r *Response) Errors() map[string]string {
	r.t.Helper()
	var raw json.RawMessage
	switch {
	case r.Page.Component != "":
		raw = lookup(r.props, "errors")
		if bag := r.req.header.Get("X-Inertia-Error-Bag"); bag != "" && raw != nil {
			raw = lookup(raw, bag)
		}
	case r.Code == http.StatusUnprocessableEntity:
		var body struct {
			Errors json.RawMessage `json:"errors"`
		}
		json.Unmarshal([]byte(r.Body), &body)
		raw = body.Errors
	}
	errs := map[string]string{}
	if raw != nil {
		if err := json.Unmarshal(raw, &errs); err != nil {
			r.t.Fatalf("%v: the errors aren't a message for each field: %s", r, raw)
		}
	}
	return errs
}

// Has reports whether the page the response shows has a prop at path, a
// name or a dotted path, as Prop takes.
func (r *Response) Has(path string) bool {
	return r.Page.Component != "" && lookup(r.props, path) != nil
}

// Cookie returns the cookie the response set under name, or nil.
func (r *Response) Cookie(name string) *http.Cookie {
	for _, ck := range (&http.Response{Header: r.Header}).Cookies() {
		if ck.Name == name {
			return ck
		}
	}
	return nil
}

// Props returns the props of the page the response shows, read into the
// struct its tug.Page declares:
//
//	props := tugtest.Props(r, Dashboard) // a DashboardProps
//
// It fails the test when the response shows another page, or none. A
// field of one of package inertia's prop types, such as a DeferProp, holds
// a function, which JSON can't bring back: it stays as it is, and Prop
// reads what went out for it.
func Props[P any](r *Response, page tug.PageOf[P]) P {
	r.t.Helper()
	var props P
	if r.Page.Component != page.Component() {
		r.t.Fatalf("%v, not the page %s", r, page.Component())
		return props
	}
	if err := json.Unmarshal(valuesOnly(r.props, reflect.TypeFor[P]()), &props); err != nil {
		r.t.Fatalf("%v: the props aren't a %s: %v", r, reflect.TypeFor[P](), err)
	}
	return props
}

// Prop returns the prop at path, read into a T. path is a prop's name, or
// a dotted path to a prop inside another, where a number picks an item of
// a list:
//
//	user := tugtest.Prop[*User](r, "auth.user")
//	title := tugtest.Prop[string](r, "posts.data.0.title")
//
// It fails the test when the response shows no page, when the page has no
// prop at path, or when it isn't a T.
func Prop[T any](r *Response, path string) T {
	r.t.Helper()
	var v T
	if r.Page.Component == "" {
		r.t.Fatalf("%v, not a page with props", r)
		return v
	}
	raw := lookup(r.props, path)
	if raw == nil {
		r.t.Fatalf("%v: the page has no prop %s; it has %s", r, path, strings.Join(slices.Sorted(maps.Keys(r.Page.Props)), ", "))
		return v
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		r.t.Fatalf("%v: prop %s isn't a %s: %v", r, path, reflect.TypeFor[T](), err)
	}
	return v
}

// Flash returns the page's flash data under key, read into a T, as Prop
// reads a prop:
//
//	codes := tugtest.Flash[[]string](r, "recoveryCodes")
//
// It fails the test when the page has no flash data under key.
func Flash[T any](r *Response, key string) T {
	r.t.Helper()
	var v T
	if r.Page.Component == "" {
		r.t.Fatalf("%v, not a page with flash data", r)
		return v
	}
	var flash map[string]json.RawMessage
	json.Unmarshal(r.flash, &flash)
	raw, ok := flash[key]
	if !ok {
		has := strings.Join(slices.Sorted(maps.Keys(flash)), ", ")
		r.t.Fatalf("%v: the page has no flash %s; it has %s", r, key, cmp.Or(has, "none"))
		return v
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		r.t.Fatalf("%v: flash %s isn't a %s: %v", r, key, reflect.TypeFor[T](), err)
	}
	return v
}

// lookup returns the JSON at path in raw: a dotted path through objects,
// where a number picks an item of a list. It's nil when there's nothing
// there.
func lookup(raw json.RawMessage, path string) json.RawMessage {
	for key := range strings.SplitSeq(path, ".") {
		var obj map[string]json.RawMessage
		if json.Unmarshal(raw, &obj) == nil && obj != nil {
			var ok bool
			if raw, ok = obj[key]; !ok {
				return nil
			}
			continue
		}
		var list []json.RawMessage
		i, err := strconv.Atoi(key)
		if err != nil || json.Unmarshal(raw, &list) != nil || i < 0 || i >= len(list) {
			return nil
		}
		raw = list[i]
	}
	return raw
}

// inertiaPath is package inertia's import path, whose prop types hold
// functions.
var inertiaPath = reflect.TypeFor[inertia.Page]().PkgPath()

// isProp reports whether t is one of package inertia's prop types, such as
// DeferProp[T], as typegen's propOf does for the TypeScript.
func isProp(t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	kind, _, generic := strings.Cut(t.Name(), "[")
	return t.PkgPath() == inertiaPath && generic && strings.HasSuffix(kind, "Prop")
}

// valuesOnly drops from raw, the JSON of a value of type t, what went out
// for fields of package inertia's prop types, which hold functions that
// JSON can't bring back. It looks inside the structs and maps props nest
// in, whose fields are named as package inertia names them.
func valuesOnly(raw json.RawMessage, t reflect.Type) json.RawMessage {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	isMap := t.Kind() == reflect.Map && t.Key().Kind() == reflect.String
	if !isMap && t.Kind() != reflect.Struct || codesItself(t) {
		return raw
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil || obj == nil {
		return raw
	}
	keep := func(key string, ft reflect.Type) {
		switch v, ok := obj[key]; {
		case !ok:
		case isProp(ft):
			delete(obj, key)
		default:
			obj[key] = valuesOnly(v, ft)
		}
	}
	if isMap {
		for key := range obj {
			keep(key, t.Elem())
		}
	} else {
		eachField(t, keep)
	}
	out, _ := json.Marshal(obj)
	return out
}

// eachField calls fn with the name and type of each of struct t's fields,
// as package inertia names a struct's fields when it holds props: by
// their json tags, with an untagged embedded struct's fields as its own.
func eachField(t reflect.Type, fn func(name string, ft reflect.Type)) {
	for i := range t.NumField() {
		sf := t.Field(i)
		tag := sf.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if sf.Anonymous && name == "" && sf.Type.Kind() == reflect.Struct {
			eachField(sf.Type, fn)
			continue
		}
		if sf.IsExported() {
			fn(cmp.Or(name, sf.Name), sf.Type)
		}
	}
}

var (
	jsonMarshaler   = reflect.TypeFor[json.Marshaler]()
	jsonUnmarshaler = reflect.TypeFor[json.Unmarshaler]()
	textMarshaler   = reflect.TypeFor[encoding.TextMarshaler]()
	textUnmarshaler = reflect.TypeFor[encoding.TextUnmarshaler]()
)

// codesItself reports whether t writes or reads its own JSON, as a
// time.Time does: package inertia takes such a value as data, whatever it
// holds.
func codesItself(t reflect.Type) bool {
	pt := reflect.PointerTo(t)
	for _, i := range []reflect.Type{jsonMarshaler, jsonUnmarshaler, textMarshaler, textUnmarshaler} {
		if t.Implements(i) || pt.Implements(i) {
			return true
		}
	}
	return false
}
