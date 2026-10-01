// Package devtools is the server's side of Inertia's DevTools, a panel of
// the browser's own, as an extension, that records each visit of an
// Inertia app: what the client did, paired with an entry of what the
// server says of each response, by the protocol at
// https://inertiajs.com/docs/v3/advanced/devtools-protocol, whose
// reference is inertia-laravel's.
//
// Under tug dev, tug's App answers each request through a Recorder, which
// keeps an entry of it, and serves the entries to the panel. Package
// inertia records the page it renders, and tug the route and where the
// handler rendered the page, in the Recording the request's context
// carries, neither importing the other.
package devtools

import (
	"context"
	"mime/multipart"
	"sync"
)

// The headers of the protocol: the extension's on a request, and the
// Recorder's on each response.
const (
	HeaderID        = "X-Inertia-Devtools-Id"         // the response's entry
	HeaderParentOut = "X-Inertia-Devtools-Parent-Out" // the batch the next request of it carries
	HeaderParent    = "X-Inertia-Devtools-Parent"     // the batch a request is of
	HeaderTab       = "X-Inertia-Devtools-Tab"        // the browser's tab
	HeaderVisit     = "X-Inertia-Devtools-Visit"      // the client's visit
	HeaderDeferred  = "X-Inertia-Devtools-Deferred"   // a deferred prop's fetch
	HeaderPoll      = "X-Inertia-Devtools-Poll"       // a poll's
)

// A Recording is what's recorded of a request as it's answered, beside
// what the Recorder sees of the request and response themselves: the page
// package inertia rendered, the route tug's router matched, where the
// handler rendered the page, and the form it read.
type Recording struct {
	id string

	mu           sync.Mutex
	component    string
	page         any // the page object, as the client got it
	props        map[string]Prop
	values       map[string]any
	route        *Route
	renderSource *Source
	form         *multipart.Form
}

type key struct{}

// With returns ctx carrying rec, for the request it records.
func With(ctx context.Context, rec *Recording) context.Context {
	return context.WithValue(ctx, key{}, rec)
}

// From returns the Recording of the request whose context is ctx, or nil
// when it isn't recorded: outside tug dev.
func From(ctx context.Context) *Recording {
	rec, _ := ctx.Value(key{}).(*Recording)
	return rec
}

// ID is the request's entry's, for the tag a first visit's page carries.
func (rec *Recording) ID() string { return rec.id }

// Page records the page package inertia rendered: its component, the page
// object as the client got it, as JSON reads, each prop's metadata, by its
// path, and the value of each of those props, as JSON reads too.
func (rec *Recording) Page(component string, page any, props map[string]Prop, values map[string]any) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.component, rec.page, rec.props, rec.values = component, page, props, values
}

// Answered records the route that answered.
func (rec *Recording) Answered(route Route) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.route = &route
}

// Rendered records where the handler rendered the page.
func (rec *Recording) Rendered(at Source) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.renderSource = &at
}

// Form records the multipart form the handler read, which the entry says
// as its fields, and its files by their names, sizes and types.
func (rec *Recording) Form(form *multipart.Form) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.form = form
}

// Source is a place in the app's Go.
type Source struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

// Prop is what an entry says of a prop, as the protocol has it.
type Prop struct {
	Shared bool `json:"shared"`

	// InertiaType is its Inertia type, "always", "defer", "optional",
	// "merge", "scroll" or "once", or null, for a prop of none.
	InertiaType *string `json:"inertiaType"`

	DeferGroup     string  `json:"deferGroup,omitempty"`
	ShareSource    *Source `json:"shareSource,omitempty"`
	Reset          bool    `json:"reset,omitempty"`
	Once           bool    `json:"once,omitempty"`
	MergeDirection string  `json:"mergeDirection,omitempty"` // "append" or "prepend"
	DeepMerge      bool    `json:"deepMerge,omitempty"`
	Rescued        bool    `json:"rescued,omitempty"`
}

// Route is the route that answered, as an entry says it. Its action is the
// function of the app's that answered, and where that's defined, which
// the Recorder sees as the response's status is written; a Route's own
// are for a response tug wrote for the app, as an error's.
type Route struct {
	Name         *string `json:"name"`
	URI          string  `json:"uri"`
	Action       *string `json:"action"`
	ActionSource *Source `json:"actionSource,omitempty"`
}
