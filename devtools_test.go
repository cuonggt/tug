package tug

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/cuonggt/tug/inertia"
	"github.com/cuonggt/tug/session"
)

// renderedAt is the line devtoolsPost renders its page on.
var renderedAt int

func devtoolsPost(c *Ctx) error {
	_, _, renderedAt, _ = runtime.Caller(0)
	return c.Inertia("Posts/Show", inertia.Props{
		"post":     map[string]any{"id": c.Param("id"), "title": "Hello", "token": "tug_abc"},
		"password": "correct horse",
		"filters":  inertia.Always("recent"),
		"comments": inertia.Merge([]string{"first"}, inertia.Prepend()),
		"plans":    inertia.Once(func() ([]string, error) { return []string{"free"}, nil }),
		"feed": inertia.Scroll(func() ([]string, inertia.Paging, error) {
			return []string{"a"}, inertia.PageNumbers(1, true), nil
		}),
		"related": inertia.Optional(func() ([]string, error) { return []string{"b"}, nil }),
		"stats":   inertia.Defer(func() (int, error) { return 7, nil }, "sidebar"),
		"chart":   inertia.Defer(func() (int, error) { return 0, errors.New("the chart is down") }).Rescue(),
	})
}

// devtoolsApp is an app with sessions under DevTools, in a directory of
// its own, which has the page component of Posts/Show. shared and byFunc
// are where it shares app, and auth.
func devtoolsApp(t *testing.T) (app *App, shared, byFunc, routed devtoolsSource) {
	t.Helper()
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(filepath.Join("resources", "js", "pages", "Posts"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("resources", "js", "pages", "Posts", "Show.tsx"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	pages, err := inertia.New(inertia.Config{Template: `<body>{{ .Inertia }}</body>`, Version: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := session.New(session.Config{Keys: [][]byte{bytes.Repeat([]byte{7}, 32)}})
	if err != nil {
		t.Fatal(err)
	}
	shared = here(1)
	pages.Share("app", "tug")
	byFunc = here(1)
	pages.ShareFunc(func(r *http.Request) inertia.Props { return inertia.Props{"auth": map[string]any{"user": "ann"}} })

	app = New(Config{Inertia: pages, Session: sessions, DevTools: true})
	routed = here(1)
	app.Get("/posts/{id}", devtoolsPost).Name("posts.show")
	return app, shared, byFunc, routed
}

type devtoolsSource struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

// here is the place lines below the caller's.
func here(lines int) devtoolsSource {
	_, file, line, _ := runtime.Caller(1)
	return devtoolsSource{file, line + lines}
}

// definedAt is the line of this file that starts with declaration.
func definedAt(t *testing.T, declaration string) devtoolsSource {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, declaration) {
			return devtoolsSource{file, i + 1}
		}
	}
	t.Fatalf("%s has no %q", file, declaration)
	return devtoolsSource{}
}

// devtoolsEntry is the entry the response rec says it has, as the panel
// reads it.
func devtoolsEntry(t *testing.T, app *App, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	id := rec.Header().Get("X-Inertia-Devtools-Id")
	got := serve(app, "GET", "/_inertia/devtools/entries/"+id, "")
	var e map[string]any
	if err := json.Unmarshal(got.Body.Bytes(), &e); got.Code != 200 || err != nil {
		t.Fatalf("the entry %q: %d %s", id, got.Code, got.Body)
	}
	return e
}

// valueAt is the value at path in v, a part of the path at each level.
func valueAt(v any, path ...string) any {
	for _, part := range path {
		m, _ := v.(map[string]any)
		v = m[part]
	}
	return v
}

// sameJSON reports whether got is want, both as JSON reads them.
func sameJSON(t *testing.T, got any, want string) bool {
	t.Helper()
	var w any
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatal(err)
	}
	return reflect.DeepEqual(got, w)
}

func sourceJSON(s devtoolsSource) string {
	data, _ := json.Marshal(s)
	return string(data)
}

func TestUnderDevToolsAPagesEntrySaysItsPropsRouteAndWhereItWasRendered(t *testing.T) {
	app, shared, byFunc, routed := devtoolsApp(t)

	rec := serve(app, "GET", "/posts/7", "")
	id := rec.Header().Get("X-Inertia-Devtools-Id")
	if rec.Code != 200 || id == "" || rec.Header().Get("X-Inertia-Devtools-Parent-Out") != id {
		t.Fatalf("a first visit got %d, its entry %q", rec.Code, id)
	}
	if tag := `<script data-inertia-devtools-id type="application/json">"` + id + `"</script>`; !strings.Contains(rec.Body.String(), tag) {
		t.Errorf("the page doesn't carry its entry's ID:\n%s", rec.Body)
	}

	e := devtoolsEntry(t, app, rec)
	if valueAt(e, "__meta", "component") != "Posts/Show" || valueAt(e, "__meta", "requestType") != "initial" || valueAt(e, "__meta", "url") != "http://example.com/posts/7" {
		t.Errorf("the meta %v", e["__meta"])
	}
	want := `{"name": "posts.show", "uri": "/posts/{id}", "action": "github.com/cuonggt/tug.devtoolsPost", "actionSource": ` + sourceJSON(definedAt(t, "func devtoolsPost(")) + `}`
	if !sameJSON(t, e["route"], want) {
		t.Errorf("the route %v, want %s", e["route"], want)
	}
	if !sameJSON(t, e["renderSource"], sourceJSON(devtoolsSource{routed.File, renderedAt + 1})) {
		t.Errorf("rendered at %v, want line %d", e["renderSource"], renderedAt+1)
	}
	wantFile, _ := os.Stat(filepath.Join("resources", "js", "pages", "Posts", "Show.tsx"))
	path, _ := e["componentPath"].(string)
	if gotFile, err := os.Stat(path); !filepath.IsAbs(path) || err != nil || !os.SameFile(gotFile, wantFile) {
		t.Errorf("the component's path %q", path)
	}

	for prop, want := range map[string]string{
		"post":     `{"shared": false, "inertiaType": null}`,
		"filters":  `{"shared": false, "inertiaType": "always"}`,
		"comments": `{"shared": false, "inertiaType": "merge", "mergeDirection": "prepend"}`,
		"plans":    `{"shared": false, "inertiaType": "once", "once": true}`,
		"feed":     `{"shared": false, "inertiaType": "scroll", "mergeDirection": "append"}`,
		"errors":   `{"shared": true, "inertiaType": null}`,
		"app":      `{"shared": true, "inertiaType": null, "shareSource": ` + sourceJSON(shared) + `}`,
		"auth":     `{"shared": true, "inertiaType": null, "shareSource": ` + sourceJSON(byFunc) + `}`,
	} {
		if got := valueAt(e, "props", prop); !sameJSON(t, got, want) {
			t.Errorf("the prop %s: %v, want %s", prop, got, want)
		}
	}
	if props := valueAt(e, "props").(map[string]any); props["stats"] != nil || props["chart"] != nil || props["related"] != nil {
		t.Errorf("the props left out of the first load are in it: %v", props)
	}
	for prop, want := range map[string]string{
		"post":     `{"id": "7", "title": "Hello", "token": "[REDACTED]"}`,
		"password": `"[REDACTED]"`,
		"feed":     `{"data": ["a"]}`,
		"auth":     `{"user": "ann"}`,
	} {
		if got := valueAt(e, "propValues", prop); !sameJSON(t, got, want) {
			t.Errorf("the value of %s: %v, want %s", prop, got, want)
		}
	}
	if got := valueAt(e, "http", "responseBody", "value", "props", "password"); got != "[REDACTED]" {
		t.Errorf("the page the client got has the password %v", got)
	}
	if got := valueAt(e, "http", "responseBody", "value", "component"); got != "Posts/Show" {
		t.Errorf("the response's body is %v", valueAt(e, "http", "responseBody"))
	}
}

func TestADeferredFetchSaysItsPropsAreDeferredAndWhichFailed(t *testing.T) {
	app, _, _, _ := devtoolsApp(t)
	captureLog(t) // the chart's failure
	first := serve(app, "GET", "/posts/7", "")

	rec := serve(app, "GET", "/posts/7", "",
		"X-Inertia", "true", "X-Inertia-Version", "v1",
		"X-Inertia-Partial-Component", "Posts/Show", "X-Inertia-Partial-Data", "stats,chart",
		"X-Inertia-Devtools-Deferred", "1", "X-Inertia-Devtools-Parent", first.Header().Get("X-Inertia-Devtools-Id"))
	if rec.Code != 200 {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	e := devtoolsEntry(t, app, rec)
	if valueAt(e, "__meta", "requestType") != "deferred" || valueAt(e, "__meta", "batchId") != first.Header().Get("X-Inertia-Devtools-Id") {
		t.Errorf("the meta %v", e["__meta"])
	}
	for prop, want := range map[string]string{
		"stats": `{"shared": false, "inertiaType": "defer", "deferGroup": "sidebar"}`,
		"chart": `{"shared": false, "inertiaType": "defer", "deferGroup": "default", "rescued": true}`,
	} {
		if got := valueAt(e, "props", prop); !sameJSON(t, got, want) {
			t.Errorf("the prop %s: %v, want %s", prop, got, want)
		}
	}
	if got := valueAt(e, "propValues", "stats"); got != 7.0 {
		t.Errorf("the value of stats %v", got)
	}
}

func TestAPartialReloadSaysWhichPropsAreOptional(t *testing.T) {
	app, _, _, _ := devtoolsApp(t)
	rec := serve(app, "GET", "/posts/7", "",
		"X-Inertia", "true", "X-Inertia-Version", "v1",
		"X-Inertia-Partial-Component", "Posts/Show", "X-Inertia-Partial-Data", "related,stats")
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "data-inertia-devtools-id") {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	e := devtoolsEntry(t, app, rec)
	if valueAt(e, "__meta", "requestType") != "partial" {
		t.Errorf("the meta %v", e["__meta"])
	}
	for prop, want := range map[string]string{
		"related": `{"shared": false, "inertiaType": "optional"}`,
		// A deferred prop a partial reload asks for is as any other.
		"stats":   `{"shared": false, "inertiaType": null}`,
		"filters": `{"shared": false, "inertiaType": "always"}`,
		"errors":  `{"shared": true, "inertiaType": null}`,
	} {
		if got := valueAt(e, "props", prop); !sameJSON(t, got, want) {
			t.Errorf("the prop %s: %v, want %s", prop, got, want)
		}
	}
	if props := valueAt(e, "props").(map[string]any); len(props) != 4 {
		t.Errorf("a partial reload has the props %v", props)
	}
}

func TestThePanelsReadsLeaveTheSessionsFlashForThePage(t *testing.T) {
	app, _, _, _ := devtoolsApp(t)
	var seen []string
	app.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seen = append(seen, r.URL.Path)
			next.ServeHTTP(w, r)
		})
	})
	app.Post("/posts", func(c *Ctx) error {
		c.Flash("success", "Post created")
		return c.Redirect("/posts/7")
	})
	v := &visitor{t: t, app: app}

	posted := v.do("POST", "/posts", `{"title":"Hello","password":"correct horse"}`)
	if posted.Code != http.StatusSeeOther {
		t.Fatalf("posting got %d", posted.Code)
	}
	// The panel reads the redirect's entry, and the list, before the page
	// it redirects to has come.
	entry := v.do("GET", "/_inertia/devtools/entries/"+posted.Header().Get("X-Inertia-Devtools-Id"), "")
	v.do("GET", "/_inertia/devtools/entries?limit=5", "")
	if p := v.page(v.do("GET", "/posts/7", "")); !reflect.DeepEqual(p.Flash, map[string]any{"success": "Post created"}) {
		t.Errorf("the page's flash is %v", p.Flash)
	}
	if strings.Join(seen, " ") != "/posts /posts/7" {
		t.Errorf("the app's middleware saw %v", seen)
	}

	var e map[string]any
	json.Unmarshal(entry.Body.Bytes(), &e)
	if !sameJSON(t, valueAt(e, "http", "requestBody"), `{"status": "present", "value": {"title": "Hello", "password": "[REDACTED]"}}`) {
		t.Errorf("the request's body %v", valueAt(e, "http", "requestBody"))
	}
	if valueAt(e, "__meta", "redirectLocation") != "/posts/7" || valueAt(e, "__meta", "status") != 303.0 {
		t.Errorf("the meta %v", e["__meta"])
	}
	if got := valueAt(e, "http", "responseHeaders", "set-cookie"); got != "[REDACTED]" {
		t.Errorf("the session's cookie is kept as %v", got)
	}
}

