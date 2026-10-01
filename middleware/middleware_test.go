package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/cuonggt/tug/internal/route"
	"github.com/cuonggt/tug/queue"
)

// captureLog sends slog.Default() to a buffer, as JSON, for the rest of the
// test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &buf
}

func respond(status int, body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		io.WriteString(w, body)
	})
}

func TestLoggerLogsTheRequestWithItsID(t *testing.T) {
	logs := captureLog(t)
	h := RequestID()(Logger()(respond(201, "hello")))
	req := httptest.NewRequest("POST", "/posts?token=secret", nil)
	req.Header.Set(RequestIDHeader, "abc-123")
	h.ServeHTTP(httptest.NewRecorder(), req)

	var line map[string]any
	if err := json.Unmarshal(logs.Bytes(), &line); err != nil {
		t.Fatalf("log line %q: %v", logs, err)
	}
	for key, want := range map[string]any{
		"msg": "request", "level": "INFO", "method": "POST", "path": "/posts",
		"status": 201.0, "size": 5.0, "request_id": "abc-123",
	} {
		if line[key] != want {
			t.Errorf("%s = %v, want %v", key, line[key], want)
		}
	}
	if _, ok := line["duration"]; !ok {
		t.Error("no duration")
	}
	if strings.Contains(logs.String(), "secret") {
		t.Error("the query string reached the log")
	}
}

func TestLoggerLogsTheRouteThatAnsweredWhenOneDid(t *testing.T) {
	answered := "GET /posts/{id}"
	post := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route.Answered(r.Context(), &answered) // as a tug App's router does
	})
	placed := func(r *http.Request) *http.Request { return r.WithContext(route.Into(r.Context())) } // as the App does
	for _, c := range []struct {
		h     http.Handler
		r     *http.Request
		route any
	}{
		{post, placed(httptest.NewRequest("GET", "/posts/7", nil)), "GET /posts/{id}"},
		{respond(404, ""), placed(httptest.NewRequest("GET", "/nowhere", nil)), nil}, // no route answered
		{respond(200, ""), httptest.NewRequest("GET", "/", nil), nil},                // nor came through an App
	} {
		logs := captureLog(t)
		Logger()(c.h).ServeHTTP(httptest.NewRecorder(), c.r)
		var line map[string]any
		if err := json.Unmarshal(logs.Bytes(), &line); err != nil {
			t.Fatalf("log line %q: %v", logs, err)
		}
		if line["route"] != c.route {
			t.Errorf("%s: route %v, want %v", c.r.URL.Path, line["route"], c.route)
		}
	}
}

func TestLoggerLevelFollowsTheStatus(t *testing.T) {
	for status, level := range map[int]string{200: "INFO", 302: "INFO", 404: "WARN", 503: "ERROR"} {
		logs := captureLog(t)
		Logger()(respond(status, "")).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
		if !strings.Contains(logs.String(), `"level":"`+level+`"`) {
			t.Errorf("a %d logged %s, want %s", status, logs, level)
		}
	}
}

func TestRecoverTurnsAPanicIntoA500AndLogsIt(t *testing.T) {
	logs := captureLog(t)
	h := Recover()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic("boom") }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	if rec.Code != 500 {
		t.Fatalf("got %d, want 500", rec.Code)
	}
	if !strings.Contains(logs.String(), "boom") || !strings.Contains(logs.String(), "goroutine") {
		t.Errorf("the log doesn't have the panic and its stack: %s", logs)
	}
}

func TestRecoverLeavesAResponseThatHasStartedAlone(t *testing.T) {
	captureLog(t)
	h := Recover()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "partial")
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	if rec.Code != 200 || rec.Body.String() != "partial" {
		t.Fatalf("got %d %q, want the response as it was started", rec.Code, rec.Body)
	}
}

