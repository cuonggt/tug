package devtools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestAnIDIsAULIDAndTheIDsSortAsTheyWereMade(t *testing.T) {
	var u ulids
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	ids := []string{u.next(at), u.next(at), u.next(at), u.next(at.Add(time.Millisecond)), u.next(at.Add(-time.Hour))}
	if !slices.IsSorted(ids) || len(slices.Compact(slices.Clone(ids))) != len(ids) {
		t.Errorf("made in order, %v aren't sorted, or repeat", ids)
	}
	for _, id := range ids {
		if !isULID(id) {
			t.Errorf("%q isn't a ULID", id)
		}
	}
	// The millisecond is the first ten digits, as in the spec's example.
	var spec ulids
	if got := spec.next(time.UnixMilli(1469918176385))[:10]; got != "01ARYZ6S41" {
		t.Errorf("made at the spec's example's time, the time is %s, want 01ARYZ6S41", got)
	}
	for _, s := range []string{"", "01K6FN4MW0", "81K6FN4MW0ZZZZZZZZZZZZZZZZ", "01K6FN4MW0ZZZZZZZZZZZZZZZU", "../../etc/passwd/xxxxxxxxxx"} {
		if isULID(s) {
			t.Errorf("%q is a ULID", s)
		}
	}
}

func TestTheAppsFramesAreThoseOfNeitherTugNorTheStandardLibrary(t *testing.T) {
	defer func(m string) { mainModule = m }(mainModule)
	mainModule = "blog" // as tug new names an app's module
	for _, c := range []struct {
		function, file string
		apps           bool
	}{
		{"main.(*app).dashboard", "/src/blog/main.go", true},
		{"blog/internal/posts.List", "/src/blog/internal/posts/posts.go", true},
		{"github.com/jackc/pgx/v5.(*Conn).Query", "/mod/github.com/jackc/pgx/v5/conn.go", true},
		{"net/http.Redirect", "/go/src/net/http/server.go", false},
		{"encoding/json.(*Encoder).Encode", "/go/src/encoding/json/stream.go", false},
		{"github.com/cuonggt/tug.(*Ctx).Redirect", "/mod/github.com/cuonggt/tug/ctx.go", false},
		{"github.com/cuonggt/tug.PageOf[...].Render", "/mod/github.com/cuonggt/tug/pages.go", false},
		{"github.com/cuonggt/tug/middleware.Logger.func1", "/mod/github.com/cuonggt/tug/middleware/logger.go", false},
		{"github.com/cuonggt/tug.TestRoutes.func1", "/src/tug/router_test.go", true},
		// A closure made by a function inlined into the app's is named as
		// the app's, in the file of the package that wrote it.
		{"main.newApp.Headers.func7.1", tugDir + "middleware/headers.go", false},
		{"main.main", tugDir + "examples/inertia/main.go", true},
	} {
		if got := apps(runtime.Frame{Function: c.function, File: c.file}); got != c.apps {
			t.Errorf("%s in %s is the app's: %v", c.function, c.file, got)
		}
	}
	if stdDir == "" || tugDir == "" {
		t.Fatalf("the standard library is in %q, and tug in %q", stdDir, tugDir)
	}
	if apps(runtime.Frame{Function: "main.newApp.StripPrefix.func1", File: stdDir + "net/http/server.go"}) {
		t.Error("the standard library's closure inlined into the app's is the app's")
	}
}

// handling calls answer as tug's adapter calls a route's handler, with
// where handling starts.
//
//go:noinline
func handling(answer func(handler uintptr)) {
	answer(Here())
}

// answering is a handler of the app's that writes the response.
//
//go:noinline
func answering(handler uintptr) (runtime.Frame, bool) {
	return answerer(handler)
}

func TestTheAnswerIsTheAppsFunctionInsideTheHandler(t *testing.T) {
	var got runtime.Frame
	var found bool
	handling(func(handler uintptr) { got, found = answering(handler) })
	if !found || !strings.HasSuffix(got.Function, ".answering") {
		t.Errorf("inside the handler, the answer is %q, %v", got.Function, found)
	}
	// Written with the handler's frame nowhere on the stack, as by
	// middleware once the handler has returned.
	if got, found := answering(reflect.ValueOf(handling).Pointer()); found {
		t.Errorf("outside the handler, the answer is %q", got.Function)
	}
}

