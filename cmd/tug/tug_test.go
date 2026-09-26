package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"flag"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestADotEnvIsReadAsLaravelWritesOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(path, []byte("# comment\n\nAPP_KEY=base64:abc=\nexport NAME=\"my app\"\nQUOTED='single'\n  SPACED = x  \n"), 0o600)
	vars, err := readDotEnv(path)
	want := []string{"APP_KEY=base64:abc=", "NAME=my app", "QUOTED=single", "SPACED=x"}
	if err != nil || !slices.Equal(vars, want) {
		t.Fatalf("got %q, %v; want %q", vars, err, want)
	}

	os.WriteFile(path, []byte("NOT A LINE\n"), 0o600)
	if _, err := readDotEnv(path); err == nil || !strings.Contains(err.Error(), ":1:") {
		t.Errorf("a bad line: %v", err)
	}
	if vars, err := readDotEnv(filepath.Join(t.TempDir(), "none")); err != nil || vars != nil {
		t.Errorf("no file: %v, %v", vars, err)
	}
}

func TestAReleaseIsATagNotABuildFromACheckout(t *testing.T) {
	for v, want := range map[string]bool{
		"v0.1.0":                               true,
		"v1.2.3-rc.1":                          true,
		"(devel)":                              false,
		"":                                     false,
		"v0.0.0-20260925051818-b571d11c4df5":   false,
		"v0.1.1-0.20260925051818-b571d11c4df5": false,
		"v0.1.0+dirty":                         false,
		"v0.0.0-20260925051818-b571d11c4df5+dirty": false,
	} {
		if got := release(v); got != want {
			t.Errorf("release(%q) = %v", v, got)
		}
	}
}

func TestTheWatcherSeesGoAndTemplatesButNotTheFrontend(t *testing.T) {
	root := t.TempDir()
	write := func(name string) {
		path := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, []byte(name+time.Now().String()), 0o644)
	}
	for _, f := range []string{"main.go", "go.mod", "app.html", "internal/x/x.go", "node_modules/p/p.go", ".tug/app.go", "public/build/a.html", "resources/js/app.tsx"} {
		write(f)
	}
	before := snapshot(root)
	var names []string
	for path := range before {
		rel, _ := filepath.Rel(root, path)
		names = append(names, filepath.ToSlash(rel))
	}
	slices.Sort(names)
	if want := []string{"app.html", "go.mod", "internal/x/x.go", "main.go"}; !slices.Equal(names, want) {
		t.Fatalf("watched %v, want %v", names, want)
	}

	time.Sleep(10 * time.Millisecond)
	write("main.go")
	os.Remove(filepath.Join(root, "app.html"))
	changed := diff(before, snapshot(root))
	if len(changed) != 2 {
		t.Errorf("changed %v, want main.go and app.html", changed)
	}
}

func TestDevTakesTheNextPortWhenItsOwnIsTaken(t *testing.T) {
	// Taken by the test, unless something else has it already, which
	// does as well.
	if ln, err := net.Listen("tcp", "127.0.0.1:8080"); err == nil {
		defer ln.Close()
	}

	var said string
	say := func(format string, a ...any) { said = format }
	addr, err := devAddr(nil, say)
	if err != nil || addr == "127.0.0.1:8080" || !strings.Contains(said, "taken") {
		t.Errorf("got %q, %v, and said %q", addr, err, said)
	}
	if _, err := devAddr([]string{"ADDR=127.0.0.1:8080"}, say); err == nil {
		t.Error("an ADDR someone else listens on was taken")
	}
}

func TestPrefixedWritesWholeLinesUnderALabel(t *testing.T) {
	var b bytes.Buffer
	p := &prefixed{mu: &sync.Mutex{}, w: &b, label: "app │"}
	p.Write([]byte("one\ntw"))
	p.Write([]byte("o\n"))
	if b.String() != "app │ one\napp │ two\n" {
		t.Fatalf("got %q", b.String())
	}
}

