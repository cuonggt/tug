// Package validate checks a struct against the rules in its validate tags,
// and says what's wrong in words for the person who filled in the form:
//
//	type PostInput struct {
//		Title string `json:"title" validate:"required,max=80"`
//		Email string `json:"email" validate:"required,email"`
//	}
//
//	err := validate.Struct(in) // Errors{"title": "title is required", ...}
//
// The rules are those of github.com/go-playground/validator, and two of
// tug's own for uploads, a *multipart.FileHeader's: file_max, its size at
// most, and file_type, what it is, told from its first bytes:
//
//	Photo *multipart.FileHeader `form:"photo" validate:"required,file_max=2MB,file_type=image/png image/jpeg"`
//
// Errors are keyed as their json tags name the fields, the way the client
// sent them, and a nested field by its path: "author.name",
// "items.0.price". A message names a field as a person reads it: by its
// label tag, `label:"email address"`, or else by its key made into words,
// "first_name" as "first name".
//
// Messages are said in English, or in the language given, in which the
// messages and the fields' names are texts of a language's file: see
// package lang. tug's BindValid gives the request's. Rule adds a rule of
// the app's own, for a tag such as slug.
package validate

import (
	"errors"
	"fmt"
	"maps"
	"mime/multipart"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/cuonggt/tug/internal/filetype"
	"github.com/cuonggt/tug/internal/label"
	"github.com/cuonggt/tug/lang"
	"github.com/go-playground/validator/v10"
)

// Errors are what's wrong with a request's values: a message for each field
// that failed, by the field's name.
type Errors map[string]string

// Add records message for field, unless the field already has one: the
// first thing wrong with a field is the one worth saying.
func (e Errors) Add(field, message string) {
	if _, ok := e[field]; !ok {
		e[field] = message
	}
}

// Error lists the messages, by field name.
func (e Errors) Error() string {
	var b strings.Builder
	for i, field := range slices.Sorted(maps.Keys(e)) {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(e[field])
	}
	return b.String()
}

// First returns the message of the field that sorts first, for a summary
// such as a 422's message.
func (e Errors) First() string {
	if len(e) == 0 {
		return ""
	}
	return e[slices.Min(slices.Collect(maps.Keys(e)))]
}

// Only returns the errors of the fields named, and of the fields nested in
// them: "author" keeps "author.name" too.
func (e Errors) Only(fields ...string) Errors {
	kept := Errors{}
	for field, msg := range e {
		for _, name := range fields {
			if field == name || strings.HasPrefix(field, name+".") {
				kept[field] = msg
				break
			}
		}
	}
	return kept
}

var (
	once   sync.Once
	checks *validator.Validate

	// mu keeps Rule's changes to the validator from checks going on, which
	// go-playground's validator doesn't do itself: it takes a rule only
	// before it checks anything. rules are the messages of Rule's rules.
	mu    sync.RWMutex
	rules = map[string]string{}
)

func validatorFor() *validator.Validate {
	once.Do(func() {
		checks = validator.New(validator.WithRequiredStructEnabled())
		checks.RegisterTagNameFunc(jsonName)
		checks.RegisterValidation("file_max", fileMax)
		checks.RegisterValidation("file_type", fileType)
	})
	return checks
}

// jsonName is a field's name as the client knows it: its json tag's, or
// else its form tag's, as an upload's field may have alone.
func jsonName(f reflect.StructField) string {
	name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
	if name == "" {
		name = f.Tag.Get("form")
	}
	switch name {
	case "-":
		return ""
	case "":
		return f.Name
	}
	return name
}

// Struct checks the struct v points to, or is, against its validate tags.
// It returns Errors when something is wrong, and nil when nothing is. The
// messages are in English, or in the language of in, one at most, as
// tug's BindValid has the request's.
func Struct(v any, in ...lang.Words) error {
	var w lang.Words
	switch len(in) {
	case 0:
	case 1:
		w = in[0]
	default:
		panic("validate: Struct takes one language at most")
	}
	mu.RLock()
	defer mu.RUnlock()
	err := validatorFor().Struct(v)
	if err == nil {
		return nil
	}
	var fieldErrs validator.ValidationErrors
	if !errors.As(err, &fieldErrs) {
		return fmt.Errorf("validate: %w", err) // not a struct, or a tag that doesn't parse
	}
	root := reflect.TypeOf(v)
	for root.Kind() == reflect.Pointer {
		root = root.Elem()
	}
	errs := Errors{}
	for _, fe := range fieldErrs {
		errs.Add(path(fe.Namespace(), root), message(fe, root, w))
	}
	return errs
}

