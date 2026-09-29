package main

import (
	"bytes"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/cuonggt/tug"
	"github.com/cuonggt/tug/inertia"
	"github.com/cuonggt/tug/tugtest"
)

// build is the shape of what `npm run build` writes, so the tests run
// without Node.
func build() fstest.MapFS {
	return fstest.MapFS{
		".vite/manifest.json": {Data: []byte(`{
			"resources/js/app.tsx": {"file": "assets/app-1.js", "isEntry": true, "css": ["assets/app-1.css"]},
			"resources/js/pages/Posts/Index.tsx": {"file": "assets/Index-1.js", "isDynamicEntry": true, "imports": ["resources/js/app.tsx"]},
			"resources/js/pages/Posts/Archive.tsx": {"file": "assets/Archive-1.js", "isDynamicEntry": true, "imports": ["resources/js/app.tsx"]},
			"resources/js/pages/Posts/Show.tsx": {"file": "assets/Show-1.js", "isDynamicEntry": true, "imports": ["resources/js/app.tsx"]},
			"resources/js/pages/Posts/Create.tsx": {"file": "assets/Create-1.js", "isDynamicEntry": true, "imports": ["resources/js/app.tsx"]},
			"resources/js/pages/Posts/Edit.tsx": {"file": "assets/Edit-1.js", "isDynamicEntry": true, "imports": ["resources/js/app.tsx"]},
			"resources/js/pages/Error.tsx": {"file": "assets/Error-1.js", "isDynamicEntry": true, "imports": ["resources/js/app.tsx"]}
		}`)},
		"assets/app-1.js": {Data: []byte("createInertiaApp()")},
	}
}

