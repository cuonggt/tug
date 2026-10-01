package vite

import (
	"bytes"
	"compress/gzip"
	"html/template"
	"io"
	"io/fs"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
)

// manifest is Vite's own example from its backend integration guide: two
// entries sharing a chunk, one with CSS, and a dynamic import.
const manifest = `{
  "_shared-B7PI925R.js": {"file": "assets/shared-B7PI925R.js", "name": "shared", "css": ["assets/shared-ChJ_j-JJ.css"]},
  "baz.js": {"file": "assets/baz-B2H3sXNv.js", "name": "baz", "src": "baz.js", "isDynamicEntry": true},
  "views/bar.js": {"file": "assets/bar-gkvgaI9m.js", "name": "bar", "src": "views/bar.js", "isEntry": true, "imports": ["_shared-B7PI925R.js"], "dynamicImports": ["baz.js"]},
  "views/foo.js": {"file": "assets/foo-BRBmoGS9.js", "name": "foo", "src": "views/foo.js", "isEntry": true, "imports": ["_shared-B7PI925R.js"], "css": ["assets/foo-5UjPuW-k.css"]},
  "styles/app.css": {"file": "assets/app-D6B9zQ1Q.css", "src": "styles/app.css", "isEntry": true}
}`

func build() fstest.MapFS {
	return fstest.MapFS{
		".vite/manifest.json":       {Data: []byte(manifest)},
		"assets/foo-BRBmoGS9.js":    {Data: []byte("console.log('foo')")},
		"assets/shared-B7PI925R.js": {Data: []byte("export {}")},
	}
}

