package inertia

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const root = `<!doctype html><html><head><title>{{ .Page.Component }}</title></head><body>{{ .Inertia }}</body></html>`

func newInertia(t *testing.T, cfg Config) *Inertia {
	t.Helper()
	if cfg.Template == "" {
		cfg.Template = root
	}
	i, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return i
}

// visit is a request from Inertia's client. headers are name, value pairs.
func visit(method, target string, headers ...string) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	r.Header.Set("X-Inertia", "true")
	for i := 0; i+1 < len(headers); i += 2 {
		r.Header.Set(headers[i], headers[i+1])
	}
	return r
}

// partial is a partial reload of component asking for only, less except.
func partial(component, only, except string) *http.Request {
	r := visit("GET", "/", "X-Inertia-Partial-Component", component)
	if only != "" {
		r.Header.Set("X-Inertia-Partial-Data", only)
	}
	if except != "" {
		r.Header.Set("X-Inertia-Partial-Except", except)
	}
	return r
}

// render renders a page for r and decodes the page object from the JSON
// response, or from the HTML of a first visit.
func render(t *testing.T, i *Inertia, r *http.Request, component string, props any) (*httptest.ResponseRecorder, Page) {
	t.Helper()
	rec := httptest.NewRecorder()
	if err := i.Render(rec, r, component, props); err != nil {
		t.Fatalf("Render: %v", err)
	}
	body := rec.Body.String()
	if !IsInertia(r) {
		m := regexp.MustCompile(`<script data-page="app" type="application/json">(.*?)</script>`).FindStringSubmatch(body)
		if m == nil {
			t.Fatalf("no page object in %s", body)
		}
		body = m[1]
	}
	var p Page
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatalf("page object %s: %v", body, err)
	}
	return rec, p
}

func keys(m map[string]any) []string {
	return slices.Sorted(func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	})
}

func TestAFirstVisitGetsAnHTMLPageWithThePageObjectInIt(t *testing.T) {
	i := newInertia(t, Config{Version: "v1"})
	rec, p := render(t, i, httptest.NewRequest("GET", "/posts?page=2", nil), "Posts/Index", Props{"posts": []string{"a"}})

	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `<title>Posts/Index</title>`) {
		t.Errorf("the template didn't get the page: %s", body)
	}
	if !strings.Contains(body, `</script><div id="app"></div>`) {
		t.Errorf("no root element after the page object: %s", body)
	}
	if p.Component != "Posts/Index" || p.URL != "/posts?page=2" || p.Version != "v1" {
		t.Errorf("page %+v", p)
	}
	if !reflect.DeepEqual(p.Props, map[string]any{"posts": []any{"a"}, "errors": map[string]any{}}) {
		t.Errorf("props %v", p.Props)
	}
	if rec.Header().Get("Vary") != "X-Inertia" {
		t.Errorf("Vary %q", rec.Header().Get("Vary"))
	}
}

func TestAPropCantCloseTheScriptElement(t *testing.T) {
	i := newInertia(t, Config{})
	rec, p := render(t, i, httptest.NewRequest("GET", "/", nil), "Home", Props{"bio": `</script><script>alert(1)</script>`})
	if strings.Count(rec.Body.String(), "</script>") != 1 {
		t.Fatalf("the prop closed the script element: %s", rec.Body)
	}
	if p.Props["bio"] != `</script><script>alert(1)</script>` {
		t.Errorf("the prop came back as %q", p.Props["bio"])
	}
	if strings.Contains(rec.Body.String(), html.EscapeString(`"component"`)) {
		t.Errorf("the page object was HTML-escaped, which JSON.parse can't read: %s", rec.Body)
	}
}

func TestAVisitFromInertiaGetsThePageObjectAsJSON(t *testing.T) {
	i := newInertia(t, Config{Version: "v1"})
	rec, p := render(t, i, visit("GET", "/posts", "X-Inertia-Version", "v1"), "Posts/Index", Props{"posts": []string{"a"}})

	for name, want := range map[string]string{"X-Inertia": "true", "Vary": "X-Inertia", "Content-Type": "application/json"} {
		if got := rec.Header().Get(name); got != want {
			t.Errorf("%s: %q, want %q", name, got, want)
		}
	}
	if p.Component != "Posts/Index" || p.URL != "/posts" || p.Version != "v1" || keys(p.Props)[0] != "errors" {
		t.Errorf("page %+v", p)
	}
}

