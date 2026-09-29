package validate

import (
	"bytes"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http/httptest"
	"net/textproto"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/cuonggt/tug/lang"
)

type Line struct {
	Product  string `json:"product" validate:"required"`
	Quantity int    `json:"quantity" validate:"min=1"`
}

type Order struct {
	Email    string   `json:"email" validate:"required,email"`
	Name     string   `json:"name" validate:"required,max=10"`
	Nickname string   `json:"nickname" validate:"omitempty,min=3"`
	Tags     []string `json:"tags" validate:"max=2"`
	Size     string   `json:"size" validate:"oneof=small medium large"`
	Lines    []Line   `json:"lines" validate:"dive"`
	Password string   `json:"password" validate:"required"`
	Confirm  string   `json:"password_confirmation" validate:"eqfield=Password"`
	Internal string   `validate:"required"`
}

func valid() Order {
	return Order{
		Email: "ann@example.com", Name: "Ann", Size: "small", Password: "secret", Confirm: "secret",
		Lines: []Line{{Product: "tea", Quantity: 1}}, Internal: "x",
	}
}

func TestAValidStructHasNoErrors(t *testing.T) {
	if err := Struct(valid()); err != nil {
		t.Fatalf("got %v", err)
	}
}

func TestEachFieldGetsAMessageUnderItsJSONNameThatNamesItInWords(t *testing.T) {
	o := valid()
	o.Email = "not an email"
	o.Name = "Annabelle Smith"
	o.Nickname = "an"
	o.Tags = []string{"a", "b", "c"}
	o.Size = "huge"
	o.Lines = []Line{{Product: "tea", Quantity: 1}, {Quantity: 0}}
	o.Confirm = "secert"
	o.Internal = ""

	var errs Errors
	if err := Struct(&o); !errors.As(err, &errs) {
		t.Fatalf("got %v, want Errors", err)
	}
	want := Errors{
		"email":                 "email must be a valid email address",
		"name":                  "name must be at most 10 characters",
		"nickname":              "nickname must be at least 3 characters",
		"tags":                  "tags must have at most 2 items",
		"size":                  "size must be one of: small, medium, large",
		"lines.1.product":       "product is required",
		"lines.1.quantity":      "quantity must be at least 1",
		"password_confirmation": "password confirmation must match password",
		"Internal":              "internal is required",
	}
	if !reflect.DeepEqual(errs, want) {
		for field, msg := range errs {
			if want[field] != msg {
				t.Errorf("%s: %q, want %q", field, msg, want[field])
			}
		}
		for field := range want {
			if _, ok := errs[field]; !ok {
				t.Errorf("no error for %s", field)
			}
		}
	}
}

// An anonymous struct, var in struct{...}, has no type name for the
// validator to start the fields' paths with.
func TestAnAnonymousStructsFieldsAreNamedTheSame(t *testing.T) {
	var in struct {
		Password string `json:"password"`
		Confirm  string `json:"password_confirmation" validate:"eqfield=Password"`
		Lines    []Line `json:"lines" validate:"dive"`
	}
	in.Password, in.Confirm = "secret", "secert"
	in.Lines = []Line{{Product: "tea", Quantity: 1}, {Product: "cake"}}

	var errs Errors
	if err := Struct(&in); !errors.As(err, &errs) {
		t.Fatalf("got %v, want Errors", err)
	}
	want := Errors{
		"password_confirmation": "password confirmation must match password",
		"lines.1.quantity":      "quantity must be at least 1",
	}
	if !reflect.DeepEqual(errs, want) {
		t.Errorf("got %v, want %v", errs, want)
	}
}

func TestAddKeepsTheFirstMessageForAField(t *testing.T) {
	errs := Errors{}
	errs.Add("title", "title is required")
	errs.Add("title", "title is taken")
	if errs["title"] != "title is required" {
		t.Fatalf("got %q", errs["title"])
	}
}