func newVite(t *testing.T, cfg Config) *Vite {
	t.Helper()
	v, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestTagsForABuildLoadTheEntryItsCSSAndItsImports(t *testing.T) {
	v := newVite(t, Config{Build: build()})
	got, err := v.Tags("", "views/foo.js")
	if err != nil {
		t.Fatal(err)
	}
	want := `<link rel="stylesheet" href="/build/assets/foo-5UjPuW-k.css">` +
		`<link rel="stylesheet" href="/build/assets/shared-ChJ_j-JJ.css">` +
		`<link rel="modulepreload" href="/build/assets/shared-B7PI925R.js">` +
		`<script type="module" src="/build/assets/foo-BRBmoGS9.js"></script>`
	if string(got) != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestTagsNameEachFileOnceAcrossEntries(t *testing.T) {
	v := newVite(t, Config{Build: build(), Base: "/static/"})
	got, err := v.Tags("", "styles/app.css", "views/foo.js", "views/bar.js")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(got), "shared-B7PI925R.js"); n != 1 {
		t.Errorf("the shared chunk is named %d times: %s", n, got)
	}
	if !strings.Contains(string(got), `<link rel="stylesheet" href="/static/assets/app-D6B9zQ1Q.css">`) {
		t.Errorf("no stylesheet for the CSS entry under the base: %s", got)
	}
	if strings.Contains(string(got), "baz") {
		t.Errorf("a dynamic import was loaded up front: %s", got)
	}
}

func TestTagsWhileTheDevServerRunsLoadFromIt(t *testing.T) {
	hot := filepath.Join(t.TempDir(), "hot")
	v := newVite(t, Config{Build: build(), HotFile: hot})

	if refresh := v.ReactRefresh(""); refresh != "" {
		t.Errorf("a build got the React preamble: %s", refresh)
	}
	os.WriteFile(hot, []byte("http://localhost:5173/\n"), 0o644)

	got, err := v.Tags("", "resources/js/app.tsx", "resources/css/app.css")
	if err != nil {
		t.Fatal(err)
	}
	want := `<script type="module" src="http://localhost:5173/@vite/client"></script>` +
		`<script type="module" src="http://localhost:5173/resources/js/app.tsx"></script>` +
		`<link rel="stylesheet" href="http://localhost:5173/resources/css/app.css">`
	if string(got) != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	if refresh := v.ReactRefresh(""); !strings.Contains(string(refresh), "import RefreshRuntime from 'http://localhost:5173/@react-refresh'") {
		t.Errorf("preamble %s", refresh)
	}

	// The dev server stops, and the build takes over without a restart.
	os.Remove(hot)
	if got, _ := v.Tags("", "views/foo.js"); strings.Contains(string(got), "5173") {
		t.Errorf("still loading from the dev server: %s", got)
	}
}

func TestTagsSayWhatToRunWhenThereIsNothingToLoad(t *testing.T) {
	v := newVite(t, Config{Build: fstest.MapFS{}})
	if _, err := v.Tags("", "resources/js/app.tsx"); err == nil || !strings.Contains(err.Error(), "npm run build") {
		t.Errorf("err = %v", err)
	}
	v = newVite(t, Config{Build: build()})
	if _, err := v.Tags("", "resources/js/pages/Nope.tsx"); err == nil || !strings.Contains(err.Error(), "resources/js/pages/Nope.tsx") {
		t.Errorf("err = %v", err)
	}
}

func TestTheVersionIsAHashOfTheManifest(t *testing.T) {
	a := newVite(t, Config{Build: build()}).Version()
	changed := build()
	changed[".vite/manifest.json"] = &fstest.MapFile{Data: []byte(`{}`)}
	b := newVite(t, Config{Build: changed}).Version()
	if a == "" || b == "" || a == b {
		t.Errorf("versions %q and %q", a, b)
	}
	if v := newVite(t, Config{}).Version(); v != "" {
		t.Errorf("no build has the version %q", v)
	}
}

func TestFuncsWorkInATemplate(t *testing.T) {
	v := newVite(t, Config{Build: build()})
	tmpl := template.Must(template.New("").Funcs(v.Funcs()).Parse(`{{ viteReactRefresh .Nonce }}{{ vite .Nonce "views/foo.js" }}`))
	var b strings.Builder
	if err := tmpl.Execute(&b, struct{ Nonce string }{"N0nce"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `<script type="module" src="/build/assets/foo-BRBmoGS9.js" nonce="N0nce"></script>`) {
		t.Fatalf("got %s", b.String())
	}
}

func TestBuiltFilesAreServedAndAssetsCachedForAYear(t *testing.T) {
	v := newVite(t, Config{Build: build()})

	rec := httptest.NewRecorder()
	v.ServeHTTP(rec, httptest.NewRequest("GET", "/build/assets/foo-BRBmoGS9.js", nil))
	if rec.Code != 200 || rec.Body.String() != "console.log('foo')" {
		t.Fatalf("got %d %q", rec.Code, rec.Body)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control %q", cc)
	}

	for _, path := range []string{"/build/.vite/manifest.json", "/build/assets", "/build/", "/build/nope.js", "/other/assets/foo-BRBmoGS9.js"} {
		rec := httptest.NewRecorder()
		v.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 404 {
			t.Errorf("%s: %d, want 404", path, rec.Code)
		}
	}
}

func TestBaseStartsAndEndsWithASlash(t *testing.T) {
	for _, base := range []string{"build/", "/build", "build"} {
		if _, err := New(Config{Base: base}); err == nil {
			t.Errorf("New took the base %q", base)
		}
	}
}

func TestTheScriptsTagsLoadCarryThePagesNonce(t *testing.T) {
	v := newVite(t, Config{Build: build()})
	got, err := v.Tags("N0nce", "views/foo.js")
	if err != nil {
		t.Fatal(err)
	}
	// Styles go by the policy's style-src, which a nonce has no part in.
	want := `<link rel="stylesheet" href="/build/assets/foo-5UjPuW-k.css">` +
		`<link rel="stylesheet" href="/build/assets/shared-ChJ_j-JJ.css">` +
		`<link rel="modulepreload" href="/build/assets/shared-B7PI925R.js" nonce="N0nce">` +
		`<script type="module" src="/build/assets/foo-BRBmoGS9.js" nonce="N0nce"></script>`
	if string(got) != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}

	hot := filepath.Join(t.TempDir(), "hot")
	os.WriteFile(hot, []byte("http://localhost:5173"), 0o644)
	v = newVite(t, Config{Build: build(), HotFile: hot})
	got, err = v.Tags("N0nce", "resources/js/app.tsx", "resources/css/app.css")
	if err != nil {
		t.Fatal(err)
	}
	want = `<script type="module" src="http://localhost:5173/@vite/client" nonce="N0nce"></script>` +
		`<script type="module" src="http://localhost:5173/resources/js/app.tsx" nonce="N0nce"></script>` +
		`<link rel="stylesheet" href="http://localhost:5173/resources/css/app.css">`
	if string(got) != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	if refresh := v.ReactRefresh("N0nce"); !strings.HasPrefix(string(refresh), `<script type="module" nonce="N0nce">`) {
		t.Errorf("preamble %s", refresh)
	}
}

func TestTagsTakeThePagesNonceFirst(t *testing.T) {
	v := newVite(t, Config{Build: build()})
	// As a template written before the nonce names its entries.
	_, err := v.Tags("views/foo.js", "views/bar.js")
	if err == nil || !strings.Contains(err.Error(), `{{ vite .Nonce "resources/js/app.tsx" }}`) {
		t.Errorf("err = %v", err)
	}
}

// script is JavaScript long enough that gzip makes it smaller.
var script = strings.Repeat("console.log('a line of the app');\n", 40)

// compressing is a build with a script that compresses, and an image.
func compressing() fstest.MapFS {
	b := build()
	b["assets/app-Q1w2E3r4.js"] = &fstest.MapFile{Data: []byte(script)}
	b["assets/logo-A1b2C3d4.png"] = &fstest.MapFile{Data: []byte("\x89PNG\r\n\x1a\n" + script)}
	return b
}

// get sends a request of method for path through v, with headers, name and
// value pairs.
func get(v *Vite, method, path string, headers ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	for i := 0; i+1 < len(headers); i += 2 {
		r.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	v.ServeHTTP(w, r)
	return w
}

func gunzip(t *testing.T, data []byte) string {
	t.Helper()
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestAFileThatCompressesGoesGzippedToABrowserThatTakesIt(t *testing.T) {
	v := newVite(t, Config{Build: compressing()})
	for _, accept := range []string{"gzip, deflate, br, zstd", "*", "GZIP", "x-gzip", "gzip;q=0.5", "*;q=0, gzip", "identity;q=0, gzip"} {
		rec := get(v, "GET", "/build/assets/app-Q1w2E3r4.js", "Accept-Encoding", accept)
		h := rec.Header()
		if rec.Code != 200 || h.Get("Content-Encoding") != "gzip" || h.Get("Vary") != "Accept-Encoding" {
			t.Fatalf("Accept-Encoding %q: %d, Content-Encoding %q, Vary %q", accept, rec.Code, h.Get("Content-Encoding"), h.Get("Vary"))
		}
		if got := gunzip(t, rec.Body.Bytes()); got != script || rec.Body.Len() >= len(script) {
			t.Errorf("Accept-Encoding %q: %d bytes, of %d, unzipped to the script: %v", accept, rec.Body.Len(), len(script), got == script)
		}
		if h.Get("Content-Length") != strconv.Itoa(rec.Body.Len()) || h.Get("Content-Type") != "text/javascript; charset=utf-8" || h.Get("Cache-Control") != "public, max-age=31536000, immutable" {
			t.Errorf("Accept-Encoding %q: the headers %v", accept, h)
		}
	}

	// As it is to a browser that doesn't take gzip, which a cache between
	// keeps apart from the gzipped.
	for _, accept := range []string{"", "identity", "deflate", "br", "zstd", "gzip;q=0", "gzip;q=0, *", "*;q=0", "gzip;q=nope", "gzip;q=NaN, *"} {
		rec := get(v, "GET", "/build/assets/app-Q1w2E3r4.js", "Accept-Encoding", accept)
		h := rec.Header()
		if rec.Code != 200 || h.Get("Content-Encoding") != "" || h.Get("Vary") != "Accept-Encoding" || rec.Body.String() != script {
			t.Errorf("Accept-Encoding %q: %d, Content-Encoding %q, Vary %q, %d bytes", accept, rec.Code, h.Get("Content-Encoding"), h.Get("Vary"), rec.Body.Len())
		}
	}
}

func TestWhatDoesntCompressGoesAsItIs(t *testing.T) {
	v := newVite(t, Config{Build: compressing()})
	for path, body := range map[string]string{
		"/build/assets/logo-A1b2C3d4.png": "\x89PNG\r\n\x1a\n" + script, // compressed by its format
		"/build/assets/foo-BRBmoGS9.js":   "console.log('foo')",         // which gzip makes longer
	} {
		rec := get(v, "GET", path, "Accept-Encoding", "gzip, deflate, br, zstd")
		if rec.Code != 200 || rec.Header().Get("Content-Encoding") != "" || rec.Header().Get("Vary") != "" || rec.Body.String() != body {
			t.Errorf("%s: %d, Content-Encoding %q, Vary %q", path, rec.Code, rec.Header().Get("Content-Encoding"), rec.Header().Get("Vary"))
		}
	}
}

func TestARangeOrAHeadIsOfTheCompressedBytes(t *testing.T) {
	v := newVite(t, Config{Build: compressing()})
	whole := get(v, "GET", "/build/assets/app-Q1w2E3r4.js", "Accept-Encoding", "gzip").Body.Bytes()

	rec := get(v, "GET", "/build/assets/app-Q1w2E3r4.js", "Accept-Encoding", "gzip", "Range", "bytes=0-9")
	want := "bytes 0-9/" + strconv.Itoa(len(whole))
	if rec.Code != 206 || rec.Header().Get("Content-Encoding") != "gzip" || rec.Header().Get("Content-Range") != want || !bytes.Equal(rec.Body.Bytes(), whole[:10]) {
		t.Errorf("a range: %d, Content-Range %q, want %q", rec.Code, rec.Header().Get("Content-Range"), want)
	}

	// One past the end is refused, with the length of what's sent then.
	rec = get(v, "GET", "/build/assets/app-Q1w2E3r4.js", "Accept-Encoding", "gzip", "Range", "bytes=99999-")
	if n := rec.Header().Get("Content-Length"); rec.Code != 416 || n != "" && n != strconv.Itoa(rec.Body.Len()) {
		t.Errorf("a range past the end: %d, Content-Length %q of %d bytes", rec.Code, n, rec.Body.Len())
	}

	rec = get(v, "HEAD", "/build/assets/app-Q1w2E3r4.js", "Accept-Encoding", "gzip")
	if rec.Code != 200 || rec.Header().Get("Content-Encoding") != "gzip" || rec.Header().Get("Content-Length") != strconv.Itoa(len(whole)) || rec.Body.Len() != 0 {
		t.Errorf("HEAD: %d, Content-Encoding %q, Content-Length %q, %d bytes", rec.Code, rec.Header().Get("Content-Encoding"), rec.Header().Get("Content-Length"), rec.Body.Len())
	}
}

func TestTheBuildsOwnCompressedCopiesGoFirst(t *testing.T) {
	b := compressing()
	b["assets/app-Q1w2E3r4.js.br"] = &fstest.MapFile{Data: []byte("the build's brotli")}
	b["assets/app-Q1w2E3r4.js.gz"] = &fstest.MapFile{Data: []byte("the build's gzip")}
	v := newVite(t, Config{Build: b})
	for accept, want := range map[string]string{
		"gzip, deflate, br, zstd": "the build's brotli",
		"br;q=0.1, gzip":          "the build's brotli",
		"gzip":                    "the build's gzip",
		"br;q=0, gzip":            "the build's gzip",
		"zstd":                    script,
	} {
		rec := get(v, "GET", "/build/assets/app-Q1w2E3r4.js", "Accept-Encoding", accept)
		if rec.Body.String() != want || rec.Header().Get("Vary") != "Accept-Encoding" || rec.Header().Get("Content-Type") != "text/javascript; charset=utf-8" {
			t.Errorf("Accept-Encoding %q: %q, Content-Encoding %q, Vary %q", accept, rec.Body, rec.Header().Get("Content-Encoding"), rec.Header().Get("Vary"))
		}
	}
}

// opens counts the files opened in a build, by name.
type opens struct {
	build fstest.MapFS
	mu    sync.Mutex
	n     map[string]int
}

func (o *opens) Open(name string) (fs.File, error) {
	o.mu.Lock()
	o.n[name]++
	o.mu.Unlock()
	return o.build.Open(name)
}

func (o *opens) Stat(name string) (fs.FileInfo, error) { return o.build.Stat(name) }

func TestRequestsAtOnceCompressAFileOnce(t *testing.T) {
	o := &opens{build: compressing(), n: map[string]int{}}
	v := newVite(t, Config{Build: o})
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range 20 {
		wg.Go(func() {
			<-start
			if rec := get(v, "GET", "/build/assets/app-Q1w2E3r4.js", "Accept-Encoding", "gzip"); gunzip(t, rec.Body.Bytes()) != script {
				t.Error("a request didn't get the script")
			}
		})
	}
	close(start)
	wg.Wait()
	if n := o.n["assets/app-Q1w2E3r4.js"]; n != 1 {
		t.Errorf("the script was read %d times", n)
	}
}