func TestPropsCanBeAStructNamedAsEncodingJSONNamesThem(t *testing.T) {
	type Paging struct {
		Page int `json:"page"`
	}
	type props struct {
		Paging
		Title   string   `json:"title"`
		Tags    []string `json:"tags,omitempty"`
		Draft   bool
		Secret  string `json:"-"`
		private string
	}
	i := newInertia(t, Config{})
	_, p := render(t, i, visit("GET", "/"), "Posts/Edit", &props{Paging: Paging{2}, Title: "Hi", Secret: "s", private: "p"})

	if want := []string{"Draft", "errors", "page", "title"}; !slices.Equal(keys(p.Props), want) {
		t.Fatalf("props %v, want %v", keys(p.Props), want)
	}
}

func TestPropsThatArentAStructOrAMapAreAnError(t *testing.T) {
	i := newInertia(t, Config{})
	if err := i.Render(httptest.NewRecorder(), visit("GET", "/"), "Home", []string{"a"}); err == nil {
		t.Fatal("rendering a slice as props wasn't an error")
	}
}

func TestAPageFromAnotherBuildReloadsInFull(t *testing.T) {
	i := newInertia(t, Config{Version: "v2"})
	ran := false
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { ran = true }))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, visit("GET", "/posts?page=2", "X-Inertia-Version", "v1"))
	if rec.Code != 409 || rec.Header().Get("X-Inertia-Location") != "/posts?page=2" || rec.Header().Get("X-Inertia-Version") != "v2" {
		t.Fatalf("got %d with %v", rec.Code, rec.Header())
	}
	if ran {
		t.Error("the handler ran for a page that's being reloaded")
	}

	// A form sent from the old build still gets through; the GET after it
	// is what reloads.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, visit("POST", "/posts", "X-Inertia-Version", "v1"))
	if rec.Code == 409 || !ran {
		t.Errorf("a POST from the old build got %d", rec.Code)
	}
}

func TestAPartialReloadGetsOnlyThePropsItAsksFor(t *testing.T) {
	i := newInertia(t, Config{})
	props := Props{"a": 1, "b": 2, "c": 3}
	for _, tc := range []struct {
		only, except string
		want         []string
	}{
		{"a,b", "", []string{"a", "b", "errors"}},
		{"", "a", []string{"b", "c", "errors"}},
		{"a, b", "b", []string{"a", "errors"}},
	} {
		_, p := render(t, i, partial("Home", tc.only, tc.except), "Home", props)
		if !slices.Equal(keys(p.Props), tc.want) {
			t.Errorf("only %q except %q: got %v, want %v", tc.only, tc.except, keys(p.Props), tc.want)
		}
	}
}

func TestAPartialReloadOfAnotherComponentGetsThePageWhole(t *testing.T) {
	i := newInertia(t, Config{})
	_, p := render(t, i, partial("Posts/Index", "a", ""), "Posts/Show", Props{"a": 1, "b": 2})
	if want := []string{"a", "b", "errors"}; !slices.Equal(keys(p.Props), want) {
		t.Fatalf("got %v, want %v", keys(p.Props), want)
	}
}

func TestAlwaysPropsAndErrorsGoOutWhetherAskedForOrNot(t *testing.T) {
	i := newInertia(t, Config{})
	props := Props{"a": 1, "flash": Always("saved"), "errors": map[string]string{"title": "is required"}}
	_, p := render(t, i, partial("Home", "a", "flash,errors"), "Home", props)
	if want := []string{"a", "errors", "flash"}; !slices.Equal(keys(p.Props), want) {
		t.Fatalf("got %v, want %v", keys(p.Props), want)
	}
	if p.Props["flash"] != "saved" {
		t.Errorf("flash = %v", p.Props["flash"])
	}
}