func TestNewFillsInTheStarter(t *testing.T) {
	root := filepath.Join(t.TempDir(), "blog")
	data := starterData{Name: "blog", Module: "example.com/blog", TugVersion: "v0.0.0", TugDir: "/src/tug"}
	if err := writeStarter(root, data); err != nil {
		t.Fatal(err)
	}
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	if mod := read("go.mod"); !strings.HasPrefix(mod, "module example.com/blog\n") || !strings.Contains(mod, "replace github.com/cuonggt/tug => /src/tug") {
		t.Errorf("go.mod:\n%s", mod)
	}
	if html := read("app.html"); !strings.Contains(html, "<title data-inertia>blog</title>") || !strings.Contains(html, "{{ .Inertia }}") {
		t.Errorf("app.html should have the name, and keep its own template: %s", html)
	}
	for _, f := range []string{"main.go", "main_test.go", "package.json", "resources/js/app.tsx", ".gitignore", "public/.gitkeep"} {
		if strings.Contains(read(f), "[[") {
			t.Errorf("%s has a placeholder left", f)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "main.go.tmpl")); err == nil {
		t.Error("a .tmpl file was copied as it was")
	}
	if _, err := os.Stat(filepath.Join(root, "auth.go")); err == nil {
		t.Error("an app without -auth has auth.go")
	}
	for _, f := range onlySSR {
		if _, err := os.Stat(filepath.Join(root, f)); err == nil {
			t.Errorf("an app without -ssr has %s", f)
		}
	}
	if strings.Contains(read("main.go"), "ssr") || strings.Contains(read("package.json"), "--ssr") {
		t.Error("an app without -ssr renders on the server")
	}
	env := read(".env")
	key, _, _ := strings.Cut(strings.SplitAfter(env, "APP_KEY=base64:")[1], "\n")
	if k, err := base64.StdEncoding.DecodeString(key); err != nil || len(k) != 32 {
		t.Errorf("the .env's key %q isn't 32 bytes of base64", key)
	}
}

func TestNewWithAuthLaysTheAuthStarterOverThePlainOne(t *testing.T) {
	root := filepath.Join(t.TempDir(), "blog")
	if err := writeStarter(root, starterData{Name: "blog", Module: "blog", TugVersion: "v0.1.0", Auth: true}); err != nil {
		t.Fatal(err)
	}
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	if main := read("main.go"); !strings.Contains(main, "usersOnly") || !strings.Contains(main, `const appName = "blog"`) {
		t.Errorf("main.go isn't the auth starter's:\n%s", main)
	}
	for _, f := range []string{"auth.go", "users.go", "jobs.go", "resources/js/pages/Auth/Login.tsx", "resources/js/pages/Dashboard.tsx", "resources/js/app.tsx"} {
		if strings.Contains(read(f), "[[ ") {
			t.Errorf("%s has a placeholder left", f)
		}
	}
	if !strings.Contains(read("settings.go"), "inertia.OptionalProp[[]string]") {
		t.Error("settings.go's two brackets, which a placeholder writes, didn't come out as Go's")
	}
	if read("go.mod") == "" || read("public/.gitkeep") != "" {
		t.Error("the plain starter's files didn't come along")
	}
	if _, err := os.Stat(filepath.Join(root, "resources/js/Layout.tsx")); err == nil {
		t.Error("the plain starter's Layout.tsx came along, which the auth starter's layouts replace")
	}
	if !strings.Contains(read(".gitignore"), "/app.db") {
		t.Error("the database isn't ignored")
	}
}

func TestNewTakesItsFlagsBeforeAndAfterTheDirectory(t *testing.T) {
	checkout, _ := filepath.Abs("../..")
	dir := filepath.Join(t.TempDir(), "mixed")
	if err := runNew([]string{"-no-install", dir, "-module", "example.com/mixed", "-tug-dir", checkout}); err != nil {
		t.Fatal(err)
	}
	if mod, _ := os.ReadFile(filepath.Join(dir, "go.mod")); !strings.HasPrefix(string(mod), "module example.com/mixed\n") {
		t.Errorf("the flag after the directory was dropped: go.mod is\n%s", mod)
	}
	two := []string{filepath.Join(t.TempDir(), "a"), filepath.Join(t.TempDir(), "b")}
	if err := runNew(two); !errors.Is(err, flag.ErrHelp) {
		t.Errorf("two directories: %v", err)
	}
}

