package middleware

import "net/http"

// CSRF rejects state-changing requests that a browser sent from another
// site, with a 403. It is net/http's CrossOriginProtection, which reads the
// Sec-Fetch-Site header every current browser sends, or else compares
// Origin with Host, so there are no tokens to put in forms or rotate, and
// Inertia's XSRF-TOKEN cookie isn't needed. GET, HEAD and OPTIONS always
// pass, so they must never change anything.
//
// Requests from trustedOrigins, such as "https://admin.example.com", pass
// too. CSRF panics on one that isn't an origin: a mistyped setting should
// stop the app at startup, not surface as 403s later.
func CSRF(trustedOrigins ...string) func(http.Handler) http.Handler {
	p := http.NewCrossOriginProtection()
	for _, origin := range trustedOrigins {
		if err := p.AddTrustedOrigin(origin); err != nil {
			panic("middleware: CSRF: " + err.Error())
		}
	}
	return p.Handler
}