func TestRecoverLetsErrAbortHandlerThrough(t *testing.T) {
	h := Recover()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic(http.ErrAbortHandler) }))
	var got any
	func() {
		defer func() { got = recover() }()
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	}()
	if got != http.ErrAbortHandler {
		t.Fatalf("recovered %v, want http.ErrAbortHandler to reach net/http", got)
	}
}

func TestRequestIDKeepsAPlainIDAndReplacesAnythingElse(t *testing.T) {
	var seen string
	h := RequestID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = RequestIDFrom(r.Context())
	}))
	for incoming, keep := range map[string]bool{
		"abc-123":               true,
		"Root=1-5759e988:bd86":  true,
		"":                      false,
		"has spaces":            false,
		"<script>":              false,
		strings.Repeat("a", 65): false,
	} {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set(RequestIDHeader, incoming)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		switch {
		case keep && seen != incoming:
			t.Errorf("%q was replaced with %q", incoming, seen)
		case !keep && (seen == incoming || len(seen) != 26):
			t.Errorf("%q gave the ID %q, want a new random one", incoming, seen)
		}
		if got := rec.Header().Get(RequestIDHeader); got != seen {
			t.Errorf("the response says %q, the context %q", got, seen)
		}
	}
}

func TestCSRFRejectsACrossSitePostAndLetsTheRestThrough(t *testing.T) {
	h := CSRF()(respond(200, "ok"))
	for _, tc := range []struct {
		method, site string
		want         int
	}{
		{"POST", "cross-site", 403},
		{"DELETE", "same-site", 403},
		{"POST", "same-origin", 200},
		{"GET", "cross-site", 200},
		{"POST", "", 200}, // no browser headers: curl, or another server
	} {
		req := httptest.NewRequest(tc.method, "/", nil)
		if tc.site != "" {
			req.Header.Set("Sec-Fetch-Site", tc.site)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("%s from %q = %d, want %d", tc.method, tc.site, rec.Code, tc.want)
		}
	}
}

func TestCSRFLetsATrustedOriginThrough(t *testing.T) {
	h := CSRF("https://admin.example.com")(respond(200, "ok"))
	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("Origin", "https://admin.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("got %d, want the trusted origin let through", rec.Code)
	}
}

func TestCSRFPanicsOnATrustedOriginThatIsNotOne(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("CSRF took an origin without a scheme")
		}
	}()
	CSRF("admin.example.com")
}

func TestCSRFLetsAPathThroughUnchecked(t *testing.T) {
	h := CSRF("/api/")(respond(200, "ok"))
	for path, want := range map[string]int{"/api/user": 200, "/api/": 200, "/settings/tokens": 403, "/api": 403} {
		req := httptest.NewRequest("POST", path, nil)
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("POST %s from another site = %d, want %d", path, rec.Code, want)
		}
	}
}

func TestCSRFPanicsOnAPathServeMuxDoesntTake(t *testing.T) {
	defer func() {
		if msg, _ := recover().(string); !strings.Contains(msg, `CSRF: "/api/{id" isn't a path to let through`) {
			t.Fatalf("panicked with %q", msg)
		}
	}()
	CSRF("/api/{id")
}

// preflight is a browser's OPTIONS, from origin, asking for method and
// headers.
func preflight(origin, method, headers string) *http.Request {
	req := httptest.NewRequest("OPTIONS", "/api/user", nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", method)
	if headers != "" {
		req.Header.Set("Access-Control-Request-Headers", headers)
	}
	return req
}

func TestCORSAnswersAPreflightFromASiteNamed(t *testing.T) {
	reached := false
	h := CORS("https://app.example.com", "")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, preflight("https://app.example.com", "DELETE", "authorization, content-type"))
	got := rec.Header()
	if rec.Code != 204 || reached || got.Get("Access-Control-Allow-Origin") != "https://app.example.com" ||
		got.Get("Access-Control-Allow-Methods") != "DELETE" || got.Get("Access-Control-Allow-Headers") != "authorization, content-type" ||
		got.Get("Access-Control-Max-Age") != "7200" || got.Get("Access-Control-Allow-Credentials") != "" {
		t.Errorf("%d, reached the app %v, headers %v", rec.Code, reached, got)
	}
}

