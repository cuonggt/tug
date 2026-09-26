package tug

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"mime/multipart"
	"net/http/httptest"
	"net/netip"
	"reflect"
	"strconv"
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

func TestAFormSentAsJSONBindsAsTheFormWould(t *testing.T) {
	// As Inertia's <Form> sends one: its FormData as an object, with every
	// value a string, and nothing for a checkbox that isn't ticked.
	type input struct {
		Name     string     `json:"name"`
		Remember bool       `json:"remember"`
		Admin    bool       `json:"admin"`
		Age      int        `json:"age"`
		Height   *float64   `json:"height"`
		Score    float32    `json:"score"`
		Big      int64      `json:"big"`
		IDs      []uint     `json:"ids"`
		Due      time.Time  `json:"due"`
		At       *time.Time `json:"at"`
		Left     time.Time  `json:"left"`
	}
	got, err := bind[input](t, "/", "POST", "/", "application/json", `{
		"name": "Ann", "remember": "on", "age": "42", "height": "", "score": "9.5",
		"big": "9007199254740993", "ids": ["1", "", "3"],
		"due": "2026-09-25", "at": "2026-09-25T14:30", "left": ""
	}`)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 25, 14, 30, 0, 0, time.UTC)
	want := input{
		Name: "Ann", Remember: true, Age: 42, Score: 9.5, Big: 9007199254740993, IDs: []uint{1, 3},
		Due: time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC), At: &at,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bound %+v, want %+v", got, want)
	}
}

func TestJSONBoolsAndNumbersStillBindAsThemselves(t *testing.T) {
	type input struct {
		Remember bool `json:"remember"`
		Age      int  `json:"age"`
	}
	got, err := bind[input](t, "/", "POST", "/", "application/json", `{"remember":true,"age":"42"}`)
	if err != nil || got != (input{Remember: true, Age: 42}) {
		t.Fatalf("bound %+v, %v", got, err)
	}
}

func TestAJSONStringThatDoesNotParseIsStillA400AndTheRestBind(t *testing.T) {
	type input struct {
		Age      int    `json:"age"`
		Remember bool   `json:"remember"`
		Level    int8   `json:"level"`
		Count    uint   `json:"count"`
		Name     string `json:"name"`
	}
	got, err := bind[input](t, "/", "POST", "/", "application/json",
		`{"age":"old","remember":"on","level":"300","count":"-1","name":"Ann"}`)
	if he := httpError(t, err); he.Code != 400 || he.Message != "age must be a whole number" {
		t.Fatalf("got %d %q, want the first field that's wrong", he.Code, he.Message)
	}
	if !got.Remember || got.Name != "Ann" {
		t.Errorf("bound %+v: the fields after the bad one should be too", got)
	}
	for body, want := range map[string]string{
		`{"level":"300"}`:      "level must be a whole number",
		`{"count":"-1"}`:       "count must be a whole number, 0 or more",
		`{"remember":"maybe"}`: "remember must be true or false",
	} {
		_, err := bind[input](t, "/", "POST", "/", "application/json", body)
		if he := httpError(t, err); he.Message != want {
			t.Errorf("%s: got %q, want %q", body, he.Message, want)
		}
	}
}

func TestJSONStringsBindInNestedStructsListsAndMaps(t *testing.T) {
	type line struct {
		Qty int `json:"qty"`
	}
	type input struct {
		Author struct {
			Age      int  `json:"age"`
			Verified bool `json:"verified"`
		} `json:"author"`
		Lines  []line         `json:"lines"`
		Counts map[string]int `json:"counts"`
		Tags   []string       `json:"tags"`
		Extra  any            `json:"extra"`
	}
	got, err := bind[input](t, "/", "POST", "/", "application/json",
		`{"author":{"age":"30","verified":"yes"},"lines":[{"qty":"2"},{"qty":"3"}],"counts":{"a":"1"},"tags":["1"],"extra":"5"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got.Author.Age != 30 || !got.Author.Verified || !reflect.DeepEqual(got.Lines, []line{{2}, {3}}) ||
		got.Counts["a"] != 1 || !reflect.DeepEqual(got.Tags, []string{"1"}) || got.Extra != "5" {
		t.Fatalf("bound %+v", got)
	}
}

// cents reads an amount in dollars, "12.34", as cents: a type that reads
// its own text.
type cents int

func (c *cents) UnmarshalText(b []byte) error {
	d, err := strconv.ParseFloat(string(b), 64)
	*c = cents(math.Round(d * 100))
	return err
}

func TestFieldsThatReadTheirOwnJSONStringsAreLeftToThem(t *testing.T) {
	type input struct {
		ID       int64      `json:"id,string"`
		Price    cents      `json:"price"`
		IP       netip.Addr `json:"ip"`
		At       time.Time  `json:"at"`
		Remember bool       `json:"remember"` // "on", which has Bind read the body again
	}
	const at = `"2026-09-25T10:00:00+00:00"` // RFC 3339, which time.Time reads itself
	got, err := bind[input](t, "/", "POST", "/", "application/json",
		`{"id":"42","price":"5","ip":"127.0.0.1","at":`+at+`,"remember":"on"}`)
	if err != nil {
		t.Fatal(err)
	}
	want := input{ID: 42, Price: 500, IP: netip.MustParseAddr("127.0.0.1"), Remember: true}
	if err := json.Unmarshal([]byte(at), &want.At); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bound %+v, want %+v", got, want)
	}
}

func TestAnEmptyJSONStringLeavesTheFieldAsTheHandlerHadIt(t *testing.T) {
	var in struct {
		Page     int   `json:"page"`
		Draft    *bool `json:"draft"`
		Remember bool  `json:"remember"`
	}
	var err error
	app := New(Config{})
	app.Post("/", func(c *Ctx) error {
		in.Page = 1
		err = c.Bind(&in)
		return nil
	})
	serve(app, "POST", "/", `{"page":"","draft":"","remember":"on"}`, "Content-Type", "application/json")
	if err != nil || in.Page != 1 || in.Draft != nil || !in.Remember {
		t.Fatalf("bound page %d, draft %v, remember %v, %v", in.Page, in.Draft, in.Remember, err)
	}
}

func TestAJSONKeyFindsTheFieldEncodingJSONFillsWithIt(t *testing.T) {
	type Paging struct {
		Page    int `json:"page"`
		PerPage int `json:"per_page"`
	}
	type input struct {
		Paging
		Page     string `json:"page"` // hides the embedded one
		Remember bool   `json:"remember"`
	}
	got, err := bind[input](t, "/", "POST", "/", "application/json",
		`{"page":"2","PER_PAGE":"20","remember":"on"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got.Page != "2" || got.Paging.Page != 0 || got.PerPage != 20 || !got.Remember {
		t.Fatalf("bound %+v", got)
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
		Draft bool   `json:"draft"`
	}
	var err1, err2 error
	app := New(Config{})
	app.Put("/posts/{id}", func(c *Ctx) error {
		err1 = c.Bind(&id)
		err2 = c.Bind(&in)
		return nil
	})
	serve(app, "PUT", "/posts/7", `{"title":"Hi","draft":"on"}`, "Content-Type", "application/json")
	if err1 != nil || err2 != nil || id.ID != 7 || in.Title != "Hi" || !in.Draft {
		t.Fatalf("bound %d and %+v, errors %v %v", id.ID, in, err1, err2)
	}
}
