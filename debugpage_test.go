package tug

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/cuonggt/tug/middleware"
)

// loadError is an error of a type of the app's, as a database driver's is.
type loadError struct{ table string }

func (e *loadError) Error() string { return "no rows in " + e.table }

// explode panics where the page should show it, in a frame of the app's.
//
//go:noinline
func explode(n int) int {
	var counts map[string]int
	counts["posts"] = n // assignment to entry in nil map
	return n
}

func debugApp(t *testing.T, cfg Config) *App {
	t.Helper()
	captureLog(t)
	cfg.Debug = true
	return New(cfg)
}

const browser = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"

func TestDebugShowsAReturnedErrorAsAPageOfWhatItWrapsAndTheRouteThatAnswered(t *testing.T) {
	app := debugApp(t, Config{})
	app.Use(middleware.RequestID())
	_, file, line, _ := runtime.Caller(0)
	app.Get("/posts/{id}", func(c *Ctx) error {
		return fmt.Errorf("loading post %s: %w", c.Param("id"), &loadError{"posts"})
	}).Name("posts.show")

	rec := serve(app, "GET", "/posts/9?page=2&token=s3cret", "", "Accept", browser,
		"Cookie", "tug_session=abc", "Authorization", "Bearer xyz", "X-Request-ID", "REQ123")
	body := rec.Body.String()
	if rec.Code != 500 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("got %d %q: %s", rec.Code, rec.Header().Get("Content-Type"), body)
	}
	for _, want := range []string{
		"<h1>loading post 9: no rows in posts</h1>",
		`<code class="type">*fmt.wrapError</code>loading post 9: no rows in posts`,
		`<code class="type">*tug.loadError</code>no rows in posts`,
		"An error a handler returns has no stack",
		"<code>GET /posts/{id}</code>",
		"<code>posts.show</code>",
		"TestDebugShowsAReturnedErrorAsAPageOfWhatItWrapsAndTheRouteThatAnswered.func1</code>",
		fmt.Sprintf("%s:%d", file, line+1),
		`<span class="line at"><span class="n">` + strconv.Itoa(line+1) + `</span>	app.Get(&#34;/posts/{id}&#34;, func(c *Ctx) error {</span>`,
		"<code>/posts/9?page=2&amp;token=[REDACTED]</code>",
		"<dt><code>{id}</code></dt><dd><code>9</code></dd>",
		"<dt>Request ID</dt><dd><code>REQ123</code></dd>",
		`<th class="mono">cookie</th><td class="mono">[REDACTED]</td>`,
		`<th class="mono">authorization</th><td class="mono">[REDACTED]</td>`,
		`<th class="mono">host</th><td class="mono">example.com</td>`,
		"<footer>Go " + strings.TrimPrefix(runtime.Version(), "go") + " · tug ",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page doesn't have %q", want)
		}
	}
	for _, secret := range []string{"s3cret", "Bearer xyz", "tug_session=abc"} {
		if strings.Contains(body, secret) {
			t.Errorf("the page shows %q", secret)
		}
	}
	if t.Failed() {
		t.Log(body)
	}
}

