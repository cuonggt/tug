package devtools

import (
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

// Caller is the frame of the app's Go that its caller was called from: the
// first on the stack that's neither tug's own, its tests aside, nor the
// standard library's, and in a request, before Serve's; and whether
// there's one. It's where the app added a route, rendered a page, or
// answered a request.
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

// apps reports whether f is the app's. A package that isn't tug's and
// whose path's first element has no dot is the standard library's, unless
// it's the app's own module's.
func apps(f runtime.Frame) bool {
	pkg := packageOf(f.Function)
	switch {
	case pkg == "github.com/cuonggt/tug" || strings.HasPrefix(pkg, "github.com/cuonggt/tug/"):
		return strings.HasSuffix(f.File, "_test.go") // tug's tests are its apps
	case pkg == "main" || mainModule != "" && (pkg == mainModule || strings.HasPrefix(pkg, mainModule+"/")):
		return true
	}
	first, _, _ := strings.Cut(pkg, "/")
	return strings.Contains(first, ".")
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
