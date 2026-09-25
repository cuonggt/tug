package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/cuonggt/tug/internal/rw"
)

// Logger logs a line for each request through slog.Default(): its method,
// path, status, size and duration, and its ID when RequestID ran before.
// A 5xx logs at Error, a 4xx at Warn, and the rest at Info.
//
// The path is logged without its query string, which can carry tokens.
func Logger() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := rw.New(w)
			next.ServeHTTP(rec, r)

			status := rec.Status()
			if status == 0 {
				status = http.StatusOK // what net/http sends for a handler that wrote nothing
			}
			level := slog.LevelInfo
			switch {
			case status >= 500:
				level = slog.LevelError
			case status >= 400:
				level = slog.LevelWarn
			}
			attrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", status),
				slog.Int64("size", rec.Size()),
				slog.Duration("duration", time.Since(start)),
			}
			if id := RequestIDFrom(r.Context()); id != "" {
				attrs = append(attrs, slog.String("request_id", id))
			}
			slog.LogAttrs(r.Context(), level, "request", attrs...)
		})
	}
}
