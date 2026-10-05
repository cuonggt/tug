// Package page is the page every app in the benchmarks serves: a post and
// its comments, as Inertia's Posts/Show. Its props are in page.json, which
// each app, in whatever language, reads once as it starts, so they're the
// same in all of them.
package page

import (
	_ "embed"
	"encoding/json"
)

// JSON is page.json.
//
//go:embed page.json
var JSON []byte

// Component is the page's component.
const Component = "Posts/Show"

// Props are the page's props.
type Props struct {
	Post     Post      `json:"post"`
	Comments []Comment `json:"comments"`
}

type Post struct {
	ID          int      `json:"id"`
	Title       string   `json:"title"`
	Body        string   `json:"body"`
	Author      Author   `json:"author"`
	Tags        []string `json:"tags"`
	PublishedAt string   `json:"published_at"`
}

type Author struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Comment struct {
	ID     int    `json:"id"`
	Author string `json:"author"`
	Body   string `json:"body"`
}

var props = func() Props {
	var p Props
	if err := json.Unmarshal(JSON, &p); err != nil {
		panic("page: page.json: " + err.Error())
	}
	return p
}()

// For is the page's props for the post with the ID id: page.json's, with
// the post's ID set, as every app sets it from the route.
func For(id int) Props {
	p := props
	p.Post.ID = id
	return p
}