func TestCORSLetsASiteNamedReadTheResponse(t *testing.T) {
	h := CORS("https://app.example.com")(respond(200, "ok"))
	req := httptest.NewRequest("GET", "/api/user", nil)
	req.Header.Set("Origin", "https://app.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	got := rec.Header()
	if rec.Code != 200 || got.Get("Access-Control-Allow-Origin") != "https://app.example.com" || got.Get("Access-Control-Expose-Headers") != "Retry-After" || got.Get("Vary") != "Origin" {
		t.Errorf("%d, headers %v", rec.Code, got)
	}
}

func TestCORSLeavesAnotherSiteAsItCame(t *testing.T) {
	h := CORS("https://app.example.com")(respond(405, "method not allowed"))
	for _, req := range []*http.Request{preflight("https://evil.example", "DELETE", "authorization"), httptest.NewRequest("GET", "/api/user", nil)} {
		if req.Method == "GET" {
			req.Header.Set("Origin", "https://evil.example")
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Header().Get("Access-Control-Allow-Origin") != "" || rec.Code != 405 || rec.Header().Get("Vary") != "Origin" {
			t.Errorf("%s from another site: %d, headers %v", req.Method, rec.Code, rec.Header())
		}
	}
}

func TestCORSStarLetsAnySite(t *testing.T) {
	h := CORS("*")(respond(200, "ok"))
	req := httptest.NewRequest("GET", "/api/posts", nil)
	req.Header.Set("Origin", "https://anyone.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" || rec.Header().Get("Vary") != "" {
		t.Errorf("headers %v", rec.Header())
	}
}

func TestCORSWithNoSitesDoesNothing(t *testing.T) {
	h := CORS(strings.Split("", ",")...)(respond(200, "ok"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, preflight("https://app.example.com", "GET", ""))
	if rec.Code != 200 || len(rec.Header().Values("Access-Control-Allow-Origin")) != 0 || rec.Header().Get("Vary") != "" {
		t.Errorf("%d, headers %v", rec.Code, rec.Header())
	}
}

func TestCORSPanicsOnWhatIsntAnOrigin(t *testing.T) {
	for _, o := range []string{"app.example.com", "https://app.example.com/", "https://app.example.com/api", "ftp://app.example.com", "https://ann@app.example.com"} {
		func() {
			defer func() {
				if msg, _ := recover().(string); !strings.Contains(msg, "isn't an origin") {
					t.Errorf("%q: panicked with %q", o, msg)
				}
			}()
			CORS(o)
		}()
	}
}

// through serves a request from the address from, with the
// X-Forwarded-For lines forwarded, behind TrustProxies(proxies...), and
// returns the RemoteAddr the handler saw.
func through(proxies []string, from string, forwarded ...string) string {
	var seen string
	h := TrustProxies(proxies...)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.RemoteAddr
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = from
	for _, line := range forwarded {
		req.Header.Add("X-Forwarded-For", line)
	}
	h.ServeHTTP(httptest.NewRecorder(), req)
	return seen
}

func TestTrustProxiesReadsTheClientFromTheEndOfXForwardedFor(t *testing.T) {
	lan := []string{"10.0.0.0/8"}
	for _, tc := range []struct {
		name      string
		proxies   []string
		from      string
		forwarded []string
		want      string
	}{
		{"one proxy", lan, "10.0.0.5:4000", []string{"203.0.113.9"}, "203.0.113.9:0"},
		{"a client that writes the header itself", lan, "10.0.0.5:4000", []string{"198.51.100.7, 203.0.113.9"}, "203.0.113.9:0"},
		{"a client that writes a proxy's address", lan, "10.0.0.5:4000", []string{"10.0.0.9, 203.0.113.9"}, "203.0.113.9:0"},
		{"a chain of proxies", lan, "10.0.0.5:4000", []string{"198.51.100.7, 203.0.113.9, 10.0.0.7, 10.0.0.6"}, "203.0.113.9:0"},
		{"the header on two lines", lan, "10.0.0.5:4000", []string{"198.51.100.7, 203.0.113.9", "10.0.0.6"}, "203.0.113.9:0"},
		{"a proxy named by its address", []string{"10.0.0.5"}, "10.0.0.5:4000", []string{"203.0.113.9"}, "203.0.113.9:0"},
		{"IPv6", []string{"2001:db8::/32"}, "[2001:db8::1]:4000", []string{"2001:db8:1::9, 2a00:1450::5, 2001:db8::7"}, "[2a00:1450::5]:0"},
		{"IPv4 written as IPv6", lan, "[::ffff:10.0.0.5]:4000", []string{"::ffff:203.0.113.9"}, "203.0.113.9:0"},
		{"an address with its port", lan, "10.0.0.5:4000", []string{"203.0.113.9:51234"}, "203.0.113.9:0"},
		{"only proxies", lan, "10.0.0.5:4000", []string{"10.0.0.7, 10.0.0.6"}, "10.0.0.7:0"},
	} {
		if got := through(tc.proxies, tc.from, tc.forwarded...); got != tc.want {
			t.Errorf("%s: RemoteAddr %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestTrustProxiesLeavesARequestAloneUnlessAProxySentIt(t *testing.T) {
	for _, tc := range []struct {
		name      string
		proxies   []string
		from      string
		forwarded []string
	}{
		{"a client that isn't behind a proxy", []string{"10.0.0.0/8"}, "198.51.100.7:4000", []string{"203.0.113.9"}},
		{"a proxy that adds no header", []string{"10.0.0.0/8"}, "10.0.0.5:4000", nil},
		{"a proxy that adds what isn't an address", []string{"10.0.0.0/8"}, "10.0.0.5:4000", []string{"203.0.113.9, unknown"}},
		{"no proxies, from an empty variable", []string{""}, "10.0.0.5:4000", []string{"203.0.113.9"}},
		{"no proxies at all", nil, "10.0.0.5:4000", []string{"203.0.113.9"}},
	} {
		if got := through(tc.proxies, tc.from, tc.forwarded...); got != tc.from {
			t.Errorf("%s: RemoteAddr %q, want it left as %q", tc.name, got, tc.from)
		}
	}
}

func TestTrustProxiesStarBelievesTheAddressThatConnectedAlone(t *testing.T) {
	// Whatever connects is the platform's proxy, so the client is the
	// address it added, and nothing before that can be believed.
	if got := through([]string{"*"}, "172.16.0.1:4000", "198.51.100.7, 203.0.113.9"); got != "203.0.113.9:0" {
		t.Errorf("RemoteAddr %q, want the address the proxy added", got)
	}
	if got := through([]string{"*"}, "172.16.0.1:4000", "203.0.113.9, 10.0.0.6"); got != "10.0.0.6:0" {
		t.Errorf("RemoteAddr %q, want the address the proxy added, as no other is a proxy", got)
	}
	if got := through([]string{"*", "10.0.0.0/8"}, "172.16.0.1:4000", "203.0.113.9, 10.0.0.6"); got != "203.0.113.9:0" {
		t.Errorf("RemoteAddr %q, want the ranges read past too", got)
	}
}

func TestTrustProxiesChangesACopyOfTheRequest(t *testing.T) {
	h := TrustProxies("10.0.0.0/8")(respond(200, "ok"))
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.5:4000"
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if req.RemoteAddr != "10.0.0.5:4000" {
		t.Errorf("the caller's request was changed: RemoteAddr %q", req.RemoteAddr)
	}
}

func TestTrustProxiesPanicsOnAProxyThatIsNotAnAddressOrARange(t *testing.T) {
	for _, proxy := range []string{"example.com", "10.0.0.0/33", "10.0.0.1:80", "10.0.0.*"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("TrustProxies took %q", proxy)
				}
			}()
			TrustProxies(proxy)
		}()
	}
}

func TestHeadersSayWhatABrowserMayDoWithEveryResponse(t *testing.T) {
	for _, status := range []int{200, 404, 500} {
		rec := httptest.NewRecorder()
		Headers(HeadersConfig{})(respond(status, "")).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
		for name, want := range map[string]string{
			"X-Content-Type-Options":     "nosniff",
			"Referrer-Policy":            "strict-origin-when-cross-origin",
			"Cross-Origin-Opener-Policy": "same-origin",
			"X-Frame-Options":            "SAMEORIGIN",
		} {
			if got := rec.Header().Get(name); got != want {
				t.Errorf("%d: %s is %q, want %q", status, name, got, want)
			}
		}
		if hsts := rec.Header().Get("Strict-Transport-Security"); hsts != "" {
			t.Errorf("%d: HSTS %q, with no HTTPS to hold the browser to", status, hsts)
		}
	}
}

func TestHeadersHoldTheBrowserToHTTPSForAYearWhenTheAppIsServedOverIt(t *testing.T) {
	rec := httptest.NewRecorder()
	Headers(HeadersConfig{HSTS: true})(respond(200, "")).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if hsts := rec.Header().Get("Strict-Transport-Security"); hsts != "max-age=31536000" {
		t.Errorf("HSTS %q: a year, for the app's host alone, and not preloaded", hsts)
	}
}

func TestAHandlerChangesTheHeadersOfItsOwnResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	framed := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Del("X-Frame-Options")
	})
	Headers(HeadersConfig{})(framed).ServeHTTP(rec, httptest.NewRequest("GET", "/embed", nil))
	if _, ok := rec.Header()["X-Frame-Options"]; ok {
		t.Error("the handler's own choice was overridden")
	}
}

// policyOf serves a request through CSP(cfg), and returns the policy it
// sent, and the nonce the handler was given.
func policyOf(t *testing.T, cfg CSPConfig) (policy map[string][]string, n string) {
	t.Helper()
	rec := httptest.NewRecorder()
	CSP(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n = NonceFrom(r.Context())
	})).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	header := rec.Header().Get("Content-Security-Policy")
	if cfg.ReportOnly {
		if header != "" {
			t.Fatalf("a report-only policy enforced: %q", header)
		}
		header = rec.Header().Get("Content-Security-Policy-Report-Only")
	}
	policy = map[string][]string{}
	for _, d := range strings.Split(header, "; ") {
		fields := strings.Fields(d)
		if len(fields) == 0 {
			t.Fatalf("an empty directive in %q", header)
		}
		policy[fields[0]] = fields[1:]
	}
	return policy, n
}