// newClient is a browser with the example open, running the build.
func newClient(t *testing.T) *tugtest.Client {
	app, err := newApp(tug.Config{}, build(), "", [][]byte{bytes.Repeat([]byte{1}, 32)}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return tugtest.New(t, app)
}

func TestTheFirstVisitLoadsTheBuildAndLeavesTheStatsForLater(t *testing.T) {
	r := newClient(t).FirstVisit("/")
	for _, tag := range []string{
		`<link rel="stylesheet" href="/build/assets/app-1.css">`,
		`<script type="module" src="/build/assets/app-1.js"></script>`,
		`<script type="module" src="/build/assets/Index-1.js"></script>`,
	} {
		if !strings.Contains(r.Body, tag) {
			t.Errorf("no %s in %s", tag, r.Body)
		}
	}
	if r.Page.Component != "Posts/Index" || r.Has("stats") || !slices.Equal(r.Page.DeferredProps["default"], []string{"stats"}) {
		t.Errorf("page %+v", r.Page)
	}
	if len(titles(r)) != 10 || tugtest.Prop[string](r, "appName") != "tug" {
		t.Errorf("props %v", r.Page.Props)
	}
	if want := (inertia.ScrollMeta{PageName: "page", NextPage: 2.0, CurrentPage: 1.0}); !reflect.DeepEqual(r.Page.ScrollProps["posts"], want) {
		t.Errorf("scrollProps %+v", r.Page.ScrollProps["posts"])
	}
}

// titles lists the titles of the posts on a page of the list.
func titles(r *tugtest.Response) []string {
	var list []string
	for _, post := range tugtest.Prop[[]Post](r, "posts.data") {
		list = append(list, post.Title)
	}
	return list
}

func TestTheListComesAPageAtATime(t *testing.T) {
	c := newClient(t)
	c.Get("/")
	r := c.Get("/?page=3", tugtest.Only("posts"), tugtest.Header("X-Inertia-Infinite-Scroll-Merge-Intent", "append"))
	if got := titles(r); len(got) != 5 || got[0] != "Post 21" {
		t.Errorf("page 3 has %v", got)
	}
	if want := (inertia.ScrollMeta{PageName: "page", PreviousPage: 2.0, CurrentPage: 3.0}); !reflect.DeepEqual(r.Page.ScrollProps["posts"], want) {
		t.Errorf("scrollProps %+v", r.Page.ScrollProps["posts"])
	}
	if !slices.Equal(r.Page.MergeProps, []string{"posts.data"}) || !slices.Equal(r.Page.MatchPropsOn, []string{"posts.data.id"}) {
		t.Errorf("mergeProps %v, matchPropsOn %v", r.Page.MergeProps, r.Page.MatchPropsOn)
	}
}

func TestEveryPostComesInNumberedPagesWithAPager(t *testing.T) {
	c := newClient(t)
	posts := tugtest.Props(c.Get("/posts?page=2"), PostsArchive).Posts
	if posts.CurrentPage != 2 || posts.LastPage != 3 || posts.Total != 25 || posts.Data[0].Title != "Post 11" || *posts.From != 11 || *posts.To != 20 {
		t.Errorf("page 2: %+v", posts)
	}
	if *posts.PrevPageURL != "/posts?page=1" || *posts.NextPageURL != "/posts?page=3" {
		t.Errorf("page 2's neighbours: %q, %q", *posts.PrevPageURL, *posts.NextPageURL)
	}
	var pages []string
	for _, link := range posts.Links {
		pages = append(pages, link.Label)
	}
	if !slices.Equal(pages, []string{"1", "2", "3"}) || !posts.Links[1].Active {
		t.Errorf("links %+v", posts.Links)
	}
	// Past the last, a page with none, which still says where the last is.
	if posts := tugtest.Props(c.Get("/posts?page=9"), PostsArchive).Posts; len(posts.Data) != 0 || posts.LastPage != 3 || posts.From != nil {
		t.Errorf("page 9: %+v", posts)
	}
}

func TestTheStatsComeWithTheReloadThatAsksForThem(t *testing.T) {
	c := newClient(t)
	c.Get("/")
	r := c.Reload(tugtest.Only("stats"))
	if stats := tugtest.Prop[Stats](r, "stats"); stats != (Stats{Posts: 25, Words: 160}) {
		t.Fatalf("stats %+v", stats)
	}
	if r.Has("posts") {
		t.Error("the reload for the stats got the posts too")
	}
}

func TestAPostWithoutTagsHasAnEmptyListOfThem(t *testing.T) {
	if r := newClient(t).Get("/posts/3"); !strings.Contains(r.Body, `"tags":[]`) {
		t.Fatalf("got %s", r.Body)
	}
}

func TestANewPostThatDoesntValidateGoesBackToTheForm(t *testing.T) {
	c := newClient(t)
	c.Get("/posts/create")
	r := c.Post("/posts", map[string]any{"title": "Hello, tug", "body": "Too short"})
	if r.Code != http.StatusSeeOther || r.Location() != "/posts/create" {
		t.Fatalf("got %v", r)
	}
	want := map[string]string{"title": "another post has that title", "body": "body must be at least 10 characters"}
	if errs := r.Follow().Errors(); !maps.Equal(errs, want) {
		t.Errorf("errors %v, want %v", errs, want)
	}
}

func TestANewPostIsCreatedAndThePageAfterSaysSo(t *testing.T) {
	c := newClient(t)
	r := c.Post("/posts", map[string]any{"title": "Forms", "body": "Validation from Go.", "tags": "go, forms, go"})
	if r.Code != http.StatusSeeOther || r.Location() != "/posts/26" {
		t.Fatalf("got %v", r)
	}
	r = r.Follow()
	if !reflect.DeepEqual(r.Page.Flash, map[string]any{"success": "Post created"}) {
		t.Errorf("flash %v", r.Page.Flash)
	}
	if post := tugtest.Props(r, PostsShow).Post; post.Title != "Forms" || !slices.Equal(post.Tags, []string{"go", "forms"}) {
		t.Errorf("post %+v", post)
	}
}

func TestAPostKeepsItsTitleWhenEdited(t *testing.T) {
	c := newClient(t)
	// Its own title isn't "another post's".
	r := c.Put("/posts/1", map[string]any{"title": "Hello, tug", "body": "Now with forms that check themselves."})
	if r.Code != http.StatusSeeOther || r.Location() != "/posts/1" {
		t.Fatalf("got %v", r)
	}
	if r = r.Follow(); !reflect.DeepEqual(r.Page.Flash, map[string]any{"success": "Post updated"}) {
		t.Errorf("flash %v", r.Page.Flash)
	}
}

func TestAFieldIsCheckedAsItsFilledIn(t *testing.T) {
	c := newClient(t)
	c.Get("/posts/create")
	r := c.Post("/posts", map[string]any{"title": "hello, TUG"}, tugtest.Validate("title"))
	if r.Code != 422 || r.Body != `{"errors":{"title":"another post has that title"},"message":"another post has that title"}` {
		t.Errorf("a taken title got %v", r)
	}
	if r := c.Post("/posts", map[string]any{"title": "Fresh"}, tugtest.Validate("title")); r.Code != 204 {
		t.Errorf("a fresh title got %v", r)
	}
	if got := titles(c.Get("/?page=3")); len(got) != 5 {
		t.Error("checking a field made a post")
	}
}

func TestDeletingAPostGoesBackToTheListWithoutIt(t *testing.T) {
	c := newClient(t)
	r := c.Delete("/posts/3", nil)
	if r.Code != http.StatusSeeOther || r.Location() != "/" {
		t.Fatalf("got %v", r)
	}
	r = r.Follow()
	if got := titles(r); slices.Contains(got, "Delete me") || got[2] != "Post 4" || r.Page.Flash["success"] != "Post deleted" {
		t.Errorf("the list starts %v after deleting post 3, flash %v", got, r.Page.Flash)
	}
}

func TestAPostThatIsntThereShowsTheErrorPage(t *testing.T) {
	c := newClient(t)
	for _, path := range []string{"/posts/99", "/posts/abc", "/posts/99/edit", "/nowhere"} {
		if r := c.Get(path); r.Code != 404 || r.Page.Component != "Error" || tugtest.Prop[int](r, "status") != 404 {
			t.Errorf("got %v, props %v", r, r.Page.Props)
		}
	}
	if msg := tugtest.Prop[string](c.Get("/posts/99"), "message"); msg != "post not found" {
		t.Errorf("message %q", msg)
	}
}

func TestAPageFromAnotherBuildReloads(t *testing.T) {
	c := newClient(t)
	c.Version = "an old build"
	if r := c.Get("/posts/1"); r.Code != http.StatusConflict || r.Location() != "/posts/1" {
		t.Fatalf("got %v with %v", r, r.Header)
	}
}

func TestTheBuildIsServed(t *testing.T) {
	r := newClient(t).Do(httptest.NewRequest("GET", "/build/assets/app-1.js", nil))
	if r.Code != 200 || r.Body != "createInertiaApp()" || !strings.Contains(r.Header.Get("Cache-Control"), "immutable") {
		t.Fatalf("got %v with %v", r, r.Header)
	}
}
