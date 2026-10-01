package tug

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"

	"github.com/cuonggt/tug/internal/devtools"
	"github.com/cuonggt/tug/internal/frames"
	"github.com/cuonggt/tug/internal/nonce"
)

//go:embed debugpage.html
var debugPageHTML string

var debugPageTemplate = template.Must(template.New("debug").Parse(debugPageHTML))

// debugError answers a server error under Config.Debug: with a page of it
// to a client that takes HTML, as a browser and Inertia's client do, which
// shows it in its modal; as JSON to one that asks for JSON first; and as
// text, the error and a panic's stack, to the rest.
func (c *Ctx) debugError(code int, err error) {
	r := c.Request()
	var pe *PanicError
	errors.As(err, &pe)
	switch {
	case wantsJSON(r):
		c.JSON(code, c.debugPage(code, err, pe).json())
	case takesHTML(r):
		var b bytes.Buffer
		if err := debugPageTemplate.Execute(&b, c.debugPage(code, err, pe)); err != nil {
			slog.ErrorContext(r.Context(), "the debug page failed", "err", err)
			break
		}
		c.rw.Header().Set("Cache-Control", "no-store")
		c.HTML(code, b.String())
		return
	}
	if c.Written() {
		return
	}
	message := err.Error()
	if pe != nil {
		message += "\n\n" + string(pe.Stack)
	}
	c.String(code, message)
}

