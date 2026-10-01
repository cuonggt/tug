package devtools

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
)

// redacted is what an entry keeps in a secret's place.
const redacted = "[REDACTED]"

// secretKeys are the keys whose values an entry never keeps, at any depth
// of a body or a prop, and in a URL's query: Laravel's, and the recovery
// codes tug's auth starter shows. A key is matched in any case, and with
// or without its underscores and hyphens, so access_token is accessToken.
var secretKeys = plainKeys(
	"password", "password_confirmation", "current_password",
	"token", "access_token", "refresh_token",
	"secret", "client_secret", "api_key",
	"recovery_codes",
)

// secretHeaders are the headers whose values an entry never keeps:
// Laravel's, a login's and a session's.
var secretHeaders = []string{
	"cookie", "set-cookie", "authorization", "proxy-authorization",
	"x-xsrf-token", "x-csrf-token",
}

// secret reports whether key is one of secretKeys.
func secret(key string) bool {
	return slices.Contains(secretKeys, plainKey(key))
}

// plainKey is key as secretKeys are compared: in lower case, without
// underscores and hyphens.
func plainKey(key string) string {
	return separators.Replace(strings.ToLower(key))
}

var separators = strings.NewReplacer("_", "", "-", "")

// plainKeys is keys, each as plainKey has it.
func plainKeys(keys ...string) []string {
	for i, k := range keys {
		keys[i] = plainKey(k)
	}
	return keys
}

// redact returns v, a value as JSON reads one, with the value of each of
// secretKeys in it, at any depth, as redacted.
func redact(v any) any {
	switch v := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, x := range v {
			if secret(k) {
				out[k] = redacted
			} else {
				out[k] = redact(x)
			}
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, x := range v {
			out[i] = redact(x)
		}
		return out
	}
	return v
}

// headers returns h as an entry keeps headers: by name, in lower case,
// each with its values together, and secretHeaders' as redacted.
func headers(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for name, values := range h {
		name = strings.ToLower(name)
		if slices.Contains(secretHeaders, name) {
			out[name] = redacted
			continue
		}
		out[name] = strings.Join(values, ", ")
	}
	return out
}

// redactQuery returns u with the value of each of secretKeys in its query
// as redacted.
func redactQuery(u *url.URL) *url.URL {
	if u.RawQuery == "" {
		return u
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return u
	}
	found := false
	for k := range q {
		if secret(k) {
			q[k] = []string{redacted}
			found = true
		}
	}
	if !found {
		return u
	}
	out := *u
	out.RawQuery = q.Encode()
	return &out
}

// RedactHeaders is h as an entry keeps it, by name in lower case, with
// the values of secretHeaders as [REDACTED], for the page a server error
// is shown with while debugging, which shows a request as DevTools do.
func RedactHeaders(h http.Header) map[string]string { return headers(h) }

// RedactQuery is u with the values of secretKeys in its query as
// [REDACTED], as an entry keeps it.
func RedactQuery(u *url.URL) *url.URL { return redactQuery(u) }
