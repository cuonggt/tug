package tugtest_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cuonggt/tug"
	"github.com/cuonggt/tug/inertia"
	"github.com/cuonggt/tug/session"
	"github.com/cuonggt/tug/tugtest"
)

type Post struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

type Stats struct {
	Posts int `json:"posts"`
}

// Counts is a struct with a prop in it.
type Counts struct {
	Views inertia.AlwaysProp[int] `json:"views"`
	Likes int                     `json:"likes"`
}

type IndexProps struct {
	Posts  []Post                         `json:"posts"`
	Stats  inertia.DeferProp[Stats]       `json:"stats"`
	Tags   inertia.OptionalProp[[]string] `json:"tags"`
	Feed   inertia.MergeProp[[]string]    `json:"feed"`
	Counts Counts                         `json:"counts"`
}

type ShowProps struct {
	Post Post `json:"post"`
}

type NoProps struct{}

var (
	Index  = tug.Page[IndexProps]("Posts/Index")
	Show   = tug.Page[ShowProps]("Posts/Show")
	Create = tug.Page[NoProps]("Posts/Create")
)

type PostInput struct {
	Title string `json:"title" validate:"required,max=20"`
}

// sessions keeps the sessions of the app newApp makes, whose cookies it
// can read and write.
func sessions(t *testing.T) *session.Store {
	t.Helper()
	s, err := session.New(session.Config{Keys: [][]byte{bytes.Repeat([]byte{1}, 32)}})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// newApp is an app with a list of posts, a form that adds one, and the
// redirects of the protocol. Every page has the session's "user".
func newApp(t *testing.T) *tug.App {
	t.Helper()
	pages, err := inertia.New(inertia.Config{Template: `<!doctype html><title>Posts</title>{{ .Inertia }}`, Version: "v2"})
	if err != nil {
		t.Fatal(err)
	}
	pages.ShareFunc(func(r *http.Request) inertia.Props {
		return inertia.Props{"user": session.From(r.Context()).Get("user")}
	})
	app := tug.New(tug.Config{Inertia: pages, Session: sessions(t), ErrorPage: "Error"})
	posts := []Post{{1, "Hello"}, {2, "Again"}}

	app.Get("/", func(c *tug.Ctx) error {
		return Index.Render(c, IndexProps{
			Posts:  posts,
			Stats:  inertia.Defer(func() (Stats, error) { return Stats{Posts: len(posts)}, nil }),
			Tags:   inertia.Optional(func() ([]string, error) { return []string{"go"}, nil }),
			Feed:   inertia.Merge([]string{"Hello"}),
			Counts: Counts{Views: inertia.Always(7), Likes: 3},
		})
	})
	app.Get("/posts/create", func(c *tug.Ctx) error { return Create.Render(c, NoProps{}) })
	app.Post("/posts", func(c *tug.Ctx) error {
		var in PostInput
		if err := c.BindValid(&in); err != nil {
			return err
		}
		posts = append(posts, Post{len(posts) + 1, in.Title})
		c.Flash("success", "Post created")
		c.Flash("ids", []int{len(posts)})
		return c.Redirect("/posts/" + strconv.Itoa(len(posts)))
	})
	app.Get("/posts/{id}", func(c *tug.Ctx) error {
		id, _ := strconv.Atoi(c.Param("id"))
		if id < 1 || id > len(posts) {
			return tug.NewHTTPError(http.StatusNotFound, "post not found")
		}
		return Show.Render(c, ShowProps{Post: posts[id-1]})
	})

	app.Get("/latest", func(c *tug.Ctx) error { return c.Redirect("/old") })
	app.Get("/old", func(c *tug.Ctx) error { return c.Redirect("/posts/1") })
	app.Get("/loop", func(c *tug.Ctx) error { return c.Redirect("/loop") })
	app.Post("/moved", func(c *tug.Ctx) error {
		http.Redirect(c.Response(), c.Request(), "/posts", http.StatusTemporaryRedirect)
		return nil
	})
	app.Post("/comments", func(c *tug.Ctx) error { return c.Redirect("/posts/1#comments") })
	app.Get("/docs", func(c *tug.Ctx) error { return c.Location("/docs.html") })
	app.Get("/docs.html", func(c *tug.Ctx) error { return c.HTML(http.StatusOK, "<h1>Docs</h1>") })
	app.Get("/away", func(c *tug.Ctx) error { return c.Location("https://elsewhere.example/") })
	app.Post("/back", func(c *tug.Ctx) error { return c.RedirectBack() })

	app.Get("/theme", func(c *tug.Ctx) error {
		ck := &http.Cookie{Name: "theme", Value: c.Query("to"), Path: "/"}
		if ck.Value == "" {
			ck.MaxAge = -1
		}
		http.SetCookie(c.Response(), ck)
		return c.NoContent(http.StatusNoContent)
	})
	app.Get("/api/theme", func(c *tug.Ctx) error {
		theme := "system"
		if ck, err := c.Request().Cookie("theme"); err == nil {
			theme = ck.Value
		}
		return c.String(http.StatusOK, theme)
	})
	app.Get("/up", func(c *tug.Ctx) error { return c.String(http.StatusOK, "up") })
	return app
}

func TestAVisitGetsThePageAsJSON(t *testing.T) {
	r := tugtest.New(t, newApp(t)).Get("/posts/1")
	if r.Code != http.StatusOK || r.Header.Get("Content-Type") != "application/json" || r.Page.Component != "Posts/Show" || r.Page.URL != "/posts/1" {
		t.Fatalf("got %v, page %+v", r, r.Page)
	}
	if post := tugtest.Props(r, Show).Post; post != (Post{1, "Hello"}) {
		t.Errorf("post %+v", post)
	}
}

func TestTheClientRunsTheAppsBuildFromItsFirstVisit(t *testing.T) {
	c := tugtest.New(t, newApp(t))
	if r := c.Get("/"); r.Code != http.StatusOK || c.Version != "v2" {
		t.Fatalf("got %v, running build %q", r, c.Version)
	}
}

func TestABrowserOnAnOldBuildLoadsThePageAgain(t *testing.T) {
	c := tugtest.New(t, newApp(t))
	c.Version = "v1"
	r := c.Get("/posts/1")
	if r.Code != http.StatusConflict || r.Location() != "/posts/1" {
		t.Fatalf("got %v", r)
	}
	r = r.Follow()
	if r.Page.Component != "Posts/Show" || !strings.Contains(r.Body, "<title>Posts</title>") || c.Version != "v2" {
		t.Errorf("loaded again: %v, running build %q", r, c.Version)
	}
}

func TestAFirstVisitReadsThePageOutOfTheHTML(t *testing.T) {
	c := tugtest.New(t, newApp(t))
	r := c.FirstVisit("/")
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "text/html") || r.Page.Component != "Posts/Index" || c.Version != "v2" {
		t.Fatalf("got %v, running build %q", r, c.Version)
	}
	if !slices.Equal(r.Page.DeferredProps["default"], []string{"stats"}) || r.Has("stats") || tugtest.Prop[string](r, "posts.0.title") != "Hello" {
		t.Errorf("deferred %v, props %v", r.Page.DeferredProps, r.Page.Props)
	}
}

