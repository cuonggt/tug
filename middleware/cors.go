package middleware

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
)

// CORS lets the pages of the sites named call the app from the browser,
// as another site's pages call an API: each an origin, as
// "https://app.example.com", or "*" for any. A request from one of them
// gets Access-Control-Allow-Origin, and the browser's preflight, an
// OPTIONS that asks first, is answered here, 204, with the methods and
// headers it asks for, before any route: an API's routes have no OPTIONS
// of their own. A request from another site passes as it came, and its
// browser keeps the response from its page.
//
// Credentials, the app's cookies, are never allowed: an API another
// site's pages call takes tokens, which a page sends itself, as
// Authorization: Bearer, so a page of another site can't borrow a login.
// Retry-After is exposed, for a page to read how long a limit's 429 says
// to wait.
//
// A blank origin is skipped, so a list split from an empty setting allows
// none, and CORS panics on one that isn't an origin: a mistyped setting
// should stop the app at startup.
func CORS(origins ...string) func(http.Handler) http.Handler {
	var allowed []string
	anyOrigin := false
	for _, o := range origins {
		switch o = strings.TrimSpace(o); {
		case o == "":
		case o == "*":
			anyOrigin = true
		case !isOrigin(o):
			panic("middleware: CORS: " + `"` + o + `" isn't an origin, as https://app.example.com`)
		default:
			allowed = append(allowed, o)
		}
	}
	if !anyOrigin && len(allowed) == 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			if !anyOrigin {
				// The response differs by who asks, which a cache mustn't
				// hand one site as another's.
				h.Add("Vary", "Origin")
			}
			origin := r.Header.Get("Origin")
			if origin == "" || !anyOrigin && !slices.Contains(allowed, origin) {
				next.ServeHTTP(w, r)
				return
			}
			allow := origin
			if anyOrigin {
				allow = "*"
			}
			h.Set("Access-Control-Allow-Origin", allow)
			method := r.Header.Get("Access-Control-Request-Method")
			if r.Method != http.MethodOptions || method == "" {
				h.Set("Access-Control-Expose-Headers", "Retry-After")
				next.ServeHTTP(w, r)
				return
			}
			h.Set("Access-Control-Allow-Methods", method)
			if headers := r.Header.Get("Access-Control-Request-Headers"); headers != "" {
				h.Set("Access-Control-Allow-Headers", headers)
			}
			// Two hours, the most Chrome keeps a preflight's answer.
			h.Set("Access-Control-Max-Age", "7200")
			h.Add("Vary", "Access-Control-Request-Method")
			h.Add("Vary", "Access-Control-Request-Headers")
			w.WriteHeader(http.StatusNoContent)
		})
	}
}

// isOrigin reports whether s is an origin: a scheme and a host, and
// nothing after them, as a browser sends one.
func isOrigin(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" &&
		u.Path == "" && u.RawQuery == "" && u.Fragment == "" && u.User == nil && s == u.Scheme+"://"+u.Host
}
