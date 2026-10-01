package middleware

import (
	"context"
	"crypto/rand"
	"net/http"
)

// RequestIDHeader carries a request's ID, in and out.
const RequestIDHeader = "X-Request-ID"

type requestIDKey struct{}

// RequestID gives each request an ID: the X-Request-ID it came with, as a
// proxy in front may set one, or else a new random one. The ID goes back in
// the response's X-Request-ID and into the request's context, where Logger
// and RequestIDFrom find it, so a log line and a person's report of an
// error can be matched up.
func RequestID() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(RequestIDHeader)
			if !plainID(id) {
				id = rand.Text()
			}
			w.Header().Set(RequestIDHeader, id)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
		})
	}
}

// RequestIDFrom returns the ID RequestID put in ctx, or "" when it didn't.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// WithRequestID returns ctx with id as the ID of the request it's from, as
// RequestID puts one, for RequestIDFrom to read: in a job a request
// pushed, say.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// CarryRequestID is a queue.Carrier of the ID RequestID gives a request,
// by the name request_id: a job the request pushes, through a queue whose
// Config.Carry has it, takes the ID, and RequestIDFrom reads it in the
// job's run, as the queue's log lines of the job say it, so a job's lines
// and its request's are matched up.
var CarryRequestID carryRequestID

type carryRequestID struct{}

func (carryRequestID) Carry(ctx context.Context, into map[string]string) {
	if id := RequestIDFrom(ctx); id != "" {
		into["request_id"] = id
	}
}

// Restore takes the ID back only when it's as plain as RequestID keeps one,
// as it ends up in logs, and in the headers of what the job sends on.
func (carryRequestID) Restore(ctx context.Context, from map[string]string) context.Context {
	if id := from["request_id"]; plainID(id) {
		return WithRequestID(ctx, id)
	}
	return ctx
}

// plainID reports whether an ID from outside is short and plain enough to
// keep. It ends up in logs and response headers, so anything else is
// replaced rather than trusted.
func plainID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, c := range []byte(id) {
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		case c == '-', c == '_', c == '.', c == ':', c == '+', c == '/', c == '=':
		default:
			return false
		}
	}
	return true
}
