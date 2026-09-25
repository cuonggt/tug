package inertia

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// counted is a prop function that counts its calls.
func counted[T any](calls *atomic.Int32, v T) func() (T, error) {
	return func() (T, error) {
		calls.Add(1)
		return v, nil
	}
}

type Dashboard struct {
	User  map[string]any `json:"user"`
	Stats struct {
		Visits DeferProp[int]  `json:"visits"`
		Sales  DeferProp[int]  `json:"sales"`
		Total  AlwaysProp[int] `json:"total"`
	} `json:"stats"`
}

func dashboard(calls *atomic.Int32) Dashboard {
	var d Dashboard
	d.User = map[string]any{"name": "Ann", "email": "ann@example.com"}
	d.Stats.Visits = Defer(counted(calls, 12))
	d.Stats.Sales = Defer(counted(calls, 3), "money")
	d.Stats.Total = Always(15)
	return d
}

func TestPropsNestedInStructsAreDeferredByTheirPaths(t *testing.T) {
	var calls atomic.Int32
	i := newInertia(t, Config{})
	_, p := render(t, i, visit("GET", "/"), "Dashboard", dashboard(&calls))

	want := map[string][]string{"default": {"stats.visits"}, "money": {"stats.sales"}}
	if !reflect.DeepEqual(p.DeferredProps, want) || calls.Load() != 0 {
		t.Errorf("deferredProps %v, worked out %d", p.DeferredProps, calls.Load())
	}
	if !reflect.DeepEqual(p.Props["stats"], map[string]any{"total": 15.0}) {
		t.Errorf("stats %v", p.Props["stats"])
	}
}

func TestAPartialReloadReachesNestedPropsByTheirPaths(t *testing.T) {
	var calls atomic.Int32
	i := newInertia(t, Config{})
	_, p := render(t, i, partial("Dashboard", "stats.visits", ""), "Dashboard", dashboard(&calls))

	// On the way to stats.visits, stats is looked into, not sent whole:
	// sales isn't asked for, and total is an Always prop.
	want := map[string]any{"errors": map[string]any{}, "stats": map[string]any{"visits": 12.0, "total": 15.0}}
	if !reflect.DeepEqual(p.Props, want) || calls.Load() != 1 {
		t.Errorf("props %v, worked out %d", p.Props, calls.Load())
	}

	_, p = render(t, i, partial("Dashboard", "user.name", ""), "Dashboard", dashboard(&calls))
	if !reflect.DeepEqual(p.Props["user"], map[string]any{"name": "Ann"}) {
		t.Errorf("user %v", p.Props["user"])
	}
	_, p = render(t, i, partial("Dashboard", "", "stats,user.email"), "Dashboard", dashboard(&calls))
	if !reflect.DeepEqual(p.Props["user"], map[string]any{"name": "Ann"}) || p.Props["stats"] != nil {
		t.Errorf("except: %v", p.Props)
	}
}

func TestWhatALazyPropReturnsIsSentWhole(t *testing.T) {
	i := newInertia(t, Config{})
	props := Props{"team": Lazy(func() (Props, error) {
		return Props{"name": "Core", "members": []string{"ann"}}, nil
	})}
	_, p := render(t, i, partial("Home", "team.name", ""), "Home", props)
	if want := map[string]any{"name": "Core", "members": []any{"ann"}}; !reflect.DeepEqual(p.Props["team"], want) {
		t.Fatalf("team %v, want the whole of what the prop returned", p.Props["team"])
	}
}

func TestMergePropsAreLabeledForTheClient(t *testing.T) {
	i := newInertia(t, Config{})
	props := Props{
		"posts":         Merge([]int{1}, MatchOn("id")),
		"notifications": Merge([]int{2}, Prepend()),
		"conversations": Merge(map[string]any{"data": []int{3}}, DeepMerge(), MatchOn("data.id")),
		"feed":          Merge(map[string]any{"data": []int{4}}, AppendAt("data"), PrependAt("pinned")),
		"activity":      Lazy(func() ([]int, error) { return []int{5}, nil }).Merge(),
	}
	_, p := render(t, i, visit("GET", "/"), "Feed", props)

	for name, got := range map[string][]string{"mergeProps": p.MergeProps, "prependProps": p.PrependProps, "deepMergeProps": p.DeepMergeProps, "matchPropsOn": p.MatchPropsOn} {
		want := map[string][]string{
			"mergeProps":     {"activity", "feed.data", "posts"},
			"prependProps":   {"feed.pinned", "notifications"},
			"deepMergeProps": {"conversations"},
			"matchPropsOn":   {"conversations.data.id", "posts.id"},
		}[name]
		if !slices.Equal(got, want) {
			t.Errorf("%s %v, want %v", name, got, want)
		}
	}
}

