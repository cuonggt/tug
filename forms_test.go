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

func TestFlashDataGoesOnThroughALocation(t *testing.T) {
	// A Location answers Inertia's client with a 409, which it follows by
	// loading the page whole, and a browser's own request with a redirect.
	for _, client := range []struct {
		name, inertia string
		code          int
	}{
		{"Inertia's client", "true", http.StatusConflict},
		{"a browser", "", http.StatusFound},
	} {
		app, _ := formApp(t)
		app.Get("/old", func(c *Ctx) error { return c.Location("/posts/1") })
		v := &visitor{t: t, app: app}

		v.do("POST", "/posts", `{"title":"Fine","body":"Fine"}`) // flashes, and redirects to /posts/1
		// Which the app has moved on from, to a page loaded whole.
		if rec := v.do("GET", "/old", "", "X-Inertia", client.inertia); rec.Code != client.code {
			t.Fatalf("%s got %d from a Location, want %d", client.name, rec.Code, client.code)
		}
		if p := v.page(v.do("GET", "/posts/1", "")); !reflect.DeepEqual(p.Flash, map[string]any{"success": "Post created"}) {
			t.Errorf("%s: after a redirect and a Location, flash %v", client.name, p.Flash)
		}
		if p := v.page(v.do("GET", "/posts/1", "")); p.Flash != nil {
			t.Errorf("%s: flash a request later %v", client.name, p.Flash)
		}
	}
}

func TestALocationsOwnFlashGoesWithWhatItCarries(t *testing.T) {
	app, _ := formApp(t)
	app.Get("/old", func(c *Ctx) error {
		c.Flash("note", "This post has moved")
		return c.Location("/posts/1")
	})
	app.Get("/older", func(c *Ctx) error {
		c.Flash("success", "Post moved") // the later of a key wins
		return c.Location("/posts/1")
	})
	v := &visitor{t: t, app: app}

	v.do("POST", "/posts", `{"title":"Fine","body":"Fine"}`)
	v.do("GET", "/old", "")
	want := map[string]any{"success": "Post created", "note": "This post has moved"}
	if p := v.page(v.do("GET", "/posts/1", "")); !reflect.DeepEqual(p.Flash, want) {
		t.Errorf("flash %v, want %v", p.Flash, want)
	}

	v.do("POST", "/posts", `{"title":"Fine","body":"Fine"}`)
	v.do("GET", "/older", "")
	if p := v.page(v.do("GET", "/posts/1", "")); !reflect.DeepEqual(p.Flash, map[string]any{"success": "Post moved"}) {
		t.Errorf("with a flash of its own under the key, flash %v", p.Flash)
	}
}

func TestClearHistoryGoesOnThroughALocation(t *testing.T) {
	app, _ := formApp(t)
	app.Get("/left", func(c *Ctx) error { return c.Location("/posts/create") })
	v := &visitor{t: t, app: app}
	v.do("POST", "/logout", "") // clears the history, and redirects
	v.do("GET", "/left", "")
	if p := v.page(v.do("GET", "/posts/create", "")); !p.ClearHistory {
		t.Fatalf("page %+v", p)
	}
}

func TestALocationWithoutASessionLeavesAsItDid(t *testing.T) {
	app := inertiaApp(t)
	app.Post("/pay", func(c *Ctx) error {
		c.Flash("note", "Paying") // with no session to carry it: a redirect's 500, never a Location's
		return c.Location("https://pay.example.com")
	})
	rec := serve(app, "POST", "/pay", "", "X-Inertia", "true")
	if rec.Code != 409 || rec.Header().Get("X-Inertia-Location") != "https://pay.example.com" {
		t.Fatalf("got %d with %v", rec.Code, rec.Header())
	}
}

// forgetApp is formApp with a form that flashes two keys, and a page, a
// redirect and a Location that each forget the first.
func forgetApp(t *testing.T) *App {
	t.Helper()
	app, _ := formApp(t)
	app.Post("/both", func(c *Ctx) error {
		c.Flash("success", "Post created")
		c.Flash("note", "Its first")
		return c.Redirect("/posts/1")
	})
	app.Get("/quiet", func(c *Ctx) error {
		c.ForgetFlash("success")
		return c.Inertia("Posts/Show", nil)
	})
	app.Get("/quiet/redirect", func(c *Ctx) error {
		c.ForgetFlash("success")
		return c.Redirect("/posts/1")
	})
	app.Get("/quiet/location", func(c *Ctx) error {
		c.ForgetFlash("success")
		return c.Location("/posts/1")
	})
	return app
}

