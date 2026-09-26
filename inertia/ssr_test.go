package inertia

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
)

// renderer is a Renderer that answers as it's told, and keeps the pages it
// was given.
type renderer struct {
	rendered Rendered
	err      error
	pages    [][]byte
}

func (r *renderer) Render(_ context.Context, page []byte) (Rendered, error) {
	r.pages = append(r.pages, page)
	return r.rendered, r.err
}

const ssrRoot = `<!doctype html><html><head>{{ .InertiaHead }}</head><body>{{ .Inertia }}</body></html>`

func TestAFirstVisitIsRenderedOnTheServerWithSSR(t *testing.T) {
	ssr := &renderer{rendered: Rendered{
		Head: []string{`<title data-inertia="">Posts</title>`, `<meta name="description" content="All the posts">`},
		Body: `<script data-page="app" type="application/json">{"component":"Posts/Index"}</script><div data-server-rendered="true" id="app"><h1>Posts</h1></div>`,
	}}
	i := newInertia(t, Config{Template: ssrRoot, SSR: ssr})
	rec := httptest.NewRecorder()
	if err := i.Render(rec, httptest.NewRequest("GET", "/posts", nil), "Posts/Index", map[string]any{"count": 2}); err != nil {
		t.Fatal(err)
	}
	want := `<head><title data-inertia="">Posts</title>` + "\n" + `<meta name="description" content="All the posts"></head>` +
		`<body>` + ssr.rendered.Body + `</body>`
	if body := rec.Body.String(); !strings.Contains(body, want) {
		t.Errorf("got\n%s\nwant the server's head and body in\n%s", body, want)
	}

	// The server was given the page object the browser would have been.
	var p Page
	if len(ssr.pages) != 1 || json.Unmarshal(ssr.pages[0], &p) != nil || p.Component != "Posts/Index" || p.URL != "/posts" || p.Props["count"] != 2.0 {
		t.Errorf("the server was given %s", ssr.pages)
	}
}

func TestAPageTheServerDidntRenderRendersInTheBrowser(t *testing.T) {
	for _, tc := range []struct {
		name   string
		ssr    *renderer
		logged string
	}{
		{"one that failed", &renderer{err: errors.New("window is not defined")}, "window is not defined"},
		{"one the server left", &renderer{}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			old := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
			t.Cleanup(func() { slog.SetDefault(old) })

			i := newInertia(t, Config{Template: ssrRoot, SSR: tc.ssr})
			rec, p := render(t, i, httptest.NewRequest("GET", "/posts", nil), "Posts/Index", nil)
			if body := rec.Body.String(); p.Component != "Posts/Index" || !strings.Contains(body, `<head></head>`) || !strings.Contains(body, `<div id="app"></div>`) {
				t.Errorf("got %s, want the page for the browser to render", body)
			}
			if got := logs.String(); tc.logged == "" && got != "" || !strings.Contains(got, tc.logged) {
				t.Errorf("logged %q, want %q", got, tc.logged)
			}
		})
	}
}

func TestInertiaVisitsAndPagesWithoutSSRArentRenderedOnTheServer(t *testing.T) {
	ssr := &renderer{rendered: Rendered{Body: `<div data-server-rendered="true" id="app"></div>`}}
	i := newInertia(t, Config{Template: ssrRoot, SSR: ssr})
	render(t, i, visit("GET", "/posts"), "Posts/Index", nil)
	r := httptest.NewRequest("GET", "/posts", nil)
	rec, _ := render(t, i, r.WithContext(WithoutSSR(r.Context())), "Posts/Index", nil)
	if len(ssr.pages) != 0 || !strings.Contains(rec.Body.String(), `<div id="app"></div>`) {
		t.Errorf("the server rendered %d pages; the first visit got %s", len(ssr.pages), rec.Body)
	}
}