func TestAnUploadIsRecordedAsItsFilesNamesSizesAndTypes(t *testing.T) {
	app, _, _, _ := devtoolsApp(t)
	app.Post("/photos", func(c *Ctx) error {
		var in struct {
			Caption string                `form:"caption"`
			Photo   *multipart.FileHeader `form:"photo"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		return c.Redirect("/posts/7")
	})
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("caption", "A cat")
	part, _ := mw.CreateFormFile("photo", "cat.png")
	part.Write([]byte("\x89PNG\r\n\x1a\n"))
	mw.Close()

	rec := serve(app, "POST", "/photos", body.String(), "X-Inertia", "true", "X-Inertia-Version", "v1", "Content-Type", mw.FormDataContentType())
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	want := `{"status": "present", "value": {"caption": "A cat", "photo": {"name": "cat.png", "size": 8, "mimeType": "application/octet-stream"}}}`
	if got := valueAt(devtoolsEntry(t, app, rec), "http", "requestBody"); !sameJSON(t, got, want) {
		t.Errorf("the request's body %v, want %s", got, want)
	}
}

func TestWithoutDevToolsNothingIsRecordedOrServed(t *testing.T) {
	t.Chdir(t.TempDir())
	app := inertiaApp(t)
	app.Get("/", func(c *Ctx) error { return c.Inertia("Home", nil) })

	rec := serve(app, "GET", "/", "")
	if rec.Header().Get("X-Inertia-Devtools-Id") != "" || strings.Contains(rec.Body.String(), "data-inertia-devtools-id") {
		t.Errorf("a page without DevTools says an entry: %v\n%s", rec.Header(), rec.Body)
	}
	if rec := serve(app, "GET", "/_inertia/devtools/entries", ""); rec.Code != 404 {
		t.Errorf("the entries got %d", rec.Code)
	}
	if _, err := os.Stat(".tug"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("without DevTools, .tug is there: %v", err)
	}
}

// devtoolsUsersOnly wraps h, as the auth starter's usersOnly does, and
// answers for it when there's no user.
func devtoolsUsersOnly(h HandlerFunc) HandlerFunc {
	return func(c *Ctx) error {
		if c.Query("user") == "" {
			return c.Redirect("/login")
		}
		return h(c)
	}
}

type devtoolsHandlers struct{}

func (devtoolsHandlers) gone(c *Ctx) error { return NewHTTPError(http.StatusGone) }

func TestTheRoutesActionIsTheFunctionOfTheAppsThatAnswered(t *testing.T) {
	app, _, _, _ := devtoolsApp(t)
	app.Get("/wrapped/{id}", devtoolsUsersOnly(devtoolsPost))
	added := here(1)
	app.Get("/gone", devtoolsHandlers{}.gone)

	wrapper := definedAt(t, "func devtoolsUsersOnly(")
	wrapper.Line++ // its closure's
	for _, c := range []struct {
		target, action string
		source         devtoolsSource
	}{
		{"/wrapped/7?user=ann", "github.com/cuonggt/tug.devtoolsPost", definedAt(t, "func devtoolsPost(")},
		// Go names a closure after the function it was inlined into, too.
		{"/wrapped/7", ".devtoolsUsersOnly.func1", wrapper},
		// An error tug answers for the handler names the route's own, by
		// the line that added it.
		{"/gone", "github.com/cuonggt/tug.devtoolsHandlers.gone", added},
	} {
		e := devtoolsEntry(t, app, serve(app, "GET", c.target, ""))
		if got, _ := valueAt(e, "route", "action").(string); !strings.HasSuffix(got, c.action) {
			t.Errorf("GET %s: the action %q, want %s", c.target, got, c.action)
		}
		if got := valueAt(e, "route", "actionSource"); !sameJSON(t, got, sourceJSON(c.source)) {
			t.Errorf("GET %s: the action's source %v, want %v", c.target, got, c.source)
		}
	}

	// A request no route took has none.
	e := devtoolsEntry(t, app, serve(app, "GET", "/nowhere", ""))
	if !sameJSON(t, e["route"], `{"name": null, "uri": "", "action": null}`) {
		t.Errorf("a 404's route %v", e["route"])
	}
}

func TestAPageOtherThanA200CarriesNoEntrysID(t *testing.T) {
	app, _, _, _ := devtoolsApp(t)
	app.Get("/gone", func(c *Ctx) error { return NewHTTPError(http.StatusGone) })
	app.config.ErrorPage = "Error"

	rec := serve(app, "GET", "/gone", "")
	if rec.Code != http.StatusGone || rec.Header().Get("X-Inertia-Devtools-Id") == "" {
		t.Fatalf("got %d, the entry %q", rec.Code, rec.Header().Get("X-Inertia-Devtools-Id"))
	}
	if strings.Contains(rec.Body.String(), "data-inertia-devtools-id") {
		t.Errorf("an error page carries its entry's ID:\n%s", rec.Body)
	}
	if e := devtoolsEntry(t, app, rec); valueAt(e, "__meta", "component") != "Error" || valueAt(e, "renderSource") != nil {
		t.Errorf("the error page's entry %v, rendered at %v", e["__meta"], e["renderSource"])
	}
}

func TestTugDevTurnsOnDevTools(t *testing.T) {
	t.Setenv("TUG_DEV", "")
	if ConfigFromEnv().DevTools {
		t.Error("DevTools are on outside tug dev")
	}
	t.Setenv("TUG_DEV", "1")
	if !ConfigFromEnv().DevTools {
		t.Error("DevTools are off under tug dev")
	}
}
