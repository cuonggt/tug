package tugtest

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/textproto"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
)

// File is a file a visit uploads, as a value in its body. A body that's a
// map with a File in it goes as multipart/form-data, as Inertia's client
// sends a form with a file in it, and its other values go as that form's
// fields, written as the client writes them: true and false as 1 and 0, a
// list's items each as tags[], and a nested object's values as
// user[name], which Bind doesn't read.
//
//	r := c.Post("/settings/profile/photo", map[string]any{
//		"photo": tugtest.File{Name: "ann.png", Content: png},
//	})
type File struct {
	// Name is the file's name, as the browser sends it: "ann.png".
	Name string

	// Type is the type the browser says the file is, which an app that
	// checks what a file is reads from its bytes instead. Default
	// application/octet-stream.
	Type string

	// Content is the file.
	Content []byte
}

var (
	fileType = reflect.TypeFor[File]()
	timeType = reflect.TypeFor[time.Time]()
)

// multipartBody writes body as a multipart form, when it's a map with a
// File in it.
func multipartBody(body any) (data []byte, contentType string, ok bool, err error) {
	v := reflect.ValueOf(body)
	if v.Kind() != reflect.Map || v.Type().Key().Kind() != reflect.String || !hasFile(v) {
		return nil, "", false, nil
	}
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	for _, k := range sortedKeys(v) {
		if err := writeForm(w, k.String(), v.MapIndex(k)); err != nil {
			return nil, "", false, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", false, err
	}
	return b.Bytes(), w.FormDataContentType(), true, nil
}

// hasFile reports whether v is a File, or has one in it, in a map or a
// list.
func hasFile(v reflect.Value) bool {
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return false
		}
		v = v.Elem()
	}
	switch {
	case !v.IsValid():
		return false
	case v.Type() == fileType:
		return true
	case v.Kind() == reflect.Map:
		for it := v.MapRange(); it.Next(); {
			if hasFile(it.Value()) {
				return true
			}
		}
	case v.Kind() == reflect.Slice || v.Kind() == reflect.Array:
		for i := range v.Len() {
			if hasFile(v.Index(i)) {
				return true
			}
		}
	}
	return false
}

// writeForm writes v under key as Inertia's client writes a value into a
// form's data, in its objectToFormData.
func writeForm(w *multipart.Writer, key string, v reflect.Value) error {
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return w.WriteField(key, "")
		}
		v = v.Elem()
	}
	if !v.IsValid() {
		return w.WriteField(key, "")
	}
	switch v.Type() {
	case fileType:
		f := v.Interface().(File)
		quote := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace
		part, err := w.CreatePart(textproto.MIMEHeader{
			"Content-Disposition": {fmt.Sprintf(`form-data; name="%s"; filename="%s"`, quote(key), quote(f.Name))},
			"Content-Type":        {cmp.Or(f.Type, "application/octet-stream")},
		})
		if err != nil {
			return err
		}
		_, err = part.Write(f.Content)
		return err
	case timeType:
		return w.WriteField(key, v.Interface().(time.Time).UTC().Format("2006-01-02T15:04:05.000Z07:00"))
	}
	switch v.Kind() {
	case reflect.Bool:
		if v.Bool() {
			return w.WriteField(key, "1")
		}
		return w.WriteField(key, "0")
	case reflect.String:
		return w.WriteField(key, v.String())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return w.WriteField(key, strconv.FormatInt(v.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return w.WriteField(key, strconv.FormatUint(v.Uint(), 10))
	case reflect.Float32, reflect.Float64:
		return w.WriteField(key, strconv.FormatFloat(v.Float(), 'f', -1, 64))
	case reflect.Slice, reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return w.WriteField(key, string(v.Bytes()))
		}
		for i := range v.Len() {
			if err := writeForm(w, key+"[]", v.Index(i)); err != nil {
				return err
			}
		}
		return nil
	case reflect.Map:
		if v.Type().Key().Kind() == reflect.String {
			for _, k := range sortedKeys(v) {
				if err := writeForm(w, key+"["+k.String()+"]", v.MapIndex(k)); err != nil {
					return err
				}
			}
			return nil
		}
	}
	// A struct, say: as encoding/json writes it.
	data, err := json.Marshal(v.Interface())
	if err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	return writeForm(w, key, reflect.ValueOf(decoded))
}

// sortedKeys are a map's keys, in order, so that a form's fields are.
func sortedKeys(m reflect.Value) []reflect.Value {
	keys := m.MapKeys()
	slices.SortFunc(keys, func(a, b reflect.Value) int { return strings.Compare(a.String(), b.String()) })
	return keys
}