func TestOptionalPropsGoOutOnlyWhenAskedFor(t *testing.T) {
	var calls atomic.Int32
	i := newInertia(t, Config{})
	props := func() Props {
		return Props{"users": Optional(func() ([]string, error) {
			calls.Add(1)
			return []string{"ann"}, nil
		})}
	}

	_, p := render(t, i, visit("GET", "/"), "Home", props())
	if _, ok := p.Props["users"]; ok || calls.Load() != 0 {
		t.Fatalf("a full visit got users %v, worked out %d times", p.Props["users"], calls.Load())
	}
	_, p = render(t, i, partial("Home", "users", ""), "Home", props())
	if !reflect.DeepEqual(p.Props["users"], []any{"ann"}) {
		t.Fatalf("asked for users and got %v", p.Props["users"])
	}
}

func TestLazyPropsAreWorkedOutOnlyWhenTheyGoOut(t *testing.T) {
	var calls atomic.Int32
	i := newInertia(t, Config{})
	props := func() Props {
		return Props{"a": 1, "posts": Lazy(func() (int, error) {
			calls.Add(1)
			return 42, nil
		})}
	}

	if _, p := render(t, i, visit("GET", "/"), "Home", props()); p.Props["posts"] != 42.0 {
		t.Fatalf("a full visit got posts %v", p.Props["posts"])
	}
	render(t, i, partial("Home", "a", ""), "Home", props())
	if calls.Load() != 1 {
		t.Fatalf("posts was worked out %d times, want once: not for the reload that didn't ask", calls.Load())
	}
}

func TestDeferredPropsAreNamedByGroupThenFetched(t *testing.T) {
	i := newInertia(t, Config{})
	props := Props{
		"posts":    []string{"a"},
		"stats":    Defer(func() (int, error) { return 7, nil }),
		"comments": Defer(func() (int, error) { return 3, nil }),
		"related":  Defer(func() (int, error) { return 1, nil }, "sidebar"),
	}

	_, p := render(t, i, visit("GET", "/"), "Posts/Show", props)
	if want := []string{"errors", "posts"}; !slices.Equal(keys(p.Props), want) {
		t.Errorf("a full visit got %v, want %v", keys(p.Props), want)
	}
	want := map[string][]string{"default": {"comments", "stats"}, "sidebar": {"related"}}
	if !reflect.DeepEqual(p.DeferredProps, want) {
		t.Errorf("deferredProps %v, want %v", p.DeferredProps, want)
	}

	_, p = render(t, i, partial("Posts/Show", "comments,stats", ""), "Posts/Show", props)
	if p.Props["stats"] != 7.0 || p.Props["comments"] != 3.0 || p.DeferredProps != nil {
		t.Errorf("the fetch got %v and deferredProps %v", p.Props, p.DeferredProps)
	}
}

func TestPropsInOneResponseAreWorkedOutConcurrently(t *testing.T) {
	// Each prop waits for the other to start, which only both running at
	// once gets past.
	a, b := make(chan struct{}), make(chan struct{})
	wait := func(mine, theirs chan struct{}) func() (bool, error) {
		return func() (bool, error) {
			close(mine)
			select {
			case <-theirs:
				return true, nil
			case <-time.After(5 * time.Second):
				return false, errors.New("the other prop never started")
			}
		}
	}
	i := newInertia(t, Config{})
	props := Props{"a": Defer(wait(a, b)), "b": Defer(wait(b, a))}
	if _, p := render(t, i, partial("Home", "a,b", ""), "Home", props); p.Props["a"] != true || p.Props["b"] != true {
		t.Fatalf("got %v", p.Props)
	}
}

func TestAPropThatFailsOrPanicsFailsTheRender(t *testing.T) {
	i := newInertia(t, Config{})
	for name, props := range map[string]Props{
		"an error": {"a": Lazy(func() (int, error) { return 0, errors.New("database down") })},
		"a panic": {
			"a": Lazy(func() (int, error) { panic("nil map") }),
			"b": Lazy(func() (int, error) { return 1, nil }),
		},
	} {
		err := i.Render(httptest.NewRecorder(), visit("GET", "/"), "Home", props)
		if err == nil || !strings.Contains(err.Error(), `prop "a"`) {
			t.Errorf("%s: Render returned %v", name, err)
		}
	}
}

