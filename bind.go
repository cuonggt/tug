package tug

import (
	"bytes"
	"cmp"
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Bind fills the struct dst points to from the request. Each field's tags
// say where its value comes from:
//
//	type UpdatePost struct {
//		ID    int64  `path:"id"`     // the {id} in "/posts/{id}"
//		Draft bool   `query:"draft"` // ?draft=1
//		Title string `json:"title"`  // the body
//	}
//
// The body is read by its Content-Type. JSON goes through encoding/json and
// its json tags. A form, urlencoded or multipart, fills a field by its form
// tag, or else its json tag's name, or else its name; a key that repeats, or
// ends in "[]", fills a slice. Files go to fields of type
// *multipart.FileHeader or []*multipart.FileHeader. An empty form value
// leaves its field alone, as an empty input means no value rather than zero.
//
// Query values are bound after the body, and path values last, so a body
// can't overwrite the ID in the URL.
//
// A value that doesn't parse is a 400, or a 404 when it's a path value,
// since /posts/abc is a page that isn't there. The error wraps a *BindError
// naming the field. The fields after it are still bound, so that checking
// them, as BindValid does, finds what's really wrong with them rather than
// what's missing. A body over Config.BodyLimit is a 413, and a body Bind
// can't read is a 415.
func (c *Ctx) Bind(dst any) error {
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("tug: Bind needs a pointer to a struct, not %T", dst)
	}
	v = v.Elem()
	fields := fieldsOf(v.Type())

	var first error
	if err := c.bindBody(dst, v, fields); err != nil {
		if !isBindError(err) {
			return err
		}
		first = err
	}
	for _, f := range fields {
		if f.query == "" {
			continue
		}
		if err := setValues(v.FieldByIndex(f.index), lookup(c.queryValues(), f.query)); err != nil {
			err = bindFailed(http.StatusBadRequest, f.query, err)
			if !isBindError(err) {
				return err
			}
			first = cmp.Or(first, err)
		}
	}
	for _, f := range fields {
		if f.path == "" {
			continue
		}
		if err := setValues(v.FieldByIndex(f.index), []string{c.r.PathValue(f.path)}); err != nil {
			return bindFailed(http.StatusNotFound, f.path, err) // no such page, whatever else is wrong
		}
	}
	return first
}

// isBindError reports whether err is a value that didn't parse, rather
// than a request Bind can't read at all.
func isBindError(err error) bool {
	var be *BindError
	return errors.As(err, &be)
}

// multipartMemory is how much of a multipart body is held in memory. Files
// past it go to temporary files, which net/http removes after the request.
const multipartMemory = 32 << 20

func (c *Ctx) bindBody(dst any, v reflect.Value, fields []field) error {
	r := c.r
	if r.ContentLength == 0 {
		return nil
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return NewHTTPError(http.StatusUnsupportedMediaType)
	}
	if c.body == nil {
		r.Body = http.MaxBytesReader(c.rw.ResponseWriter, r.Body, c.app.config.BodyLimit)
	}

	switch {
	case mediaType == "application/json" || strings.HasSuffix(mediaType, "+json"):
		// Kept, so that a handler can bind the body more than once, as one
		// that finds a post by its {id} and then binds the form does.
		if c.body == nil {
			data, err := io.ReadAll(r.Body)
			if err != nil {
				return bodyFailed(err)
			}
			c.body = append([]byte{}, data...)
		}
		return bindJSON(c.body, dst)
	case mediaType == "application/x-www-form-urlencoded":
		if err := r.ParseForm(); err != nil {
			return bodyFailed(err)
		}
		return bindForm(v, fields, r.PostForm, nil)
	case mediaType == "multipart/form-data":
		if err := r.ParseMultipartForm(multipartMemory); err != nil {
			return bodyFailed(err)
		}
		return bindForm(v, fields, r.MultipartForm.Value, r.MultipartForm.File)
	}
	return NewHTTPError(http.StatusUnsupportedMediaType)
}

