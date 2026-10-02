// Command inertia is tug's example of an Inertia app: posts, shown by React
// pages that get their props from Go handlers, and written with forms that
// check themselves as they're filled in. The list's stats are a deferred
// prop, so the list shows before they're counted. A post made, changed or
// deleted in one browser shows in the others, which hear of it on the
// posts channel, a broadcast.Hub's, and reload what they show of it.
//
// With the Vite dev server, which reloads the pages as they change:
//
//	npm install
//	npm run dev                          # writes public/hot while it runs
//	ADDR=127.0.0.1:8080 go run .         # in another terminal
//
// With the frontend built into the binary:
//
//	npm run build && go build && ADDR=127.0.0.1:8080 ./inertia
//
// Sessions are encrypted with APP_KEY; without one, the example makes up a
// key, and its sessions end when it stops.
package main

import (
	"cmp"
	"context"
	"crypto/rand"
	"embed"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cuonggt/tug"
	"github.com/cuonggt/tug/broadcast"
	"github.com/cuonggt/tug/inertia"
	"github.com/cuonggt/tug/middleware"
	"github.com/cuonggt/tug/session"
	"github.com/cuonggt/tug/validate"
	"github.com/cuonggt/tug/vite"
)

//go:embed app.html
var rootTemplate string

// public holds what `npm run build` writes to public/build. The .gitkeep
// lets it compile before the first build.
//
//go:embed all:public
var public embed.FS

