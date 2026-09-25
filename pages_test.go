package tug

import (
	"net/http"
	"strings"
	"testing"

	"github.com/cuonggt/tug/inertia"
)

type showProps struct {
	Post  string                 `json:"post"`
	Stats inertia.DeferProp[int] `json:"stats"`
}

var postsShow = Page[showProps]("Posts/Show")

func inertiaApp(t *testing.T) *App {
	t.Helper()
	pages, err := inertia.New(inertia.Config{Template: `<body>{{ .Inertia }}</body>`, Version: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	return New(Config{Inertia: pages})
}

func TestATypedPageRendersItsProps(t *testing.T) {
	app := inertiaApp(t)
	app.Get("/posts/{id}", func(c *Ctx) error {
		return postsShow.Render(c, showProps{
			Post:  "post " + c.Param("id"),
			Stats: inertia.Defer(func() (int, error) { return 7, nil }),
		})
	})

	rec := serve(app, "GET", "/posts/1", "", "X-Inertia", "true", "X-Inertia-Version", "v1")
	want := `{"component":"Posts/Show","props":{"errors":{},"post":"post 1"},"url":"/posts/1","version":"v1","deferredProps":{"default":["stats"]}}`
	if rec.Code != 200 || rec.Body.String() != want {
		t.Fatalf("got %d\n%s\nwant\n%s", rec.Code, rec.Body, want)
	}
	rec = serve(app, "GET", "/posts/1", "")
	if !strings.HasPrefix(rec.Body.String(), `<body><script data-page="app" type="application/json">{"component":"Posts/Show"`) {
		t.Errorf("a first visit got %s", rec.Body)
	}
}

func TestEveryResponseGoesThroughTheInertiaMiddleware(t *testing.T) {
	app := inertiaApp(t)
	ran := false
	app.Get("/posts", func(c *Ctx) error {
		ran = true
		return c.Inertia("Posts/Index", nil)
	})
	app.Delete("/posts/{id}", WrapHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/posts", http.StatusFound) // a plain handler's 302
	})))

	rec := serve(app, "GET", "/posts", "", "X-Inertia", "true", "X-Inertia-Version", "v0")
	if rec.Code != 409 || ran {
		t.Errorf("a visit from another build got %d, and the handler ran: %v", rec.Code, ran)
	}
	rec = serve(app, "DELETE", "/posts/1", "", "X-Inertia", "true")
	if rec.Code != 303 {
		t.Errorf("a plain handler's 302 after a DELETE came back %d, want 303", rec.Code)
	}
	if rec := serve(app, "GET", "/missing", ""); rec.Code != 404 || rec.Header().Get("Vary") != "X-Inertia" {
		t.Errorf("a 404 got Vary %q", rec.Header().Get("Vary"))
	}
}

func TestLocationLeavesTheAppWithAFullPageLoad(t *testing.T) {
	app := inertiaApp(t)
	app.Post("/pay", func(c *Ctx) error { return c.Location("https://pay.example.com") })

	rec := serve(app, "POST", "/pay", "", "X-Inertia", "true")
	if rec.Code != 409 || rec.Header().Get("X-Inertia-Location") != "https://pay.example.com" {
		t.Fatalf("got %d with %v", rec.Code, rec.Header())
	}
}

func TestRenderingAPageNeedsConfigInertia(t *testing.T) {
	captureLog(t)
	app := New(Config{})
	app.Get("/", func(c *Ctx) error { return c.Inertia("Home", nil) })
	if rec := serve(app, "GET", "/", ""); rec.Code != 500 {
		t.Fatalf("got %d, want a 500", rec.Code)
	}
}
