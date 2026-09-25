// Package middleware is net/http middleware for tug apps. Each one is a
// plain func(http.Handler) http.Handler, so it works with tug's Use and
// Group and with any other router.
//
// Order matters, and this is the usual one:
//
//	app.Use(middleware.RequestID(), middleware.Logger(), middleware.Recover(), middleware.CSRF())
//
// RequestID first, so every log line carries the ID; Logger before Recover,
// so a panic is logged as the 500 that Recover makes of it.
package middleware