func TestCSPGivesEachResponseANonceOfItsOwn(t *testing.T) {
	first, n := policyOf(t, CSPConfig{})
	if len(n) < 22 {
		t.Fatalf("the nonce %q is too short to guess", n)
	}
	if want := []string{"'nonce-" + n + "'", "'strict-dynamic'"}; !slices.Equal(first["script-src"], want) {
		t.Errorf("script-src %v, want %v", first["script-src"], want)
	}
	second, again := policyOf(t, CSPConfig{})
	if again == n || slices.Equal(first["script-src"], second["script-src"]) {
		t.Errorf("two responses had the nonce %q", n)
	}
	if NonceFrom(httptest.NewRequest("GET", "/", nil).Context()) != "" {
		t.Error("a request without CSP has a nonce")
	}
}

func TestCSPLetsAPageLoadTheAppsOwnAlone(t *testing.T) {
	policy, _ := policyOf(t, CSPConfig{})
	for name, want := range map[string][]string{
		"default-src":     {"'self'"},
		"style-src":       {"'self'", "'unsafe-inline'"},
		"img-src":         {"'self'", "data:", "blob:"},
		"font-src":        {"'self'", "data:"},
		"connect-src":     {"'self'"},
		"object-src":      {"'none'"},
		"base-uri":        {"'none'"},
		"form-action":     {"'self'"},
		"frame-ancestors": {"'self'"},
	} {
		if !slices.Equal(policy[name], want) {
			t.Errorf("%s %v, want %v", name, policy[name], want)
		}
	}
	if _, ok := policy["report-uri"]; ok {
		t.Error("a report-uri with no ReportPath")
	}
}

