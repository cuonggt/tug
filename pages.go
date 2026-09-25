package tug

import (
	"cmp"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync"

	"github.com/cuonggt/tug/inertia"
	"github.com/cuonggt/tug/internal/typegen"
)

// Props are a page's props, for a page without a struct of its own.
type Props = inertia.Props

// PageOf is an Inertia page component, and the props it takes. Page
// declares one.
type PageOf[P any] struct{ component string }

// Page declares an Inertia page component, and the props it takes:
//
//	type PostsIndexProps struct {
//		Posts []Post                   `json:"posts"`
//		Stats inertia.DeferProp[Stats] `json:"stats"`
//	}
//
//	var PostsIndex = tug.Page[PostsIndexProps]("Posts/Index")
//
//	func index(c *tug.Ctx) error {
//		return PostsIndex.Render(c, PostsIndexProps{Posts: posts, Stats: inertia.Defer(countWords)})
//	}
//
// Declared this way, a page can only be rendered with the props it takes,
// and tug gen writes their TypeScript, for the page component to take them
// too. A component declared twice with different props panics.
func Page[P any](component string) PageOf[P] {
	declare(component, reflect.TypeFor[P]())
	return PageOf[P]{component: component}
}

// Component returns the page's component name, as "Posts/Index".
func (p PageOf[P]) Component() string { return p.component }

// Render renders the page with props, as Ctx.Inertia does.
func (p PageOf[P]) Render(c *Ctx, props P) error {
	return c.Inertia(p.component, props)
}

// declared are the pages Page has declared, by component.
var declared = struct {
	sync.Mutex
	pages map[string]reflect.Type
}{pages: map[string]reflect.Type{}}

func declare(component string, props reflect.Type) {
	declared.Lock()
	defer declared.Unlock()
	if other, ok := declared.pages[component]; ok && other != props {
		panic(fmt.Sprintf("tug: page %q is declared twice, with props %s and %s", component, other, props))
	}
	declared.pages[component] = props
}

func declaredPages() []typegen.Page {
	declared.Lock()
	defer declared.Unlock()
	var pages []typegen.Page
	for component, props := range declared.pages {
		pages = append(pages, typegen.Page{Component: component, Props: props})
	}
	slices.SortFunc(pages, func(a, b typegen.Page) int { return cmp.Compare(a.Component, b.Component) })
	return pages
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
