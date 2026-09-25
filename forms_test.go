package tug

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cuonggt/tug/inertia"
	"github.com/cuonggt/tug/session"
	"github.com/cuonggt/tug/validate"
)

type postInput struct {
	Title string `json:"title" validate:"required,max=20"`
	Body  string `json:"body" validate:"required"`
	Stars int    `json:"stars"`
}

// formApp is an Inertia app with sessions, and a form to post to. created
// counts the posts it has made, which a Precognition request mustn't.
func formApp(t *testing.T) (*App, *int) {
	t.Helper()
	pages, err := inertia.New(inertia.Config{Template: `<body>{{ .Inertia }}</body>`, Version: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := session.New(session.Config{Keys: [][]byte{bytes.Repeat([]byte{7}, 32)}})
	if err != nil {
		t.Fatal(err)
	}
	app := New(Config{Inertia: pages, Session: sessions})
	created := 0

	app.Get("/posts/create", func(c *Ctx) error { return c.Inertia("Posts/Create", nil) })
	app.Get("/posts/1", func(c *Ctx) error { return c.Inertia("Posts/Show", nil) })
	app.Post("/posts", func(c *Ctx) error {
		var in postInput
		err := c.BindValid(&in, func(errs validate.Errors) {
			if in.Title == "Taken" {
				errs.Add("title", "title is taken")
			}
		})
		if err != nil {
			return err
		}
		created++
		c.Flash("success", "Post created")
		return c.Redirect("/posts/1")
	})
	app.Post("/posts/preview", func(c *Ctx) error {
		c.Flash("note", "Just a preview")
		return c.Inertia("Posts/Show", nil)
	})
	app.Post("/logout", func(c *Ctx) error {
		c.ClearHistory()
		return c.Redirect("/posts/create")
	})
	return app, &created
}

// visitor sends requests as Inertia's client in a browser does, keeping
// the session cookie between them.
type visitor struct {
	t      *testing.T
	app    *App
	cookie *http.Cookie
}

func (v *visitor) do(method, target, body string, headers ...string) *httptest.ResponseRecorder {
	v.t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("X-Inertia", "true")
	req.Header.Set("X-Inertia-Version", "v1")
	req.Header.Set("Accept", "text/html, application/xhtml+xml")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	if v.cookie != nil {
		req.AddCookie(v.cookie)
	}
	rec := httptest.NewRecorder()
	v.app.ServeHTTP(rec, req)
	for _, c := range rec.Result().Cookies() {
		v.cookie = c
	}
	return rec
}

func (v *visitor) page(rec *httptest.ResponseRecorder) inertia.Page {
	v.t.Helper()
	var p inertia.Page
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		v.t.Fatalf("%d %s: %v", rec.Code, rec.Body, err)
	}
	return p
}

func TestAFormThatDoesntValidateGoesBackWithItsErrors(t *testing.T) {
	app, created := formApp(t)
	v := &visitor{t: t, app: app}

	rec := v.do("POST", "/posts", `{"title":"Far too long for a title","stars":"five"}`, "Referer", "http://example.com/posts/create")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/posts/create" {
		t.Fatalf("got %d to %q: %s", rec.Code, rec.Header().Get("Location"), rec.Body)
	}
	want := map[string]any{
		"title": "title must be at most 20 characters",
		"body":  "body is required",
		"stars": "stars must be a whole number",
	}
	if p := v.page(v.do("GET", "/posts/create", "")); !reflect.DeepEqual(p.Props["errors"], want) {
		t.Errorf("errors %v, want %v", p.Props["errors"], want)
	}
	if p := v.page(v.do("GET", "/posts/create", "")); !reflect.DeepEqual(p.Props["errors"], map[string]any{}) {
		t.Errorf("the errors were still there a request later: %v", p.Props["errors"])
	}
	if *created != 0 {
		t.Errorf("%d posts made from forms that didn't validate", *created)
	}
}

func TestAFormWithAnErrorBagGetsItsErrorsUnderIt(t *testing.T) {
	app, _ := formApp(t)
	v := &visitor{t: t, app: app}
	bag := []string{"Referer", "http://example.com/posts/create", "X-Inertia-Error-Bag", "createPost"}

	v.do("POST", "/posts", `{"title":"Taken","body":"x"}`, bag...)
	// The client sends the header again on the GET after the redirect.
	p := v.page(v.do("GET", "/posts/create", "", bag...))
	if want := map[string]any{"createPost": map[string]any{"title": "title is taken"}}; !reflect.DeepEqual(p.Props["errors"], want) {
		t.Fatalf("errors %v, want %v", p.Props["errors"], want)
	}
}

