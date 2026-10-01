package frames

import (
	"runtime"
	"testing"
)

func TestAFrameIsTheAppsTugsTheStandardLibrarysOrADependencys(t *testing.T) {
	if StdDir == "" || TugDir == "" {
		t.Fatalf("the standard library is in %q, and tug in %q", StdDir, TugDir)
	}
	for _, c := range []struct {
		function, file string
		owner          Owner
	}{
		{"main.(*app).dashboard", "/src/blog/main.go", App},
		{"blog/internal/posts.List", "/src/blog/internal/posts/posts.go", App},
		{"github.com/jackc/pgx/v5.(*Conn).Query", "/mod/github.com/jackc/pgx/v5/conn.go", Dep},
		{"net/http.Redirect", "/go/src/net/http/server.go", Std},
		{"net/http.Redirect", StdDir + "net/http/server.go", Std},
		{"github.com/cuonggt/tug.(*Ctx).Redirect", "/mod/github.com/cuonggt/tug/ctx.go", Tug},
		{"github.com/cuonggt/tug/middleware.Logger.func1", "/mod/github.com/cuonggt/tug/middleware/logger.go", Tug},
		{"github.com/cuonggt/tug.TestRoutes.func1", "/src/tug/router_test.go", App},
		// A closure made by a function inlined into the app's is named as
		// the app's, in the file of the package that wrote it.
		{"main.newApp.Headers.func7.1", TugDir + "middleware/headers.go", Tug},
		{"main.newApp.StripPrefix.func1", StdDir + "net/http/server.go", Std},
		{"main.main", TugDir + "examples/inertia/main.go", App},
	} {
		if got := Of(runtime.Frame{Function: c.function, File: c.file}, "blog"); got != c.owner {
			t.Errorf("%s in %s is %v's, want %v's", c.function, c.file, got, c.owner)
		}
	}
}
