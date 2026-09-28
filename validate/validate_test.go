package validate

import (
	"bytes"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http/httptest"
	"net/textproto"
	"reflect"
	"strings"
	"testing"
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

func TestEachFieldGetsAMessageUnderItsJSONName(t *testing.T) {
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
		"password_confirmation": "password_confirmation must match password",
		"Internal":              "Internal is required",
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
		"password_confirmation": "password_confirmation must match password",
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
		bytes int64
		words string
	}{
		"2MB":    {2 << 20, "2 MB"},
		"500 kb": {500 << 10, "500 KB"},
		"1.5MB":  {3 << 19, "1.5 MB"},
		"1GB":    {1 << 30, "1 GB"},
		"100B":   {100, "100 bytes"},
		"1":      {1, "1 byte"},
	} {
		if n, words := sizeLimit(p); n != want.bytes || words != want.words {
			t.Errorf("%s: %d, %q", p, n, words)
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