func TestAKeyTheRequestBeforeFlashedCanBeForgotten(t *testing.T) {
	v := &visitor{t: t, app: forgetApp(t)}
	kept := map[string]any{"note": "Its first"}

	v.do("POST", "/both", "{}")
	if p := v.page(v.do("GET", "/quiet", "")); !reflect.DeepEqual(p.Flash, kept) {
		t.Errorf("the page that forgot a key shows %v, want %v", p.Flash, kept)
	}
	if p := v.page(v.do("GET", "/posts/1", "")); p.Flash != nil {
		t.Errorf("a request later, flash %v", p.Flash)
	}

	// Nor is it shown by the page after a redirect, or a Location, that
	// forgot it, which the other key reaches as it would have.
	for _, path := range []string{"/quiet/redirect", "/quiet/location"} {
		v.do("POST", "/both", "{}")
		v.do("GET", path, "")
		if p := v.page(v.do("GET", "/posts/1", "")); !reflect.DeepEqual(p.Flash, kept) {
			t.Errorf("the page after %s shows %v, want %v", path, p.Flash, kept)
		}
		if p := v.page(v.do("GET", "/posts/1", "")); p.Flash != nil {
			t.Errorf("a request after %s's page, flash %v", path, p.Flash)
		}
	}
}

func TestAKeyTheHandlerFlashedItselfCanBeForgotten(t *testing.T) {
	app, _ := formApp(t)
	app.Post("/one", func(c *Ctx) error {
		c.Flash("success", "Saved")
		c.Flash("note", "Kept")
		c.ForgetFlash("success")
		return c.Redirect("/posts/1")
	})
	app.Post("/none", func(c *Ctx) error {
		c.Flash("success", "Saved")
		c.ForgetFlash("success")
		return c.Redirect("/posts/1")
	})
	app.Post("/shown", func(c *Ctx) error {
		c.Flash("success", "Saved")
		c.Flash("note", "Kept")
		c.ForgetFlash("success")
		return c.Inertia("Posts/Show", nil)
	})
	v := &visitor{t: t, app: app}

	v.do("POST", "/one", "{}")
	if p := v.page(v.do("GET", "/posts/1", "")); !reflect.DeepEqual(p.Flash, map[string]any{"note": "Kept"}) {
		t.Errorf("after the redirect, flash %v", p.Flash)
	}
	if p := v.page(v.do("POST", "/shown", "{}")); !reflect.DeepEqual(p.Flash, map[string]any{"note": "Kept"}) {
		t.Errorf("on the page it renders itself, flash %v", p.Flash)
	}

	// With nothing left to flash, nothing is: no session is begun to carry
	// an empty flash, which the next request would write a cookie to end.
	fresh := &visitor{t: t, app: app}
	if rec := fresh.do("POST", "/none", "{}"); rec.Code != http.StatusSeeOther || len(rec.Result().Cookies()) != 0 {
		t.Errorf("flashing a key and forgetting it got %d, with the cookies %v", rec.Code, rec.Result().Cookies())
	}
	if p := fresh.page(fresh.do("GET", "/posts/1", "")); p.Flash != nil {
		t.Errorf("the page after shows %v", p.Flash)
	}
}

func TestAKeyFlashedAgainAfterItsForgottenIsShown(t *testing.T) {
	app := forgetApp(t)
	app.Post("/again", func(c *Ctx) error {
		c.Flash("success", "Saved")
		c.ForgetFlash("success")
		c.Flash("success", "Saved, after all")
		return c.Redirect("/posts/1")
	})
	app.Get("/renamed", func(c *Ctx) error {
		c.ForgetFlash("success") // the request before's
		c.Flash("success", "Post made")
		return c.Inertia("Posts/Show", nil)
	})
	v := &visitor{t: t, app: app}

	v.do("POST", "/again", "{}")
	if p := v.page(v.do("GET", "/posts/1", "")); !reflect.DeepEqual(p.Flash, map[string]any{"success": "Saved, after all"}) {
		t.Errorf("flash %v", p.Flash)
	}
	v.do("POST", "/both", "{}")
	want := map[string]any{"success": "Post made", "note": "Its first"}
	if p := v.page(v.do("GET", "/renamed", "")); !reflect.DeepEqual(p.Flash, want) {
		t.Errorf("flash %v, want %v", p.Flash, want)
	}
}

func TestForgettingAKeyThatIsntThereChangesNothing(t *testing.T) {
	app := forgetApp(t)
	app.Get("/same", func(c *Ctx) error {
		c.ForgetFlash("warning", "error")
		c.ForgetFlash()
		return c.Inertia("Posts/Show", nil)
	})
	v := &visitor{t: t, app: app}

	v.do("POST", "/both", "{}")
	want := map[string]any{"success": "Post created", "note": "Its first"}
	if p := v.page(v.do("GET", "/same", "")); !reflect.DeepEqual(p.Flash, want) {
		t.Errorf("flash %v, want %v", p.Flash, want)
	}
	// And with nothing flashed at all.
	if p := v.page(v.do("GET", "/same", "")); p.Flash != nil {
		t.Errorf("flash %v", p.Flash)
	}
}

