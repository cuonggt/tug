package middleware

import (
	"fmt"
	"net/http"
	"strings"
)

// CSRF rejects state-changing requests that a browser sent from another
// site, with a 403. It is net/http's CrossOriginProtection, which reads the
// Sec-Fetch-Site header every current browser sends, or else compares
// Origin with Host, so there are no tokens to put in forms or rotate, and
// Inertia's XSRF-TOKEN cookie isn't needed. GET, HEAD and OPTIONS always
// pass, so they must never change anything.
//
// Each of trusted is an origin whose requests pass too, such as
// "https://admin.example.com", or a path, such as "/api/", a pattern of
// ServeMux's whose requests pass unchecked: routes that take a token,
// never the session's cookie, which a page of another site can't borrow,
// as an API's do. CSRF panics on one that's neither: a mistyped setting
// should stop the app at startup, not surface as 403s later.
func CSRF(trusted ...string) func(http.Handler) http.Handler {
	p := http.NewCrossOriginProtection()
	for _, entry := range trusted {
		if strings.HasPrefix(entry, "/") {
			bypass(p, entry)
		} else if err := p.AddTrustedOrigin(entry); err != nil {
			panic("middleware: CSRF: " + err.Error())
		}
	}
	return p.Handler
}

// bypass lets the requests pattern matches through p unchecked, saying
// whose the panic is for a pattern ServeMux doesn't take.
func bypass(p *http.CrossOriginProtection, pattern string) {
	defer func() {
		if v := recover(); v != nil {
			panic(fmt.Sprintf("middleware: CSRF: %q isn't a path to let through: %v", pattern, v))
		}
	}()
	p.AddInsecureBypassPattern(pattern)
}
