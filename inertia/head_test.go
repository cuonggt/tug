package inertia

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// withHead is r with head given to its page, as a handler gives it.
func withHead(r *http.Request, head ...HeadElement) *http.Request {
	return r.WithContext(WithHead(r.Context(), head...))
}

// headOf is the head prop of p, as the client reads it.
func headOf(t *testing.T, p Page) []string {
	t.Helper()
	list, ok := p.Props["head"].([]any)
	if !ok {
		t.Fatalf("no head prop in %v", p.Props)
	}
	var head []string
	for _, e := range list {
		head = append(head, e.(string))
	}
	return head
}

func TestEachHeadElementIsWrittenEscapedWithItsKey(t *testing.T) {
	i := newInertia(t, Config{})
	r := withHead(visit("GET", "/posts/1"),
		Title(`Tom & Jerry <3`),
		Meta("description", `A "cat" & a <mouse>`),
		Property("og:image", "https://example.com/a.png?w=1&h=2"),
		Link("canonical", "https://example.com/posts/1"),
		Property("og:image", "https://example.com/b.png").Key("og:image:2"),
	)
	_, p := render(t, i, r, "Posts/Show", nil)
	want := []string{
		`<title data-inertia="title">Tom &amp; Jerry &lt;3</title>`,
		`<meta data-inertia="description" name="description" content="A &#34;cat&#34; &amp; a &lt;mouse&gt;">`,
		`<meta data-inertia="og:image" property="og:image" content="https://example.com/a.png?w=1&amp;h=2">`,
		`<link data-inertia="canonical" rel="canonical" href="https://example.com/posts/1">`,
		`<meta data-inertia="og:image:2" property="og:image" content="https://example.com/b.png">`,
	}
	if got := headOf(t, p); !slices.Equal(got, want) {
		t.Errorf("head\n%q\nwant\n%q", got, want)
	}
}

func TestALaterHeadElementOfAKeyReplacesTheOneBefore(t *testing.T) {
	i := newInertia(t, Config{})
	// The site's, from middleware, then the page's, from its handler.
	r := withHead(visit("GET", "/posts/1"), Title("Blog"), Property("og:image", "/site.png"), Meta("description", "A blog"))
	r = withHead(r, Meta("description", "One post"), Title("A post"))
	_, p := render(t, i, r, "Posts/Show", nil)
	want := []string{
		`<title data-inertia="title">A post</title>`,
		`<meta data-inertia="og:image" property="og:image" content="/site.png">`,
		`<meta data-inertia="description" name="description" content="One post">`,
	}
	if got := headOf(t, p); !slices.Equal(got, want) {
		t.Errorf("head\n%q\nwant\n%q", got, want)
	}
}

func TestTheHeadIsThePagesOwnNotShared(t *testing.T) {
	i := newInertia(t, Config{})
	i.Share("head", []string{"<title>Shared</title>"})
	r := withHead(visit("GET", "/"), Title("Home"))
	_, p := render(t, i, r, "Home", nil)
	if got := headOf(t, p); !slices.Equal(got, []string{`<title data-inertia="title">Home</title>`}) {
		t.Errorf("head %q, want the request's over the shared prop", got)
	}
	// An instant visit takes the shared props on to the next page, which
	// has a head of its own.
	if slices.Contains(p.SharedProps, "head") {
		t.Errorf("sharedProps %v has the head", p.SharedProps)
	}

	// A page's own head prop wins, and the HTML has none of the request's.
	own := Props{"head": []string{`<meta name="robots" content="noindex">`}}
	_, p = render(t, i, withHead(visit("GET", "/"), Title("Home")), "Home", own)
	if got := headOf(t, p); !slices.Equal(got, []string{`<meta name="robots" content="noindex">`}) {
		t.Errorf("head %q, want the page's own", got)
	}
	i = newInertia(t, Config{Template: ssrRoot})
	rec, _ := render(t, i, withHead(httptest.NewRequest("GET", "/", nil), Title("Home")), "Home", own)
	if !strings.Contains(rec.Body.String(), "<head></head>") {
		t.Errorf("the HTML has a head beside the page's own head prop: %s", rec.Body)
	}
}