func TestAFormThatDoesntValidateGoesBackToThePageItWasSentFrom(t *testing.T) {
	c := tugtest.New(t, newApp(t))
	c.Get("/posts/create")
	r := c.Post("/posts", map[string]any{"title": ""})
	if r.Code != http.StatusSeeOther || r.Location() != "/posts/create" {
		t.Fatalf("got %v", r)
	}
	r = r.Follow()
	if r.Page.Component != "Posts/Create" || r.Errors()["title"] != "title is required" {
		t.Fatalf("back at the form: %v, errors %v", r, r.Errors())
	}
	if errs := c.Reload().Errors(); len(errs) != 0 {
		t.Errorf("the errors were shown again: %v", errs)
	}
}

func TestAFormsErrorsComeUnderItsErrorBag(t *testing.T) {
	c := tugtest.New(t, newApp(t))
	c.Get("/posts/create")
	r := c.Post("/posts", map[string]any{"title": "Far too long for a title"}, tugtest.ErrorBag("newPost")).Follow()
	if r.Errors()["title"] != "title must be at most 20 characters" {
		t.Errorf("errors %v, from the prop %v", r.Errors(), r.Page.Props["errors"])
	}
}

func TestFlashDataReadsIntoAType(t *testing.T) {
	c := tugtest.New(t, newApp(t))
	r := c.Post("/posts", `{"title":"Third"}`).Follow()
	if r.Page.Flash["success"] != "Post created" || !slices.Equal(tugtest.Flash[[]int](r, "ids"), []int{3}) {
		t.Errorf("%v: flash %v", r, r.Page.Flash)
	}
}

func TestPropsReadIntoTheTypesTheirPageDeclares(t *testing.T) {
	r := tugtest.New(t, newApp(t)).Get("/")
	// counts.views is a prop type, which Props leaves to Prop.
	props := tugtest.Props(r, Index)
	if len(props.Posts) != 2 || props.Posts[1].Title != "Again" || props.Counts.Likes != 3 {
		t.Errorf("props %+v", props)
	}
	if tugtest.Prop[int](r, "counts.views") != 7 || !r.Has("counts.likes") || r.Has("counts.shares") {
		t.Errorf("counts %v", r.Page.Props["counts"])
	}
}

