package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
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
			"resources/js/pages/Posts/Show.tsx": {"file": "assets/Show-1.js", "isDynamicEntry": true, "imports": ["resources/js/app.tsx"]},
			"resources/js/pages/Posts/Create.tsx": {"file": "assets/Create-1.js", "isDynamicEntry": true, "imports": ["resources/js/app.tsx"]},
			"resources/js/pages/Posts/Edit.tsx": {"file": "assets/Edit-1.js", "isDynamicEntry": true, "imports": ["resources/js/app.tsx"]},
			"resources/js/pages/Error.tsx": {"file": "assets/Error-1.js", "isDynamicEntry": true, "imports": ["resources/js/app.tsx"]}
		}`)},
		"assets/app-1.js": {Data: []byte("createInertiaApp()")},
	}
}

// client is Inertia's client in a browser: it keeps the session cookie,
// and sends the build's version with each visit.
type client struct {
	t       *testing.T
	app     *tug.App
	version string
	cookie  *http.Cookie
}

func newClient(t *testing.T) *client {
	app, err := newApp(tug.Config{}, build(), "", [][]byte{bytes.Repeat([]byte{1}, 32)}, 0)
	if err != nil {
		t.Fatal(err)
	}
	c := &client{t: t, app: app}
	c.version = pageIn(t, c.first("/").Body.String()).Version
	return c
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

// first is a first visit, the browser loading a page whole.
func (c *client) first(target string) *httptest.ResponseRecorder {
	return c.send(httptest.NewRequest("GET", target, nil))
}

// visit is a visit from Inertia's client, with a JSON body when there is
// one. headers are name, value pairs.
func (c *client) visit(method, target, body string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("X-Inertia", "true")
	req.Header.Set("X-Inertia-Version", c.version)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	return c.send(req)
}

func (c *client) send(req *http.Request) *httptest.ResponseRecorder {
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	rec := httptest.NewRecorder()
	c.app.ServeHTTP(rec, req)
	for _, ck := range rec.Result().Cookies() {
		c.cookie = ck
	}
	return rec
}

func (c *client) page(rec *httptest.ResponseRecorder) inertia.Page {
	c.t.Helper()
	var p inertia.Page
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		c.t.Fatalf("%d %s: %v", rec.Code, rec.Body, err)
	}
	return p
}

func TestTheFirstVisitLoadsTheBuildAndLeavesTheStatsForLater(t *testing.T) {
	c := newClient(t)
	html := c.first("/").Body.String()
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
	if len(titles(p)) != 10 || p.Props["appName"] != "tug" {
		t.Errorf("props %v", p.Props)
	}
	if want := (inertia.ScrollMeta{PageName: "page", NextPage: 2.0, CurrentPage: 1.0}); !reflect.DeepEqual(p.ScrollProps["posts"], want) {
		t.Errorf("scrollProps %+v", p.ScrollProps["posts"])
	}
}

// titles lists the titles of the posts on a page of the list.
func titles(p inertia.Page) []string {
	var list []string
	for _, post := range p.Props["posts"].(map[string]any)["data"].([]any) {
		list = append(list, post.(map[string]any)["title"].(string))
	}
	return list
}

func TestTheListComesAPageAtATime(t *testing.T) {
	c := newClient(t)
	p := c.page(c.visit("GET", "/?page=3", "", "X-Inertia-Partial-Component", "Posts/Index", "X-Inertia-Partial-Data", "posts",
		"X-Inertia-Infinite-Scroll-Merge-Intent", "append"))
	if got := titles(p); len(got) != 5 || got[0] != "Post 21" {
		t.Errorf("page 3 has %v", got)
	}
	if want := (inertia.ScrollMeta{PageName: "page", PreviousPage: 2.0, CurrentPage: 3.0}); !reflect.DeepEqual(p.ScrollProps["posts"], want) {
		t.Errorf("scrollProps %+v", p.ScrollProps["posts"])
	}
	if !slices.Equal(p.MergeProps, []string{"posts.data"}) || !slices.Equal(p.MatchPropsOn, []string{"posts.data.id"}) {
		t.Errorf("mergeProps %v, matchPropsOn %v", p.MergeProps, p.MatchPropsOn)
	}
}

func TestTheStatsComeWithTheReloadThatAsksForThem(t *testing.T) {
	c := newClient(t)
	p := c.page(c.visit("GET", "/", "", "X-Inertia-Partial-Component", "Posts/Index", "X-Inertia-Partial-Data", "stats"))
	if got, _ := p.Props["stats"].(map[string]any); got["posts"] != 25.0 || got["words"] != 160.0 {
		t.Fatalf("stats %v", p.Props["stats"])
	}
	if _, ok := p.Props["posts"]; ok {
		t.Error("the reload for the stats got the posts too")
	}
}

func TestAPostWithoutTagsHasAnEmptyListOfThem(t *testing.T) {
	c := newClient(t)
	if rec := c.visit("GET", "/posts/3", ""); !strings.Contains(rec.Body.String(), `"tags":[]`) {
		t.Fatalf("got %s", rec.Body)
	}
}

func TestANewPostThatDoesntValidateGoesBackToTheForm(t *testing.T) {
	c := newClient(t)
	rec := c.visit("POST", "/posts", `{"title":"Hello, tug","body":"Too short"}`, "Referer", "http://example.com/posts/create")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/posts/create" {
		t.Fatalf("got %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	p := c.page(c.visit("GET", "/posts/create", ""))
	want := map[string]any{"title": "another post has that title", "body": "body must be at least 10 characters"}
	if !reflect.DeepEqual(p.Props["errors"], want) {
		t.Errorf("errors %v, want %v", p.Props["errors"], want)
	}
}

func TestANewPostIsCreatedAndThePageAfterSaysSo(t *testing.T) {
	c := newClient(t)
	rec := c.visit("POST", "/posts", `{"title":"Forms","body":"Validation from Go.","tags":"go, forms, go"}`)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/posts/26" {
		t.Fatalf("got %d to %q: %s", rec.Code, rec.Header().Get("Location"), rec.Body)
	}
	p := c.page(c.visit("GET", "/posts/26", ""))
	if !reflect.DeepEqual(p.Flash, map[string]any{"success": "Post created"}) {
		t.Errorf("flash %v", p.Flash)
	}
	post := p.Props["post"].(map[string]any)
	if post["title"] != "Forms" || !reflect.DeepEqual(post["tags"], []any{"go", "forms"}) {
		t.Errorf("post %v", post)
	}
}

func TestAPostKeepsItsTitleWhenEdited(t *testing.T) {
	c := newClient(t)
	// Its own title isn't "another post's".
	rec := c.visit("PUT", "/posts/1", `{"title":"Hello, tug","body":"Now with forms that check themselves."}`)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/posts/1" {
		t.Fatalf("got %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	if p := c.page(c.visit("GET", "/posts/1", "")); !reflect.DeepEqual(p.Flash, map[string]any{"success": "Post updated"}) {
		t.Errorf("flash %v", p.Flash)
	}
}

func TestAFieldIsCheckedAsItsFilledIn(t *testing.T) {
	c := newClient(t)
	precog := []string{"Precognition", "true", "Precognition-Validate-Only", "title", "Accept", "application/json"}
	rec := c.visit("POST", "/posts", `{"title":"hello, TUG"}`, precog...)
	if rec.Code != 422 || rec.Body.String() != `{"errors":{"title":"another post has that title"},"message":"another post has that title"}` {
		t.Errorf("a taken title got %d %s", rec.Code, rec.Body)
	}
	if rec := c.visit("POST", "/posts", `{"title":"Fresh"}`, precog...); rec.Code != 204 {
		t.Errorf("a fresh title got %d %s", rec.Code, rec.Body)
	}
	if p := c.page(c.visit("GET", "/?page=3", "")); len(titles(p)) != 5 {
		t.Error("checking a field made a post")
	}
}

func TestDeletingAPostGoesBackToTheListWithoutIt(t *testing.T) {
	c := newClient(t)
	rec := c.visit("DELETE", "/posts/3", "")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("delete = %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	p := c.page(c.visit("GET", "/", ""))
	if got := titles(p); slices.Contains(got, "Delete me") || got[2] != "Post 4" || p.Flash["success"] != "Post deleted" {
		t.Errorf("the list starts %v after deleting post 3, flash %v", got, p.Flash)
	}
}

func TestAPostThatIsntThereShowsTheErrorPage(t *testing.T) {
	c := newClient(t)
	for _, path := range []string{"/posts/99", "/posts/abc", "/posts/99/edit", "/nowhere"} {
		rec := c.visit("GET", path, "")
		if p := c.page(rec); rec.Code != 404 || p.Component != "Error" || p.Props["status"] != 404.0 {
			t.Errorf("%s = %d %+v", path, rec.Code, p)
		}
	}
	if p := c.page(c.visit("GET", "/posts/99", "")); p.Props["message"] != "post not found" {
		t.Errorf("message %v", p.Props["message"])
	}
}

func TestAPageFromAnotherBuildReloads(t *testing.T) {
	c := newClient(t)
	c.version = "an old build"
	if rec := c.visit("GET", "/posts/1", ""); rec.Code != http.StatusConflict || rec.Header().Get("X-Inertia-Location") != "/posts/1" {
		t.Fatalf("got %d with %v", rec.Code, rec.Header())
	}
}

func TestTheBuildIsServed(t *testing.T) {
	c := newClient(t)
	rec := c.first("/build/assets/app-1.js")
	if rec.Code != 200 || rec.Body.String() != "createInertiaApp()" || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("got %d %q with %v", rec.Code, rec.Body, rec.Header())
	}
}