func bindJSON(data []byte, dst any) error {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	err := json.Unmarshal(data, dst)
	if err == nil {
		return nil
	}
	var te *json.UnmarshalTypeError
	if errors.As(err, &te) && te.Field != "" {
		be := &BindError{Field: te.Field, Reason: reasonFor(te.Type), Err: err}
		return &HTTPError{Code: http.StatusBadRequest, Message: be.Error(), Err: be}
	}
	return &HTTPError{Code: http.StatusBadRequest, Message: "invalid JSON", Err: err}
}

// bodyFailed is the error for a body that couldn't be read.
func bodyFailed(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return &HTTPError{Code: http.StatusRequestEntityTooLarge, Err: err}
	}
	return &HTTPError{Code: http.StatusBadRequest, Err: err}
}

var (
	fileType  = reflect.TypeFor[*multipart.FileHeader]()
	filesType = reflect.TypeFor[[]*multipart.FileHeader]()
)

func bindForm(v reflect.Value, fields []field, values url.Values, files map[string][]*multipart.FileHeader) error {
	var first error
	for _, f := range fields {
		if f.form == "" {
			continue
		}
		fv := v.FieldByIndex(f.index)
		switch fv.Type() {
		case fileType:
			if fhs := lookup(files, f.form); len(fhs) > 0 {
				fv.Set(reflect.ValueOf(fhs[0]))
			}
		case filesType:
			if fhs := lookup(files, f.form); len(fhs) > 0 {
				fv.Set(reflect.ValueOf(fhs))
			}
		default:
			if err := setValues(fv, lookup(values, f.form)); err != nil {
				err = bindFailed(http.StatusBadRequest, f.form, err)
				if !isBindError(err) {
					return err
				}
				first = cmp.Or(first, err)
			}
		}
	}
	return first
}

// lookup returns what a form or query has under name, or else under
// "name[]", the way PHP and many form libraries spell a list.
func lookup[T any](m map[string][]T, name string) []T {
	if vs, ok := m[name]; ok {
		return vs
	}
	return m[name+"[]"]
}

// bindFailed turns a value that didn't parse into the error Bind returns.
// A field that Bind can't fill at all is the program's mistake rather than
// the client's, and stays a plain error: a 500.
func bindFailed(code int, name string, err error) error {
	var be *BindError
	if !errors.As(err, &be) {
		return err
	}
	be.Field = name
	if code == http.StatusNotFound {
		return &HTTPError{Code: code, Err: be}
	}
	return &HTTPError{Code: code, Message: be.Error(), Err: be}
}

// setValues sets v from the strings a form, query or path has for it: all
// of them for a slice, the first otherwise.
func setValues(v reflect.Value, vals []string) error {
	if len(vals) == 0 {
		return nil
	}
	t := v.Type()
	if t.Kind() != reflect.Slice || t.Elem().Kind() == reflect.Uint8 {
		return setScalar(v, vals[0])
	}
	s := reflect.MakeSlice(t, 0, len(vals))
	for _, str := range vals {
		if str == "" {
			continue
		}
		e := reflect.New(t.Elem()).Elem()
		if err := setScalar(e, str); err != nil {
			return err
		}
		s = reflect.Append(s, e)
	}
	v.Set(s)
	return nil
}

var (
	timeType            = reflect.TypeFor[time.Time]()
	textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()
)