func TestAPartialReloadIsOfThePageTheClientIsOn(t *testing.T) {
	c := tugtest.New(t, newApp(t))
	c.Get("/")
	r := c.Reload(tugtest.Only("stats"))
	if tugtest.Prop[Stats](r, "stats").Posts != 2 || r.Has("posts") {
		t.Errorf("the reload for the stats: %v", r.Page.Props)
	}
	r = c.Reload(tugtest.Only("tags"))
	if tags := tugtest.Prop[[]string](r, "tags"); !slices.Equal(tags, []string{"go"}) {
		t.Errorf("tags %v", tags)
	}
	tugtest.Props(r, Index) // a list where the props type has a prop type
	if r = c.Reload(tugtest.Except("posts")); r.Has("posts") || !r.Has("counts.likes") {
		t.Errorf("the reload for all but the posts: %v", r.Page.Props)
	}
}

func TestResetAsksForAPropAfresh(t *testing.T) {
	c := tugtest.New(t, newApp(t))
	c.Get("/")
	if r := c.Reload(tugtest.Only("feed")); !slices.Equal(r.Page.MergeProps, []string{"feed"}) {
		t.Errorf("the feed, to add to: mergeProps %v", r.Page.MergeProps)
	}
	if r := c.Reload(tugtest.Only("feed"), tugtest.Reset("feed")); len(r.Page.MergeProps) != 0 {
		t.Errorf("the feed afresh: mergeProps %v", r.Page.MergeProps)
	}
}

func TestPrecognitionChecksTheFieldsItNames(t *testing.T) {
	c := tugtest.New(t, newApp(t))
	c.Get("/posts/create")
	r := c.Post("/posts", map[string]any{"title": "Far too long for a title"}, tugtest.Validate("title"))
	if r.Code != http.StatusUnprocessableEntity || r.Errors()["title"] != "title must be at most 20 characters" {
		t.Errorf("a title too long: %v", r)
	}
	if r := c.Post("/posts", map[string]any{"title": "Fine"}, tugtest.Validate("title")); r.Code != http.StatusNoContent {
		t.Errorf("a fine title: %v", r)
	}
	if posts := tugtest.Props(c.Get("/"), Index).Posts; len(posts) != 2 {
		t.Errorf("checking fields made posts: %v", posts)
	}
}

func TestFollowGoesWhereTheRedirectsLead(t *testing.T) {
	c := tugtest.New(t, newApp(t))
	if r := c.Get("/latest").Follow(); r.Page.Component != "Posts/Show" || r.Page.URL != "/posts/1" {
		t.Errorf("two redirects on: %v", r)
	}
	// A 307 makes the request again, body and all.
	if r := c.Post("/moved", map[string]any{"title": "Moved"}).Follow(); tugtest.Props(r, Show).Post.Title != "Moved" {
		t.Errorf("after a 307: %v", r)
	}
	// A redirect to a #fragment is a 409 that the client visits itself.
	r := c.Post("/comments", nil)
	if r.Code != http.StatusConflict || r.Location() != "/posts/1#comments" {
		t.Fatalf("got %v", r)
	}
	if r = r.Follow(); r.Page.URL != "/posts/1" {
		t.Errorf("following a #fragment: %v", r)
	}
	// Location leaves Inertia for a page that isn't one.
	if r := c.Get("/docs").Follow(); r.Code != http.StatusOK || r.Body != "<h1>Docs</h1>" || r.Page.Component != "" {
		t.Errorf("following Location: %v", r)
	}
}

func TestTheRefererIsThePageTheClientIsOn(t *testing.T) {
	c := tugtest.New(t, newApp(t))
	if r := c.Post("/back", nil); r.Location() != "/" {
		t.Errorf("back from no page: %v", r)
	}
	c.Get("/posts/2")
	if r := c.Post("/back", nil); r.Location() != "/posts/2" {
		t.Errorf("back from a post: %v", r)
	}
	if r := c.Post("/back", nil, tugtest.Header("Referer", "")); r.Location() != "/" {
		t.Errorf("back with no Referer: %v", r)
	}
}