// takesHTML reports whether the client takes HTML, as a browser says in
// its Accept header, and Inertia's client too.
func takesHTML(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

// debugPage is what the page a server error is shown with, under
// Config.Debug, says of it.
type debugPage struct {
	Status     int
	StatusText string
	Message    string
	Errors     []debugCause
	Returned   bool // a handler returned the error, which has no stack
	Panic      *debugPanic
	Route      *debugRoute
	Request    debugRequest
	Go, Tug    string // their versions
	Nonce      string
}

// debugCause is the error, or one it wraps.
type debugCause struct {
	Type    string
	Message string
	Depth   int // how far inside errors that join several, as errors.Join's, it is
}

// debugPanic is a panic's stack, from the frame that panicked, in runs of
// the app's frames, each shown open, and of the others, tug's, the
// standard library's and the dependencies', folded; or as the text of
// PanicError's Stack, for one with no frames, as one an app made.
type debugPanic struct {
	Groups []debugGroup
	Stack  string
	frames []runtime.Frame
}

type debugGroup struct {
	App     bool
	Summary string // what a folded run's frames are
	Frames  []debugFrame
}

type debugFrame struct {
	Function string
	File     string
	Line     int
	Link     template.URL // to the frame in the editor Config.Editor names
	Lines    []debugLine  // of the app's frame's source, around Line
}

type debugLine struct {
	N    int
	Text string
	At   bool // the frame's own
}

type debugRoute struct {
	Route   string // as String says it, "GET /posts/{id}"
	Name    string
	Handler string      // the function, as Go names it
	Added   *debugFrame // where the app added it, with its lines
}

type debugRequest struct {
	Method  string
	URL     string
	ID      string // the request's ID, which RequestID sends back
	Values  []debugPair
	Headers []debugPair
}

type debugPair struct{ Name, Value string }

// debugPage is the page of err, a server error, which pe is when err is
// a handler's panic.
func (c *Ctx) debugPage(code int, err error, pe *PanicError) debugPage {
	r := c.Request()
	editor := c.app.config.Editor
	src := sources{}
	p := debugPage{
		Status:     code,
		StatusText: http.StatusText(code),
		Message:    err.Error(),
		Errors:     causes(err),
		Returned:   pe == nil,
		Go:         strings.TrimPrefix(runtime.Version(), "go"),
		Tug:        tugVersion,
		Nonce:      nonce.From(r.Context()),
	}
	if pe != nil {
		p.Panic = panicOf(pe, editor, src)
	}
	if rt := c.route; rt != nil {
		p.Route = &debugRoute{Route: rt.shown, Name: rt.name, Handler: funcName(rt.h)}
		if at := rt.added; at != nil {
			p.Route.Added = &debugFrame{File: at.File, Line: at.Line, Link: editorLink(editor, at.File, at.Line), Lines: src.around(at.File, at.Line, 2)}
		}
	}
	p.Request = requestOf(r, c.route, c.rw.Header().Get("X-Request-ID"))
	return p
}

// json is the page as JSON, for a client that asks for it: the error, what
// it wraps, a panic's frames, and the route.
func (p debugPage) json() any {
	type cause struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	type frame struct {
		Function string `json:"function"`
		File     string `json:"file"`
		Line     int    `json:"line"`
	}
	out := struct {
		Message string  `json:"message"`
		Errors  []cause `json:"errors"`
		Stack   []frame `json:"stack,omitempty"`
		Route   string  `json:"route,omitempty"`
	}{Message: p.Message}
	for _, e := range p.Errors {
		out.Errors = append(out.Errors, cause{e.Type, e.Message})
	}
	if p.Panic != nil {
		for _, f := range p.Panic.frames {
			out.Stack = append(out.Stack, frame{f.Function, f.File, f.Line})
		}
	}
	if p.Route != nil {
		out.Route = p.Route.Route
	}
	return out
}

// causes are err and each error it wraps, by Unwrap, those an error that
// joins several wraps a level further in, and a panic's value that isn't
// an error.
func causes(err error) []debugCause {
	var out []debugCause
	var walk func(err error, depth int)
	walk = func(err error, depth int) {
		for err != nil && len(out) < 32 {
			out = append(out, debugCause{Type: fmt.Sprintf("%T", err), Message: err.Error(), Depth: min(depth, 4)})
			if pe, ok := err.(*PanicError); ok {
				if _, isErr := pe.Value.(error); !isErr {
					out = append(out, debugCause{Type: fmt.Sprintf("%T", pe.Value), Message: fmt.Sprint(pe.Value), Depth: min(depth, 4)})
				}
			}
			switch e := err.(type) {
			case interface{ Unwrap() []error }:
				for _, inner := range e.Unwrap() {
					walk(inner, depth+1)
				}
				return
			case interface{ Unwrap() error }:
				err = e.Unwrap()
			default:
				return
			}
		}
	}
	walk(err, 0)
	return out
}

// panicOf is the stack of pe, from the frame that panicked.
func panicOf(pe *PanicError, editor string, src sources) *debugPanic {
	if len(pe.pcs) == 0 {
		return &debugPanic{Stack: string(pe.Stack)}
	}
	var fs []runtime.Frame
	stack := runtime.CallersFrames(pe.pcs)
	for {
		f, more := stack.Next()
		fs = append(fs, f)
		if !more {
			break
		}
	}
	fs = fromThePanic(fs)

	p := &debugPanic{frames: fs}
	var owners [][]frames.Owner // of each group's frames, for a folded one's summary
	for _, f := range fs {
		owner := frames.Of(f, frames.Module)
		app := owner == frames.App
		if n := len(p.Groups); n == 0 || p.Groups[n-1].App != app {
			p.Groups = append(p.Groups, debugGroup{App: app})
			owners = append(owners, nil)
		}
		frame := debugFrame{Function: f.Function, File: f.File, Line: f.Line, Link: editorLink(editor, f.File, f.Line)}
		if app {
			frame.Lines = src.around(f.File, f.Line, 5)
		}
		g, o := &p.Groups[len(p.Groups)-1], &owners[len(owners)-1]
		g.Frames = append(g.Frames, frame)
		if !slices.Contains(*o, owner) {
			*o = append(*o, owner)
		}
	}
	for i := range p.Groups {
		if g := &p.Groups[i]; !g.App {
			g.Summary = folded(len(g.Frames), owners[i])
		}
	}
	return p
}

// fromThePanic is fs, a recovered panic's stack, from the frame that
// panicked: past runtime.gopanic's, and the runtime's frames on the way to
// it, as an index out of range or a nil pointer has.
func fromThePanic(fs []runtime.Frame) []runtime.Frame {
	for i, f := range fs {
		if f.Function == "runtime.gopanic" {
			fs = fs[i+1:]
			break
		}
	}
	for len(fs) > 1 && strings.HasPrefix(fs[0].Function, "runtime.") {
		fs = fs[1:]
	}
	return fs
}

// folded is what a folded run of n frames is, by whose they are: "3
// frames of tug and the standard library".
func folded(n int, owners []frames.Owner) string {
	names := make([]string, len(owners))
	for i, o := range owners {
		names[i] = o.String()
	}
	whose := strings.Join(names, ", ")
	if len(names) > 1 {
		whose = strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
	}
	if n == 1 {
		return "1 frame of " + whose
	}
	return strconv.Itoa(n) + " frames of " + whose
}

// sources are the lines of the files a page shows, each read once, or
// nil for a file that can't be read, as a build with -trimpath names
// files that aren't anywhere.
type sources map[string][]string

// around is the lines of file around line, n on each side, with line
// marked, or nil when the file can't be read.
func (s sources) around(file string, line, n int) []debugLine {
	lines, ok := s[file]
	if !ok {
		if data, err := os.ReadFile(file); err == nil {
			lines = strings.Split(string(data), "\n")
		}
		s[file] = lines
	}
	if line < 1 || line > len(lines) {
		return nil
	}
	from, to := max(line-n, 1), min(line+n, len(lines))
	out := make([]debugLine, 0, to-from+1)
	for i := from; i <= to; i++ {
		out = append(out, debugLine{N: i, Text: strings.TrimSuffix(lines[i-1], "\r"), At: i == line})
	}
	return out
}

// requestOf is what the page says of r, which rt answered: its query's and
// its headers' secrets as DevTools keeps them, [REDACTED].
func requestOf(r *http.Request, rt *Route, id string) debugRequest {
	u := devtools.RedactQuery(r.URL)
	shown := u.Path
	if u.RawQuery != "" {
		q, err := url.QueryUnescape(u.RawQuery)
		if err != nil {
			q = u.RawQuery
		}
		shown += "?" + q
	}
	req := debugRequest{Method: r.Method, URL: shown, ID: id}
	if rt != nil {
		for _, name := range wildcards(rt.path) {
			req.Values = append(req.Values, debugPair{"{" + name + "}", r.PathValue(name)})
		}
	}
	headers := devtools.RedactHeaders(r.Header)
	headers["host"] = r.Host
	for name, value := range headers {
		req.Headers = append(req.Headers, debugPair{name, value})
	}
	slices.SortFunc(req.Headers, func(a, b debugPair) int { return strings.Compare(a.Name, b.Name) })
	return req
}

// wildcards are the names of the wildcards of path, a route's, in order:
// id of {id}, and path of {path...}.
func wildcards(path string) []string {
	var names []string
	for seg := range strings.SplitSeq(path, "/") {
		if name, ok := strings.CutPrefix(seg, "{"); ok && seg != "{$}" {
			names = append(names, strings.TrimSuffix(strings.TrimSuffix(name, "}"), "..."))
		}
	}
	return names
}

// editors are the editors Config.Editor names, each a link to a file at a
// line, as the editor's own handler of links takes it.
var editors = map[string]func(file string, line int) string{
	"vscode": func(file string, line int) string { return "vscode://file" + rooted(file) + ":" + strconv.Itoa(line) },
	"cursor": func(file string, line int) string { return "cursor://file" + rooted(file) + ":" + strconv.Itoa(line) },
	"zed":    func(file string, line int) string { return "zed://file" + rooted(file) + ":" + strconv.Itoa(line) },
	"goland": func(file string, line int) string {
		return "goland://open?file=" + url.QueryEscape(filepath.ToSlash(file)) + "&line=" + strconv.Itoa(line)
	},
	"sublime": func(file string, line int) string {
		return "subl://open?url=" + url.QueryEscape("file://"+rooted(file)) + "&line=" + strconv.Itoa(line)
	},
}

// editorLink is a link that opens file at line in the editor editor names,
// or by editor itself, a link of the app's with {file} and {line} in it;
// or "" for none. It's a template.URL, as html/template lets through only
// the schemes of the web.
func editorLink(editor, file string, line int) template.URL {
	if editor == "" || file == "" {
		return ""
	}
	if link, ok := editors[editor]; ok {
		return template.URL(link(file, line))
	}
	if strings.Contains(editor, "{file}") {
		return template.URL(strings.NewReplacer("{file}", escapedPath(file), "{line}", strconv.Itoa(line)).Replace(editor))
	}
	return ""
}

// escapedPath is file with each of its parts escaped for a link, and its
// slashes kept.
func escapedPath(file string) string {
	parts := strings.Split(filepath.ToSlash(file), "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

// rooted is file as a link's path, starting with a slash, as a Windows
// path doesn't.
func rooted(file string) string {
	return "/" + strings.TrimPrefix(escapedPath(file), "/")
}

// tugVersion is the version of tug the app was built with, as its build
// says: "(devel)" for a checkout of it.
var tugVersion = func() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	if info.Main.Path == "github.com/cuonggt/tug" {
		return info.Main.Version
	}
	for _, d := range info.Deps {
		if d.Path != "github.com/cuonggt/tug" {
			continue
		}
		if d.Replace != nil {
			if d.Replace.Version != "" {
				return d.Replace.Version
			}
			return "(devel)"
		}
		return d.Version
	}
	return ""
}()
