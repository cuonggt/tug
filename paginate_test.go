package tug

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cuonggt/tug/inertia"
)

// items is a list of n, numbered from 1, as a table's rows would be, with
// what fetch was asked for last.
type items struct {
	n       int
	fetched []int // limit and offset
}

func (l *items) count() (int, error) { return l.n, nil }

func (l *items) fetch(limit, offset int) ([]int, error) {
	l.fetched = []int{limit, offset}
	var page []int
	for i := offset + 1; i <= l.n && len(page) < limit; i++ {
		page = append(page, i)
	}
	return page, nil
}

// paginate runs f in a handler for a request to target, and returns what it
// made.
func paginate[P any](t *testing.T, target string, f func(c *Ctx) (P, error)) P {
	t.Helper()
	var p P
	var err error
	app := New(Config{})
	app.Get("/posts", func(c *Ctx) error {
		p, err = f(c)
		return nil
	})
	serve(app, "GET", target, "")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPaginateReadsThePageAskedForAndLinksThePagesAroundIt(t *testing.T) {
	list := &items{n: 25}
	p := paginate(t, "/posts?status=draft&page=2", func(c *Ctx) (Paginated[int], error) {
		return Paginate(c, 10, list.count, list.fetch)
	})
	if !slices.Equal(list.fetched, []int{10, 10}) {
		t.Errorf("fetch was asked for %v, want a limit of 10 at 10", list.fetched)
	}
	if p.CurrentPage != 2 || p.LastPage != 3 || p.Total != 25 || p.PerPage != 10 || *p.From != 11 || *p.To != 20 || len(p.Data) != 10 || p.Data[0] != 11 {
		t.Errorf("got %+v", p)
	}
	for name, got := range map[string]string{
		"first": p.FirstPageURL, "last": p.LastPageURL, "prev": *p.PrevPageURL, "next": *p.NextPageURL, "path": p.Path,
	} {
		want := map[string]string{
			"first": "/posts?page=1&status=draft", "last": "/posts?page=3&status=draft",
			"prev": "/posts?page=1&status=draft", "next": "/posts?page=3&status=draft", "path": "/posts",
		}[name]
		if got != want {
			t.Errorf("%s: %q, want %q", name, got, want)
		}
	}
	if got := labels(p.Links); got != "1 [2] 3" {
		t.Errorf("links %s", got)
	}

	p = paginate(t, "/posts?page=3", func(c *Ctx) (Paginated[int], error) {
		return Paginate(c, 10, list.count, list.fetch)
	})
	if len(p.Data) != 5 || *p.From != 21 || *p.To != 25 || p.NextPageURL != nil {
		t.Errorf("the last page: %+v", p)
	}
}

// labels writes a pager's links as a person sees them, the page shown in
// brackets.
func labels(links []PageLink) string {
	var words []string
	for _, l := range links {
		if l.Active {
			words = append(words, "["+l.Label+"]")
		} else {
			words = append(words, l.Label)
		}
	}
	return strings.Join(words, " ")
}

func TestAPageThatIsntANumberOrIsUnder1IsTheFirst(t *testing.T) {
	for _, page := range []string{"", "x", "0", "-3", "1.5", "2x"} {
		list := &items{n: 25}
		p := paginate(t, "/posts?page="+page, func(c *Ctx) (Paginated[int], error) {
			return Paginate(c, 10, list.count, list.fetch)
		})
		if p.CurrentPage != 1 || p.Data[0] != 1 || p.PrevPageURL != nil {
			t.Errorf("?page=%s: %+v", page, p)
		}
	}
}

func TestAPagePastTheLastIsEmptyAndRunsNoQueryForItsItems(t *testing.T) {
	list := &items{n: 25}
	p := paginate(t, "/posts?page=999999999999999999", func(c *Ctx) (Paginated[int], error) {
		return Paginate(c, 10, list.count, list.fetch)
	})
	if list.fetched != nil {
		t.Errorf("fetch was asked for %v", list.fetched)
	}
	if p.CurrentPage != 999999999999999999 || p.LastPage != 3 || p.From != nil || p.To != nil || p.NextPageURL != nil || *p.PrevPageURL != "/posts?page=999999999999999998" {
		t.Errorf("got %+v", p)
	}
	if data, _ := json.Marshal(p); !strings.Contains(string(data), `"data":[]`) {
		t.Errorf("the items went out as %s", data)
	}
}

func TestAListWithNoItemsHasOnePageAndNone(t *testing.T) {
	list := &items{}
	p := paginate(t, "/posts", func(c *Ctx) (Paginated[int], error) {
		return Paginate(c, 10, list.count, list.fetch)
	})
	if list.fetched != nil || p.LastPage != 1 || p.Total != 0 || len(p.Data) != 0 || p.From != nil || labels(p.Links) != "[1]" {
		t.Errorf("got %+v, fetched %v", p, list.fetched)
	}
}

func TestTheLinksLeaveOutPagesAsLaravelsPagerDoes(t *testing.T) {
	link := func(n int) string { return "?page=" + strconv.Itoa(n) }
	for _, tc := range []struct {
		page, last int
		want       string
	}{
		{1, 1, "[1]"},
		{5, 13, "1 2 3 4 [5] 6 7 8 9 10 11 12 13"},
		{1, 20, "[1] 2 3 4 5 6 7 8 9 10 ... 19 20"},
		{7, 20, "1 2 3 4 5 6 [7] 8 9 10 ... 19 20"},
		{10, 20, "1 2 ... 7 8 9 [10] 11 12 13 ... 19 20"},
		{14, 20, "1 2 ... 11 12 13 [14] 15 16 17 18 19 20"},
		{20, 20, "1 2 ... 11 12 13 14 15 16 17 18 19 [20]"},
		{25, 20, "1 2 ... 11 12 13 14 15 16 17 18 19 20"},
	} {
		if got := labels(pageWindow(tc.page, tc.last, link)); got != tc.want {
			t.Errorf("page %d of %d: %s, want %s", tc.page, tc.last, got, tc.want)
		}
	}
}

func TestPaginatedIsWrittenAsLaravelsPaginatorWritesIt(t *testing.T) {
	list := &items{n: 3}
	p := paginate(t, "/posts?page=1", func(c *Ctx) (Paginated[int], error) {
		return Paginate(c, 2, list.count, list.fetch)
	})
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"current_page":1,"data":[1,2],"first_page_url":"/posts?page=1","from":1,"last_page":2,` +
		`"last_page_url":"/posts?page=2","links":[{"url":"/posts?page=1","label":"1","active":true},` +
		`{"url":"/posts?page=2","label":"2","active":false}],"next_page_url":"/posts?page=2","path":"/posts",` +
		`"per_page":2,"prev_page_url":null,"to":2,"total":3}`
	if string(data) != want {
		t.Errorf("got\n%s\nwant\n%s", data, want)
	}
}

func TestSimplePaginateFetchesOneMoreThanAPageToKnowIfTheresANext(t *testing.T) {
	list := &items{n: 25}
	p := paginate(t, "/posts?page=2", func(c *Ctx) (SimplePaginated[int], error) {
		return SimplePaginate(c, 10, list.fetch)
	})
	if !slices.Equal(list.fetched, []int{11, 10}) || len(p.Data) != 10 || *p.NextPageURL != "/posts?page=3" || *p.PrevPageURL != "/posts?page=1" || p.CurrentPageURL != "/posts?page=2" || *p.From != 11 {
		t.Errorf("page 2: %+v, fetched %v", p, list.fetched)
	}
	p = paginate(t, "/posts?page=3", func(c *Ctx) (SimplePaginated[int], error) {
		return SimplePaginate(c, 10, list.fetch)
	})
	if len(p.Data) != 5 || p.NextPageURL != nil || *p.To != 25 {
		t.Errorf("the last page: %+v", p)
	}
	// A page too far on for its offset to be an int has no items.
	list.fetched = nil
	p = paginate(t, "/posts?page=9223372036854775807", func(c *Ctx) (SimplePaginated[int], error) {
		return SimplePaginate(c, 10, list.fetch)
	})
	if list.fetched != nil || len(p.Data) != 0 {
		t.Errorf("a page past any offset: %+v, fetched %v", p, list.fetched)
	}
}

// at is where a post sits in the order a feed reads them.
type at struct {
	ID int `json:"id"`
}

func TestCursorPaginateGoesOnFromTheLastItemsCursor(t *testing.T) {
	var asked []*at
	fetch := func(after *at, limit int) ([]int, error) {
		asked = append(asked, after)
		start := 1
		if after != nil {
			start = after.ID + 1
		}
		var page []int
		for i := start; i <= 25 && len(page) < limit; i++ {
			page = append(page, i)
		}
		return page, nil
	}
	cursorOf := func(id int) at { return at{ID: id} }
	first := paginate(t, "/posts?sort=new", func(c *Ctx) (CursorPaginated[int], error) {
		return CursorPaginate(c, 10, fetch, cursorOf)
	})
	if asked[0] != nil || len(first.Data) != 10 || first.NextCursor == nil {
		t.Fatalf("the first page: %+v, asked %v", first, asked)
	}
	if want := "/posts?cursor=" + *first.NextCursor + "&sort=new"; *first.NextPageURL != want {
		t.Errorf("the next page's link: %q, want %q", *first.NextPageURL, want)
	}
	second := paginate(t, *first.NextPageURL, func(c *Ctx) (CursorPaginated[int], error) {
		return CursorPaginate(c, 10, fetch, cursorOf)
	})
	if asked[1] == nil || asked[1].ID != 10 || second.Data[0] != 11 {
		t.Errorf("the second page: %+v, asked after %+v", second, asked[1])
	}
	third := paginate(t, *second.NextPageURL, func(c *Ctx) (CursorPaginated[int], error) {
		return CursorPaginate(c, 10, fetch, cursorOf)
	})
	if len(third.Data) != 5 || third.NextCursor != nil || third.NextPageURL != nil {
		t.Errorf("the last page: %+v", third)
	}

	// A cursor that doesn't decode is the first page; one made up starts
	// where it says.
	for cursor, want := range map[string]int{"!!!": 1, "bm90IGpzb24": 1, "eyJpZCI6MjB9": 21} { // not JSON; {"id":20}
		p := paginate(t, "/posts?cursor="+cursor, func(c *Ctx) (CursorPaginated[int], error) {
			return CursorPaginate(c, 10, fetch, cursorOf)
		})
		if p.Data[0] != want {
			t.Errorf("?cursor=%s starts at %d, want %d", cursor, p.Data[0], want)
		}
	}
}

func TestPageNameReadsAndLinksAParameterOfItsOwn(t *testing.T) {
	list := &items{n: 25}
	p := paginate(t, "/posts?page=3&comments_page=2", func(c *Ctx) (Paginated[int], error) {
		return Paginate(c, 10, list.count, list.fetch, PageName("comments_page"))
	})
	if p.CurrentPage != 2 || *p.NextPageURL != "/posts?comments_page=3&page=3" {
		t.Errorf("got page %d, next %q", p.CurrentPage, *p.NextPageURL)
	}
	if paging := p.Paging(); paging.PageName != "comments_page" {
		t.Errorf("Paging %+v", paging)
	}
}

func TestPagingIsWhereAPageSitsForAScroll(t *testing.T) {
	list := &items{n: 25}
	p := paginate(t, "/posts?page=2", func(c *Ctx) (Paginated[int], error) {
		return Paginate(c, 10, list.count, list.fetch)
	})
	if got, want := p.Paging(), (inertia.Paging{PageName: "page", Previous: 1, Next: 3, Current: 2}); !reflect.DeepEqual(got, want) {
		t.Errorf("Paginate: %+v, want %+v", got, want)
	}
	s := paginate(t, "/posts?page=3", func(c *Ctx) (SimplePaginated[int], error) {
		return SimplePaginate(c, 10, list.fetch)
	})
	if got, want := s.Paging(), (inertia.Paging{PageName: "page", Previous: 2, Current: 3}); !reflect.DeepEqual(got, want) {
		t.Errorf("SimplePaginate: %+v, want %+v", got, want)
	}
	cp := paginate(t, "/posts?cursor=eyJpZCI6MTB9", func(c *Ctx) (CursorPaginated[int], error) {
		return CursorPaginate(c, 10, func(after *at, limit int) ([]int, error) { return list.fetch(limit, after.ID) }, func(id int) at { return at{ID: id} })
	})
	if got := cp.Paging(); got.PageName != "cursor" || got.Current != "eyJpZCI6MTB9" || got.Next != *cp.NextCursor || got.Previous != nil {
		t.Errorf("CursorPaginate: %+v", got)
	}
}

func TestPaginateSaysWhatFailed(t *testing.T) {
	down := errors.New("the database is down")
	app := New(Config{})
	var got error
	app.Get("/posts", func(c *Ctx) error {
		_, got = Paginate(c, 10, func() (int, error) { return 0, down }, (&items{}).fetch)
		return nil
	})
	// A page of no items is the program's mistake: a panic, which the app
	// answers as a 500.
	app.Get("/none", func(c *Ctx) error {
		_, err := Paginate(c, 0, (&items{}).count, (&items{}).fetch)
		return err
	})
	serve(app, "GET", "/posts", "")
	if !errors.Is(got, down) {
		t.Errorf("got %v", got)
	}
	logs := captureLog(t)
	if rec := serve(app, "GET", "/none", ""); rec.Code != http.StatusInternalServerError || !strings.Contains(logs.String(), "perPage is 0") {
		t.Errorf("perPage 0: %d, and the log says %s", rec.Code, logs)
	}
}

type paginatedPost struct {
	ID int `json:"id"`
}

type paginatedProps struct {
	Posts    Paginated[paginatedPost]       `json:"posts"`
	Feed     CursorPaginated[paginatedPost] `json:"feed"`
	Archived SimplePaginated[paginatedPost] `json:"archived"`
}

var paginatedPage = Page[paginatedProps]("Test/Paginated")

func TestTugGenWritesEachKindOfPageAsItsOwnType(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gen.json")
	if err := New(Config{}).gen(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out struct{ Pages string }
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"posts: Paginated_paginatedPost",
		"export interface Paginated_paginatedPost {\n  current_page: number\n  data: paginatedPost[]\n  first_page_url: string\n  from: number | null\n",
		"  links: PageLink[]\n",
		"  prev_page_url: string | null\n",
		"export interface PageLink {\n  url: string | null\n  label: string\n  active: boolean\n}",
		"export interface CursorPaginated_paginatedPost {\n  data: paginatedPost[]\n  path: string\n  per_page: number\n  next_cursor: string | null\n  next_page_url: string | null\n}",
		"export interface SimplePaginated_paginatedPost {\n  current_page: number\n  current_page_url: string\n",
	} {
		if !strings.Contains(out.Pages, want) {
			t.Errorf("pages.ts hasn't\n%s\nin\n%s", want, out.Pages)
		}
	}
	if strings.Contains(out.Pages, "total") && strings.Contains(out.Pages[strings.Index(out.Pages, "export interface SimplePaginated_"):], "total") {
		t.Error("a simple page has a total in its TypeScript")
	}
	_ = paginatedPage
}