func TestAResetOrAPropNotAskedForGetsNoMergeLabel(t *testing.T) {
	i := newInertia(t, Config{})
	props := Props{"posts": Merge([]int{1}), "comments": Merge([]int{2})}

	r := partial("Feed", "posts,comments", "")
	r.Header.Set("X-Inertia-Reset", "posts")
	if _, p := render(t, i, r, "Feed", props); !slices.Equal(p.MergeProps, []string{"comments"}) {
		t.Errorf("with posts reset: mergeProps %v", p.MergeProps)
	}
	if _, p := render(t, i, partial("Feed", "comments", ""), "Feed", props); !slices.Equal(p.MergeProps, []string{"comments"}) {
		t.Errorf("asking for comments: mergeProps %v", p.MergeProps)
	}
}

func TestAOncePropIsLeftOutForAClientThatHasIt(t *testing.T) {
	var calls atomic.Int32
	i := newInertia(t, Config{})
	expires := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	props := func() Props {
		return Props{
			"plans": Once(counted(&calls, []string{"basic"})),
			"roles": Once(counted(&calls, []string{"admin"}), As("all-roles"), Until(expires)),
		}
	}

	_, p := render(t, i, visit("GET", "/"), "Billing", props())
	ms := expires.UnixMilli()
	want := map[string]OnceMeta{"plans": {Prop: "plans"}, "all-roles": {Prop: "roles", ExpiresAt: &ms}}
	if !reflect.DeepEqual(p.OnceProps, want) || len(p.Props) != 3 {
		t.Fatalf("onceProps %v, props %v", p.OnceProps, p.Props)
	}

	calls.Store(0)
	_, p = render(t, i, visit("GET", "/", "X-Inertia-Except-Once-Props", "plans,all-roles"), "Billing", props())
	if _, ok := p.Props["plans"]; ok || calls.Load() != 0 {
		t.Errorf("a client with the props got %v, worked out %d", p.Props, calls.Load())
	}
	if !reflect.DeepEqual(p.OnceProps, want) {
		t.Errorf("onceProps %v, want them named all the same", p.OnceProps)
	}

	// A partial reload that asks gets it, whatever the client has.
	r := partial("Billing", "plans", "")
	r.Header.Set("X-Inertia-Except-Once-Props", "plans")
	if _, p := render(t, i, r, "Billing", props()); p.Props["plans"] == nil {
		t.Error("a partial reload that asked for plans didn't get them")
	}
}

func TestAFreshOncePropIsSentEvenToAClientThatHasIt(t *testing.T) {
	i := newInertia(t, Config{})
	r := visit("GET", "/", "X-Inertia-Except-Once-Props", "plans")
	props := Props{"plans": Once(func() ([]string, error) { return []string{"new"}, nil }, Fresh(true))}
	if _, p := render(t, i, r, "Billing", props); p.Props["plans"] == nil {
		t.Fatal("a fresh once prop was left out")
	}
}

func TestADeferredOncePropTheClientHasIsntFetchedAgain(t *testing.T) {
	i := newInertia(t, Config{})
	props := Props{"permissions": Defer(func() ([]string, error) { return nil, nil }).Once()}

	if _, p := render(t, i, visit("GET", "/"), "Users", props); !reflect.DeepEqual(p.DeferredProps, map[string][]string{"default": {"permissions"}}) {
		t.Errorf("first: deferredProps %v", p.DeferredProps)
	}
	_, p := render(t, i, visit("GET", "/", "X-Inertia-Except-Once-Props", "permissions"), "Users", props)
	if p.DeferredProps != nil || p.OnceProps["permissions"].Prop != "permissions" {
		t.Errorf("after: deferredProps %v, onceProps %v", p.DeferredProps, p.OnceProps)
	}
}

func posts(page int) ScrollProp[string] {
	return Scroll(func() ([]string, Paging, error) {
		return []string{"post " + string(rune('0'+page))}, PageNumbers(page, page < 3), nil
	})
}

func TestAScrollPropGoesOutWithItsPageAndMergesAtItsData(t *testing.T) {
	i := newInertia(t, Config{})
	_, p := render(t, i, visit("GET", "/posts?page=2"), "Posts", Props{"posts": posts(2).MatchOn("id")})

	if !reflect.DeepEqual(p.Props["posts"], map[string]any{"data": []any{"post 2"}}) {
		t.Errorf("posts %v", p.Props["posts"])
	}
	want := ScrollMeta{PageName: "page", PreviousPage: 1.0, NextPage: 3.0, CurrentPage: 2.0}
	if got := p.ScrollProps["posts"]; !reflect.DeepEqual(got, want) {
		t.Errorf("scrollProps %+v, want %+v", got, want)
	}
	if !slices.Equal(p.MergeProps, []string{"posts.data"}) || !slices.Equal(p.MatchPropsOn, []string{"posts.data.id"}) {
		t.Errorf("mergeProps %v, matchPropsOn %v", p.MergeProps, p.MatchPropsOn)
	}
}