// Rule adds a rule of the app's own, for a tag, as `validate:"slug"`:
// check reports whether a field's value keeps it, given the tag's
// parameter, "VN" for phone=VN, and message says what's wrong when it
// doesn't, in English, with :field for the field's name and :param for the
// parameter, as a language's file translates it:
//
//	validate.Rule("slug", func(s string, _ string) bool {
//		return slugPattern.MatchString(s)
//	}, ":field must be letters, digits and dashes")
//
// T is the type of the fields the rule checks, or one they have the same
// kind as, as a type Slug string is a string, or an interface they have: a
// field of another type panics as it's checked, as a test finds. Rules are
// added as the app starts, as its routes are; one added again replaces the
// rule before, as an app's tests that make the app afresh add them again.
// A name the validator keeps for itself, as omitempty, panics.
func Rule[T any](name string, check func(value T, param string) bool, message string) {
	want := reflect.TypeFor[T]()
	fn := func(fl validator.FieldLevel) bool {
		v := fl.Field()
		if !v.IsValid() {
			return false
		}
		switch {
		case v.Type() == want:
		case want.Kind() == reflect.Interface && v.Type().Implements(want):
		case v.Kind() == want.Kind() && v.Type().ConvertibleTo(want):
			v = v.Convert(want)
		default:
			panic(fmt.Sprintf("validate: the rule %s checks a %s, and %s is a %s", name, want, fl.FieldName(), v.Type()))
		}
		return check(v.Interface().(T), fl.Param())
	}
	mu.Lock()
	defer mu.Unlock()
	if err := validatorFor().RegisterValidation(name, fn); err != nil {
		panic("validate: " + err.Error())
	}
	rules[name] = message
}

// path turns the validator's namespace, "PostInput.items[0].price", into
// the dotted path a client names the field by, "items.0.price".
func path(namespace string, root reflect.Type) string {
	return strings.NewReplacer("[", ".", "]", "").Replace(inRoot(namespace, root))
}

// inRoot is a namespace without the struct type's name, which the
// validator starts it with, unless the type has none, as an anonymous
// struct, var in struct{...}, doesn't.
func inRoot(namespace string, root reflect.Type) string {
	if name := root.Name(); name != "" {
		return strings.TrimPrefix(namespace, name+".")
	}
	return namespace
}

