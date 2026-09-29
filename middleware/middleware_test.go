package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
