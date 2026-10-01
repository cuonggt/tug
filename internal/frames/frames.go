// Package frames tells whose a frame of a goroutine's stack is: the app's
// own, tug's, the standard library's, or a dependency's. Inertia's DevTools
// look for the app's function that answered a request by it, and the page
// a server error is shown with while debugging folds the frames that
// aren't the app's.
package frames

import (
	"net/http"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
)

// Owner is whose a frame is.
type Owner int

const (
	App Owner = iota // the app's own, its package main's and its module's, and tug's tests' and examples'
	Tug
	Std // the standard library's
	Dep // another module's, one the app depends on
)

// String names whose a frame is, as a sentence says it.
func (o Owner) String() string {
	switch o {
	case App:
		return "the app"
	case Tug:
		return "tug"
	case Std:
		return "the standard library"
	}
	return "a dependency"
}

// Module is the path of the app's module, whose packages are the app's
// however it's spelled, as tug new's "blog", with no dot, is.
var Module = func() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		return info.Main.Path
	}
	return ""
}()

// TugDir and StdDir are the directories of tug's Go and the standard
// library's, as the runtime names files. A function's name is of the
// function the compiler made it in, so a closure of tug's, made by a
// function inlined into one of the app's, as the starters' newApp inlines
// middleware.Headers, has the app's name, main.newApp.Headers.func7.1:
// its file is still tug's. Either is "" when it can't be told, as a build
// with -trimpath names the standard library's files with no directory.
var TugDir, StdDir = sourceDirs()

func sourceDirs() (tug, std string) {
	if _, file, _, ok := runtime.Caller(0); ok {
		tug, _ = strings.CutSuffix(file, "internal/frames/frames.go")
	}
	if fn := runtime.FuncForPC(reflect.ValueOf(http.NotFound).Pointer()); fn != nil {
		file, _ := fn.FileLine(fn.Entry())
		std, _ = strings.CutSuffix(file, "net/http/server.go")
	}
	return tug, std
}

// Of says whose f is, with module the path of the app's module: tug's, by
// its package or its file, though tug's tests and examples are apps; the
// standard library's, by its file where that can be told, and by its
// package, whose path's first element has no dot, unless it's the app's
// own module's; the app's, by its package, main or its module's; and
// otherwise a dependency's.
func Of(f runtime.Frame, module string) Owner {
	pkg := packageOf(f.Function)
	if tugs(pkg) || within(f.File, TugDir) {
		if strings.HasSuffix(f.File, "_test.go") || within(f.File, TugDir+"examples/") {
			return App
		}
		return Tug
	}
	if within(f.File, StdDir) {
		return Std
	}
	if pkg == "main" || module != "" && (pkg == module || strings.HasPrefix(pkg, module+"/")) {
		return App
	}
	if first, _, _ := strings.Cut(pkg, "/"); strings.Contains(first, ".") {
		return Dep
	}
	return Std
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