func TestSecretsAreRedactedAtAnyDepthAndInAnyCase(t *testing.T) {
	v := map[string]any{
		"name":     "Ann",
		"Password": "correct horse",
		"user":     map[string]any{"accessToken": "tug_abc", "email": "ann@example.com"},
		"keys":     []any{map[string]any{"client-secret": "s3cr3t"}},
		"flash":    map[string]any{"recoveryCodes": []any{"a-b", "c-d"}},
	}
	got, _ := json.Marshal(redact(v))
	want := `{"Password":"[REDACTED]","flash":{"recoveryCodes":"[REDACTED]"},"keys":[{"client-secret":"[REDACTED]"}],"name":"Ann","user":{"accessToken":"[REDACTED]","email":"ann@example.com"}}`
	if string(got) != want {
		t.Errorf("redacted\n%s\nwant\n%s", got, want)
	}

	h := http.Header{"Cookie": {"tug_session=abc"}, "Authorization": {"Bearer tug_abc"}, "Accept": {"text/html", "application/json"}}
	if got := headers(h); got["cookie"] != redacted || got["authorization"] != redacted || got["accept"] != "text/html, application/json" {
		t.Errorf("headers %v", got)
	}

	r := httptest.NewRequest("GET", "/reset?token=abc&email=ann%40example.com", nil)
	if got := fullURL(r); got != "http://example.com/reset?email=ann%40example.com&token=%5BREDACTED%5D" {
		t.Errorf("the URL %s", got)
	}
}

// entryWith is an entry of the tab, made at at.
func entryWith(u *ulids, tab *string, at time.Time) entry {
	return entry{Meta: meta{ID: u.next(at), TabUUID: tab, Utime: float64(at.UnixMicro()) / 1e6, RequestType: "navigate"}}
}

func TestTheStoreKeepsTheNewestOfEachTabForADayAcrossRuns(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".tug", "devtools")
	now := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	st := &store{dir: dir, now: func() time.Time { return now }}
	var u ulids
	tab := "tab-1"
	var first string
	for i := range keepPerTab + 5 {
		e := entryWith(&u, &tab, now)
		if i == 0 {
			first = e.Meta.ID
		}
		if err := st.save(e); err != nil {
			t.Fatal(err)
		}
	}
	for range 3 {
		st.save(entryWith(&u, nil, now)) // from a browser without the extension
	}
	metas := st.list()
	if len(metas) != keepPerTab+3 {
		t.Fatalf("kept %d, want the newest %d of the tab and the 3 with none", len(metas), keepPerTab)
	}
	if !slices.IsSortedFunc(metas, func(a, b meta) int { return strings.Compare(b.ID, a.ID) }) {
		t.Error("the list isn't the newest first")
	}
	if _, ok := st.get(first); ok {
		t.Error("the tab's oldest is still kept past its newest 100")
	}

	// Another run, as tug dev builds the app again, reads what's kept.
	again := &store{dir: dir, now: func() time.Time { return now }}
	if got := len(again.list()); got != keepPerTab+3 {
		t.Errorf("another run lists %d", got)
	}
	if data, ok := again.get(metas[0].ID); !ok || !strings.Contains(string(data), metas[0].ID) {
		t.Errorf("another run gets %s, %v", data, ok)
	}

	// A day on, what's older goes, as the next is kept.
	now = now.Add(keepFor + time.Minute)
	again.save(entryWith(&u, &tab, now))
	if got := again.list(); len(got) != 1 {
		t.Errorf("a day on, %d are kept, want the new one", len(got))
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*"))
	if len(files) != 1 {
		t.Errorf("a day on, the files are %v", files)
	}
}

// served is the response to r through a Recorder in dir, with next as the
// app, and the entry it kept.
func served(t *testing.T, rc *Recorder, r *http.Request, next http.HandlerFunc) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	rc.Serve(w, r, next)
	id := w.Header().Get(HeaderID)
	data, ok := rc.store.get(id)
	if !ok {
		t.Fatalf("%s %s: no entry %q", r.Method, r.URL, id)
	}
	var e map[string]any
	if err := json.Unmarshal(data, &e); err != nil {
		t.Fatal(err)
	}
	return w, e
}

// at is the value at path in v, a part of the path at each level.
func at(v any, path string) any {
	for part := range strings.SplitSeq(path, ".") {
		m, _ := v.(map[string]any)
		v = m[part]
	}
	return v
}

