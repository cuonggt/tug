package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/cuonggt/tug/internal/rw"
)

// Recover answers a panic with a 500 and logs it with its stack through
// slog.Default(), so one bad request costs no more than itself.
//
// A tug app already recovers its own handlers, http.Handlers on its routes
// included, and answers through its ErrorHandler. Recover is for the rest:
// panics in middleware, and in handlers served by another router.
func Recover() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := rw.New(w)
			defer func() {
				v := recover()
				if v == nil {
					return
				}
				if v == http.ErrAbortHandler {
					panic(v) // net/http's way of dropping a response on purpose
				}
				slog.ErrorContext(r.Context(), "panic",
					"method", r.Method, "path", r.URL.Path, "err", v, "stack", string(debug.Stack()))
				if !rec.Written() {
					http.Error(rec, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(rec, r)
		})
	}
}
