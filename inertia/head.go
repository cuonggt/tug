package inertia

import (
	"context"
	"html/template"
	"slices"
	"strings"
)

// A HeadElement is an element of a page's <head>: its title, a meta tag or
// a link, which Title, Meta, Property and Link make. A page's elements go
// out as its head prop, which Inertia's client, with its serverHead option
// on, puts in the document's head as it shows the page. They're in the
// HTML of a first visit too, for what reads a page without running its
// scripts: a search engine's crawler, or the app that makes a link's
// preview. WithHead gives a page its elements.
type HeadElement struct {
	tag   string      // "title", "meta" or "link"
	key   string      // its data-inertia, which the client matches elements by
	text  string      // a title's
	attrs [][2]string // a meta's or a link's, in order
}

// Title is the page's title. The client says it through its title
// callback, as it says a page's <Head title>, and Config.Title says it the
// same way in a first visit's HTML.
func Title(text string) HeadElement {
	return HeadElement{tag: "title", key: "title", text: text}
}

// Meta is a meta tag by its name, as the page's description:
//
//	inertia.Meta("description", post.Summary)
func Meta(name, content string) HeadElement {
	return HeadElement{tag: "meta", key: name, attrs: [][2]string{{"name", name}, {"content", content}}}
}

// Property is a meta tag by its property, as Open Graph's, which a link's
// preview is made from:
//
//	inertia.Property("og:image", post.ImageURL)
func Property(property, content string) HeadElement {
	return HeadElement{tag: "meta", key: property, attrs: [][2]string{{"property", property}, {"content", content}}}
}

// Link is a link tag by its rel, as the page's canonical address:
//
//	inertia.Link("canonical", "https://example.com/posts/1")
func Link(rel, href string) HeadElement {
	return HeadElement{tag: "link", key: rel, attrs: [][2]string{{"rel", rel}, {"href", href}}}
}

// Key gives the element a key of its own, in place of its name, its
// property or its rel, which an element is keyed by: one replaces the one
// before it of the same key, and the client matches them by it from page
// to page, so a second og:image needs a key of its own. A page's own
// <Head> element with the same head-key wins over it.
func (e HeadElement) Key(key string) HeadElement {
	e.key = key
	return e
}

// html is the element as the client takes it, its key first, as the
// client writes one, and each value escaped, as html/template escapes
// text: the client puts it in the document as HTML.
func (e HeadElement) html() string {
	var b strings.Builder
	b.WriteString("<" + e.tag + ` data-inertia="` + template.HTMLEscapeString(e.key) + `"`)
	for _, attr := range e.attrs {
		b.WriteString(" " + attr[0] + `="` + template.HTMLEscapeString(attr[1]) + `"`)
	}
	b.WriteString(">")
	if e.tag == "title" {
		b.WriteString(template.HTMLEscapeString(e.text) + "</title>")
	}
	return b.String()
}

// WithHead returns a context whose page has head in its <head>, after the
// elements the context had: an element replaces the one before it of the
// same key, where it was. It's for middleware, as one that gives every
// page the site's image, and for a handler of any router; a tug handler
// has Ctx.Head.
func WithHead(ctx context.Context, head ...HeadElement) context.Context {
	merged := slices.Clone(headFrom(ctx))
	for _, e := range head {
		if i := slices.IndexFunc(merged, func(m HeadElement) bool { return m.key == e.key }); i >= 0 {
			merged[i] = e
		} else {
			merged = append(merged, e)
		}
	}
	return context.WithValue(ctx, headKey, merged)
}

func headFrom(ctx context.Context) []HeadElement {
	head, _ := ctx.Value(headKey).([]HeadElement)
	return head
}

// headProp is head as the head prop has it, which the client reads: each
// element's HTML, its title as it was given, for the client's title
// callback to say.
func headProp(head []HeadElement) []string {
	out := make([]string, len(head))
	for n, e := range head {
		out[n] = e.html()
	}
	return out
}

// headHTML is head as a first visit's HTML has it, its title said by
// title, as the client's callback says it in the browser, where it finds
// the other elements by their keys and leaves them be. The title has no
// key: React's and Vue's adapters replace a title without one with their
// own, the same, and Svelte sets the document's title in the first one
// there, which its adapter's head would take away from it were it keyed.
func headHTML(head []HeadElement, title func(string) string) template.HTML {
	out := make([]string, len(head))
	for n, e := range head {
		if e.tag != "title" {
			out[n] = e.html()
			continue
		}
		text := e.text
		if title != nil {
			text = title(text)
		}
		out[n] = "<title>" + template.HTMLEscapeString(text) + "</title>"
	}
	return template.HTML(strings.Join(out, "\n"))
}