func TestEachResponseSaysItsEntryAndBatch(t *testing.T) {
	rc := New(t.TempDir())
	ok := func(w http.ResponseWriter, r *http.Request) {}

	first := httptest.NewRequest("GET", "/", nil)
	w, e := served(t, rc, first, ok)
	id := w.Header().Get(HeaderID)
	if !isULID(id) || w.Header().Get(HeaderParentOut) != id || at(e, "__meta.batchId") != nil {
		t.Errorf("a first visit: id %q, parent out %q, batch %v", id, w.Header().Get(HeaderParentOut), at(e, "__meta.batchId"))
	}

	next := httptest.NewRequest("GET", "/posts", nil)
	next.Header.Set("X-Inertia", "true")
	next.Header.Set(HeaderParent, id)
	next.Header.Set(HeaderTab, "tab-1")
	next.Header.Set(HeaderVisit, "visit-7")
	w, e = served(t, rc, next, ok)
	if w.Header().Get(HeaderParentOut) != id || at(e, "__meta.batchId") != id || at(e, "__meta.tabUuid") != "tab-1" || at(e, "__meta.visitId") != "visit-7" {
		t.Errorf("a visit of the batch: parent out %q, meta %v", w.Header().Get(HeaderParentOut), e["__meta"])
	}

	prefetch := next.Clone(next.Context())
	prefetch.Header.Set("Purpose", "prefetch")
	w, e = served(t, rc, prefetch, ok)
	if w.Header().Get(HeaderParentOut) != w.Header().Get(HeaderID) || at(e, "__meta.requestType") != "prefetch" {
		t.Errorf("a prefetch: parent out %q, its own %q", w.Header().Get(HeaderParentOut), w.Header().Get(HeaderID))
	}
}

func TestEachKindOfRequestIsNamedAsTheProtocolNamesIt(t *testing.T) {
	rc := New(t.TempDir())
	ok := func(w http.ResponseWriter, r *http.Request) {}
	page := func(w http.ResponseWriter, r *http.Request) {
		From(r.Context()).Page("Home", map[string]any{}, nil, nil)
	}
	for _, c := range []struct {
		headers []string
		next    http.HandlerFunc
		want    string
	}{
		{nil, page, "initial"},
		{nil, ok, "http"},
		{[]string{"Precognition", "true", "X-Inertia", "true"}, ok, "precognition"},
		{[]string{"X-Inertia", "true", HeaderDeferred, "1", "X-Inertia-Partial-Component", "Home"}, page, "deferred"},
		{[]string{"X-Inertia", "true", HeaderPoll, "1", "X-Inertia-Partial-Component", "Home"}, page, "poll"},
		{[]string{"X-Inertia", "true", "X-Inertia-Partial-Component", "Home"}, page, "partial"},
		{[]string{"X-Inertia", "true", "Purpose", "prefetch"}, page, "prefetch"},
		{[]string{"X-Inertia", "true"}, page, "navigate"},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		for i := 0; i+1 < len(c.headers); i += 2 {
			r.Header.Set(c.headers[i], c.headers[i+1])
		}
		if _, e := served(t, rc, r, c.next); at(e, "__meta.requestType") != c.want {
			t.Errorf("with %v: %v, want %s", c.headers, at(e, "__meta.requestType"), c.want)
		}
	}
}

func TestABodyIsKeptWhenItsTextAndOtherwiseLeftOutSayingWhy(t *testing.T) {
	rc := New(t.TempDir())
	write := func(contentType, body string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			io.ReadAll(r.Body)
			w.Header().Set("Content-Type", contentType)
			io.WriteString(w, body)
		}
	}
	inertia := func(method, contentType, body string) *http.Request {
		r := httptest.NewRequest(method, "/", strings.NewReader(body))
		r.Header.Set("X-Inertia", "true")
		r.Header.Set("Content-Type", contentType)
		return r
	}
	big := strings.Repeat("x", bodyLimit+1)
	for _, c := range []struct {
		name      string
		r         *http.Request
		next      http.HandlerFunc
		req, resp string
	}{
		{"JSON", inertia("POST", "application/json", `{"email":"ann@example.com","password":"correct horse"}`), write("application/json", `{"token":"tug_abc","ok":true}`),
			`{"status":"present","value":{"email":"ann@example.com","password":"[REDACTED]"}}`, `{"status":"present","value":{"ok":true,"token":"[REDACTED]"}}`},
		{"a form", inertia("PUT", "application/x-www-form-urlencoded", "name=Ann&current_password=x"), write("text/plain; charset=utf-8", "saved"),
			`{"status":"present","value":{"current_password":"[REDACTED]","name":"Ann"}}`, `{"status":"present","value":"saved"}`},
		{"an HTML form's", httptest.NewRequest("POST", "/", strings.NewReader("password=x")), write("text/html", ""),
			`{"reason":"non-inertia-request","status":"omitted"}`, `{"status":"empty"}`},
		{"too large", inertia("POST", "text/plain", big), write("text/plain", big),
			`{"reason":"too-large","status":"omitted"}`, `{"reason":"too-large","status":"omitted"}`},
		{"binary", inertia("POST", "application/octet-stream", "\xff\xfe"), write("image/png", "\x89PNG"),
			`{"reason":"binary","status":"omitted"}`, `{"reason":"non-textual","status":"omitted"}`},
		{"a stream", httptest.NewRequest("GET", "/", nil), func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(w, "data: hello\n\n")
			w.(http.Flusher).Flush()
		}, `{"status":"empty"}`, `{"reason":"streamed","status":"omitted"}`},
	} {
		_, e := served(t, rc, c.r, c.next)
		req, _ := json.Marshal(at(e, "http.requestBody"))
		resp, _ := json.Marshal(at(e, "http.responseBody"))
		if string(req) != c.req || string(resp) != c.resp {
			t.Errorf("%s: the request's %s, want %s; the response's %s, want %s", c.name, req, c.req, resp, c.resp)
		}
	}
}

