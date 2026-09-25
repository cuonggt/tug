package tug

import (
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// serve sends one request through h. headers are name, value pairs.
func serve(h http.Handler, method, target, body string, headers ...string) *httptest.ResponseRecorder {
	var br io.Reader
	if body != "" {
		br = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, br)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func text(s string) HandlerFunc {
	return func(c *Ctx) error { return c.String(http.StatusOK, s) }
}

func panics(f func()) (did bool) {
	defer func() { did = recover() != nil }()
	f()
	return false
}

func TestARouteTakesItsMethodAndPathAndNothingElse(t *testing.T) {
	app := New(Config{})
	app.Get("/posts", text("posts"))

	if rec := serve(app, "GET", "/posts", ""); rec.Code != 200 || rec.Body.String() != "posts" {
		t.Fatalf("GET /posts = %d %q", rec.Code, rec.Body)
	}
	if rec := serve(app, "HEAD", "/posts", ""); rec.Code != 200 {
		t.Errorf("HEAD /posts = %d, want the GET route's 200", rec.Code)
	}
	rec := serve(app, "POST", "/posts", "")
	if rec.Code != 405 || rec.Header().Get("Allow") != "GET, HEAD" {
		t.Errorf("POST /posts = %d with Allow %q, want 405 with GET, HEAD", rec.Code, rec.Header().Get("Allow"))
	}
	if rec := serve(app, "GET", "/posts/1", ""); rec.Code != 404 || rec.Body.String() != "Not Found" {
		t.Errorf("GET /posts/1 = %d %q, want 404", rec.Code, rec.Body)
	}
}

func TestTheRootPathIsTheHomePageAndNothingElse(t *testing.T) {
	app := New(Config{})
	app.Get("/", text("home"))

	if rec := serve(app, "GET", "/", ""); rec.Code != 200 || rec.Body.String() != "home" {
		t.Fatalf("GET / = %d %q", rec.Code, rec.Body)
	}
	if rec := serve(app, "GET", "/about", ""); rec.Code != 404 {
		t.Errorf("GET /about = %d, want 404 rather than the home page", rec.Code)
	}
}

func TestAWildcardAtTheEndTakesEverythingUnderIt(t *testing.T) {
	app := New(Config{})
	app.Get("/files/{path...}", func(c *Ctx) error { return c.String(200, c.Param("path")) })

	if rec := serve(app, "GET", "/files/docs/a.txt", ""); rec.Body.String() != "docs/a.txt" {
		t.Fatalf("GET /files/docs/a.txt = %d %q", rec.Code, rec.Body)
	}
}

func TestAGroupPrefixesItsRoutesAndItsOwnPageIsThePrefix(t *testing.T) {
	app := New(Config{})
	admin := app.Group("/admin/")
	admin.Get("/", text("dashboard"))
	admin.Get("/users", text("users"))

	for path, want := range map[string]string{"/admin": "dashboard", "/admin/users": "users"} {
		if rec := serve(app, "GET", path, ""); rec.Code != 200 || rec.Body.String() != want {
			t.Errorf("GET %s = %d %q, want %q", path, rec.Code, rec.Body, want)
		}
	}
}

func TestATrailingSlashRedirectsToTheRouteWithoutIt(t *testing.T) {
	app := New(Config{})
	app.Get("/posts", text("posts"))

	rec := serve(app, "GET", "/posts/?page=2", "")
	if rec.Code != http.StatusTemporaryRedirect || rec.Header().Get("Location") != "/posts?page=2" {
		t.Fatalf("GET /posts/?page=2 = %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	if rec := serve(app, "GET", "/drafts/", ""); rec.Code != 404 {
		t.Errorf("a trailing slash with no route behind it = %d, want 404", rec.Code)
	}
}

func TestMiddlewareRunsTheAppsThenTheGroupsOutsideInThenTheRoutes(t *testing.T) {
	var ran []string
	mark := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ran = append(ran, name)
				next.ServeHTTP(w, r)
			})
		}
	}
	app := New(Config{})
	api := app.Group("/api", mark("api"))
	v1 := api.Group("/v1")
	v1.Get("/posts", func(c *Ctx) error {
		ran = append(ran, "handler")
		return nil
	}, mark("route"))
	v1.Use(mark("v1")) // after its route, and still around it
	app.Use(mark("app"))

	serve(app, "GET", "/api/v1/posts", "")
	if want := []string{"app", "api", "v1", "route", "handler"}; !slices.Equal(ran, want) {
		t.Fatalf("ran %v, want %v", ran, want)
	}
}