func TestDebugShowsAPanicsStackWithTheAppsFramesOpenAndTheRestFolded(t *testing.T) {
	app := debugApp(t, Config{})
	app.Get("/explode", func(c *Ctx) error {
		explode(1)
		return nil
	})

	body := serve(app, "GET", "/explode", "", "Accept", browser).Body.String()
	_, file, _, _ := runtime.Caller(0)
	fn := runtime.FuncForPC(reflect.ValueOf(explode).Pointer())
	_, start := fn.FileLine(fn.Entry())
	at := start + 2 // the line of explode that panics
	for _, want := range []string{
		"<h1>panic: assignment to entry in nil map</h1>",
		`<code class="type">*tug.PanicError</code>panic: assignment to entry in nil map`,
		`<code class="type">runtime.plainError</code>assignment to entry in nil map`,
		// The first frame is the one that panicked, the app's, open, with
		// its line marked among those around it.
		`<h2>The stack</h2>
<div class="frame">
<div class="head"><div class="func mono">github.com/cuonggt/tug.explode</div><div class="file mono">` + fmt.Sprintf("%s:%d", file, at) + `</div></div>`,
		`<span class="line at"><span class="n">` + strconv.Itoa(at) + `</span>	counts[&#34;posts&#34;] = n // assignment to entry in nil map</span>`,
		`<span class="line"><span class="n">` + strconv.Itoa(at-1) + `</span>	var counts map[string]int</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page doesn't have %q", want)
		}
	}
	// tug's frames and the standard library's fold, and say whose they are.
	if !regexp.MustCompile(`<details class="folded">\n<summary>\d+ frames of tug and the standard library</summary>`).MatchString(body) {
		t.Error("the page doesn't fold tug's frames and the standard library's")
	}
	if strings.Contains(body, "An error a handler returns has no stack") {
		t.Error("the page says a panic has no stack")
	}
	if t.Failed() {
		t.Log(body)
	}
}

func TestTheDebugPageRunsNoScriptAndCarriesTheResponsesNonce(t *testing.T) {
	app := debugApp(t, Config{})
	app.Use(middleware.CSP(middleware.CSPConfig{}))
	app.Get("/explode", func(c *Ctx) error {
		explode(1)
		return nil
	})

	rec := serve(app, "GET", "/explode", "", "Accept", browser)
	nonce := regexp.MustCompile(`'nonce-([^']+)'`).FindStringSubmatch(rec.Header().Get("Content-Security-Policy"))
	if nonce == nil {
		t.Fatalf("no nonce in %q", rec.Header().Get("Content-Security-Policy"))
	}
	body := rec.Body.String()
	if !strings.Contains(body, `<style nonce="`+nonce[1]+`">`) {
		t.Error("the page's style doesn't carry the response's nonce")
	}
	if strings.Contains(body, "<script") {
		t.Error("the page has a script, which Inertia's modal wouldn't run")
	}
}

func TestTheDebugPageLinksEachFrameToTheEditor(t *testing.T) {
	app := debugApp(t, Config{Editor: "vscode"})
	app.Get("/explode", func(c *Ctx) error {
		explode(1)
		return nil
	})
	body := serve(app, "GET", "/explode", "", "Accept", browser).Body.String()
	_, file, _, _ := runtime.Caller(0)
	if !strings.Contains(body, `<a href="vscode://file`+file+`:`) {
		t.Errorf("no link to the editor in %s", body)
	}
}

func TestAnEditorsLinkOpensAFileAtALine(t *testing.T) {
	for editor, want := range map[string]string{
		"vscode":                            "vscode://file/src/my%20app/main.go:12",
		"cursor":                            "cursor://file/src/my%20app/main.go:12",
		"zed":                               "zed://file/src/my%20app/main.go:12",
		"goland":                            "goland://open?file=%2Fsrc%2Fmy+app%2Fmain.go&line=12",
		"sublime":                           "subl://open?url=file%3A%2F%2F%2Fsrc%2Fmy%2520app%2Fmain.go&line=12",
		"myeditor://open?f={file}&l={line}": "myeditor://open?f=/src/my%20app/main.go&l=12",
		"notepad":                           "",
		"":                                  "",
	} {
		if got := string(editorLink(editor, "/src/my app/main.go", 12)); got != want {
			t.Errorf("%q: %q, want %q", editor, got, want)
		}
	}
	if got := string(editorLink("vscode", `C:\src\blog\main.go`, 3)); runtime.GOOS == "windows" && got != "vscode://file/C:/src/blog/main.go:3" {
		t.Errorf("a Windows path: %q", got)
	}
}

func TestAFrameWhoseFileIsntThereHasNoLines(t *testing.T) {
	src := sources{}
	if lines := src.around("/nowhere/main.go", 3, 5); lines != nil {
		t.Errorf("lines of a file that isn't there: %v", lines)
	}
	_, file, line, _ := runtime.Caller(0)
	if lines := src.around(file, line, 1); len(lines) != 3 || !lines[1].At || lines[1].N != line {
		t.Errorf("lines around %d: %+v", line, lines)
	}
	if lines := src.around(file, 1_000_000, 1); lines != nil {
		t.Errorf("lines past the file's end: %v", lines)
	}
}

