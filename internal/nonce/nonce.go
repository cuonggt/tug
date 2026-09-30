// Package nonce carries a response's Content-Security-Policy nonce in its
// request's context: package middleware makes it, and package inertia
// hands it to the root template, and neither imports the other.
package nonce

import "context"

type key struct{}

// With returns a context that carries n as the response's nonce.
func With(ctx context.Context, n string) context.Context {
	return context.WithValue(ctx, key{}, n)
}

// From returns the nonce in ctx, or "" when there's none.
func From(ctx context.Context) string {
	n, _ := ctx.Value(key{}).(string)
	return n
}
