package tugtest_test

import (
	"maps"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cuonggt/tug"
	"github.com/cuonggt/tug/inertia"
	"github.com/cuonggt/tug/tugtest"
)

type SearchProps struct {
	Method  string   `json:"method"`
	Term    string   `json:"term"`
	Results []string `json:"results"`
}

type FeedProps struct {
	Bell  inertia.LazyProp[int64]  `json:"bell"`
	Count inertia.LazyProp[int64]  `json:"count"`
	Stats inertia.DeferProp[int64] `json:"stats"`
}

var (
	Search = tug.Page[SearchProps]("Search")
	Feed   = tug.Page[FeedProps]("Feed")
)

// liveApp has a search by QUERY, whose page a GET of its path shows too,
// and a page of live props, each a count of the times it was worked out.
func liveApp(t *testing.T) *tug.App {
	t.Helper()
	pages, err := inertia.New(inertia.Config{Template: `<!doctype html><title>Live</title>{{ .Inertia }}`, Version: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	app := tug.New(tug.Config{Inertia: pages})
	search := func(c *tug.Ctx) error {
		var in struct {
			Term string `json:"term"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		var results []string
		for _, s := range []string{"go", "golang", "rust"} {
			if in.Term != "" && strings.HasPrefix(s, in.Term) {
				results = append(results, s)
			}
		}
		return Search.Render(c, SearchProps{Method: c.Request().Method, Term: in.Term, Results: results})
	}
	app.Get("/search", search)
	app.Query("/search", search)

	var bell, count, stats atomic.Int64
	counted := func(n *atomic.Int64) func() (int64, error) { return func() (int64, error) { return n.Add(1), nil } }
	app.Get("/feed", func(c *tug.Ctx) error {
		return Feed.Render(c, FeedProps{
			Bell:  inertia.Lazy(counted(&bell)).Live(inertia.On("/broadcasts", "notification")),
			Count: inertia.Lazy(counted(&count)).Live(inertia.On("/posts/events", "created", "deleted")),
			Stats: inertia.Defer(counted(&stats)).Live(inertia.On("/posts/events", "created")),
		})
	})
	return app
}

func TestAPageAQueryRenderedReloadsByAQueryWithTheSameBody(t *testing.T) {
	c := tugtest.New(t, liveApp(t))
	r := c.Query("/search", map[string]string{"term": "go"})
	if p := tugtest.Props(r, Search); p.Method != "QUERY" || !slices.Equal(p.Results, []string{"go", "golang"}) {
		t.Fatalf("the search: %+v", p)
	}
	r = c.Reload(tugtest.Only("method", "results"))
	if p := tugtest.Props(r, Search); p.Method != "QUERY" || p.Term != "go" || !slices.Equal(p.Results, []string{"go", "golang"}) {
		t.Errorf("the search reloaded: %+v, want it sent again, by a QUERY, with its term", p)
	}
	// The page a GET shows, as a refresh does, is reloaded by a GET.
	c.Get("/search")
	r = c.Reload(tugtest.Only("method", "term"))
	if p := tugtest.Props(r, Search); p.Method != "GET" || p.Term != "" {
		t.Errorf("the page a GET showed, reloaded: %+v", p)
	}
}

func TestHearReloadsThePropsOfThePageThatListenForTheEvent(t *testing.T) {
	c := tugtest.New(t, liveApp(t))
	c.Get("/feed")
	r := c.Hear("/broadcasts", "notification")
	if got := slices.Sorted(maps.Keys(r.Page.Props)); !slices.Equal(got, []string{"bell", "errors"}) {
		t.Errorf("hearing a notification reloaded %v, want the bell alone", got)
	}
	if tugtest.Prop[int64](r, "bell") != 2 {
		t.Errorf("the bell after the reload: %v", r.Page.Props["bell"])
	}
	// The deferred stats listen for created too, and aren't asked for
	// before the page has them, as the client asks for none it hasn't.
	r = c.Hear("/posts/events", "created")
	if got := slices.Sorted(maps.Keys(r.Page.Props)); !slices.Equal(got, []string{"count", "errors"}) {
		t.Errorf("hearing a post made before the stats came reloaded %v, want the count alone", got)
	}
	c.Reload(tugtest.Only("stats"))
	r = c.Hear("/posts/events", "created")
	if got := slices.Sorted(maps.Keys(r.Page.Props)); !slices.Equal(got, []string{"count", "errors", "stats"}) {
		t.Errorf("hearing a post made reloaded %v, want the count and the stats", got)
	}
	// The page the client is on holds every prop as it came last.
	if bell := tugtest.Prop[int64](r, "bell"); bell != 2 {
		t.Errorf("the bell the page holds: %d, want 2, as the last reload left it", bell)
	}
}

func TestHearingAnEventNoPropListensForFailsTheTest(t *testing.T) {
	f := &failure{TB: t}
	c := tugtest.New(f, liveApp(t))
	c.Get("/feed")
	c.Hear("/posts/events", "updated")
	if want := "hearing updated on /posts/events: no prop of the page's listens for it, of its live props [bell count stats]"; f.msg != want {
		t.Errorf("failed with %q, want %q", f.msg, want)
	}
}
