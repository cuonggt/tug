package tug

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	"github.com/cuonggt/tug/inertia"
)

// Paginated is a page of a list in numbered pages, as Paginate makes it:
// its items, and where it sits among the pages, under the keys Laravel's
// paginators write it with as JSON, so that a pager written for theirs
// reads it. The links are paths, with the query the list was shown with.
type Paginated[T any] struct {
	CurrentPage  int        `json:"current_page"`
	Data         []T        `json:"data"`
	FirstPageURL string     `json:"first_page_url"`
	From         *int       `json:"from"` // the first item's place in the list, from 1, or null for a page with none
	LastPage     int        `json:"last_page"`
	LastPageURL  string     `json:"last_page_url"`
	Links        []PageLink `json:"links"` // the pages around this one, for a pager
	NextPageURL  *string    `json:"next_page_url"`
	Path         string     `json:"path"`
	PerPage      int        `json:"per_page"`
	PrevPageURL  *string    `json:"prev_page_url"`
	To           *int       `json:"to"` // the last item's place, or null
	Total        int        `json:"total"`

	pageName string
}

// PageLink is a pager's link to a page, by its number, or its "...",
// which has no URL, for the pages it leaves out.
type PageLink struct {
	URL    *string `json:"url"`
	Label  string  `json:"label"`
	Active bool    `json:"active"` // it's the page shown
}

// SimplePaginated is a page of a list in numbered pages without a count,
// as SimplePaginate makes it, under the keys Laravel's simple paginator
// writes: it knows whether there's a next page, and not how many there
// are.
type SimplePaginated[T any] struct {
	CurrentPage    int     `json:"current_page"`
	CurrentPageURL string  `json:"current_page_url"`
	Data           []T     `json:"data"`
	FirstPageURL   string  `json:"first_page_url"`
	From           *int    `json:"from"`
	NextPageURL    *string `json:"next_page_url"`
	Path           string  `json:"path"`
	PerPage        int     `json:"per_page"`
	PrevPageURL    *string `json:"prev_page_url"`
	To             *int    `json:"to"`

	pageName string
}

// CursorPaginated is a page of a list by cursor, as CursorPaginate makes
// it, under the keys Laravel's cursor paginator writes: the cursor of the
// page after it, and its link, null on the last.
type CursorPaginated[T any] struct {
	Data        []T     `json:"data"`
	Path        string  `json:"path"`
	PerPage     int     `json:"per_page"`
	NextCursor  *string `json:"next_cursor"`
	NextPageURL *string `json:"next_page_url"`

	pageName string
	current  string // the cursor this page was asked for with, or ""
}

// A PageOption changes how Paginate, SimplePaginate and CursorPaginate
// read the page asked for, and link the others.
type PageOption func(*pageOptions)

type pageOptions struct {
	name string
}

// PageName names the query parameter that picks the page, "page" by
// default, and "cursor" by cursor: another is for a second list on one
// page, as "comments_page", whose links leave the first list's page as it
// is.
func PageName(name string) PageOption {
	return func(o *pageOptions) { o.name = name }
}

