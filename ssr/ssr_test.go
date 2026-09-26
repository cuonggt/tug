package ssr_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/cuonggt/tug/ssr"
)

var ctx = context.Background()

// ssrServer answers as Inertia's SSR server does, at path, rendering a page
// as a title and a div with its component's name.
func ssrServer(t *testing.T, path string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var page struct{ Component string }
		if r.URL.Path != path || r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&page) != nil {
			http.Error(w, "not Inertia's SSR", http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"head": []string{"<title>" + page.Component + "</title>"},
			"body": `<div data-server-rendered="true" id="app">` + page.Component + " at " + path + "</div>",
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAPageRendersThroughTheDevServerWhileItRuns(t *testing.T) {
	dev, prod := ssrServer(t, "/__inertia_ssr"), ssrServer(t, "/render")
	running := dev.URL
	g := &ssr.Gateway{DevServer: func() string { return running }, URL: prod.URL}

	got, err := g.Render(ctx, []byte(`{"component":"Home"}`))
	if err != nil || !strings.Contains(got.Body, "Home at /__inertia_ssr") || len(got.Head) != 1 || got.Head[0] != "<title>Home</title>" {
		t.Fatalf("with the dev server running: %+v, %v", got, err)
	}
	running = ""
	if got, err := g.Render(ctx, []byte(`{"component":"Home"}`)); err != nil || !strings.Contains(got.Body, "Home at /render") {
		t.Errorf("with it stopped: %+v, %v; want the SSR server's", got, err)
	}
}

func TestNoServerRendersNothingAndSaysNothing(t *testing.T) {
	for _, g := range []*ssr.Gateway{
		{},
		{DevServer: func() string { return "" }, Server: &ssr.Server{}},
	} {
		if got, err := g.Render(ctx, []byte(`{"component":"Home"}`)); err != nil || got.Body != "" {
			t.Errorf("got %+v, %v; want nothing, for the browser to render", got, err)
		}
	}
}

func TestAPageThatFailsToRenderSaysWhyAsInertiaTellsIt(t *testing.T) {
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":"window is not defined","type":"browser-api","component":"Home",`+
			`"sourceLocation":"resources/js/pages/Home.tsx:12:3","hint":"Wrap browser-specific code in a useEffect lifecycle hook."}`)
	}))
	defer failing.Close()
	_, err := (&ssr.Gateway{URL: failing.URL}).Render(ctx, []byte(`{"component":"Home"}`))
	want := "ssr: window is not defined, at resources/js/pages/Home.tsx:12:3. Wrap browser-specific code in a useEffect lifecycle hook."
	if err == nil || err.Error() != want {
		t.Errorf("got %v, want %q", err, want)
	}
}

func TestADevServerStillLoadingTheAppRendersNothing(t *testing.T) {
	loading := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "null")
	}))
	defer loading.Close()
	g := &ssr.Gateway{DevServer: func() string { return loading.URL }}
	if got, err := g.Render(ctx, []byte(`{"component":"Home"}`)); err != nil || got.Body != "" {
		t.Errorf("got %+v, %v; want nothing, for the browser to render", got, err)
	}
}

func TestAPageThatTakesTooLongRendersInTheBrowser(t *testing.T) {
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer slow.Close()
	defer close(release)
	g := &ssr.Gateway{URL: slow.URL, Timeout: 50 * time.Millisecond}
	start := time.Now()
	if _, err := g.Render(ctx, []byte(`{"component":"Home"}`)); err == nil || time.Since(start) > time.Second {
		t.Errorf("got %v after %v, want an error after the timeout", err, time.Since(start))
	}
}

// fakeBundle is an SSR bundle as vite build --ssr makes one, but small: it
// answers at SSR_PORT as Inertia's server does, says where it runs from,
// and exits when asked to, as one that crashed would.
var fakeBundle = fstest.MapFS{"ssr.mjs": {Data: []byte(`
import { createServer } from 'node:http'
createServer((req, res) => {
  if (req.url === '/health') return res.end('{"status":"OK"}')
  if (req.url === '/exit') process.exit(1)
  let body = ''
  req.on('data', (chunk) => (body += chunk))
  req.on('end', () => {
    const page = JSON.parse(body)
    res.end(JSON.stringify({
      head: ['<title>' + page.component + '</title>'],
      body: '<div data-server-rendered="true" id="app">' + page.component + ' in ' + process.cwd() + '</div>',
    }))
  })
}).listen(Number(process.env.SSR_PORT), '127.0.0.1')
`)}}

// quiet sends what the package logs to a buffer, for the test to read.
func quiet(t *testing.T) *lockedBuffer {
	t.Helper()
	var logs lockedBuffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &logs
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// eventually waits for f to be true, for five seconds at most.
func eventually(t *testing.T, what string, f func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !f(); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("waited 5 seconds for %s", what)
		}
	}
}

func TestServerRunsTheBundleWithNodeAndStartsItAgainWhenItStops(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil || testing.Short() {
		t.Skip("needs Node")
	}
	logs := quiet(t)
	s := &ssr.Server{Bundle: fakeBundle}
	g := &ssr.Gateway{Server: s}
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error)
	go func() { done <- s.Run(runCtx) }()
	// A test that fails still stops Node, which would outlive it otherwise.
	finish := sync.OnceValue(func() error {
		stop()
		return <-done
	})
	t.Cleanup(func() { finish() })

	eventually(t, "the server to answer", func() bool { return s.URL() != "" })
	first, err := g.Render(ctx, []byte(`{"component":"Home"}`))
	if err != nil || !strings.HasPrefix(first.Body, `<div data-server-rendered="true" id="app">Home in `) {
		t.Fatalf("rendered %+v, %v", first, err)
	}
	dir := strings.TrimSuffix(strings.TrimPrefix(first.Body, `<div data-server-rendered="true" id="app">Home in `), "</div>")

	// It crashes, and pages render in the browser, until it's back.
	url := s.URL()
	http.Get(url + "/exit")
	eventually(t, "the server to be gone", func() bool { return s.URL() == "" })
	if got, err := g.Render(ctx, []byte(`{"component":"Home"}`)); err != nil || got.Body != "" {
		t.Errorf("with the server gone: %+v, %v; want nothing, for the browser", got, err)
	}
	eventually(t, "the server to be back", func() bool { return s.URL() != "" })
	if got, err := g.Render(ctx, []byte(`{"component":"Home"}`)); err != nil || got.Body == "" {
		t.Errorf("with the server back: %+v, %v", got, err)
	}

	if err := finish(); err != nil {
		t.Errorf("Run returned %v", err)
	}
	if s.URL() != "" {
		t.Error("the server still has a URL after Run")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the bundle is still at %s: %v", dir, err)
	}
	if out := logs.String(); !strings.Contains(out, "the SSR server stopped") {
		t.Errorf("logged:\n%s", out)
	}
}

func TestWithoutABundleOrNodePagesRenderInTheBrowser(t *testing.T) {
	logs := quiet(t)
	for _, s := range []*ssr.Server{
		{Bundle: fstest.MapFS{".gitkeep": {}}},
		{Bundle: fakeBundle, Node: "no-such-node"},
	} {
		done := make(chan error)
		go func() { done <- s.Run(ctx) }()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Run returned %v, want nil: the app serves without SSR", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Run went on with nothing to run")
		}
	}
	if out := logs.String(); !strings.Contains(out, "no SSR bundle") || !strings.Contains(out, "Node isn't here") {
		t.Errorf("logged:\n%s", out)
	}
}