func TestForgettingFlashDataLeavesAFormsErrors(t *testing.T) {
	app, _ := formApp(t)
	app.Get("/posts/new", func(c *Ctx) error {
		c.ForgetFlash("success", "errors", errorsKey)
		return c.Inertia("Posts/Create", nil)
	})
	v := &visitor{t: t, app: app}

	v.do("POST", "/posts", `{"title":""}`, "Referer", "http://example.com/posts/new")
	if p := v.page(v.do("GET", "/posts/new", "")); p.Props["errors"].(map[string]any)["title"] != "title is required" {
		t.Errorf("the form's page shows the errors %v", p.Props["errors"])
	}
}

func TestForgettingWithoutASessionTakesBackTheHandlersOwn(t *testing.T) {
	app := inertiaApp(t)
	app.Get("/", func(c *Ctx) error {
		c.Flash("success", "Saved")
		c.Flash("note", "Kept")
		c.ForgetFlash("success", "warning")
		return c.Inertia("Home", nil)
	})
	v := &visitor{t: t, app: app}
	if p := v.page(v.do("GET", "/", "")); !reflect.DeepEqual(p.Flash, map[string]any{"note": "Kept"}) {
		t.Errorf("flash %v", p.Flash)
	}
}

func TestAPartialReloadThatForgetsLeavesTheFlashForItsVisit(t *testing.T) {
	v := &visitor{t: t, app: forgetApp(t)}
	v.do("POST", "/both", "{}")

	// The page's own reload, as its bell's, before the visit the flash was
	// left for: it shows none of it, and leaves the session as it came.
	rec := v.do("GET", "/quiet", "", partial("Posts/Show")...)
	if p := v.page(rec); p.Flash != nil || len(rec.Result().Cookies()) != 0 {
		t.Errorf("the reload showed %v, and wrote %v", p.Flash, rec.Result().Cookies())
	}
	// The visit forgets the key itself, or, to a page that doesn't, shows it.
	want := map[string]any{"success": "Post created", "note": "Its first"}
	if p := v.page(v.do("GET", "/posts/1", "")); !reflect.DeepEqual(p.Flash, want) {
		t.Errorf("the visit after shows %v, want %v", p.Flash, want)
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

// partial is the headers of a partial reload of component, as a page's
// bell's.
func partial(component string) []string {
	return []string{"X-Inertia-Partial-Component", component, "X-Inertia-Partial-Data", "post"}
}

func TestAPartialReloadLeavesTheFlashForThePageItWasLeftFor(t *testing.T) {
	app, _ := formApp(t)
	v := &visitor{t: t, app: app}
	if rec := v.do("POST", "/posts", `{"title":"Hello","body":"A post of mine"}`); rec.Code != http.StatusSeeOther {
		t.Fatalf("posting got %d", rec.Code)
	}
	flashed := v.cookie

	// The bell's reload, before the form's redirect loads its page, in
	// this tab or another, with the one cookie: it shows nothing, and
	// writes no cookie.
	rec := v.do("GET", "/posts/1", "", partial("Posts/Show")...)
	if p := v.page(rec); p.Flash != nil || len(rec.Result().Cookies()) != 0 {
		t.Errorf("the reload showed %v, and wrote %v", p.Flash, rec.Result().Cookies())
	}
	if p := v.page(v.do("GET", "/posts/1", "")); !reflect.DeepEqual(p.Flash, map[string]any{"success": "Post created"}) {
		t.Errorf("the page the flash was left for shows %v", p.Flash)
	}
	shown := v.cookie

	// And after it, with the cookie the redirect came with: still nothing,
	// and no cookie to bring the flash back.
	v.cookie = flashed
	rec = v.do("GET", "/posts/1", "", partial("Posts/Show")...)
	if p := v.page(rec); p.Flash != nil || len(rec.Result().Cookies()) != 0 {
		t.Errorf("a reload after showed %v, and wrote %v", p.Flash, rec.Result().Cookies())
	}
	v.cookie = shown
	if p := v.page(v.do("GET", "/posts/1", "")); p.Flash != nil {
		t.Errorf("the flash came back: %v", p.Flash)
	}
}

func TestAPartialReloadLeavesAFormsErrorsForItsPage(t *testing.T) {
	app, _ := formApp(t)
	v := &visitor{t: t, app: app}
	if rec := v.do("POST", "/posts", `{"title":""}`, "Referer", "http://example.com/posts/create"); rec.Code != http.StatusSeeOther {
		t.Fatalf("posting got %d", rec.Code)
	}
	rec := v.do("GET", "/posts/create", "", partial("Posts/Create")...)
	if p := v.page(rec); len(p.Props["errors"].(map[string]any)) != 0 || len(rec.Result().Cookies()) != 0 {
		t.Errorf("the reload showed the errors %v, and wrote %v", p.Props["errors"], rec.Result().Cookies())
	}
	if p := v.page(v.do("GET", "/posts/create", "")); p.Props["errors"].(map[string]any)["title"] != "title is required" {
		t.Errorf("the form's page shows the errors %v", p.Props["errors"])
	}
}