func TestCSPReportOnlyBlocksNothing(t *testing.T) {
	policy, _ := policyOf(t, CSPConfig{ReportOnly: true})
	if len(policy["script-src"]) != 2 {
		t.Errorf("script-src %v", policy["script-src"])
	}
}

func TestCSPAddsTheSourcesItsGiven(t *testing.T) {
	policy, _ := policyOf(t, CSPConfig{Sources: map[string][]string{
		"img-src":   {"https://photos.s3.us-east-1.amazonaws.com", " ", "data:"},
		"media-src": {"https://videos.example.com"},
	}})
	if want := []string{"'self'", "data:", "blob:", "https://photos.s3.us-east-1.amazonaws.com"}; !slices.Equal(policy["img-src"], want) {
		t.Errorf("img-src %v, want %v", policy["img-src"], want)
	}
	if want := []string{"https://videos.example.com"}; !slices.Equal(policy["media-src"], want) {
		t.Errorf("media-src %v, want %v", policy["media-src"], want)
	}
	if want := []string{"'self'"}; !slices.Equal(policy["default-src"], want) {
		t.Errorf("another directive changed: default-src %v", policy["default-src"])
	}
	// The policy the map came from is left as it was, for the next CSP.
	if again, _ := policyOf(t, CSPConfig{}); len(again["img-src"]) != 3 {
		t.Errorf("img-src %v after another CSP's sources", again["img-src"])
	}
}

