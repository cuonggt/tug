// Command inertia is tug's example of an Inertia app: posts, shown by React
// pages that get their props from Go handlers. The list's stats are a
// deferred prop, so the list shows before they're counted.
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
package main

import (
	"cmp"
	"embed"
	"io/fs"
	"log"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cuonggt/tug"
	"github.com/cuonggt/tug/inertia"
	"github.com/cuonggt/tug/middleware"
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
	app, err := newApp(tug.ConfigFromEnv(), build, "public/hot", 400*time.Millisecond)
	if err != nil {
		log.Fatal(err)
	}
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// newApp puts the example together. countTime is how long counting the
// stats takes, standing in for a slow query.
func newApp(cfg tug.Config, build fs.FS, hotFile string, countTime time.Duration) (*tug.App, error) {
	assets, err := vite.New(vite.Config{Build: build, HotFile: hotFile})
	if err != nil {
		return nil, err
	}
	pages, err := inertia.New(inertia.Config{
		Template: rootTemplate,
		Funcs:    assets.Funcs(),
		Version:  assets.Version(),
	})
	if err != nil {
		return nil, err
	}
	pages.Share("appName", "tug")

	cfg.Inertia = pages
	app := tug.New(cfg)
	app.Use(middleware.RequestID(), middleware.Logger(), middleware.Recover(), middleware.CSRF())
	app.Get("/build/{path...}", tug.WrapHandler(assets))

	p := newPosts(countTime)
	app.Get("/", p.index).Name("posts.index")
	app.Get("/posts/{id}", p.show).Name("posts.show")
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

// The pages, and the props each takes. resources/js/types.ts has the same
// types in TypeScript.

type PostsIndexProps struct {
	Posts []Post                   `json:"posts"`
	Stats inertia.DeferProp[Stats] `json:"stats"`
}

var PostsIndex = tug.Page[PostsIndexProps]("Posts/Index")

type PostsShowProps struct {
	Post Post `json:"post"`
}

var PostsShow = tug.Page[PostsShowProps]("Posts/Show")

// posts keeps the posts in memory.
type posts struct {
	mu        sync.Mutex
	byID      map[int64]Post
	countTime time.Duration
}

func newPosts(countTime time.Duration) *posts {
	p := &posts{byID: make(map[int64]Post), countTime: countTime}
	for _, post := range []Post{
		{ID: 1, Title: "Hello, tug", Body: "Pages rendered by React, with props from Go handlers.", Tags: []string{"go", "inertia"}},
		{ID: 2, Title: "No API in between", Body: "A handler returns its page and props, and Inertia does the rest.", Tags: []string{"react"}},
		// No tags: a nil slice, which still reaches the page as [].
		{ID: 3, Title: "Delete me", Body: "This post is here to be deleted."},
	} {
		p.byID[post.ID] = post
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

func (p *posts) count() (Stats, error) {
	time.Sleep(p.countTime)
	var s Stats
	for _, post := range p.list() {
		s.Posts++
		s.Words += len(strings.Fields(post.Body))
	}
	return s, nil
}

func (p *posts) index(c *tug.Ctx) error {
	return PostsIndex.Render(c, PostsIndexProps{
		Posts: p.list(),
		Stats: inertia.Defer(p.count),
	})
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
	return PostsShow.Render(c, PostsShowProps{Post: post})
}

func (p *posts) destroy(c *tug.Ctx) error {
	post, err := p.find(c)
	if err != nil {
		return err
	}
	p.mu.Lock()
	delete(p.byID, post.ID)
	p.mu.Unlock()
	return c.RedirectRoute("posts.index")
}
