package devtools

import (
	"net/http"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
)

// serveFunc is Serve's name on the stack: a request's frames past it are
// the server's, or whatever called the App, never the app's answer.
var serveFunc = runtime.FuncForPC(reflect.ValueOf((*Recorder).Serve).Pointer()).Name()

// mainModule is the path of the app's module, whose packages are the
// app's however it's spelled, as tug new's "blog", with no dot, is.
var mainModule = func() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		return info.Main.Path
	}
	return ""
}()

// tugDir and stdDir are the directories of tug's Go and the standard
// library's, as the runtime names files. A function's name is of the
// function the compiler made it in, so a closure of tug's, made by a
// function inlined into one of the app's, as the starters' newApp inlines
// middleware.Headers, has the app's name, main.newApp.Headers.func7.1:
// its file is still tug's. Either is "" when it can't be told, as a build
// with -trimpath names the standard library's files with no directory.
var tugDir, stdDir = sourceDirs()

func sourceDirs() (tug, std string) {
	if _, file, _, ok := runtime.Caller(0); ok {
		tug, _ = strings.CutSuffix(file, "internal/devtools/caller.go")
	}
	if fn := runtime.FuncForPC(reflect.ValueOf(http.NotFound).Pointer()); fn != nil {
		file, _ := fn.FileLine(fn.Entry())
		std, _ = strings.CutSuffix(file, "net/http/server.go")
	}
	return tug, std
}

// Caller is the frame of the app's Go that its caller was called from: the
// first on the stack that's the app's, and in a request, before Serve's;
// and whether there's one. It's where the app added a route, rendered a
// page, or read a request.
func Caller() (runtime.Frame, bool) {
	pc := make([]uintptr, 64)
	frames := runtime.CallersFrames(pc[:runtime.Callers(2, pc)])
	for {
		f, more := frames.Next()
		switch {
		case f.Function == serveFunc:
			return runtime.Frame{}, false
		case apps(f):
			return f, true
		case !more:
			return runtime.Frame{}, false
		}
	}
}

// Here is where the function that calls it starts, the entry of the
// function the compiler made, which inlining doesn't change.
func Here() uintptr {
	pc, _, _, ok := runtime.Caller(1)
	if !ok {
		return 0
	}
	return runtime.FuncForPC(pc).Entry()
}

// answerer is the function of the app's that wrote the response it's
// called under, inside the route's handler, whose frame is the one of the
// function that starts at handler: the first of the app's frames before
// it, and whether there's one. A frame past the handler's is only on the
// way, as an app's middleware is.
func answerer(handler uintptr) (runtime.Frame, bool) {
	if handler == 0 {
		return runtime.Frame{}, false
	}
	pc := make([]uintptr, 64)
	frames := runtime.CallersFrames(pc[:runtime.Callers(2, pc)])
	var app runtime.Frame
	found := false
	for {
		f, more := frames.Next()
		switch {
		case f.Entry == handler:
			return app, found
		case f.Function == serveFunc || !more:
			return runtime.Frame{}, false
		case !found && apps(f):
			app, found = f, true
		}
	}
}

// apps reports whether f is the app's: neither tug's, by its package or
// its file, though tug's tests and examples are apps, nor the standard
// library's, by its file where that can be told, and by its package: one
// whose path's first element has no dot is the standard library's, unless
// it's the app's own module's.
func apps(f runtime.Frame) bool {
	pkg := packageOf(f.Function)
	if tugs(pkg) || within(f.File, tugDir) {
		return strings.HasSuffix(f.File, "_test.go") || within(f.File, tugDir+"examples/")
	}
	if within(f.File, stdDir) {
		return false
	}
	if pkg == "main" || mainModule != "" && (pkg == mainModule || strings.HasPrefix(pkg, mainModule+"/")) {
		return true
	}
	first, _, _ := strings.Cut(pkg, "/")
	return strings.Contains(first, ".")
}

func tugs(pkg string) bool {
	return pkg == "github.com/cuonggt/tug" || strings.HasPrefix(pkg, "github.com/cuonggt/tug/")
}

// within reports whether file is in dir, which "" is none.
func within(file, dir string) bool {
	return dir != "" && strings.HasPrefix(file, dir)
}

// packageOf is the import path of the package of the function the runtime
// names name: "net/http" of "net/http.(*conn).serve".
func packageOf(name string) string {
	slash := strings.LastIndexByte(name, '/') + 1
	if dot := strings.IndexByte(name[slash:], '.'); dot >= 0 {
		return name[:slash+dot]
	}
	return name
}

// At is where f is: the line of the call it was making.
func At(f runtime.Frame) Source {
	return Source{File: f.File, Line: f.Line}
}

// defined is where the function of f is defined, or, for one the compiler
// inlined, whose start the runtime doesn't keep, where f is.
func defined(f runtime.Frame) *Source {
	if f.Func == nil {
		at := At(f)
		return &at
	}
	file, line := f.Func.FileLine(f.Entry)
	return &Source{File: file, Line: line}
}