func TestNewWithSSRAddsTheAppOnTheServer(t *testing.T) {
	for _, auth := range []bool{false, true} {
		root := filepath.Join(t.TempDir(), "blog")
		if err := writeStarter(root, starterData{Name: "blog", Module: "blog", TugVersion: "v0.1.0", Auth: auth, SSR: true}); err != nil {
			t.Fatal(err)
		}
		read := func(name string) string {
			b, err := os.ReadFile(filepath.Join(root, name))
			if err != nil {
				t.Fatal(err)
			}
			return string(b)
		}
		for _, f := range onlySSR {
			read(f)
		}
		for f, want := range map[string]string{
			"main.go":        "ssr.Gateway{DevServer: assets.DevServer",
			"package.json":   `"build": "vite build && vite build --ssr"`,
			"vite.config.ts": "outDir: 'ssr/build'",
			"app.html":       "{{ .InertiaHead }}",
			"Dockerfile":     "FROM gcr.io/distroless/nodejs24-debian12",
			".gitignore":     "/ssr/build/",
			".dockerignore":  "ssr/build",
			".env.example":   "SSR_URL=",
		} {
			if got := read(f); !strings.Contains(got, want) || strings.Contains(got, "[[") {
				t.Errorf("auth %v: %s has no %q, or a placeholder left:\n%s", auth, f, want, got)
			}
		}
	}
}

// TestANewAppBuildsAndPassesItsOwnTests makes an app of each kind as a
// person would, with its Go modules and npm packages, and runs what it
// comes with, the frontend's build included: type-checking doesn't run the
// Vite and Tailwind plugins a build goes through.
func TestANewAppBuildsAndPassesItsOwnTests(t *testing.T) {
	if testing.Short() {
		t.Skip("installs the new app's packages")
	}
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("needs npm")
	}
	checkout, _ := filepath.Abs("../..")
	for _, kind := range []struct {
		name  string
		flags []string
	}{
		{"plain", nil},
		{"auth", []string{"-auth"}},
		{"plain with SSR", []string{"-ssr"}},
		{"auth with SSR", []string{"-auth", "-ssr"}},
	} {
		t.Run(kind.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "blog")
			if err := runNew(append([]string{dir, "-tug-dir", checkout}, kind.flags...)); err != nil {
				t.Fatal(err)
			}
			for _, f := range []string{"resources/js/tug/pages.ts", "resources/js/tug/routes.ts", "go.sum", "node_modules"} {
				if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
					t.Errorf("tug new didn't make %s", f)
				}
			}
			for _, c := range [][]string{{"go", "vet", "./..."}, {"go", "test", "./..."}, {"npm", "run", "typecheck"}, {"npm", "run", "build"}} {
				cmd := exec.Command(c[0], c[1:]...)
				cmd.Dir = dir
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Errorf("%s: %v\n%s", strings.Join(c, " "), err, out)
				}
			}
			// Where the Go server looks for the build.
			if _, err := os.Stat(filepath.Join(dir, "public/build/.vite/manifest.json")); err != nil {
				t.Errorf("the build has no manifest where the server reads it: %v", err)
			}
			if slices.Contains(kind.flags, "-ssr") {
				rendersOnTheServer(t, dir)
			}
		})
	}
}

// rendersOnTheServer builds the app in dir, runs it as it runs deployed,
// and checks a first visit comes back rendered on the server, by Node.
func rendersOnTheServer(t *testing.T, dir string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, "ssr/build/ssr.mjs")); err != nil {
		t.Fatalf("the build has no SSR bundle where the server embeds it: %v", err)
	}
	build := exec.Command("go", "build", "-o", "app", ".")
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	vars, err := readDotEnv(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	app := exec.Command(filepath.Join(dir, "app"))
	app.Dir = dir
	app.Env = append(append(os.Environ(), vars...), "ADDR="+addr)
	app.Stdout, app.Stderr = &out, &out
	if err := app.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		app.Process.Signal(os.Interrupt)
		app.Wait()
	}()

	var page string
	for deadline := time.Now().Add(30 * time.Second); !strings.Contains(page, `data-server-rendered="true"`); time.Sleep(100 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("no first visit came back rendered on the server; the last was\n%s\nand the app said\n%s", page, out.String())
		}
		if resp, err := http.Get("http://" + addr + "/"); err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			page = string(b)
		}
	}
}