// setScalar parses s into v. An empty s leaves v as it is.
func setScalar(v reflect.Value, s string) error {
	if s == "" {
		return nil
	}
	t := v.Type()
	switch {
	case t.Kind() == reflect.Pointer:
		p := reflect.New(t.Elem())
		if err := setScalar(p.Elem(), s); err != nil {
			return err
		}
		v.Set(p)
		return nil
	case t == timeType:
		tm, err := parseTime(s)
		if err != nil {
			return &BindError{Reason: reasonFor(t), Err: err}
		}
		v.Set(reflect.ValueOf(tm))
		return nil
	case reflect.PointerTo(t).Implements(textUnmarshalerType):
		if err := v.Addr().Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(s)); err != nil {
			return &BindError{Reason: reasonFor(t), Err: err}
		}
		return nil
	}

	var err error
	switch t.Kind() {
	case reflect.String:
		v.SetString(s)
	case reflect.Bool:
		var b bool
		if b, err = parseBool(s); err == nil {
			v.SetBool(b)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var n int64
		if n, err = strconv.ParseInt(s, 10, t.Bits()); err == nil {
			v.SetInt(n)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		var n uint64
		if n, err = strconv.ParseUint(s, 10, t.Bits()); err == nil {
			v.SetUint(n)
		}
	case reflect.Float32, reflect.Float64:
		var f float64
		if f, err = strconv.ParseFloat(s, t.Bits()); err == nil {
			v.SetFloat(f)
		}
	case reflect.Slice: // []byte: setValues sends no other slice here
		v.SetBytes([]byte(s))
	default:
		return fmt.Errorf("tug: Bind can't fill a field of type %s", t)
	}
	if err != nil {
		return &BindError{Reason: reasonFor(t), Err: err}
	}
	return nil
}

// reasonFor says what a value for a field of type t has to be, in words for
// the person who sent it.
func reasonFor(t reflect.Type) string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch {
	case t == timeType:
		return "must be a date"
	case reflect.PointerTo(t).Implements(textUnmarshalerType):
		return "is invalid"
	}
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "must be a whole number"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "must be a whole number, 0 or more"
	case reflect.Float32, reflect.Float64:
		return "must be a number"
	case reflect.Bool:
		return "must be true or false"
	case reflect.String:
		return "must be text"
	case reflect.Slice, reflect.Array:
		return "must be a list"
	case reflect.Map, reflect.Struct:
		return "must be an object"
	}
	return "is invalid"
}

// parseBool reads what a checkbox sends, "on", as well as the usual words.
func parseBool(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "1", "true", "on", "yes":
		return true, nil
	case "0", "false", "off", "no":
		return false, nil
	}
	return false, fmt.Errorf("%q is not true or false", s)
}

// timeLayouts are RFC 3339, and what HTML's datetime-local and date inputs
// send. A time without a zone is taken as UTC.
var timeLayouts = []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02"}

func parseTime(s string) (time.Time, error) {
	var err error
	for _, layout := range timeLayouts {
		var t time.Time
		if t, err = time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, err
}

// field is where Bind finds a struct field's value.
type field struct {
	index             []int
	path, query, form string
}

var fieldCache sync.Map // reflect.Type → []field

func fieldsOf(t reflect.Type) []field {
	if fs, ok := fieldCache.Load(t); ok {
		return fs.([]field)
	}
	fs := collectFields(t, nil)
	fieldCache.Store(t, fs)
	return fs
}

// collectFields lists t's fields, and those of the structs embedded in it,
// promoted the way encoding/json promotes them.
func collectFields(t reflect.Type, index []int) []field {
	var fs []field
	for i := range t.NumField() {
		sf := t.Field(i)
		idx := append(slices.Clone(index), i)
		if sf.Anonymous && sf.Type.Kind() == reflect.Struct && sf.Tag == "" {
			fs = append(fs, collectFields(sf.Type, idx)...)
			continue
		}
		if !sf.IsExported() {
			continue
		}
		fs = append(fs, field{
			index: idx,
			path:  sf.Tag.Get("path"),
			query: sf.Tag.Get("query"),
			form:  formName(sf),
		})
	}
	return fs
}

// formName is a field's form key: its form tag, or else its json tag's
// name, or else its name. "-" in either tag leaves the field out.
func formName(sf reflect.StructField) string {
	if name, ok := sf.Tag.Lookup("form"); ok {
		if name == "-" {
			return ""
		}
		return name
	}
	name, _, _ := strings.Cut(sf.Tag.Get("json"), ",")
	switch name {
	case "-":
		return ""
	case "":
		return sf.Name
	}
	return name
}