func pageOptionsOf(opts []PageOption, name string, perPage int) pageOptions {
	if perPage < 1 {
		panic(fmt.Sprintf("tug: a page has an item at least, and perPage is %d", perPage))
	}
	o := pageOptions{name: name}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// Paginate is one page of a list in numbered pages of perPage items: the
// one ?page asks for. count says how many items the list has, and fetch
// gets a page of them, by the limit and offset it's given, as SQL's LIMIT
// and OFFSET take them: the query is the app's, so tug has no SQL.
//
//	posts, err := tug.Paginate(c, 20, store.CountPosts, store.Posts)
//
// A page that isn't a number, or is under 1, is the first, as Laravel has
// it, and a page past the last is empty, not a 404, as a list can shrink
// between two visits: count runs first, so fetch isn't asked for a page
// that has no items. The page size is the app's alone: a client that
// could pick it would pick how much the database reads.
func Paginate[T any](c *Ctx, perPage int, count func() (int, error), fetch func(limit, offset int) ([]T, error), opts ...PageOption) (Paginated[T], error) {
	o := pageOptionsOf(opts, "page", perPage)
	total, err := count()
	if err != nil {
		return Paginated[T]{}, err
	}
	total = max(total, 0)
	page := c.pageNumber(o.name)
	last := max((total+perPage-1)/perPage, 1)
	var items []T
	if total > 0 && page <= last {
		if items, err = fetch(perPage, (page-1)*perPage); err != nil {
			return Paginated[T]{}, err
		}
		items = items[:min(len(items), perPage)]
	}
	link := c.pageLinks(o.name)
	p := Paginated[T]{
		CurrentPage:  page,
		FirstPageURL: link(1),
		LastPage:     last,
		LastPageURL:  link(last),
		Links:        pageWindow(page, last, link),
		Path:         c.r.URL.EscapedPath(),
		PerPage:      perPage,
		Total:        total,
		pageName:     o.name,
	}
	p.Data, p.From, p.To = span(items, page, perPage)
	if page > 1 {
		p.PrevPageURL = ref(link(page - 1))
	}
	if page < last {
		p.NextPageURL = ref(link(page + 1))
	}
	return p, nil
}

// Paging is where the page sits, for inertia.Scroll, so that one query
// feeds numbered pages and <InfiniteScroll> alike:
//
//	inertia.Scroll(func() ([]Post, inertia.Paging, error) {
//		posts, err := tug.Paginate(c, 20, store.CountPosts, store.Posts)
//		return posts.Data, posts.Paging(), err
//	})
func (p Paginated[T]) Paging() inertia.Paging {
	paging := inertia.PageNumbers(p.CurrentPage, p.CurrentPage < p.LastPage)
	paging.PageName = p.pageName
	return paging
}

// SimplePaginate is one page of a list in numbered pages, as Paginate is,
// for a list too long to count at each visit: fetch is asked for one
// item more than a page, to know whether there's a page after it.
func SimplePaginate[T any](c *Ctx, perPage int, fetch func(limit, offset int) ([]T, error), opts ...PageOption) (SimplePaginated[T], error) {
	o := pageOptionsOf(opts, "page", perPage)
	page := c.pageNumber(o.name)
	var items []T
	// A page so far on that its offset doesn't fit an int has no items.
	if page-1 <= (math.MaxInt-perPage-1)/perPage {
		var err error
		if items, err = fetch(perPage+1, (page-1)*perPage); err != nil {
			return SimplePaginated[T]{}, err
		}
	}
	more := len(items) > perPage
	items = items[:min(len(items), perPage)]
	link := c.pageLinks(o.name)
	p := SimplePaginated[T]{
		CurrentPage:    page,
		CurrentPageURL: link(page),
		FirstPageURL:   link(1),
		Path:           c.r.URL.EscapedPath(),
		PerPage:        perPage,
		pageName:       o.name,
	}
	p.Data, p.From, p.To = span(items, page, perPage)
	if page > 1 {
		p.PrevPageURL = ref(link(page - 1))
	}
	if more {
		p.NextPageURL = ref(link(page + 1))
	}
	return p, nil
}

// Paging is where the page sits, for inertia.Scroll.
func (p SimplePaginated[T]) Paging() inertia.Paging {
	paging := inertia.PageNumbers(p.CurrentPage, p.NextPageURL != nil)
	paging.PageName = p.pageName
	return paging
}

// CursorPaginate is one page of a list by cursor, for a list whose items
// come and go as it's read, as a feed's: fetch gets the items after the
// one at the cursor, nil for the first page, limit of them, one more than
// a page, to know whether there's a next; cursor is where an item sits in
// the query's order, the app's own value, such as its time and its ID:
//
//	posts, err := tug.CursorPaginate(c, 20,
//		func(after *PostCursor, limit int) ([]Post, error) { return store.PostsAfter(after, limit) },
//		func(p Post) PostCursor { return PostCursor{At: p.CreatedAt, ID: p.ID} })
//
// The link to the next page carries the last item's cursor, as JSON, in
// base64url, under ?cursor. It isn't signed, as Laravel's aren't: it's
// only where to start, and one made up starts somewhere else within what
// the query lets the page see. One that doesn't decode is the first page.
// Cursors go forward: a page before one runs the query the other way,
// which is the app's to write.
func CursorPaginate[T, C any](c *Ctx, perPage int, fetch func(after *C, limit int) ([]T, error), cursor func(item T) C, opts ...PageOption) (CursorPaginated[T], error) {
	o := pageOptionsOf(opts, "cursor", perPage)
	raw := c.Query(o.name)
	var after *C
	if raw != "" {
		if data, err := base64.RawURLEncoding.DecodeString(raw); err == nil {
			var v C
			if json.Unmarshal(data, &v) == nil {
				after = &v
			}
		}
	}
	items, err := fetch(after, perPage+1)
	if err != nil {
		return CursorPaginated[T]{}, err
	}
	more := len(items) > perPage
	items = items[:min(len(items), perPage)]
	p := CursorPaginated[T]{
		Data:     nonNil(items),
		Path:     c.r.URL.EscapedPath(),
		PerPage:  perPage,
		pageName: o.name,
	}
	if after != nil {
		p.current = raw
	}
	if more {
		data, err := json.Marshal(cursor(items[len(items)-1]))
		if err != nil {
			return CursorPaginated[T]{}, fmt.Errorf("tug: a cursor that isn't JSON: %w", err)
		}
		next := base64.RawURLEncoding.EncodeToString(data)
		p.NextCursor = &next
		p.NextPageURL = ref(c.pageLinksTo(o.name)(next))
	}
	return p, nil
}

// Paging is where the page sits, for inertia.Scroll: the cursor it was
// asked for with, and the next page's.
func (p CursorPaginated[T]) Paging() inertia.Paging {
	paging := inertia.Paging{PageName: p.pageName}
	if p.current != "" {
		paging.Current = p.current
	}
	if p.NextCursor != nil {
		paging.Next = *p.NextCursor
	}
	return paging
}

// pageNumber reads the page the query parameter name asks for: 1, unless
// it's a number more than that.
func (c *Ctx) pageNumber(name string) int {
	page, err := strconv.Atoi(c.Query(name))
	if err != nil || page < 1 {
		return 1
	}
	return page
}

// pageLinks makes the links to the list's pages, by number: the request's
// path and query, with the page's parameter changed alone, so that a
// list's filters and order go with it.
func (c *Ctx) pageLinks(name string) func(page int) string {
	to := c.pageLinksTo(name)
	return func(page int) string { return to(strconv.Itoa(page)) }
}

func (c *Ctx) pageLinksTo(name string) func(value string) string {
	path, query := c.r.URL.EscapedPath(), c.r.URL.Query()
	return func(value string) string {
		query.Set(name, value)
		return path + "?" + query.Encode()
	}
}

// pageWindow links the pages around page, of last, as Laravel's pager
// does, three on each side: every page when there are under 14, and
// otherwise the first two and the last two besides, with "..." for the
// pages left out between them.
func pageWindow(page, last int, link func(int) string) []PageLink {
	const side, window = 3, 3 + 4
	var pages []int // 0 is "..."
	add := func(from, to int) {
		for n := from; n <= to; n++ {
			pages = append(pages, n)
		}
	}
	switch {
	case last < side*2+8:
		add(1, last)
	case page <= window:
		add(1, window+side)
		pages = append(pages, 0)
		add(last-1, last)
	case page > last-window:
		add(1, 2)
		pages = append(pages, 0)
		add(last-(window+side-1), last)
	default:
		add(1, 2)
		pages = append(pages, 0)
		add(page-side, page+side)
		pages = append(pages, 0)
		add(last-1, last)
	}
	links := make([]PageLink, len(pages))
	for i, n := range pages {
		if n == 0 {
			links[i] = PageLink{Label: "..."}
			continue
		}
		links[i] = PageLink{URL: ref(link(n)), Label: strconv.Itoa(n), Active: n == page}
	}
	return links
}

// span is a page's items, never nil, and their places in the list, from
// 1, or nil without any.
func span[T any](items []T, page, perPage int) ([]T, *int, *int) {
	if len(items) == 0 {
		return nonNil(items), nil, nil
	}
	from := (page-1)*perPage + 1
	return items, ref(from), ref(from + len(items) - 1)
}

// nonNil is items, or an empty list for none, so that they go out as []
// rather than null, as the page's JSON has them from Laravel.
func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

func ref[T any](v T) *T { return &v }