func main() {
	build, err := fs.Sub(public, "public/build")
	if err != nil {
		log.Fatal(err)
	}
	keys, err := session.KeysFromEnv()
	if errors.Is(err, session.ErrNoKey) {
		// Fine for trying the example out. A real app stops here instead:
		// with a key made up at each start, a restart signs everyone out.
		slog.Warn("APP_KEY isn't set, so sessions end when the server stops")
		key := make([]byte, 32)
		rand.Read(key)
		keys = [][]byte{key}
	} else if err != nil {
		log.Fatal(err)
	}
	app, err := newApp(tug.ConfigFromEnv(), build, "public/hot", keys, 400*time.Millisecond)
	if err != nil {
		log.Fatal(err)
	}
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// newApp puts the example together. countTime is how long counting the
// stats takes, standing in for a slow query.
func newApp(cfg tug.Config, build fs.FS, hotFile string, keys [][]byte, countTime time.Duration) (*tug.App, error) {
	assets, err := vite.New(vite.Config{Build: build, HotFile: hotFile})
	if err != nil {
		return nil, err
	}
	pages, err := inertia.New(inertia.Config{
		Template: rootTemplate,
		Funcs:    assets.Funcs(),
		Version:  assets.Version(),
		// A page's title from its handler, said in a first visit's HTML as
		// app.tsx's title callback says it in the browser.
		Title: func(title string) string { return title + " · tug" },
	})
	if err != nil {
		return nil, err
	}
	pages.Share("appName", "tug")
	sessions, err := session.New(session.Config{Keys: keys})
	if err != nil {
		return nil, err
	}

	cfg.Inertia = pages
	cfg.Session = sessions
	cfg.ErrorPage = "Error"
	app := tug.New(cfg)
	// Each response says what a browser may do with it, and a page, which
	// scripts it may run: those that carry its nonce, as app.html's do, and
	// what they load, with the dev server's while it runs.
	app.Use(
		middleware.RequestID(), middleware.Logger(), middleware.Recover(),
		middleware.Headers(middleware.HeadersConfig{HSTS: strings.HasPrefix(cfg.URL, "https://")}),
		middleware.CSP(middleware.CSPConfig{ReportPath: "/csp-reports", DevServer: assets.DevServer}),
		middleware.CSRF(),
	)
	app.Get("/build/{path...}", tug.WrapHandler(assets))

	// The events go to the pages open on this instance alone, as the posts
	// are kept in its memory: an app of several gives the hub a Store, its
	// database, and runs it with app.Go(hub.Run).
	p := newPosts(countTime, &broadcast.Hub{})
	app.Get("/", p.index).Name("posts.index")
	app.Get("/posts", p.archive).Name("posts.archive")
	app.Get("/posts.csv", p.export).Name("posts.export")
	app.Get("/posts/events", p.events).Name("posts.events")
	app.Get("/posts/create", p.create).Name("posts.create")
	app.Post("/posts", p.store).Name("posts.store").Takes(PostInput{})
	app.Get("/posts/{id}", p.show).Name("posts.show")
	app.Get("/posts/{id}/edit", p.edit).Name("posts.edit")
	app.Put("/posts/{id}", p.update).Name("posts.update").Takes(PostInput{})
	app.Delete("/posts/{id}", p.destroy).Name("posts.destroy")
	return app, nil
}

type Post struct {
	ID    int64    `json:"id"`
	Title string   `json:"title"`
	Body  string   `json:"body"`
	Tags  []string `json:"tags"`
}

type Stats struct {
	Posts int `json:"posts"`
	Words int `json:"words"`
}

// The pages, and the props each takes, whose TypeScript tug gen writes in
// resources/js/tug.

type PostsIndexProps struct {
	Posts inertia.ScrollProp[Post] `json:"posts"` // a page at a time, as the list scrolls
	Stats inertia.DeferProp[Stats] `json:"stats"`
}

var PostsIndex = tug.Page[PostsIndexProps]("Posts/Index")

// Success is the flash data a post's handler leaves for the next page, a
// message for its banner, whose TypeScript, FlashData, tug gen writes too.
var Success = tug.Flash[string]("success")

type PostsArchiveProps struct {
	Posts tug.Paginated[Post] `json:"posts"` // a page by its number, with the links of a pager
}

var PostsArchive = tug.Page[PostsArchiveProps]("Posts/Archive")

type PostsShowProps struct {
	Post Post `json:"post"`
}

var PostsShow = tug.Page[PostsShowProps]("Posts/Show")

type PostsCreateProps struct{}

var PostsCreate = tug.Page[PostsCreateProps]("Posts/Create")

type PostsEditProps struct {
	Post Post `json:"post"`
}

var PostsEdit = tug.Page[PostsEditProps]("Posts/Edit")

// PostInput is what the post form sends.
type PostInput struct {
	Title string `json:"title" validate:"required,max=80"`
	Body  string `json:"body" validate:"required,min=10"`
	Tags  string `json:"tags" validate:"max=100"` // comma separated
}

func (in PostInput) post(id int64) Post {
	var tags []string
	for tag := range strings.SplitSeq(in.Tags, ",") {
		if tag = strings.TrimSpace(tag); tag != "" && !slices.Contains(tags, tag) {
			tags = append(tags, tag)
		}
	}
	return Post{ID: id, Title: strings.TrimSpace(in.Title), Body: in.Body, Tags: tags}
}

// posts keeps the posts in memory, and tells the pages open of each
// change, on hub's posts channel.
type posts struct {
	mu        sync.Mutex
	byID      map[int64]Post
	last      int64
	countTime time.Duration
	hub       *broadcast.Hub
}

func newPosts(countTime time.Duration, hub *broadcast.Hub) *posts {
	p := &posts{byID: make(map[int64]Post), countTime: countTime, hub: hub}
	for _, post := range []Post{
		{ID: 1, Title: "Hello, tug", Body: "Pages rendered by React, with props from Go handlers.", Tags: []string{"go", "inertia"}},
		{ID: 2, Title: "No API in between", Body: "A handler returns its page and props, and Inertia does the rest.", Tags: []string{"react"}},
		// No tags: a nil slice, which still reaches the page as [].
		{ID: 3, Title: "Delete me", Body: "This post is here to be deleted."},
	} {
		p.byID[post.ID] = post
		p.last = post.ID
	}
	// Enough more for the list to scroll through, a page at a time.
	for p.last < 25 {
		p.last++
		p.byID[p.last] = Post{ID: p.last, Title: fmt.Sprintf("Post %d", p.last), Body: "One of many, to scroll through."}
	}
	return p
}

func (p *posts) list() []Post {
	p.mu.Lock()
	defer p.mu.Unlock()
	list := slices.Collect(maps.Values(p.byID))
	slices.SortFunc(list, func(a, b Post) int { return cmp.Compare(a.ID, b.ID) })
	return list
}

// total is how many posts there are, for the list in numbered pages.
func (p *posts) total() (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.byID), nil
}

// page is the posts in order from offset, limit of them at most, as a
// query's LIMIT and OFFSET would read them from a table.
func (p *posts) page(limit, offset int) ([]Post, error) {
	list := p.list()
	from := min(offset, len(list))
	return list[from:min(from+limit, len(list))], nil
}

func (p *posts) count() (Stats, error) {
	time.Sleep(p.countTime)
	var s Stats
	for _, post := range p.list() {
		s.Posts++
		s.Words += len(strings.Fields(post.Body))
	}
	return s, nil
}

// titleFree is a check that no post but the one with id has in's title.
func (p *posts) titleFree(in *PostInput, id int64) func(validate.Errors) {
	return func(errs validate.Errors) {
		p.mu.Lock()
		defer p.mu.Unlock()
		for _, post := range p.byID {
			if post.ID != id && strings.EqualFold(post.Title, strings.TrimSpace(in.Title)) {
				errs.Add("title", "another post has that title")
			}
		}
	}
}

// perPage is how many posts a page of the list has.
const perPage = 10

