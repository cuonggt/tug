package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/cuonggt/tug"
	"github.com/cuonggt/tug/inertia"
)

// build is the shape of what `npm run build` writes, so the tests run
// without Node.
func build() fstest.MapFS {
	return fstest.MapFS{
		".vite/manifest.json": {Data: []byte(`{
			"resources/js/app.tsx": {"file": "assets/app-1.js", "isEntry": true, "css": ["assets/app-1.css"]},
			"resources/js/pages/Posts/Index.tsx": {"file": "assets/Index-1.js", "isDynamicEntry": true, "imports": ["resources/js/app.tsx"]},
			"resources/js/pages/Posts/Show.tsx": {"file": "assets/Show-1.js", "isDynamicEntry": true, "imports": ["resources/js/app.tsx"]}
		}`)},
		"assets/app-1.js": {Data: []byte("createInertiaApp()")},
	}
}

type client struct {
	t       *testing.T
	app     *tug.App
	version string
}

func newClient(t *testing.T) client {
	app, err := newApp(tug.Config{}, build(), "", 0)
	if err != nil {
		t.Fatal(err)
	}
	return client{t, app, versionOf(t, app)}
}

// versionOf reads the build's version off a first visit, as the browser
// does.
func versionOf(t *testing.T, app *tug.App) string {
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	return pageIn(t, rec.Body.String()).Version
}

func pageIn(t *testing.T, html string) inertia.Page {
	t.Helper()
	m := regexp.MustCompile(`<script data-page="app" type="application/json">(.*?)</script>`).FindStringSubmatch(html)
	if m == nil {
		t.Fatalf("no page object in %s", html)
	}
	var p inertia.Page
	if err := json.Unmarshal([]byte(m[1]), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

// visit is a visit from Inertia's client. headers are name, value pairs.
func (c client) visit(method, target string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req.Header.Set("X-Inertia", "true")
	req.Header.Set("X-Inertia-Version", c.version)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	c.app.ServeHTTP(rec, req)
	return rec
}

func (c client) page(rec *httptest.ResponseRecorder) inertia.Page {
	c.t.Helper()
	var p inertia.Page
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		c.t.Fatalf("%d %s: %v", rec.Code, rec.Body, err)
	}
	return p
}

func TestTheFirstVisitLoadsTheBuildAndLeavesTheStatsForLater(t *testing.T) {
	c := newClient(t)
	rec := httptest.NewRecorder()
	c.app.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	html := rec.Body.String()
	for _, tag := range []string{
		`<link rel="stylesheet" href="/build/assets/app-1.css">`,
		`<script type="module" src="/build/assets/app-1.js"></script>`,
		`<script type="module" src="/build/assets/Index-1.js"></script>`,
	} {
		if !strings.Contains(html, tag) {
			t.Errorf("no %s in %s", tag, html)
		}
	}
	p := pageIn(t, html)
	if p.Component != "Posts/Index" || p.Props["stats"] != nil || !slices.Equal(p.DeferredProps["default"], []string{"stats"}) {
		t.Errorf("page %+v", p)
	}
	if len(p.Props["posts"].([]any)) != 3 || p.Props["appName"] != "tug" {
		t.Errorf("props %v", p.Props)
	}
}

func TestTheStatsComeWithTheReloadThatAsksForThem(t *testing.T) {
	c := newClient(t)
	p := c.page(c.visit("GET", "/", "X-Inertia-Partial-Component", "Posts/Index", "X-Inertia-Partial-Data", "stats"))
	want := map[string]any{"posts": 3.0, "words": 28.0}
	if got, _ := p.Props["stats"].(map[string]any); got["posts"] != want["posts"] || got["words"] != want["words"] {
		t.Fatalf("stats %v, want %v", p.Props["stats"], want)
	}
	if _, ok := p.Props["posts"]; ok {
		t.Error("the reload for the stats got the posts too")
	}
}

func TestAPostWithoutTagsHasAnEmptyListOfThem(t *testing.T) {
	c := newClient(t)
	rec := c.visit("GET", "/posts/3")
	if !strings.Contains(rec.Body.String(), `"tags":[]`) {
		t.Fatalf("got %s", rec.Body)
	}
}

func TestDeletingAPostGoesBackToTheListWithoutIt(t *testing.T) {
	c := newClient(t)
	rec := c.visit("DELETE", "/posts/3")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("delete = %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	p := c.page(c.visit("GET", "/"))
	if n := len(p.Props["posts"].([]any)); n != 2 {
		t.Errorf("%d posts after deleting one of 3", n)
	}
}

func TestAPostThatIsntThereIsNotFound(t *testing.T) {
	c := newClient(t)
	for _, path := range []string{"/posts/99", "/posts/abc"} {
		if rec := c.visit("GET", path); rec.Code != 404 {
			t.Errorf("%s = %d", path, rec.Code)
		}
	}
}

func TestAPageFromAnotherBuildReloads(t *testing.T) {
	c := newClient(t)
	c.version = "an old build"
	if rec := c.visit("GET", "/posts/1"); rec.Code != http.StatusConflict || rec.Header().Get("X-Inertia-Location") != "/posts/1" {
		t.Fatalf("got %d with %v", rec.Code, rec.Header())
	}
}

func TestTheBuildIsServed(t *testing.T) {
	c := newClient(t)
	rec := httptest.NewRecorder()
	c.app.ServeHTTP(rec, httptest.NewRequest("GET", "/build/assets/app-1.js", nil))
	if rec.Code != 200 || rec.Body.String() != "createInertiaApp()" || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("got %d %q with %v", rec.Code, rec.Body, rec.Header())
	}
}
