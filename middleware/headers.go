package middleware

import (
	"maps"
	"net/http"
	"slices"
	"strings"
)

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

	// Permissions allows a feature the Permissions-Policy turns off, or
	// adds one, by its allowlist: self, *, or origins in quotes, space
	// separated, as {"camera": "self"} or
	// {"payment": `self "https://pay.example.com"`}.
	Permissions map[string]string
}

// permissionsOff are the powerful features the Permissions-Policy turns off
// unless Permissions allows them: those Chrome knows, as it logs an error
// for a name it doesn't. Passkeys' publickey-credentials-get and -create,
// and fullscreen and autoplay, which a video needs, keep their default, the
// app's own pages.
var permissionsOff = []string{
	"camera", "microphone", "geolocation", "payment", "usb", "serial",
	"hid", "bluetooth", "midi", "display-capture", "browsing-topics",
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
//   - Permissions-Policy, which turns off the powerful features no page of
//     the app's uses, as a script that got into one would: the camera, the
//     microphone, the browser's location, payments, USB, serial ports, HID,
//     Bluetooth, MIDI, capturing the screen, and the Topics API. A page that
//     uses one is the app's to allow, in Permissions.
//   - With HSTS, Strict-Transport-Security, for a year.
//
// A handler whose response needs another, as a page another site frames,
// sets its own, or deletes one.
//
// Headers panics on a feature or an allowlist it can't put in the header:
// a mistyped setting should stop the app at startup.
func Headers(cfg HeadersConfig) func(http.Handler) http.Handler {
	permissions := permissionsPolicy(cfg.Permissions)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Cross-Origin-Opener-Policy", "same-origin")
			h.Set("X-Frame-Options", "SAMEORIGIN")
			h.Set("Permissions-Policy", permissions)
			if cfg.HSTS {
				h.Set("Strict-Transport-Security", "max-age=31536000")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// permissionsPolicy is the Permissions-Policy header: permissionsOff, each
// off, as feature=(), or as allowed says, then the features allowed adds,
// in order, so the header is the same whatever order the map has.
func permissionsPolicy(allowed map[string]string) string {
	for name, allow := range allowed {
		if !directiveName(name) {
			panic(`middleware: Headers: "` + name + `" isn't a feature, as camera`)
		}
		for _, m := range strings.Fields(allow) {
			if m != "self" && m != "*" && !quotedOrigin(m) {
				panic(`middleware: Headers: "` + allow + `" isn't an allowlist, as self, or self "https://pay.example.com"`)
			}
		}
	}
	features := slices.Clone(permissionsOff)
	for _, name := range slices.Sorted(maps.Keys(allowed)) {
		if !slices.Contains(features, name) {
			features = append(features, name)
		}
	}
	parts := make([]string, len(features))
	for i, name := range features {
		switch allow := strings.Join(strings.Fields(allowed[name]), " "); allow {
		case "*":
			parts[i] = name + "=*"
		default:
			parts[i] = name + "=(" + allow + ")"
		}
	}
	return strings.Join(parts, ", ")
}

// quotedOrigin reports whether m is an origin in quotes, as
// "https://pay.example.com", as an allowlist writes one.
func quotedOrigin(m string) bool {
	if len(m) < 2 || m[0] != '"' || m[len(m)-1] != '"' {
		return false
	}
	scheme, host, ok := strings.Cut(m[1:len(m)-1], "://")
	return ok && (scheme == "https" || scheme == "http") && host != "" &&
		!strings.ContainsAny(host, `/"?#,;() `)
}