func TestDebugAnswersJSONToAClientThatAsksForItAndTextToTheRest(t *testing.T) {
	app := debugApp(t, Config{})
	app.Get("/explode", func(c *Ctx) error {
		explode(1)
		return nil
	})
	app.Get("/posts", func(c *Ctx) error { return fmt.Errorf("listing: %w", &loadError{"posts"}) })

	rec := serve(app, "GET", "/explode", "", "Accept", "application/json")
	var got struct {
		Message string
		Errors  []struct{ Type, Message string }
		Stack   []struct {
			Function, File string
			Line           int
		}
		Route string
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("%d %s: %v", rec.Code, rec.Body, err)
	}
	if got.Message != "panic: assignment to entry in nil map" || len(got.Errors) < 2 || got.Errors[0].Type != "*tug.PanicError" ||
		len(got.Stack) == 0 || got.Stack[0].Function != "github.com/cuonggt/tug.explode" || got.Route != "GET /explode" {
		t.Errorf("JSON: %s", rec.Body)
	}
	rec = serve(app, "GET", "/posts", "", "Accept", "application/json")
	if body := rec.Body.String(); !strings.Contains(body, `"message":"listing: no rows in posts"`) || strings.Contains(body, `"stack"`) {
		t.Errorf("a returned error's JSON: %s", body)
	}

	// curl takes anything, and gets the text.
	rec = serve(app, "GET", "/explode", "", "Accept", "*/*")
	if body := rec.Body.String(); !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") ||
		!strings.HasPrefix(body, "panic: assignment to entry in nil map\n\ngoroutine ") {
		t.Errorf("text: %q %q", rec.Header().Get("Content-Type"), body)
	}
}

func TestWithoutDebugAServerErrorShowsNoneOfItsDetails(t *testing.T) {
	captureLog(t)
	app := New(Config{})
	app.Get("/explode", func(c *Ctx) error {
		explode(1)
		return nil
	})
	rec := serve(app, "GET", "/explode", "", "Accept", browser)
	if rec.Code != 500 || rec.Body.String() != "Internal Server Error" {
		t.Errorf("got %d %q", rec.Code, rec.Body)
	}
}

func TestTheCausesAreEachErrorWrappedAndEachJoined(t *testing.T) {
	err := fmt.Errorf("saving: %w", errors.Join(errors.New("title is taken"), &loadError{"tags"}))
	want := []debugCause{
		{"*fmt.wrapError", "saving: title is taken\nno rows in tags", 0},
		{"*errors.joinError", "title is taken\nno rows in tags", 0},
		{"*errors.errorString", "title is taken", 1},
		{"*tug.loadError", "no rows in tags", 1},
	}
	if got := causes(err); !reflect.DeepEqual(got, want) {
		t.Errorf("causes:\n%+v\nwant\n%+v", got, want)
	}
	// A panic's value that isn't an error is one of them, with its type.
	if got := causes(&PanicError{Value: 42}); len(got) != 2 || got[1] != (debugCause{"int", "42", 0}) {
		t.Errorf("a panic's causes: %+v", got)
	}
}

func TestAPanicErrorWithoutItsFramesShowsItsStackAsText(t *testing.T) {
	app := debugApp(t, Config{})
	// As an app's own recover makes one, with the stack as text alone.
	app.Get("/", func(c *Ctx) error {
		return &PanicError{Value: "worker died", Stack: []byte("goroutine 7 [running]:\nmain.work()")}
	})
	body := serve(app, "GET", "/", "", "Accept", browser).Body.String()
	if !strings.Contains(body, "<h1>panic: worker died</h1>") || !strings.Contains(body, "<pre class=\"stack\">goroutine 7 [running]:\nmain.work()</pre>") {
		t.Errorf("the page: %s", body)
	}
}
