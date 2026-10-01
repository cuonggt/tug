// Package route keeps the route that answered a request in its context:
// tug's App puts a place for it there as the request comes in, before any
// middleware, its router fills the place in as a route answers, and
// tug.RouteOf and package middleware's Logger read it once the handler has
// run, whatever copies of the request the middleware in between made.
// Package middleware doesn't import tug.
package route

import (
	"context"
	"sync/atomic"
)

type key struct{}

// place is a request's context with a place for the route that answers
// it, in one allocation. The router writes it and middleware reads it,
// maybe on another goroutine, as behind http.TimeoutHandler.
type place struct {
	context.Context
	route atomic.Pointer[string]
}

func (p *place) Value(k any) any {
	if k == (key{}) {
		return p
	}
	return p.Context.Value(k)
}

// Into returns ctx with an empty place for the route that answers its
// request.
func Into(ctx context.Context) context.Context {
	return &place{Context: ctx}
}

// Answered records route as the one that answered the request whose
// context is ctx, when ctx has a place for it.
func Answered(ctx context.Context, route *string) {
	if p, ok := ctx.Value(key{}).(*place); ok {
		p.route.Store(route)
	}
}

// Of returns the route that answered the request whose context is ctx, ""
// while none has, and whether ctx has a place for one.
func Of(ctx context.Context) (string, bool) {
	p, ok := ctx.Value(key{}).(*place)
	if !ok {
		return "", false
	}
	if r := p.route.Load(); r != nil {
		return *r, true
	}
	return "", true
}