func TestCSPLetsInTheDevServerWhileItRuns(t *testing.T) {
	dev := "http://localhost:5173"
	policy, n := policyOf(t, CSPConfig{DevServer: func() string { return dev }})
	for name, want := range map[string][]string{
		"style-src":   {"'self'", "'unsafe-inline'", dev},
		"img-src":     {"'self'", "data:", "blob:", dev},
		"font-src":    {"'self'", "data:", dev},
		"connect-src": {"'self'", dev, "ws://localhost:5173"},
		"script-src":  {"'nonce-" + n + "'", "'strict-dynamic'"},
	} {
		if !slices.Equal(policy[name], want) {
			t.Errorf("%s %v, want %v", name, policy[name], want)
		}
	}
	dev = ""
	if policy, _ := policyOf(t, CSPConfig{DevServer: func() string { return dev }}); len(policy["connect-src"]) != 1 {
		t.Errorf("connect-src %v once the dev server stopped", policy["connect-src"])
	}
}

// report is what Chrome sends report-uri of a script its policy blocked.
const report = `{"csp-report":{"document-uri":"https://example.com/posts","referrer":"","violated-directive":"script-src-elem","effective-directive":"script-src-elem","original-policy":"...","disposition":"enforce","blocked-uri":"https://cdn.example.net/x.js","line-number":12,"source-file":"https://example.com/posts","status-code":200,"script-sample":""}}`