func (p *posts) index(c *tug.Ctx) error {
	return PostsIndex.Render(c, PostsIndexProps{
		// The page ?page asks for, as the archive's is, and where it sits,
		// for the list to ask for the next as it scrolls.
		Posts: inertia.Scroll(func() ([]Post, inertia.Paging, error) {
			page, err := tug.Paginate(c, perPage, p.total, p.page)
			return page.Data, page.Paging(), err
		}).MatchOn("id"),
		Stats: inertia.Defer(p.count),
	})
}

// archive is every post, in numbered pages, with a pager.
func (p *posts) archive(c *tug.Ctx) error {
	page, err := tug.Paginate(c, perPage, p.total, p.page)
	if err != nil {
		return err
	}
	return PostsArchive.Render(c, PostsArchiveProps{Posts: page})
}

// export is every post as CSV, for a spreadsheet, written a row at a time
// as the rows would come from a query, rather than all at once.
func (p *posts) export(c *tug.Ctx) error {
	return c.StreamDownload("posts.csv", "text/csv; charset=utf-8", func(w io.Writer) error {
		out := csv.NewWriter(w)
		out.Write([]string{"id", "title", "tags"})
		for _, post := range p.list() {
			out.Write([]string{strconv.FormatInt(post.ID, 10), cell(post.Title), cell(strings.Join(post.Tags, ", "))})
		}
		out.Flush()
		return out.Error()
	})
}

// cell is what someone wrote, as a spreadsheet's cell: one that starts as a
// formula does, as =HYPERLINK(...), would run as one when the file is
// opened, and a quote before it keeps it text.
func cell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

func (p *posts) create(c *tug.Ctx) error {
	return PostsCreate.Render(c, PostsCreateProps{})
}

func (p *posts) store(c *tug.Ctx) error {
	var in PostInput
	if err := c.BindValid(&in, p.titleFree(&in, 0)); err != nil {
		return err
	}
	p.mu.Lock()
	p.last++
	post := in.post(p.last)
	p.byID[post.ID] = post
	p.mu.Unlock()

	p.changed(c, "created", post.ID)
	Success.Set(c, "Post created")
	return c.RedirectRoute("posts.show", post.ID)
}

// postID is the {id} of a post's URL.
type postID struct {
	ID int64 `path:"id"`
}

func (p *posts) find(c *tug.Ctx) (Post, error) {
	var in postID
	if err := c.Bind(&in); err != nil {
		return Post{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	post, ok := p.byID[in.ID]
	if !ok {
		return Post{}, tug.NewHTTPError(http.StatusNotFound, "post not found")
	}
	return post, nil
}

func (p *posts) show(c *tug.Ctx) error {
	post, err := p.find(c)
	if err != nil {
		return err
	}
	// The post's title and words, in the first visit's HTML too, for a
	// search engine, and a link's preview.
	c.Head(inertia.Title(post.Title), inertia.Meta("description", post.Body))
	return PostsShow.Render(c, PostsShowProps{Post: post})
}

func (p *posts) edit(c *tug.Ctx) error {
	post, err := p.find(c)
	if err != nil {
		return err
	}
	return PostsEdit.Render(c, PostsEditProps{Post: post})
}

func (p *posts) update(c *tug.Ctx) error {
	post, err := p.find(c)
	if err != nil {
		return err
	}
	var in PostInput
	if err := c.BindValid(&in, p.titleFree(&in, post.ID)); err != nil {
		return err
	}
	p.mu.Lock()
	p.byID[post.ID] = in.post(post.ID)
	p.mu.Unlock()

	p.changed(c, "updated", post.ID)
	Success.Set(c, "Post updated")
	return c.RedirectRoute("posts.show", post.ID)
}

func (p *posts) destroy(c *tug.Ctx) error {
	post, err := p.find(c)
	if err != nil {
		return err
	}
	p.mu.Lock()
	delete(p.byID, post.ID)
	p.mu.Unlock()

	p.changed(c, "deleted", post.ID)
	Success.Set(c, "Post deleted")
	return c.RedirectRoute("posts.index")
}

// changed tells the pages open that the post with id was created, updated
// or deleted, for them to reload what they show of it. The event says what
// changed, not the post, which a page asks for as it reloads.
func (p *posts) changed(c *tug.Ctx, what string, id int64) {
	if err := p.hub.Publish(c.Context(), "posts", what, map[string]int64{"id": id}); err != nil {
		slog.WarnContext(c.Context(), "the pages open weren't told of a post", "post", id, "err", err)
	}
}

// events is the posts channel, as server-sent events, which the pages
// follow with EventSource. A page that falls behind has its stream ended,
// and connects again, reloading what it shows.
func (p *posts) events(c *tug.Ctx) error {
	return c.Events(func(ctx context.Context, send func(tug.Event) error) error {
		for e := range p.hub.Subscribe(ctx, "posts") {
			if err := send(tug.Event{Name: e.Name, Data: e.Data}); err != nil {
				return err
			}
		}
		return nil
	})
}