func TestSharedPropsReachEveryPageAndAPagesOwnWin(t *testing.T) {
	i := newInertia(t, Config{})
	i.Share("app", "tug")
	i.Share("title", "shared")
	i.ShareFunc(func(r *http.Request) Props { return Props{"path": r.URL.Path} })

	r := visit("GET", "/posts")
	r = r.WithContext(WithProps(r.Context(), Props{"user": "ann"}))
	_, p := render(t, i, r, "Home", Props{"title": "own"})

	want := map[string]any{"app": "tug", "title": "own", "path": "/posts", "user": "ann", "errors": map[string]any{}}
	if !reflect.DeepEqual(p.Props, want) {
		t.Errorf("props %v, want %v", p.Props, want)
	}
	if want := []string{"app", "path", "title", "user"}; !slices.Equal(p.SharedProps, want) {
		t.Errorf("sharedProps %v, want %v", p.SharedProps, want)
	}
}

func TestNilSlicesAndMapsGoOutEmptyAtAnyDepth(t *testing.T) {
	type Post struct {
		Title string            `json:"title"`
		Tags  []string          `json:"tags"`
		Meta  map[string]string `json:"meta"`
		Next  *Post             `json:"next"`
	}
	posts := []Post{{Title: "a", Next: &Post{Title: "b"}}}
	i := newInertia(t, Config{})
	rec, _ := render(t, i, visit("GET", "/"), "Home", Props{"posts": posts, "none": []Post(nil), "any": any([]int(nil))})

	body := rec.Body.String()
	for _, want := range []string{
		`"none":[]`, `"any":[]`,
		`{"title":"a","tags":[],"meta":{},"next":{"title":"b","tags":[],"meta":{},"next":null}}`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("no %s in %s", want, body)
		}
	}
	if posts[0].Tags != nil || posts[0].Next.Tags != nil {
		t.Error("the props passed in were changed")
	}
}

func TestHistoryEncryptionIsSentOnlyWhenOn(t *testing.T) {
	r := visit("GET", "/")
	if _, p := render(t, newInertia(t, Config{}), r, "Home", nil); p.EncryptHistory || p.ClearHistory {
		t.Errorf("page %+v", p)
	}
	rec, p := render(t, newInertia(t, Config{EncryptHistory: true}), r, "Home", nil)
	if !p.EncryptHistory || strings.Contains(rec.Body.String(), "clearHistory") {
		t.Errorf("encrypted page %s", rec.Body)
	}

	r = r.WithContext(WithClearHistory(WithEncryptHistory(context.Background(), false)))
	if _, p := render(t, newInertia(t, Config{EncryptHistory: true}), r, "Home", nil); p.EncryptHistory || !p.ClearHistory {
		t.Errorf("with the context's say: %+v", p)
	}
}

func TestTheMiddlewareTurnsA302AfterAPutPatchOrDeleteIntoA303(t *testing.T) {
	i := newInertia(t, Config{})
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/posts", http.StatusFound)
	}))
	for method, want := range map[string]int{"PUT": 303, "PATCH": 303, "DELETE": 303, "POST": 302} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, visit(method, "/posts/1"))
		if rec.Code != want {
			t.Errorf("%s: %d, want %d", method, rec.Code, want)
		}
	}
}

func TestTheMiddlewareAddsVaryToEveryResponseOnce(t *testing.T) {
	i := newInertia(t, Config{})
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Accept-Encoding")
		i.Render(w, r, "Home", nil)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if got := rec.Header().Values("Vary"); !slices.Equal(got, []string{"X-Inertia", "Accept-Encoding"}) {
		t.Fatalf("Vary %v", got)
	}
}

func TestLocationLeavesWithAFullPageLoad(t *testing.T) {
	rec := httptest.NewRecorder()
	Location(rec, visit("GET", "/"), "https://example.com/pay")
	if rec.Code != 409 || rec.Header().Get("X-Inertia-Location") != "https://example.com/pay" {
		t.Errorf("Inertia's client got %d with %v", rec.Code, rec.Header())
	}
	rec = httptest.NewRecorder()
	Location(rec, httptest.NewRequest("POST", "/", nil), "https://example.com/pay")
	if rec.Code != 303 || rec.Header().Get("Location") != "https://example.com/pay" {
		t.Errorf("a browser got %d to %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestNewNeedsARootTemplateThatParses(t *testing.T) {
	for _, tmpl := range []string{"", "  ", "{{ .Inertia"} {
		if _, err := New(Config{Template: tmpl}); err == nil {
			t.Errorf("New took the template %q", tmpl)
		}
	}
}