func TestARedirectAndItsStatusAndTimeAreKept(t *testing.T) {
	rc := New(t.TempDir())
	r := httptest.NewRequest("POST", "/posts", nil)
	r.Header.Set("X-Inertia", "true")
	_, e := served(t, rc, r, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/posts/7", http.StatusSeeOther)
	})
	if at(e, "__meta.status") != 303.0 || at(e, "__meta.redirectLocation") != "/posts/7" || at(e, "__meta.method") != "POST" || at(e, "__meta.url") != "http://example.com/posts" {
		t.Errorf("meta %v", e["__meta"])
	}
	if ms, ok := at(e, "__meta.serverTimingMs").(float64); !ok || ms < 0 {
		t.Errorf("the time %v", at(e, "__meta.serverTimingMs"))
	}
	if ts, _ := at(e, "__meta.timestamp").(string); !strings.HasSuffix(ts, "Z") || len(ts) != len("2026-10-01T09:00:00.000Z") {
		t.Errorf("the timestamp %q", ts)
	}
}

func TestThePanelReadsTheEntriesNewestFirstByComponentAndKind(t *testing.T) {
	rc := New(t.TempDir())
	page := func(component string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			From(r.Context()).Page(component, map[string]any{}, nil, nil)
		}
	}
	var ids []string
	for _, component := range []string{"Home", "Posts/Index", "Posts/Index"} {
		w, _ := served(t, rc, httptest.NewRequest("GET", "/", nil), page(component))
		ids = append(ids, w.Header().Get(HeaderID))
	}
	w, _ := served(t, rc, httptest.NewRequest("GET", "/up", nil), func(w http.ResponseWriter, r *http.Request) {})
	ids = append(ids, w.Header().Get(HeaderID))

	list := func(query string) []string {
		t.Helper()
		w := httptest.NewRecorder()
		rc.Serve(w, httptest.NewRequest("GET", endpoints+query, nil), nil)
		var metas []meta
		if err := json.Unmarshal(w.Body.Bytes(), &metas); w.Code != 200 || err != nil {
			t.Fatalf("%s: %d %s", query, w.Code, w.Body)
		}
		var got []string
		for _, m := range metas {
			got = append(got, m.ID)
		}
		return got
	}
	for query, want := range map[string][]string{
		"":                       {ids[3], ids[2], ids[1], ids[0]},
		"?component=Posts/Index": {ids[2], ids[1]},
		"?type=http":             {ids[3]},
		"?exclude=initial":       {ids[3]},
		"?offset=1&limit=2":      {ids[2], ids[1]},
	} {
		if got := list(query); !slices.Equal(got, want) {
			t.Errorf("listing %q: %v, want %v", query, got, want)
		}
	}
	if got := list(""); len(got) != 4 {
		t.Errorf("the panel's own requests were recorded: %d entries", len(got))
	}

	for path, code := range map[string]int{
		endpoints + "/" + ids[0]:                     200,
		endpoints + "/01K6FN4MW0ZZZZZZZZZZZZZZZZ":    404,
		endpoints + "/..%2F..%2Fgo.mod":              404,
		"/_inertia/devtools":                         404,
		fmt.Sprintf("%s/%s/more", endpoints, ids[0]): 404,
	} {
		w := httptest.NewRecorder()
		rc.Serve(w, httptest.NewRequest("GET", path, nil), nil)
		if w.Code != code || w.Header().Get("Content-Type") != "application/json" {
			t.Errorf("GET %s: %d %s, want %d", path, w.Code, w.Header().Get("Content-Type"), code)
		}
	}
	w = httptest.NewRecorder()
	rc.Serve(w, httptest.NewRequest("DELETE", endpoints, nil), nil)
	if w.Code != 405 || w.Header().Get("Allow") != "GET, HEAD" {
		t.Errorf("DELETE: %d, Allow %q", w.Code, w.Header().Get("Allow"))
	}
}

func TestAnEntryThatCantBeKeptLeavesTheResponseAlone(t *testing.T) {
	file := filepath.Join(t.TempDir(), "a file, not a directory")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	rc := New(file)
	w := httptest.NewRecorder()
	rc.Serve(w, httptest.NewRequest("GET", "/", nil), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "hello") }))
	if w.Code != 200 || w.Body.String() != "hello" {
		t.Errorf("the response %d %q", w.Code, w.Body)
	}
}