// The texts of validate's messages, in English, as a language's file has
// them: :field is the field's name, and a message with a count has a form
// for one and for more, split by |.
var (
	// plain are the messages of the tags that say nothing but the field.
	plain = map[string]string{
		"required":             ":field is required",
		"required_if":          ":field is required",
		"required_unless":      ":field is required",
		"required_with":        ":field is required",
		"required_with_all":    ":field is required",
		"required_without":     ":field is required",
		"required_without_all": ":field is required",
		"email":                ":field must be a valid email address",
		"url":                  ":field must be a valid URL",
		"http_url":             ":field must be a valid URL",
		"uri":                  ":field must be a valid URL",
		"uuid":                 ":field must be a valid UUID",
		"uuid4":                ":field must be a valid UUID",
		"uuid7":                ":field must be a valid UUID",
		"alpha":                ":field must contain only letters",
		"alphanum":             ":field must contain only letters and numbers",
		"numeric":              ":field must be a number",
		"number":               ":field must be a number",
		"boolean":              ":field must be true or false",
		"datetime":             ":field must be a valid date",
		"unique":               ":field must not repeat a value",
	}

	// valued are the messages of the tags that say their parameter, :value,
	// or another field's name, :other.
	valued = map[string]string{
		"eq":         ":field must be :value",
		"ne":         ":field must not be :value",
		"oneof":      ":field must be one of: :value",
		"contains":   ":field must contain :value",
		"excludes":   ":field must not contain :value",
		"startswith": ":field must start with :value",
		"endswith":   ":field must end with :value",
		"eqfield":    ":field must match :other",
		"nefield":    ":field must be different from :other",
		"file_max":   ":field must be at most :value",
		"file_type":  ":field must be :value",
	}

	// bounds are the messages of the tags that bound a length, a count or a
	// number, by what they bound: a text's characters, a list's items, or
	// the number itself.
	bounds = map[string]struct{ text, list, number string }{
		"min": {":field must be at least :count character|:field must be at least :count characters",
			":field must have at least :count item|:field must have at least :count items", ":field must be at least :value"},
		"max": {":field must be at most :count character|:field must be at most :count characters",
			":field must have at most :count item|:field must have at most :count items", ":field must be at most :value"},
		"gt": {":field must be more than :count character|:field must be more than :count characters",
			":field must have more than :count item|:field must have more than :count items", ":field must be more than :value"},
		"lt": {":field must be less than :count character|:field must be less than :count characters",
			":field must have less than :count item|:field must have less than :count items", ":field must be less than :value"},
		"len": {":field must be exactly :count character|:field must be exactly :count characters",
			":field must have exactly :count item|:field must have exactly :count items", ":field must be exactly :value"},
	}

	// inBytes is a size in bytes, for file_max's message.
	inBytes = ":count byte|:count bytes"

	// invalid is the message of a tag with none of its own.
	invalid = ":field is invalid"
)

func init() {
	bounds["gte"], bounds["lte"] = bounds["min"], bounds["max"]
}

// Texts lists what validate says, in English, as a language's file has
// the texts: its messages, and those of the rules Rule has added. The
// names of fields and of file types are texts too, which tug lang finds
// in the app's own Go.
func Texts() []string {
	mu.RLock()
	defer mu.RUnlock()
	all := []string{inBytes, invalid}
	all = slices.AppendSeq(all, maps.Values(plain))
	all = slices.AppendSeq(all, maps.Values(valued))
	all = slices.AppendSeq(all, maps.Values(rules))
	for _, b := range bounds {
		all = append(all, b.text, b.list, b.number)
	}
	slices.Sort(all)
	return slices.Compact(all)
}

// message says what's wrong with a field in w's language, as "title must
// be at most 80 characters".
func message(fe validator.FieldError, root reflect.Type, w lang.Words) string {
	field := w.T(fieldName(root, fe))
	tag, p := fe.Tag(), fe.Param()
	if text, ok := plain[tag]; ok {
		return w.T(text, "field", field)
	}
	if b, ok := bounds[tag]; ok {
		n, _ := strconv.Atoi(p)
		switch fe.Kind() {
		case reflect.String:
			return w.Choice(b.text, n, "field", field, "count", p)
		case reflect.Slice, reflect.Array, reflect.Map:
			return w.Choice(b.list, n, "field", field, "count", p)
		}
		return w.T(b.number, "field", field, "value", p)
	}
	value := p
	switch tag {
	case "oneof":
		value = strings.Join(strings.Fields(p), ", ")
	case "contains", "excludes", "startswith", "endswith":
		value = strconv.Quote(p)
	case "eqfield", "nefield":
		return w.T(valued[tag], "field", field, "other", w.T(sibling(root, fe.StructNamespace(), p)))
	case "file_max":
		_, num, unit := sizeLimit(p)
		value = num + " " + unit
		if unit == "" {
			n, _ := strconv.Atoi(num)
			value = w.Choice(inBytes, n, "count", num)
		}
	case "file_type":
		value = w.T(filetype.Names(fileTypes(p)))
	}
	if text, ok := valued[tag]; ok {
		return w.T(text, "field", field, "value", value)
	}
	if text, ok := rules[tag]; ok {
		return w.T(text, "field", field, "param", p)
	}
	return w.T(invalid, "field", field)
}

// fieldName names the field fe is about as a message does: its label
// tag, or else its key made into words.
func fieldName(root reflect.Type, fe validator.FieldError) string {
	if f, ok := structField(root, inRoot(fe.StructNamespace(), root)); ok {
		return label.Of(f, fe.Field())
	}
	return label.Readable(fe.Field())
}

