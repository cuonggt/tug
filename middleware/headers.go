package middleware

import "net/http"

// HeadersConfig is what Headers sends beyond what every app wants.
type HeadersConfig struct {
	// HSTS sends Strict-Transport-Security, so a browser that has reached
	// the app over HTTPS goes on doing so for a year, whatever a link or a
	// person types, and a network in between can't hand it a page over
	// plain HTTP. Set it when the app is served over HTTPS, as an https://
	// APP_URL says: behind a proxy that ends TLS, the requests themselves
	// come over plain HTTP. It covers the app's host alone, and isn't
	// preloaded: both are promises about more than the app, slow to take
	// back.
	HSTS bool
}

// Headers sets the headers that say what a browser may do with the app's
// responses, on every one:
//
//   - X-Content-Type-Options: nosniff, so a file is only ever the type it's
//     sent as, and an upload that looks like a script isn't run as one.
//   - Referrer-Policy: strict-origin-when-cross-origin, so a link to another
//     site tells it which site it came from, and not the page's whole URL,
//     with whatever is in its path and query.
//   - Cross-Origin-Opener-Policy: same-origin, so a window of another site's
//     that a page opens, or that opens one of the app's pages, can't reach
//     into it.
//   - X-Frame-Options: SAMEORIGIN, so another site can't put a page in a
//     frame, under a page of its own, to have a person click what they
//     can't see. CSP's policy says so too, with frame-ancestors, but only
//     once it's enforced.
//   - With HSTS, Strict-Transport-Security, for a year.
//
// A handler whose response needs another, as a page another site frames,
// sets its own, or deletes one.
func Headers(cfg HeadersConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Cross-Origin-Opener-Policy", "same-origin")
			h.Set("X-Frame-Options", "SAMEORIGIN")
			if cfg.HSTS {
				h.Set("Strict-Transport-Security", "max-age=31536000")
			}
			next.ServeHTTP(w, r)
		})
	}
}