func TestScrollingUpPrependsAndAResetStartsAgain(t *testing.T) {
	i := newInertia(t, Config{})
	r := partial("Posts", "posts", "")
	r.Header.Set("X-Inertia-Infinite-Scroll-Merge-Intent", "prepend")
	if _, p := render(t, i, r, "Posts", Props{"posts": posts(1)}); !slices.Equal(p.PrependProps, []string{"posts.data"}) || p.MergeProps != nil {
		t.Errorf("prepend: mergeProps %v, prependProps %v", p.MergeProps, p.PrependProps)
	}

	r = partial("Posts", "posts", "")
	r.Header.Set("X-Inertia-Reset", "posts")
	_, p := render(t, i, r, "Posts", Props{"posts": posts(1)})
	if !p.ScrollProps["posts"].Reset || p.MergeProps != nil {
		t.Errorf("reset: scrollProps %+v, mergeProps %v", p.ScrollProps["posts"], p.MergeProps)
	}
}

func TestAScrollPropCanWaitForTheFirstLoadToShow(t *testing.T) {
	i := newInertia(t, Config{})
	_, p := render(t, i, visit("GET", "/"), "Posts", Props{"posts": posts(1).Defer()})
	if p.Props["posts"] != nil || !reflect.DeepEqual(p.DeferredProps, map[string][]string{"default": {"posts"}}) {
		t.Fatalf("props %v, deferredProps %v", p.Props, p.DeferredProps)
	}
}

func TestARescuedPropThatFailsIsLeftOutAndNamed(t *testing.T) {
	var logs bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })

	i := newInertia(t, Config{})
	props := Props{
		"permissions": Defer(func() ([]string, error) { return nil, errors.New("ldap is down") }).Rescue(),
		"teams":       Defer(func() ([]string, error) { panic("nil map") }).Rescue(),
		"users":       Defer(func() ([]string, error) { return []string{"ann"}, nil }),
	}
	rec, p := render(t, i, partial("Users", "permissions,teams,users", ""), "Users", props)
	if rec.Code != 200 || !slices.Equal(p.RescuedProps, []string{"permissions", "teams"}) {
		t.Fatalf("got %d, rescuedProps %v", rec.Code, p.RescuedProps)
	}
	if _, ok := p.Props["permissions"]; ok || p.Props["users"] == nil {
		t.Errorf("props %v", p.Props)
	}
	if !strings.Contains(logs.String(), "ldap is down") || !strings.Contains(logs.String(), "nil map") {
		t.Errorf("the failures weren't logged: %s", logs.String())
	}

	// Without Rescue, a failure is the response's.
	props["users"] = Defer(func() ([]string, error) { return nil, errors.New("db is down") })
	if err := i.Render(httptest.NewRecorder(), partial("Users", "users", ""), "Users", props); err == nil {
		t.Error("a failed prop without Rescue didn't fail the render")
	}
}

func TestARedirectToAFragmentBecomesA409TheClientFollows(t *testing.T) {
	i := newInertia(t, Config{})
	h := i.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/posts/1#comments", http.StatusSeeOther)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, visit("POST", "/posts/1/comments"))
	if rec.Code != 409 || rec.Header().Get("X-Inertia-Redirect") != "/posts/1#comments" || rec.Header().Get("Location") != "" || rec.Body.Len() != 0 {
		t.Errorf("got %d with %v and %q", rec.Code, rec.Header(), rec.Body)
	}

	for name, r := range map[string]*http.Request{
		"a prefetch":           visit("GET", "/posts/1/latest", "Purpose", "prefetch"),
		"a visit from outside": httptest.NewRequest("POST", "/posts/1/comments", nil),
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != http.StatusSeeOther {
			t.Errorf("%s got %d, want the redirect as it was", name, rec.Code)
		}
	}
}

func TestPreserveFragmentAndAStatusReachThePage(t *testing.T) {
	i := newInertia(t, Config{})
	r := visit("GET", "/posts/1")
	rec := httptest.NewRecorder()
	if err := i.RenderStatus(rec, r.WithContext(WithPreserveFragment(r.Context())), http.StatusNotFound, "Error", Props{"status": 404}); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 404 || !strings.Contains(rec.Body.String(), `"preserveFragment":true`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}

	rec = httptest.NewRecorder()
	i.RenderStatus(rec, httptest.NewRequest("GET", "/missing", nil), http.StatusNotFound, "Error", nil)
	if rec.Code != 404 || !strings.Contains(rec.Body.String(), `<script data-page="app"`) {
		t.Errorf("a first visit got %d %s", rec.Code, rec.Body)
	}
}