// sibling names the field that eqfield and nefield compare with, which the
// validator gives by its Go name, as a message names it.
func sibling(root reflect.Type, structNamespace, goName string) string {
	path := goName
	if ns := inRoot(structNamespace, root); strings.Contains(ns, ".") {
		path = ns[:strings.LastIndex(ns, ".")+1] + goName
	}
	if f, ok := structField(root, path); ok {
		if name := jsonName(f); name != "" {
			return label.Of(f, name)
		}
	}
	return label.Readable(goName)
}

// structField finds the field that path names in root, by Go names, as
// the validator gives them without the root's own: "Lines[1].Product".
func structField(root reflect.Type, path string) (reflect.StructField, bool) {
	t := root
	parts := strings.Split(path, ".")
	for i, part := range parts {
		part, _, _ = strings.Cut(part, "[")
		for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array || t.Kind() == reflect.Map {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct {
			return reflect.StructField{}, false
		}
		f, ok := t.FieldByName(part)
		if !ok {
			return reflect.StructField{}, false
		}
		if i == len(parts)-1 {
			return f, true
		}
		t = f.Type
	}
	return reflect.StructField{}, false
}

// fileMax checks that an upload is no bigger than the tag's size, such as
// 2MB.
func fileMax(fl validator.FieldLevel) bool {
	limit, _, _ := sizeLimit(fl.Param())
	return uploaded(fl, "file_max").Size <= limit
}

// fileType checks that an upload is one of the tag's types, such as
// "image/png image/jpeg", by its first bytes, as http.DetectContentType
// tells them: what its name and the browser say are the user's to say.
func fileType(fl validator.FieldLevel) bool {
	types := fileTypes(fl.Param())
	t, err := filetype.OfUpload(uploaded(fl, "file_type"))
	return err == nil && slices.Contains(types, t)
}

// uploaded is the upload a file tag checks. A field of another type is the
// program's mistake, which a test finds.
func uploaded(fl validator.FieldLevel, tag string) *multipart.FileHeader {
	v := fl.Field()
	if v.Type() != reflect.TypeFor[multipart.FileHeader]() {
		panic(fmt.Sprintf("validate: %s is for an upload, a *multipart.FileHeader, and %s is a %s", tag, fl.FieldName(), v.Type()))
	}
	if v.CanAddr() {
		return v.Addr().Interface().(*multipart.FileHeader)
	}
	fh := v.Interface().(multipart.FileHeader)
	return &fh
}

// sizeLimit reads file_max's size, a number of bytes, KB, MB or GB, each
// 1024 of the one before, such as 2MB or 1.5 MB, and returns it in bytes,
// and as it's written, for a message: its number, and its unit, "" for
// bytes.
func sizeLimit(p string) (int64, string, string) {
	s := strings.TrimSpace(p)
	i := strings.IndexFunc(s, func(r rune) bool { return (r < '0' || r > '9') && r != '.' })
	if i < 0 {
		i = len(s)
	}
	num, unit := s[:i], strings.ToUpper(strings.TrimSpace(s[i:]))
	n, err := strconv.ParseFloat(num, 64)
	scale := map[string]float64{"": 1, "B": 1, "KB": 1 << 10, "MB": 1 << 20, "GB": 1 << 30}[unit]
	if err != nil || n < 0 || scale == 0 {
		panic(fmt.Sprintf("validate: file_max=%s isn't a size, such as 500KB or 2MB", p))
	}
	if scale == 1 {
		unit = ""
	}
	return int64(n * scale), num, unit
}

// fileTypes reads file_type's types. One that sniffing can't tell, such as
// image/svg+xml, would turn away every file: that's the program's mistake.
func fileTypes(p string) []string {
	types := strings.Fields(p)
	if len(types) == 0 {
		panic("validate: file_type names no types, such as file_type=image/png image/jpeg")
	}
	for _, t := range types {
		if !filetype.Known(t) {
			panic(fmt.Sprintf("validate: file_type=%s: %s isn't a type a file's first bytes tell; they tell %s", p, t, strings.Join(slices.Sorted(slices.Values(filetype.All())), ", ")))
		}
	}
	return types
}
