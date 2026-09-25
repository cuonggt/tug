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