func TestAPartialReloadLeavesTheHeadOutUnlessItAsks(t *testing.T) {
	i := newInertia(t, Config{})
	props := Props{"posts": []string{"a"}, "stats": 2}
	_, p := render(t, i, withHead(partial("Posts/Index", "posts", ""), Title("Posts")), "Posts/Index", props)
	if _, ok := p.Props["head"]; ok {
		t.Errorf("a reload of the posts got the head: %v", p.Props)
	}
	_, p = render(t, i, withHead(partial("Posts/Index", "head", ""), Title("Posts")), "Posts/Index", props)
	if got := headOf(t, p); !slices.Equal(got, []string{`<title data-inertia="title">Posts</title>`}) {
		t.Errorf("head %q", got)
	}
}

func TestAFirstVisitHasTheHeadInItsHTMLWithItsTitleAsTheClientSaysIt(t *testing.T) {
	i := newInertia(t, Config{Template: ssrRoot, Title: func(title string) string { return title + " · Blog" }})
	r := withHead(httptest.NewRequest("GET", "/posts/1", nil), Title("Tom & Jerry"), Meta("description", "One post"))
	rec, p := render(t, i, r, "Posts/Show", nil)
	want := `<head><title>Tom &amp; Jerry · Blog</title>` + "\n" +
		`<meta data-inertia="description" name="description" content="One post"></head>`
	if body := rec.Body.String(); !strings.Contains(body, want) {
		t.Errorf("got\n%s\nwant the head in\n%s", body, want)
	}
	// The client says the title itself, through its callback.
	if got := headOf(t, p); got[0] != `<title data-inertia="title">Tom &amp; Jerry</title>` {
		t.Errorf("head %q, want the title as it was given", got)
	}

	// A page with no head has none.
	rec, _ = render(t, i, httptest.NewRequest("GET", "/", nil), "Home", nil)
	if !strings.Contains(rec.Body.String(), "<head></head>") {
		t.Errorf("got %s, want an empty head", rec.Body)
	}
}

func TestAPageRenderedOnTheServerHasItsHeadFromThereAlone(t *testing.T) {
	head := []HeadElement{Title("Posts"), Meta("description", "All the posts")}
	ssr := &renderer{rendered: Rendered{
		Head: []string{`<title data-inertia="title">Posts · Blog</title>`, `<meta data-inertia="description" name="description" content="All the posts">`},
		Body: `<div data-server-rendered="true" id="app"><h1>Posts</h1></div>`,
	}}
	i := newInertia(t, Config{Template: ssrRoot, SSR: ssr, Title: func(title string) string { return title + " · Blog" }})
	rec := httptest.NewRecorder()
	if err := i.Render(rec, withHead(httptest.NewRequest("GET", "/posts", nil), head...), "Posts/Index", nil); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	if strings.Count(body, "<title") != 1 || strings.Count(body, `name="description"`) != 1 {
		t.Errorf("the head isn't the server's alone:\n%s", body)
	}

	// The server was given the head, for the client's code to put in.
	var given Page
	if json.Unmarshal(ssr.pages[0], &given) != nil || headOf(t, given)[0] != `<title data-inertia="title">Posts</title>` {
		t.Errorf("the server was given %s", ssr.pages[0])
	}

	// A page the server didn't render has tug's.
	for _, tc := range []struct {
		name string
		r    *http.Request
		ssr  *renderer
	}{
		{"one that failed", httptest.NewRequest("GET", "/posts", nil), &renderer{err: errors.New("window is not defined")}},
		{"one left to the browser", httptest.NewRequest("GET", "/posts", nil).WithContext(WithoutSSR(context.Background())), ssr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
			t.Cleanup(func() { slog.SetDefault(old) })

			i := newInertia(t, Config{Template: ssrRoot, SSR: tc.ssr, Title: func(title string) string { return title + " · Blog" }})
			rec, _ := render(t, i, withHead(tc.r, head...), "Posts/Index", nil)
			if !strings.Contains(rec.Body.String(), `<head><title>Posts · Blog</title>`) {
				t.Errorf("got %s, want tug's head", rec.Body)
			}
		})
	}
}