func TestAnAPIClientGetsA422ThatListsTheErrors(t *testing.T) {
	app, _ := formApp(t)
	rec := serve(app, "POST", "/posts", `{"title":"Taken"}`, "Content-Type", "application/json", "Accept", "application/json")
	want := `{"errors":{"body":"body is required","title":"title is taken"},"message":"body is required"}`
	if rec.Code != 422 || rec.Body.String() != want {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
}

func TestAPrecognitionRequestIsAnsweredWithoutRunningTheRest(t *testing.T) {
	app, created := formApp(t)
	precog := []string{"Content-Type", "application/json", "Accept", "application/json", "Precognition", "true"}

	rec := serve(app, "POST", "/posts", `{"title":"Fine","body":"Fine"}`, precog...)
	if rec.Code != 204 || rec.Header().Get("Precognition-Success") != "true" || rec.Header().Get("Precognition") != "true" {
		t.Errorf("a valid form got %d with %v", rec.Code, rec.Header())
	}
	if !slices.Contains(rec.Header().Values("Vary"), "Precognition") {
		t.Errorf("Vary %q", rec.Header().Values("Vary"))
	}

	// Only the fields it names count: the body is empty, but not touched yet.
	rec = serve(app, "POST", "/posts", `{"title":"Taken"}`, append(precog, "Precognition-Validate-Only", "title")...)
	if rec.Code != 422 || rec.Header().Get("Precognition") != "true" || rec.Body.String() != `{"errors":{"title":"title is taken"},"message":"title is taken"}` {
		t.Errorf("an invalid title got %d %s", rec.Code, rec.Body)
	}
	rec = serve(app, "POST", "/posts", `{"title":"Fine"}`, append(precog, "Precognition-Validate-Only", "title")...)
	if rec.Code != 204 {
		t.Errorf("a valid title, with the body not yet filled in, got %d %s", rec.Code, rec.Body)
	}

	if *created != 0 {
		t.Errorf("Precognition requests made %d posts", *created)
	}
}

func TestFlashDataReachesThePageAfterARedirectOnce(t *testing.T) {
	app, created := formApp(t)
	v := &visitor{t: t, app: app}

	if rec := v.do("POST", "/posts", `{"title":"Fine","body":"Fine"}`); rec.Code != http.StatusSeeOther || *created != 1 {
		t.Fatalf("got %d, and %d posts", rec.Code, *created)
	}
	if p := v.page(v.do("GET", "/posts/1", "")); !reflect.DeepEqual(p.Flash, map[string]any{"success": "Post created"}) {
		t.Errorf("flash after the redirect %v", p.Flash)
	}
	if p := v.page(v.do("GET", "/posts/1", "")); p.Flash != nil {
		t.Errorf("flash a request later %v", p.Flash)
	}
}

func TestFlashDataShownStraightAwayIsntShownAgain(t *testing.T) {
	app, _ := formApp(t)
	v := &visitor{t: t, app: app}

	if p := v.page(v.do("POST", "/posts/preview", `{}`)); !reflect.DeepEqual(p.Flash, map[string]any{"note": "Just a preview"}) {
		t.Errorf("flash %v", p.Flash)
	}
	if p := v.page(v.do("GET", "/posts/1", "")); p.Flash != nil {
		t.Errorf("shown again on the next page: %v", p.Flash)
	}
}

func TestFlashDataWaitsOutAReloadForANewBuild(t *testing.T) {
	app, _ := formApp(t)
	v := &visitor{t: t, app: app}

	v.do("POST", "/posts", `{"title":"Fine","body":"Fine"}`)
	if rec := v.do("GET", "/posts/1", "", "X-Inertia-Version", "v0"); rec.Code != http.StatusConflict {
		t.Fatalf("a visit from another build got %d", rec.Code)
	}
	if p := v.page(v.do("GET", "/posts/1", "")); !reflect.DeepEqual(p.Flash, map[string]any{"success": "Post created"}) {
		t.Errorf("flash after the reload %v", p.Flash)
	}
}

func TestClearHistoryReachesThePageAfterARedirect(t *testing.T) {
	app, _ := formApp(t)
	v := &visitor{t: t, app: app}
	v.do("POST", "/logout", "")
	if p := v.page(v.do("GET", "/posts/create", "")); !p.ClearHistory {
		t.Fatalf("page %+v", p)
	}
}

func TestFlashDataWithoutASessionCantRedirect(t *testing.T) {
	captureLog(t)
	app := New(Config{})
	app.Post("/posts", func(c *Ctx) error {
		c.Flash("success", "Post created")
		return c.Redirect("/posts/1")
	})
	if rec := serve(app, "POST", "/posts", ""); rec.Code != 500 {
		t.Fatalf("got %d, want a 500 rather than a message lost", rec.Code)
	}
}

func TestGoingBackStaysOnThisSite(t *testing.T) {
	app, _ := formApp(t)
	v := &visitor{t: t, app: app}
	rec := v.do("POST", "/posts", `{}`, "Referer", "https://evil.example.net/phish")
	if rec.Header().Get("Location") != "/" {
		t.Fatalf("went back to %q", rec.Header().Get("Location"))
	}
}

func TestValidateChecksValuesThatDontComeFromTheRequest(t *testing.T) {
	app := New(Config{})
	var got error
	app.Get("/", func(c *Ctx) error {
		got = c.Validate(postInput{Title: "Fine"})
		return nil
	})
	serve(app, "GET", "/", "")
	if want := (validate.Errors{"body": "body is required"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestFlashDataGoesOnThroughAChainOfRedirects(t *testing.T) {
	app, _ := formApp(t)
	app.Get("/old", func(c *Ctx) error { return c.Redirect("/posts/1") })
	v := &visitor{t: t, app: app}

	v.do("POST", "/posts", `{"title":"Fine","body":"Fine"}`) // flashes, and redirects to /posts/1
	v.do("GET", "/old", "")                                  // which the app moved on from
	if p := v.page(v.do("GET", "/posts/1", "")); !reflect.DeepEqual(p.Flash, map[string]any{"success": "Post created"}) {
		t.Fatalf("after two redirects, flash %v", p.Flash)
	}
}

func TestPreserveFragmentReachesThePageAfterARedirect(t *testing.T) {
	app, _ := formApp(t)
	app.Post("/posts/1/comments", func(c *Ctx) error {
		c.PreserveFragment()
		return c.Redirect("/posts/1")
	})
	v := &visitor{t: t, app: app}
	v.do("POST", "/posts/1/comments", `{}`)
	if p := v.page(v.do("GET", "/posts/1", "")); !p.PreserveFragment {
		t.Fatalf("page %+v", p)
	}
	if p := v.page(v.do("GET", "/posts/1", "")); p.PreserveFragment {
		t.Error("the page after that kept the fragment too")
	}
}
