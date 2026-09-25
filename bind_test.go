package tug

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http/httptest"
	"net/netip"
	"reflect"
	"strings"
	"testing"
	"time"
)

const formType = "application/x-www-form-urlencoded"

// bind runs Bind for one request to a route, and returns what it filled.
func bind[T any](t *testing.T, route, method, target, contentType, body string) (T, error) {
	t.Helper()
	var dst T
	var err error
	app := New(Config{})
	app.Handle(method, route, func(c *Ctx) error {
		err = c.Bind(&dst)
		return nil
	})
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	app.ServeHTTP(httptest.NewRecorder(), req)
	return dst, err
}

// httpError returns the *HTTPError in err, failing the test without one.
func httpError(t *testing.T, err error) *HTTPError {
	t.Helper()
	var he *HTTPError
	if !errors.As(err, &he) {
		t.Fatalf("err = %v, want an *HTTPError", err)
	}
	return he
}

func TestBindTakesThePathTheQueryAndAJSONBodyTogether(t *testing.T) {
	type input struct {
		ID    int64    `path:"id"`
		Draft bool     `query:"draft"`
		Title string   `json:"title"`
		Tags  []string `json:"tags"`
	}
	got, err := bind[input](t, "/posts/{id}", "PUT", "/posts/7?draft=1", "application/json",
		`{"title":"Hello","tags":["a","b"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if want := (input{ID: 7, Draft: true, Title: "Hello", Tags: []string{"a", "b"}}); !reflect.DeepEqual(got, want) {
		t.Fatalf("bound %+v, want %+v", got, want)
	}
}

func TestThePathValueWinsOverTheBody(t *testing.T) {
	type input struct {
		ID int64 `path:"id" json:"id"`
	}
	got, err := bind[input](t, "/posts/{id}", "PUT", "/posts/7", "application/json", `{"id":99}`)
	if err != nil || got.ID != 7 {
		t.Fatalf("ID = %d, %v; want the 7 from the URL", got.ID, err)
	}
}

func TestAFormFillsFieldsByFormTagThenJSONNameThenFieldName(t *testing.T) {
	type input struct {
		Title string `form:"post_title" json:"title"`
		Body  string `json:"body"`
		Slug  string
		Skip  string `form:"-"`
	}
	got, err := bind[input](t, "/posts", "POST", "/posts", formType,
		"post_title=Hi&title=wrong&body=Text&Slug=hi&Skip=no")
	if err != nil {
		t.Fatal(err)
	}
	if want := (input{Title: "Hi", Body: "Text", Slug: "hi"}); got != want {
		t.Fatalf("bound %+v, want %+v", got, want)
	}
}

func TestAFormListIsARepeatedKeyOrOneEndingInBrackets(t *testing.T) {
	type input struct {
		Tags []string `form:"tags"`
		IDs  []int    `form:"ids"`
	}
	got, err := bind[input](t, "/", "POST", "/", formType, "tags=a&tags=b&ids[]=1&ids[]=2")
	if err != nil {
		t.Fatal(err)
	}
	if want := (input{Tags: []string{"a", "b"}, IDs: []int{1, 2}}); !reflect.DeepEqual(got, want) {
		t.Fatalf("bound %+v, want %+v", got, want)
	}
}

func TestAnEmptyInputLeavesItsFieldAlone(t *testing.T) {
	type input struct {
		Age   int  `form:"age"`
		Score *int `form:"score"`
		Admin bool `form:"admin"`
	}
	got, err := bind[input](t, "/", "POST", "/", formType, "age=&score=&admin=")
	if err != nil || got != (input{}) {
		t.Fatalf("bound %+v, %v; want nothing set and no error", got, err)
	}
}

func TestCheckboxesAndHTMLDateInputsBind(t *testing.T) {
	type input struct {
		Agree bool      `form:"agree"`
		Due   time.Time `form:"due"`
		At    time.Time `form:"at"`
	}
	got, err := bind[input](t, "/", "POST", "/", formType, "agree=on&due=2026-09-25&at=2026-09-25T14:30")
	if err != nil {
		t.Fatal(err)
	}
	want := input{
		Agree: true,
		Due:   time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC),
		At:    time.Date(2026, 9, 25, 14, 30, 0, 0, time.UTC),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bound %+v, want %+v", got, want)
	}
}

func TestAFieldThatParsesItselfFromTextBinds(t *testing.T) {
	type input struct {
		IP netip.Addr `query:"ip"`
	}
	got, err := bind[input](t, "/", "GET", "/?ip=127.0.0.1", "", "")
	if err != nil || got.IP != netip.MustParseAddr("127.0.0.1") {
		t.Fatalf("IP = %v, %v", got.IP, err)
	}
}

func TestEmbeddedStructsBindTheirFields(t *testing.T) {
	type Paging struct {
		Page int `query:"page"`
	}
	type input struct {
		Paging
		Q string `query:"q"`
	}
	got, err := bind[input](t, "/", "GET", "/?page=2&q=go", "", "")
	if err != nil || got.Page != 2 || got.Q != "go" {
		t.Fatalf("bound %+v, %v", got, err)
	}
}

func TestAMultipartFormBindsItsFieldsAndFiles(t *testing.T) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("title", "Hi")
	fw, _ := mw.CreateFormFile("photo", "cat.jpg")
	fw.Write([]byte("meow"))
	mw.Close()

	type input struct {
		Title string                `form:"title"`
		Photo *multipart.FileHeader `form:"photo"`
	}
	got, err := bind[input](t, "/", "POST", "/", mw.FormDataContentType(), body.String())
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Hi" || got.Photo == nil || got.Photo.Filename != "cat.jpg" || got.Photo.Size != 4 {
		t.Fatalf("bound title %q and photo %+v", got.Title, got.Photo)
	}
}

func TestAValueThatDoesNotParseIsA400ThatNamesTheField(t *testing.T) {
	type input struct {
		Age int `form:"age"`
	}
	_, err := bind[input](t, "/", "POST", "/", formType, "age=abc")
	if he := httpError(t, err); he.Code != 400 || he.Message != "age must be a whole number" {
		t.Fatalf("got %d %q", he.Code, he.Message)
	}
	var be *BindError
	if !errors.As(err, &be) || be.Field != "age" {
		t.Errorf("err = %v, want a *BindError for age", err)
	}
}

func TestAJSONValueOfTheWrongTypeIsA400ThatNamesTheField(t *testing.T) {
	type input struct {
		Author struct {
			Age int `json:"age"`
		} `json:"author"`
	}
	_, err := bind[input](t, "/", "POST", "/", "application/json", `{"author":{"age":"old"}}`)
	if he := httpError(t, err); he.Code != 400 || he.Message != "author.age must be a whole number" {
		t.Fatalf("got %d %q", he.Code, he.Message)
	}
}

func TestAPathValueThatDoesNotParseIsA404(t *testing.T) {
	type input struct {
		ID int64 `path:"id"`
	}
	_, err := bind[input](t, "/posts/{id}", "GET", "/posts/abc", "", "")
	if he := httpError(t, err); he.Code != 404 {
		t.Fatalf("got %d, want 404: /posts/abc is a post that isn't there", he.Code)
	}
}

func TestMalformedJSONIsA400(t *testing.T) {
	type input struct {
		Title string `json:"title"`
	}
	_, err := bind[input](t, "/", "POST", "/", "application/json", `{"title":`)
	if he := httpError(t, err); he.Code != 400 || he.Message != "invalid JSON" {
		t.Fatalf("got %d %q", he.Code, he.Message)
	}
}

func TestABodyOverTheLimitIsA413(t *testing.T) {
	for contentType, body := range map[string]string{
		"application/json": `{"title":"far too long for the limit"}`,
		formType:           "title=far+too+long+for+the+limit",
	} {
		var err error
		app := New(Config{BodyLimit: 16})
		app.Post("/", func(c *Ctx) error {
			var in struct {
				Title string `json:"title"`
			}
			err = c.Bind(&in)
			return nil
		})
		serve(app, "POST", "/", body, "Content-Type", contentType)
		if he := httpError(t, err); he.Code != 413 {
			t.Errorf("%s: got %d, want 413", contentType, he.Code)
		}
	}
}

func TestABodyBindCannotReadIsA415(t *testing.T) {
	for _, contentType := range []string{"text/csv", ""} {
		_, err := bind[struct{}](t, "/", "POST", "/", contentType, "a,b")
		if he := httpError(t, err); he.Code != 415 {
			t.Errorf("Content-Type %q: got %d, want 415", contentType, he.Code)
		}
	}
}

func TestBindNeedsAPointerToAStruct(t *testing.T) {
	var err error
	app := New(Config{})
	app.Get("/", func(c *Ctx) error {
		err = c.Bind(struct{}{})
		return nil
	})
	serve(app, "GET", "/", "")
	var he *HTTPError
	if err == nil || errors.As(err, &he) {
		t.Fatalf("err = %v, want a plain error: it's the program's mistake", err)
	}
}

func TestAFieldBindCannotFillIsTheProgramsMistakeNotTheClients(t *testing.T) {
	type input struct {
		Meta map[string]string `form:"meta"`
	}
	_, err := bind[input](t, "/", "POST", "/", formType, "meta=x")
	var he *HTTPError
	if err == nil || errors.As(err, &he) {
		t.Fatalf("err = %v, want a plain error, which is a 500", err)
	}
}

func TestABindErrorIsAnsweredWithWhatToFix(t *testing.T) {
	app := New(Config{})
	app.Post("/", func(c *Ctx) error {
		var in struct {
			Age int `json:"age"`
		}
		return c.Bind(&in)
	})
	rec := serve(app, "POST", "/", `{"age":"x"}`, "Content-Type", "application/json", "Accept", "application/json")
	if rec.Code != 400 || rec.Body.String() != `{"message":"age must be a whole number"}` {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
}

func TestAJSONBodyCanBeBoundTwice(t *testing.T) {
	var id struct {
		ID int64 `path:"id"`
	}
	var in struct {
		Title string `json:"title"`
	}
	var err1, err2 error
	app := New(Config{})
	app.Put("/posts/{id}", func(c *Ctx) error {
		err1 = c.Bind(&id)
		err2 = c.Bind(&in)
		return nil
	})
	serve(app, "PUT", "/posts/7", `{"title":"Hi"}`, "Content-Type", "application/json")
	if err1 != nil || err2 != nil || id.ID != 7 || in.Title != "Hi" {
		t.Fatalf("bound %d and %q, errors %v %v", id.ID, in.Title, err1, err2)
	}
}