func TestTheAppsMiddlewareWrapsRequestsNoRouteTakes(t *testing.T) {
	app := New(Config{})
	app.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Seen", "yes")
			next.ServeHTTP(w, r)
		})
	})

	rec := serve(app, "GET", "/missing", "")
	if rec.Code != 404 || rec.Header().Get("X-Seen") != "yes" {
		t.Fatalf("GET /missing = %d with X-Seen %q", rec.Code, rec.Header().Get("X-Seen"))
	}
}

func TestACatchAllRouteTakesWhatNoOtherRouteDoes(t *testing.T) {
	app := New(Config{})
	app.Get("/posts", text("posts"))
	app.Any("/{path...}", text("fallback"))

	if rec := serve(app, "GET", "/posts", ""); rec.Body.String() != "posts" {
		t.Errorf("GET /posts = %q", rec.Body)
	}
	if rec := serve(app, "DELETE", "/anything/else", ""); rec.Code != 200 || rec.Body.String() != "fallback" {
		t.Errorf("DELETE /anything/else = %d %q, want the catch-all", rec.Code, rec.Body)
	}
}

func TestACatchAllForGetLeavesOtherMethodsA405(t *testing.T) {
	app := New(Config{})
	app.Get("/{path...}", text("spa"))

	rec := serve(app, "POST", "/somewhere", "")
	if rec.Code != 405 || rec.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("POST /somewhere = %d with Allow %q, want 405 with GET, HEAD", rec.Code, rec.Header().Get("Allow"))
	}
}

func TestAddingToTheAppOnceItIsServingPanics(t *testing.T) {
	app := New(Config{})
	route := app.Get("/", text("home"))
	serve(app, "GET", "/", "")

	for what, add := range map[string]func(){
		"a route":    func() { app.Get("/late", text("late")) },
		"middleware": func() { app.Use(func(h http.Handler) http.Handler { return h }) },
		"a group":    func() { app.Group("/late") },
		"a name":     func() { route.Name("home") },
	} {
		if !panics(add) {
			t.Errorf("adding %s after the first request didn't panic", what)
		}
	}
}

func TestRoutesThatTakeTheSameRequestsPanic(t *testing.T) {
	app := New(Config{})
	app.Get("/posts/{id}", text("by id"))
	if !panics(func() { app.Get("/posts/{slug}", text("by slug")) }) {
		t.Fatal("a second route for the same requests didn't panic")
	}
}

func TestARoutePathStartsWithASlash(t *testing.T) {
	app := New(Config{})
	if !panics(func() { app.Get("posts", text("posts")) }) {
		t.Error("a route path without a leading slash didn't panic")
	}
	if !panics(func() { app.Group("admin") }) {
		t.Error("a group prefix without a leading slash didn't panic")
	}
}

func TestARouteNameBelongsToOneRoute(t *testing.T) {
	app := New(Config{})
	app.Get("/posts", text("posts")).Name("posts")
	if !panics(func() { app.Get("/articles", text("articles")).Name("posts") }) {
		t.Fatal("naming a second route the same didn't panic")
	}
}

func TestURLFillsWildcardsInOrderAndEscapesThem(t *testing.T) {
	app := New(Config{})
	app.Get("/", text("")).Name("home")
	app.Get("/posts/{id}", text("")).Name("posts.show")
	app.Get("/users/{user}/posts/{post}", text("")).Name("users.posts.show")
	app.Get("/files/{path...}", text("")).Name("files")
	app.Group("/admin").Get("/", text("")).Name("admin")

	for _, tc := range []struct {
		name   string
		params []any
		want   string
	}{
		{"home", nil, "/"},
		{"posts.show", []any{42}, "/posts/42"},
		{"posts.show", []any{"a b/c"}, "/posts/a%20b%2Fc"},
		{"users.posts.show", []any{"ann", 7}, "/users/ann/posts/7"},
		{"files", []any{"docs/read me.txt"}, "/files/docs/read%20me.txt"},
		{"admin", nil, "/admin"},
	} {
		if got, err := app.URL(tc.name, tc.params...); err != nil || got != tc.want {
			t.Errorf("URL(%q, %v) = %q, %v; want %q", tc.name, tc.params, got, err, tc.want)
		}
	}

	for _, tc := range []struct {
		name   string
		params []any
	}{
		{"nope", nil},
		{"posts.show", nil},
		{"posts.show", []any{1, 2}},
		{"posts.show", []any{""}},
	} {
		if got, err := app.URL(tc.name, tc.params...); err == nil {
			t.Errorf("URL(%q, %v) = %q, want an error", tc.name, tc.params, got)
		}
	}
}
