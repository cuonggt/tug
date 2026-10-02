package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// routesGo is an app's Go that adds routes each way the router has, a
// line of it for each, as the comments number them.
const routesGo = `package main

func newApp(a *app) *tug.App {
	app := tug.New()
	app.Get("/", home).Name("home") // 5
	app.Post("/login", a.guestsOnly(a.login)).Name("login.store").Takes(LoginInput{}) // 6
	app.Post("/register", // 7
		a.guestsOnly(a.register), // 8
	).Name("register.store") // 9
	api := app.Group("/api")
	api.Get("/user", a.tokenUsers("user:read", apiUser)) // 11
	app.Handle("GET", "/files/{key...}", tug.WrapHandler(files)) // 12
	resources(app, "/posts", a.posts) // 13
	app.Get("/hello", func(c *tug.Ctx) error { return c.String(200, "hi") }) // 14
	return app
}

func resources(app *tug.App, path string, h tug.HandlerFunc) {
	app.Get(path, h) // 19
}
`

func TestTheHandlerIsReadFromTheLineThatAddedTheRoute(t *testing.T) {
	file := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(file, []byte(routesGo), 0o644); err != nil {
		t.Fatal(err)
	}
	src := sources{}
	for _, c := range []struct {
		line int
		want string
	}{
		{5, "home"},
		{6, "a.guestsOnly(a.login)"},
		// The runtime gives a call over several lines one of them.
		{7, "a.guestsOnly(a.register)"},
		{8, "a.guestsOnly(a.register)"},
		{9, "a.guestsOnly(a.register)"},
		{11, `a.tokenUsers("user:read", apiUser)`},
		{12, "tug.WrapHandler(files)"},
		{14, "func(c *tug.Ctx) error {…}"},
		// A helper's line gives its own variable, which says nothing: the
		// function is the handler's, as Go names it.
		{19, "main.(*app).posts"},
		// A line that adds no route, as a helper's call.
		{13, "main.(*app).posts"},
	} {
		got := src.handler(listedRoute{File: file, Line: c.line, Function: "main.(*app).posts"})
		if got != c.want {
			t.Errorf("line %d: %q, want %q", c.line, got, c.want)
		}
	}
	// A file that isn't there gives the function's name.
	if got := src.handler(listedRoute{File: filepath.Join(t.TempDir(), "gone.go"), Line: 3, Function: "main.home"}); got != "main.home" {
		t.Errorf("a file that isn't there: %q", got)
	}
}

func TestTheRoutesAreListedByPathThenMethodAndThoseOfATextAlone(t *testing.T) {
	dir := t.TempDir()
	routes := []listedRoute{
		{Method: "POST", Path: "/login", Name: "login.store", Function: "main.(*app).login", File: filepath.Join(dir, "main.go"), Line: 6},
		{Method: "ANY", Path: "/files/{key...}", Function: "main.files"},
		{Method: "GET", Path: "/login", Name: "login", Function: "main.loginPage"},
		{Method: "DELETE", Path: "/posts/{id}", Name: "posts.destroy", Function: "main.destroy"},
		{Method: "GET", Path: "/posts/{id}", Name: "posts.show", Function: "main.show"},
		{Method: "GET", Path: "/", Name: "home", Function: "main.home"},
	}
	var got []string
	for _, r := range listRoutes(routes, "", dir) {
		got = append(got, r.Method+" "+r.Path)
	}
	if want := []string{"GET /", "ANY /files/{key...}", "GET /login", "POST /login", "GET /posts/{id}", "DELETE /posts/{id}"}; !slices.Equal(got, want) {
		t.Errorf("listed %q, want %q", got, want)
	}

	// By path or name, in any case.
	login := listRoutes(routes, "LOGIN", dir)
	if len(login) != 2 || login[0].Name != "login" || login[1].Name != "login.store" {
		t.Errorf("the routes of login: %+v", login)
	}
	if shows := listRoutes(routes, "posts.show", dir); len(shows) != 1 || shows[0].Path != "/posts/{id}" {
		t.Errorf("the routes named posts.show: %+v", shows)
	}
	// Its file as the app's directory has it.
	if login[1].File != "main.go" {
		t.Errorf("the file of POST /login: %q", login[1].File)
	}
}

func TestTheTableHasAColumnEachAndTheHandlerLast(t *testing.T) {
	var out strings.Builder
	err := printRoutes(&out, []listedRoute{
		{Method: "GET", Path: "/", Name: "home", Handler: "home", File: "main.go", Line: 5},
		{Method: "POST", Path: "/login", Name: "login.store", Takes: "LoginInput", Handler: "a.guestsOnly(a.login)", File: "main.go", Line: 6},
		{Method: "ANY", Path: "/files/{key...}", Handler: "main.files"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `METHOD  PATH             NAME         TAKES       ADDED      HANDLER
GET     /                home                     main.go:5  home
POST    /login           login.store  LoginInput  main.go:6  a.guestsOnly(a.login)
ANY     /files/{key...}                                      main.files
`
	if out.String() != want {
		t.Errorf("the table:\n%s\nwant\n%s", out.String(), want)
	}
}

func TestRoutesTakesOneTextAtMost(t *testing.T) {
	if err := runRoutes([]string{"login", "posts"}, &strings.Builder{}); err == nil || !strings.Contains(err.Error(), "one text") {
		t.Errorf("two texts: %v", err)
	}
}