func TestOnlyKeepsTheFieldsNamedAndWhatsNestedInThem(t *testing.T) {
	errs := Errors{"title": "a", "author.name": "b", "authority": "c", "body": "d"}
	got := errs.Only("title", "author")
	if want := (Errors{"title": "a", "author.name": "b"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestErrorAndFirstReadInFieldOrder(t *testing.T) {
	errs := Errors{"title": "title is required", "body": "body is required"}
	if errs.Error() != "body is required; title is required" || errs.First() != "body is required" {
		t.Fatalf("Error %q, First %q", errs.Error(), errs.First())
	}
}

func TestAStructThatCantBeCheckedIsAnErrorOfItsOwn(t *testing.T) {
	err := Struct("not a struct")
	var errs Errors
	if err == nil || errors.As(err, &errs) {
		t.Fatalf("got %v, want an error that isn't Errors", err)
	}
}

// upload is a file as a form sends it, named and typed as the browser
// says.
func upload(t *testing.T, name, contentType string, content []byte) *multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, _ := w.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": {`form-data; name="file"; filename="` + name + `"`},
		"Content-Type":        {contentType},
	})
	part.Write(content)
	w.Close()
	r := httptest.NewRequest("POST", "/", &body)
	r.Header.Set("Content-Type", w.FormDataContentType())
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		t.Fatal(err)
	}
	return r.MultipartForm.File["file"][0]
}

var png = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")

type PhotoInput struct {
	Photo   *multipart.FileHeader   `form:"photo" validate:"required,file_max=1KB,file_type=image/png image/jpeg"`
	Scans   []*multipart.FileHeader `form:"scans" validate:"max=2,dive,file_type=application/pdf"`
	Banner  *multipart.FileHeader   `form:"banner" validate:"omitempty,file_max=2MB,file_type=image/png image/webp image/gif"`
	Caption string                  `form:"caption" validate:"max=10"`
}

