package main

import (
	"bytes"
	"encoding/base64"
	"net"
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
	env := read(".env")
	key, _, _ := strings.Cut(strings.SplitAfter(env, "APP_KEY=base64:")[1], "\n")
	if k, err := base64.StdEncoding.DecodeString(key); err != nil || len(k) != 32 {
		t.Errorf("the .env's key %q isn't 32 bytes of base64", key)
	}
}

// TestANewAppBuildsAndPassesItsOwnTests makes an app as a person would,
// with its Go modules and npm packages, and runs what it comes with.
func TestANewAppBuildsAndPassesItsOwnTests(t *testing.T) {
	if testing.Short() {
		t.Skip("installs the new app's packages")
	}
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("needs npm")
	}
	dir := filepath.Join(t.TempDir(), "blog")
	checkout, _ := filepath.Abs("../..")
	if err := runNew([]string{dir, "-tug-dir", checkout}); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"resources/js/tug/pages.ts", "resources/js/tug/routes.ts", "go.sum", "node_modules"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("tug new didn't make %s", f)
		}
	}
	for _, c := range [][]string{{"go", "vet", "./..."}, {"go", "test", "./..."}, {"npm", "run", "typecheck"}} {
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s: %v\n%s", strings.Join(c, " "), err, out)
		}
	}
}
