package vite

import (
	"html/template"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	got, err := v.Tags("views/foo.js")
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
	got, err := v.Tags("styles/app.css", "views/foo.js", "views/bar.js")
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

	if refresh := v.ReactRefresh(); refresh != "" {
		t.Errorf("a build got the React preamble: %s", refresh)
	}
	os.WriteFile(hot, []byte("http://localhost:5173/\n"), 0o644)

	got, err := v.Tags("resources/js/app.tsx", "resources/css/app.css")
	if err != nil {
		t.Fatal(err)
	}
	want := `<script type="module" src="http://localhost:5173/@vite/client"></script>` +
		`<script type="module" src="http://localhost:5173/resources/js/app.tsx"></script>` +
		`<link rel="stylesheet" href="http://localhost:5173/resources/css/app.css">`
	if string(got) != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	if refresh := v.ReactRefresh(); !strings.Contains(string(refresh), "import RefreshRuntime from 'http://localhost:5173/@react-refresh'") {
		t.Errorf("preamble %s", refresh)
	}

	// The dev server stops, and the build takes over without a restart.
	os.Remove(hot)
	if got, _ := v.Tags("views/foo.js"); strings.Contains(string(got), "5173") {
		t.Errorf("still loading from the dev server: %s", got)
	}
}

func TestTagsSayWhatToRunWhenThereIsNothingToLoad(t *testing.T) {
	v := newVite(t, Config{Build: fstest.MapFS{}})
	if _, err := v.Tags("resources/js/app.tsx"); err == nil || !strings.Contains(err.Error(), "npm run build") {
		t.Errorf("err = %v", err)
	}
	v = newVite(t, Config{Build: build()})
	if _, err := v.Tags("resources/js/pages/Nope.tsx"); err == nil || !strings.Contains(err.Error(), "resources/js/pages/Nope.tsx") {
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
	tmpl := template.Must(template.New("").Funcs(v.Funcs()).Parse(`{{ viteReactRefresh }}{{ vite "views/foo.js" }}`))
	var b strings.Builder
	if err := tmpl.Execute(&b, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `<script type="module" src="/build/assets/foo-BRBmoGS9.js"></script>`) {
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