func TestSessionChangesTheSessionAsARequestWould(t *testing.T) {
	c := tugtest.New(t, newApp(t))
	c.Post("/posts", map[string]any{"title": "Third"}) // flashes, for the page after
	c.Session(sessions(t), func(s *session.Session) { s.Set("user", "ann") })
	r := c.Get("/posts/3")
	if tugtest.Prop[string](r, "user") != "ann" || r.Page.Flash["success"] != "Post created" {
		t.Errorf("user %v, flash %v", r.Page.Props["user"], r.Page.Flash)
	}
}

func TestTheCookiesTheAppSetsGoWithEachRequestTillItDropsThem(t *testing.T) {
	c := tugtest.New(t, newApp(t))
	theme := func() string { return c.Do(httptest.NewRequest("GET", "/api/theme", nil)).Body }
	if ck := c.Get("/theme?to=dark").Cookie("theme"); ck == nil || ck.Value != "dark" {
		t.Fatalf("cookie %v", ck)
	}
	if got := theme(); got != "dark" {
		t.Errorf("the theme is %q", got)
	}
	c.Get("/theme")
	if got := theme(); got != "system" {
		t.Errorf("dropped, the theme is %q", got)
	}
}

func TestAResponseDescribesItself(t *testing.T) {
	c := tugtest.New(t, newApp(t))
	for r, want := range map[*tugtest.Response]string{
		c.Get("/posts/1"): "GET /posts/1: 200 Posts/Show",
		c.Get("/posts/9"): "GET /posts/9: 404 Error",
		c.Get("/old"):     "GET /old: 302 to /posts/1",
		c.Do(httptest.NewRequest("GET", "/up", nil)): "GET /up: 200 up",
		c.Post("/posts", nil, tugtest.Validate()):    `POST /posts: 422 {"errors":{"title":"title is required"},"message":"title is required"}`,
	} {
		if got := r.String(); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}

// failure is a testing.TB that keeps the first failure it's given, rather
// than ending the test, for the tests of how tugtest fails one.
type failure struct {
	testing.TB
	msg string
}

func (f *failure) Helper() {}

func (f *failure) Fatalf(format string, args ...any) {
	if f.msg == "" {
		f.msg = fmt.Sprintf(format, args...)
	}
}

func TestAskingForWhatIsntThereFailsTheTest(t *testing.T) {
	for _, tc := range []struct {
		name string
		ask  func(c *tugtest.Client)
		want string
	}{
		{"a prop the page doesn't have", func(c *tugtest.Client) { tugtest.Prop[int](c.Get("/"), "likes") },
			"GET /: 200 Posts/Index: the page has no prop likes; it has counts, errors, feed, posts, user"},
		{"a prop of another type", func(c *tugtest.Client) { tugtest.Prop[string](c.Get("/"), "posts") },
			"GET /: 200 Posts/Index: prop posts isn't a string: "},
		{"a prop of no page", func(c *tugtest.Client) { tugtest.Prop[string](c.Get("/old"), "post") },
			"GET /old: 302 to /posts/1, not a page with props"},
		{"another page's props", func(c *tugtest.Client) { tugtest.Props(c.Get("/old"), Show) },
			"GET /old: 302 to /posts/1, not the page Posts/Show"},
		{"flash data the page doesn't have", func(c *tugtest.Client) { tugtest.Flash[string](c.Get("/"), "success") },
			"GET /: 200 Posts/Index: the page has no flash success; it has none"},
		{"a redirect from a page", func(c *tugtest.Client) { c.Get("/").Follow() },
			"GET /: 200 Posts/Index: that isn't a redirect to follow"},
		{"a redirect to another site", func(c *tugtest.Client) { c.Get("/away").Follow() },
			"GET /away: 409 to https://elsewhere.example/: that leads to another site"},
		{"redirects without end", func(c *tugtest.Client) { c.Get("/loop").Follow() },
			"GET /loop: 302 to /loop: ten redirects on, and still redirected"},
		{"a partial reload of no page", func(c *tugtest.Client) { c.Get("/", tugtest.Only("stats")) },
			"GET /: a partial reload is of the page the client is on, and it isn't on one yet: visit it first"},
		{"a reload of no page", func(c *tugtest.Client) { c.Reload() },
			"reloading: the client isn't on a page yet: visit one first"},
		{"a target that isn't a path", func(c *tugtest.Client) { c.Get("posts") },
			"GET posts: a target is a path, such as /posts, or a URL"},
		{"a body that isn't JSON", func(c *tugtest.Client) { c.Post("/posts", func() {}) },
			"POST /posts: the body isn't JSON: "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &failure{TB: t}
			tc.ask(tugtest.New(f, newApp(t)))
			if !strings.HasPrefix(f.msg, tc.want) {
				t.Errorf("failed with %q, want %q", f.msg, tc.want)
			}
		})
	}
}
