package tug

import (
	"errors"

	"github.com/cuonggt/tug/inertia"
)

// Props are a page's props, for a page without a struct of its own.
type Props = inertia.Props

// Page is an Inertia page component, and the props it takes:
//
//	type PostsIndexProps struct {
//		Posts []Post                      `json:"posts"`
//		Stats inertia.DeferProp[Stats]    `json:"stats"`
//	}
//
//	var PostsIndex = tug.Page[PostsIndexProps]("Posts/Index")
//
//	func index(c *tug.Ctx) error {
//		return PostsIndex.Render(c, PostsIndexProps{Posts: posts, Stats: inertia.Defer(countWords)})
//	}
//
// Declared this way, a page can only be rendered with the props it takes,
// and the compiler says so when it isn't.
type Page[P any] string

// Render renders the page with props, as Ctx.Inertia does.
func (p Page[P]) Render(c *Ctx, props P) error {
	return c.Inertia(string(p), props)
}

// Inertia renders an Inertia page component with props: an HTML page for a
// first visit, and the page object as JSON for a visit from Inertia's
// client. props is a struct or a map; see inertia.Inertia.Render. The app
// needs Config.Inertia.
func (c *Ctx) Inertia(component string, props any) error {
	pages := c.app.config.Inertia
	if pages == nil {
		return errors.New("tug: rendering an Inertia page needs Config.Inertia")
	}
	return pages.Render(&c.rw, c.pageRequest(), component, props)
}

// Location sends the client to url with a full page load, which is how an
// Inertia app leaves for another site: see inertia.Location.
func (c *Ctx) Location(url string) error {
	inertia.Location(&c.rw, c.r, url)
	return nil
}