func TestCSPLogsWhatABrowserReportsItBlocked(t *testing.T) {
	for _, reportOnly := range []bool{false, true} {
		logs := captureLog(t)
		policy, _ := policyOf(t, CSPConfig{ReportOnly: reportOnly, ReportPath: "/csp-reports"})
		if want := []string{"/csp-reports"}; !slices.Equal(policy["report-uri"], want) {
			t.Errorf("report-uri %v, want %v", policy["report-uri"], want)
		}
		routed := false
		h := CSP(CSPConfig{ReportOnly: reportOnly, ReportPath: "/csp-reports"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			routed = true
		}))
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/csp-reports", strings.NewReader(report))
		req.Header.Set("Content-Type", "application/csp-report")
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent || routed {
			t.Fatalf("report-only %v: %d, and routed %v", reportOnly, rec.Code, routed)
		}
		var line map[string]any
		if err := json.Unmarshal(logs.Bytes(), &line); err != nil {
			t.Fatalf("log line %q: %v", logs, err)
		}
		msg := "a page's Content-Security-Policy blocked what it asked for"
		if reportOnly {
			msg = "a page's Content-Security-Policy would have blocked what it asked for"
		}
		for key, want := range map[string]any{
			"level": "WARN", "msg": msg, "page": "https://example.com/posts", "directive": "script-src-elem",
			"blocked": "https://cdn.example.net/x.js", "source": "https://example.com/posts:12",
		} {
			if line[key] != want {
				t.Errorf("report-only %v: %s = %v, want %v", reportOnly, key, line[key], want)
			}
		}
		if _, ok := line["sample"]; ok {
			t.Errorf("report-only %v: logged an empty sample", reportOnly)
		}
	}
}

func TestCSPTurnsAwayWhatIsntAReport(t *testing.T) {
	logs := captureLog(t)
	var routed []string
	h := CSP(CSPConfig{ReportPath: "/csp-reports"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routed = append(routed, r.Method+" "+r.URL.Path)
	}))
	for _, body := range []string{"", "{}", "not JSON", `{"csp-report":` + strings.Repeat(" ", maxReport) + `{}}`} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("POST", "/csp-reports", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%.20q: %d", body, rec.Code)
		}
	}
	if logs.Len() != 0 {
		t.Errorf("logged %s", logs)
	}
	// Only a POST to the path is a report; the rest are the app's.
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/csp-reports", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/csp-reports/more", strings.NewReader(report)))
	if want := []string{"GET /csp-reports", "POST /csp-reports/more"}; !slices.Equal(routed, want) {
		t.Errorf("routed %v, want %v", routed, want)
	}
}

func TestCSPPanicsOnWhatItCantPutInTheHeader(t *testing.T) {
	for _, cfg := range []CSPConfig{
		{Sources: map[string][]string{"img src": {"https://photos.example.com"}}},
		{Sources: map[string][]string{"IMG-SRC": {"https://photos.example.com"}}},
		{Sources: map[string][]string{"img-src": {"https://photos.example.com; script-src *"}}},
		{Sources: map[string][]string{"img-src": {"https://a.example.com https://b.example.com"}}},
		{ReportPath: "csp-reports"},
		{ReportPath: "/csp-reports; script-src *"},
	} {
		func() {
			defer func() {
				if msg, _ := recover().(string); !strings.HasPrefix(msg, "middleware: CSP: ") {
					t.Errorf("%+v: panicked with %q", cfg, msg)
				}
			}()
			CSP(cfg)
		}()
	}
}

func TestARequestsIDIsCarriedIntoAJobsContext(t *testing.T) {
	var c queue.Carrier = CarryRequestID // as a queue's Config.Carry takes it
	carried := map[string]string{}
	c.Carry(WithRequestID(context.Background(), "QW3RTY"), carried)
	if carried["request_id"] != "QW3RTY" {
		t.Fatalf("carried %v", carried)
	}
	if id := RequestIDFrom(c.Restore(context.Background(), carried)); id != "QW3RTY" {
		t.Errorf("the job's context has the ID %q", id)
	}
	// Nothing to carry, and an ID that isn't plain, come to nothing.
	nothing := map[string]string{}
	c.Carry(context.Background(), nothing)
	if len(nothing) != 0 {
		t.Errorf("a context with no request carried %v", nothing)
	}
	if id := RequestIDFrom(c.Restore(context.Background(), map[string]string{"request_id": "a\nb"})); id != "" {
		t.Errorf("an ID with a line break came back as %q", id)
	}
}