func TestAnUploadIsCheckedBySizeAndByWhatItIs(t *testing.T) {
	pdf := upload(t, "scan.pdf", "application/pdf", []byte("%PDF-1.7"))
	for _, c := range []struct {
		name string
		in   PhotoInput
		want Errors
	}{
		{"a PNG", PhotoInput{Photo: upload(t, "ann.png", "image/png", png), Scans: []*multipart.FileHeader{pdf}}, nil},
		{"none", PhotoInput{}, Errors{"photo": "photo is required"}},
		{"too big", PhotoInput{Photo: upload(t, "ann.png", "image/png", append(png, make([]byte, 1024)...))}, Errors{"photo": "photo must be at most 1 KB"}},
		{"a page named as a PNG", PhotoInput{Photo: upload(t, "ann.png", "image/png", []byte("<!DOCTYPE html><script>"))}, Errors{"photo": "photo must be a PNG or JPEG image"}},
		{"an SVG", PhotoInput{Photo: upload(t, "ann.svg", "image/svg+xml", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`))}, Errors{"photo": "photo must be a PNG or JPEG image"}},
		{"a list", PhotoInput{Photo: upload(t, "ann.png", "image/png", png), Scans: []*multipart.FileHeader{pdf, upload(t, "b.pdf", "application/pdf", png)}}, Errors{"scans.1": "scans[1] must be a PDF file"}},
		{"too many", PhotoInput{Photo: upload(t, "ann.png", "image/png", png), Scans: []*multipart.FileHeader{pdf, pdf, pdf}}, Errors{"scans": "scans must have at most 2 items"}},
		{"an optional one", PhotoInput{Photo: upload(t, "ann.png", "image/png", png), Banner: upload(t, "b.jpg", "image/jpeg", []byte("\xff\xd8\xff\xe0"))}, Errors{"banner": "banner must be a PNG, WebP or GIF image"}},
	} {
		err := Struct(&c.in)
		var errs Errors
		if c.want == nil && err != nil || c.want != nil && (!errors.As(err, &errs) || !reflect.DeepEqual(errs, c.want)) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
	}
}

func TestAFileTagsSizeIsSaidAsItsWritten(t *testing.T) {
	for p, want := range map[string]struct {
		bytes     int64
		num, unit string
	}{
		"2MB":    {2 << 20, "2", "MB"},
		"500 kb": {500 << 10, "500", "KB"},
		"1.5MB":  {3 << 19, "1.5", "MB"},
		"1GB":    {1 << 30, "1", "GB"},
		"100B":   {100, "100", ""},
		"1":      {1, "1", ""},
	} {
		if n, num, unit := sizeLimit(p); n != want.bytes || num != want.num || unit != want.unit {
			t.Errorf("%s: %d, %q %q", p, n, num, unit)
		}
	}
	big := upload(t, "a.png", "image/png", append(png, make([]byte, 100)...))
	for in, want := range map[any]string{
		&struct {
			F *multipart.FileHeader `form:"f" validate:"file_max=100B"`
		}{big}: "f must be at most 100 bytes",
		&struct {
			F *multipart.FileHeader `form:"f" validate:"file_max=1"`
		}{big}: "f must be at most 1 byte",
	} {
		if err := Struct(in); err == nil || err.Error() != want {
			t.Errorf("got %v, want %q", err, want)
		}
	}
}

func TestAFileTagThatCantWorkPanics(t *testing.T) {
	for _, c := range []struct {
		name string
		in   any
		want string
	}{
		{"a type sniffing can't tell", &struct {
			F *multipart.FileHeader `validate:"file_type=image/svg+xml"`
		}{upload(t, "a.svg", "image/svg+xml", nil)}, "image/svg+xml isn't a type a file's first bytes tell"},
		{"a size that isn't one", &struct {
			F *multipart.FileHeader `validate:"file_max=2 megs"`
		}{upload(t, "a.png", "image/png", png)}, "file_max=2 megs isn't a size"},
		{"a field that isn't an upload", &struct {
			F string `validate:"file_max=2MB"`
		}{"a.png"}, "file_max is for an upload, a *multipart.FileHeader, and F is a string"},
	} {
		func() {
			defer func() {
				if v := recover(); v == nil || !strings.Contains(fmt.Sprint(v), c.want) {
					t.Errorf("%s: panicked with %v", c.name, v)
				}
			}()
			Struct(c.in)
		}()
	}
}

func TestALabelTagNamesTheFieldInItsMessages(t *testing.T) {
	var in struct {
		Email   string `json:"email" label:"email address" validate:"required"`
		Confirm string `json:"email_confirmation" validate:"eqfield=Email"`
		Starts  string `json:"startsAt" validate:"required"`
	}
	in.Confirm = "ann@example.com"
	var errs Errors
	if err := Struct(&in); !errors.As(err, &errs) {
		t.Fatalf("got %v", err)
	}
	want := Errors{
		"email":              "email address is required",
		"email_confirmation": "email confirmation must match email address",
		"startsAt":           "starts at is required",
	}
	if !reflect.DeepEqual(errs, want) {
		t.Errorf("got %v, want %v", errs, want)
	}
}

// vietnamese is a language whose file has some of validate's texts, and
// some fields' names.
func vietnamese(t *testing.T) lang.Words {
	t.Helper()
	c, err := lang.Load(fstest.MapFS{"vi.json": &fstest.MapFile{Data: []byte(`{
		":field is required": "Vui lòng nhập :field",
		":field must be at most :count character|:field must be at most :count characters": ":Field tối đa :count ký tự",
		":field must be a PNG or JPEG image": "",
		"a PNG or JPEG image": "ảnh PNG hoặc JPEG",
		":field must be :value": ":field phải là :value",
		"name": "tên",
		"email address": "địa chỉ email"
	}`)}}, "en")
	if err != nil {
		t.Fatal(err)
	}
	return c.In("vi")
}

func TestMessagesAreSaidInTheLanguageGiven(t *testing.T) {
	var in struct {
		Name  string                `json:"name" validate:"required,max=3"`
		Email string                `json:"email" label:"email address" validate:"required"`
		Nick  string                `json:"nick" validate:"required"`
		Photo *multipart.FileHeader `form:"photo" validate:"omitempty,file_type=image/png image/jpeg"`
	}
	in.Name = "Annabelle"
	in.Photo = upload(t, "a.gif", "image/gif", []byte("GIF89a"))
	var errs Errors
	if err := Struct(&in, vietnamese(t)); !errors.As(err, &errs) {
		t.Fatalf("got %v", err)
	}
	want := Errors{
		"name":  "Tên tối đa 3 ký tự",
		"email": "Vui lòng nhập địa chỉ email",
		"nick":  "Vui lòng nhập nick", // a name the file doesn't have is English's
		"photo": "photo phải là ảnh PNG hoặc JPEG",
	}
	if !reflect.DeepEqual(errs, want) {
		t.Errorf("got %v, want %v", errs, want)
	}
}

type Slug string

func TestARuleOfTheAppsOwnChecksItsTag(t *testing.T) {
	Rule("slug", func(s string, _ string) bool {
		return s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyz0123456789-") == ""
	}, ":field must be letters, digits and dashes")
	Rule("prefix", func(s, prefix string) bool { return strings.HasPrefix(s, prefix) }, ":field must start with :param")

	type In struct {
		Handle Slug   `json:"handle" validate:"slug"`
		Path   string `json:"path" validate:"slug,prefix=blog-"`
	}
	if err := Struct(In{Handle: "ann-2", Path: "blog-first"}); err != nil {
		t.Errorf("a valid one: %v", err)
	}
	var errs Errors
	if err := Struct(In{Handle: "Ann!", Path: "first"}); !errors.As(err, &errs) {
		t.Fatalf("got %v", err)
	}
	want := Errors{"handle": "handle must be letters, digits and dashes", "path": "path must start with blog-"}
	if !reflect.DeepEqual(errs, want) {
		t.Errorf("got %v, want %v", errs, want)
	}
	if texts := Texts(); !slices.Contains(texts, ":field must be letters, digits and dashes") || !slices.Contains(texts, ":field must start with :param") {
		t.Errorf("Texts() hasn't the rules' messages: %q", texts)
	}
}

func TestARuleAddedAgainReplacesTheOneBefore(t *testing.T) {
	type In struct {
		Code string `json:"code" validate:"twice"`
	}
	Rule("twice", func(s string, _ string) bool { return false }, ":field is wrong")
	Rule("twice", func(s string, _ string) bool { return s == "ok" }, ":field isn't ok")
	if err := Struct(In{Code: "ok"}); err != nil {
		t.Errorf("the second rule: %v", err)
	}
	if err := Struct(In{Code: "no"}); err == nil || err.Error() != "code isn't ok" {
		t.Errorf("the second rule's message: %v", err)
	}
}

func TestARuleOnAFieldOfAnotherTypePanics(t *testing.T) {
	Rule("even", func(n int, _ string) bool { return n%2 == 0 }, ":field must be even")
	defer func() {
		if v := recover(); v == nil || !strings.Contains(fmt.Sprint(v), "the rule even checks a int, and Count is a string") {
			t.Errorf("panicked with %v", v)
		}
	}()
	Struct(struct {
		Count string `validate:"even"`
	}{"3"})
}

func TestARuleCantTakeATagTheValidatorKeeps(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("a rule named omitempty was added")
		}
	}()
	Rule("omitempty", func(s string, _ string) bool { return true }, "")
}

func TestTextsListsWhatValidateSays(t *testing.T) {
	texts := Texts()
	for _, want := range []string{
		":field is required",
		":field must be at most :count character|:field must be at most :count characters",
		":field must have at least :count item|:field must have at least :count items",
		":field must be at least :value",
		":field must match :other",
		":count byte|:count bytes",
		":field is invalid",
	} {
		if !slices.Contains(texts, want) {
			t.Errorf("Texts() hasn't %q", want)
		}
	}
	if !slices.IsSorted(texts) || len(slices.Compact(slices.Clone(texts))) != len(texts) {
		t.Error("Texts() isn't sorted, or repeats one")
	}
}
