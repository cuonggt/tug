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
// Fields are named as their json tags name them, the way the client sent
// them, and a nested field by its path: "author.name", "items.0.price".
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
// It returns Errors when something is wrong, and nil when nothing is.
func Struct(v any) error {
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
		errs.Add(path(fe.Namespace(), root), message(fe, root))
	}
	return errs
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

// message says what's wrong with a field, as "title must be at most 80
// characters".
func message(fe validator.FieldError, root reflect.Type) string {
	f, p := fe.Field(), fe.Param()
	switch fe.Tag() {
	case "required", "required_if", "required_unless", "required_with", "required_with_all",
		"required_without", "required_without_all":
		return f + " is required"
	case "email":
		return f + " must be a valid email address"
	case "url", "http_url", "uri":
		return f + " must be a valid URL"
	case "uuid", "uuid4", "uuid7":
		return f + " must be a valid UUID"
	case "min", "gte":
		return f + " must " + bound(fe.Kind(), "at least", p)
	case "max", "lte":
		return f + " must " + bound(fe.Kind(), "at most", p)
	case "gt":
		return f + " must " + bound(fe.Kind(), "more than", p)
	case "lt":
		return f + " must " + bound(fe.Kind(), "less than", p)
	case "len":
		return f + " must " + bound(fe.Kind(), "exactly", p)
	case "eq":
		return f + " must be " + p
	case "ne":
		return f + " must not be " + p
	case "oneof":
		return f + " must be one of: " + strings.Join(strings.Fields(p), ", ")
	case "alpha":
		return f + " must contain only letters"
	case "alphanum":
		return f + " must contain only letters and numbers"
	case "numeric", "number":
		return f + " must be a number"
	case "boolean":
		return f + " must be true or false"
	case "datetime":
		return f + " must be a valid date"
	case "eqfield":
		return f + " must match " + sibling(root, fe.StructNamespace(), p)
	case "nefield":
		return f + " must be different from " + sibling(root, fe.StructNamespace(), p)
	case "contains":
		return f + " must contain " + strconv.Quote(p)
	case "excludes":
		return f + " must not contain " + strconv.Quote(p)
	case "startswith":
		return f + " must start with " + strconv.Quote(p)
	case "endswith":
		return f + " must end with " + strconv.Quote(p)
	case "unique":
		return f + " must not repeat a value"
	case "file_max":
		_, words := sizeLimit(p)
		return f + " must be at most " + words
	case "file_type":
		return f + " must be " + filetype.Names(fileTypes(p))
	}
	return f + " is invalid"
}

// bound says what a limit means for a value of kind k: a length for text, a
// count for a list, and the value itself for a number.
func bound(k reflect.Kind, limit, n string) string {
	switch k {
	case reflect.String:
		return "be " + limit + " " + n + " " + plural(n, "character", "characters")
	case reflect.Slice, reflect.Array, reflect.Map:
		return "have " + limit + " " + n + " " + plural(n, "item", "items")
	}
	return "be " + limit + " " + n
}

func plural(n, one, many string) string {
	if n == "1" {
		return one
	}
	return many
}

// sibling names the field that eqfield and nefield compare with, which the
// validator gives by its Go name, as the client knows it.
func sibling(root reflect.Type, structNamespace, goName string) string {
	t := root
	parts := strings.Split(inRoot(structNamespace, root), ".")
	for _, part := range parts[:len(parts)-1] {
		part, _, _ = strings.Cut(part, "[")
		f, ok := t.FieldByName(part)
		if !ok {
			return strings.ToLower(goName)
		}
		t = f.Type
		for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array || t.Kind() == reflect.Map {
			t = t.Elem()
		}
	}
	if f, ok := t.FieldByName(goName); ok {
		if name := jsonName(f); name != "" {
			return name
		}
	}
	return strings.ToLower(goName)
}

// fileMax checks that an upload is no bigger than the tag's size, such as
// 2MB.
func fileMax(fl validator.FieldLevel) bool {
	limit, _ := sizeLimit(fl.Param())
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
// and in words for a message: "2 MB".
func sizeLimit(p string) (int64, string) {
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
	words := num + " " + unit
	if scale == 1 {
		words = num + " " + plural(num, "byte", "bytes")
	}
	return int64(n * scale), words
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
