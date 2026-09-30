package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"go/format"
	"io"
	"io/fs"
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
	for _, f := range []string{"main.go", "go.mod", "app.html", "internal/x/x.go", "node_modules/p/p.go", ".tug/app.go", "public/build/a.html", "resources/js/app.tsx", "lang/vi.json", "package.json", "resources/js/lang/vi.json", "migrations/20261001093000_create_posts.sql", "db/schema.sql"} {
		write(f)
	}
	before := snapshot(root)
	var names []string
	for path := range before {
		rel, _ := filepath.Rel(root, path)
		names = append(names, filepath.ToSlash(rel))
	}
	slices.Sort(names)
	if want := []string{"app.html", "go.mod", "internal/x/x.go", "lang/vi.json", "main.go", "migrations/20261001093000_create_posts.sql"}; !slices.Equal(names, want) {
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

func TestDevGivesTheAppTheAddressItShowsUnlessItHasOne(t *testing.T) {
	for _, tc := range []struct {
		env  []string
		want string
	}{
		{nil, "http://localhost:8081"},
		{[]string{"APP_URL=https://tunnel.example"}, "https://tunnel.example"},
		{[]string{"APP_URL="}, "http://localhost:8081"}, // as .env.example has it
		{[]string{"APP_URL=https://tunnel.example", "APP_URL="}, "http://localhost:8081"},
	} {
		env := devEnv(tc.env, "127.0.0.1:8081")
		if got := envValue(env, "APP_URL"); got != tc.want {
			t.Errorf("with %q: APP_URL %q, want %q", tc.env, got, tc.want)
		}
		if envValue(env, "ADDR") != "127.0.0.1:8081" || envValue(env, "TUG_DEV") != "1" {
			t.Errorf("with %q: %q", tc.env, env)
		}
	}
}

func TestDevShowsTheAppAtLocalhost(t *testing.T) {
	for addr, want := range map[string]string{
		"127.0.0.1:8080": "localhost:8080",
		"0.0.0.0:3000":   "0.0.0.0:3000",
		"[::1]:8080":     "[::1]:8080",
	} {
		if got := shown(addr); got != want {
			t.Errorf("%s is shown as %s, want %s", addr, got, want)
		}
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

// frontends are the frontends tug new makes an app with, and ownFiles the
// extension of the files that only an app with each has: its pages and
// components.
var (
	frontends = []string{"react", "vue", "svelte"}
	ownFiles  = map[string]string{"react": ".tsx", "vue": ".vue", "svelte": ".svelte"}
)

// othersFiles are the files in root that are another frontend's than
// frontend: a .vue file in React's app, say.
func othersFiles(t *testing.T, root, frontend string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		for f, ext := range ownFiles {
			if err == nil && f != frontend && strings.HasSuffix(path, ext) {
				rel, _ := filepath.Rel(root, path)
				found = append(found, rel)
			}
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// reader returns what reads the files of the app in root, failing the test
// on one that isn't there.
func reader(t *testing.T, root string) func(name string) string {
	return func(name string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
}

// formatted fails the test for each Go file of the app in root that isn't
// as gofmt writes it, naming the file and the first line gofmt changes. A
// slip in a template, as a struct's comments aligned by hand, is in every
// app made from it, whose gofmt -l, or editor, shows it to its owner as a
// change of their own. The tests that make each kind with writeStarter
// check it, with no npm: an app's Go is all the starter's, and installing
// adds its modules, packages and types.
func formatted(t *testing.T, root string) {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && filepath.Ext(path) == ".go" {
			files = append(files, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(root, path)
		gofmt, err := format.Source(src)
		switch {
		case err != nil:
			t.Errorf("%s isn't Go that gofmt reads: %v", rel, err)
		case !bytes.Equal(src, gofmt):
			line, was, is := firstDifference(src, gofmt)
			t.Errorf("%s isn't as gofmt writes it, from line %d: %q, which gofmt writes %q", rel, line, was, is)
		}
	}
}

// firstDifference is the number of the first line where a and b differ,
// and that line of each.
func firstDifference(a, b []byte) (int, string, string) {
	as, bs := strings.Split(string(a), "\n"), strings.Split(string(b), "\n")
	for i := range max(len(as), len(bs)) {
		var x, y string
		if i < len(as) {
			x = as[i]
		}
		if i < len(bs) {
			y = bs[i]
		}
		if x != y {
			return i + 1, x, y
		}
	}
	return 0, "", ""
}

func TestNewFillsInTheStarter(t *testing.T) {
	for _, frontend := range frontends {
		t.Run(frontend, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "blog")
			data := starterData{Name: "blog", Module: "example.com/blog", TugVersion: "v0.0.0", TugDir: "/src/tug", Frontend: frontend}
			if err := writeStarter(root, data); err != nil {
				t.Fatal(err)
			}
			formatted(t, root)
			read := reader(t, root)
			if mod := read("go.mod"); !strings.HasPrefix(mod, "module example.com/blog\n") || !strings.Contains(mod, "replace github.com/cuonggt/tug => /src/tug") {
				t.Errorf("go.mod:\n%s", mod)
			}
			html := read("app.html")
			if !strings.Contains(html, "<title data-inertia>blog</title>") || !strings.Contains(html, "{{ .Inertia }}") {
				t.Errorf("app.html should have the name, and keep its own template: %s", html)
			}
			if want := `{{ vite .Nonce "resources/js/app.` + data.Script() + `" (printf "resources/js/pages/%s.` + data.Component() + `" .Page.Component) }}`; !strings.Contains(html, want) {
				t.Errorf("app.html doesn't load the app and its page from %s's files: %s", data.Framework(), html)
			}
			if strings.Contains(html, "viteReactRefresh") != data.React() {
				t.Errorf("app.html has React's refresh preamble in a %s app, or not in React's: %s", data.Framework(), html)
			}
			for _, f := range []string{"main.go", "main_test.go", "package.json", "vite.config.ts", "README.md", "resources/js/app." + data.Script(), "resources/js/pages/Home." + data.Component(), ".gitignore", "public/.gitkeep"} {
				if strings.Contains(read(f), "[[") {
					t.Errorf("%s has a placeholder left", f)
				}
			}
			if !strings.Contains(read("package.json"), `"`+data.Adapter()+`"`) {
				t.Errorf("package.json doesn't have %s, Inertia for %s", data.Adapter(), data.Framework())
			}
			if others := othersFiles(t, root, frontend); len(others) > 0 {
				t.Errorf("a %s app has another frontend's files: %v", data.Framework(), others)
			}
			if _, err := os.Stat(filepath.Join(root, "main.go.tmpl")); err == nil {
				t.Error("a .tmpl file was copied as it was")
			}
			if _, err := os.Stat(filepath.Join(root, "auth.go")); err == nil {
				t.Error("an app without -auth has auth.go")
			}
			for _, f := range onlySSR(data) {
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
		})
	}
}

func TestNewWithAuthLaysTheAuthStarterOverThePlainOne(t *testing.T) {
	for _, frontend := range frontends {
		t.Run(frontend, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "blog")
			data := starterData{Name: "blog", Module: "blog", TugVersion: "v0.1.0", Frontend: frontend, Auth: true, Database: "sqlite"}
			if err := writeStarter(root, data); err != nil {
				t.Fatal(err)
			}
			formatted(t, root)
			read := reader(t, root)
			if main := read("main.go"); !strings.Contains(main, "usersOnly") || !strings.Contains(main, `const appName = "blog"`) {
				t.Errorf("main.go isn't the auth starter's:\n%s", main)
			}
			for _, f := range []string{"auth.go", "users.go", "jobs.go", "db.go", "users_db.go", "throttles_db.go", "cache_db.go", "tokens_db.go", "broadcasts.go", "broadcasts_db.go", "abilities.go", "admin.go", "notifications.go", "notifications_db.go", "migrations.go", "migrations_db.go", "resources/js/pages/Auth/Login." + data.Component(), "resources/js/pages/Dashboard." + data.Component(), "resources/js/app." + data.Script()} {
				if strings.Contains(read(f), "[[ ") {
					t.Errorf("%s has a placeholder left", f)
				}
			}
			if !strings.Contains(read("settings.go"), "inertia.OptionalProp[[]string]") {
				t.Error("settings.go's two brackets, which a placeholder writes, didn't come out as Go's")
			}
			// Passkeys in the browser are one file for every frontend, with
			// Inertia's router from the frontend's own package.
			if !strings.Contains(read("resources/js/lib/passkeys.ts"), "import { router } from '"+data.Adapter()+"'") {
				t.Errorf("lib/passkeys.ts doesn't take the router from %s", data.Adapter())
			}
			if read("go.mod") == "" || read("public/.gitkeep") != "" {
				t.Error("the plain starter's files didn't come along")
			}
			for _, f := range notInAuth(data) {
				if _, err := os.Stat(filepath.Join(root, f)); err == nil {
					t.Errorf("the plain starter's %s came along, which the auth starter's layouts replace", f)
				}
			}
			if others := othersFiles(t, root, frontend); len(others) > 0 {
				t.Errorf("a %s app has another frontend's files: %v", data.Framework(), others)
			}
			if !strings.Contains(read(".gitignore"), "/app.db") {
				t.Error("the database isn't ignored")
			}
			if !strings.Contains(read("db.go"), `"modernc.org/sqlite"`) || strings.Contains(read(".env"), "DB_URL") {
				t.Error("the app's database isn't SQLite")
			}
			if _, err := os.Stat(filepath.Join(root, "compose.yaml")); err == nil {
				t.Error("an app on SQLite has a compose.yaml, for a database it hasn't")
			}
		})
	}
}

func TestNewWithPostgresOrMySQLLaysItsSQLOverTheAuthStarter(t *testing.T) {
	for _, c := range []struct{ database, driver, image, placeholder, devURL string }{
		{"postgres", `"github.com/jackc/pgx/v5/stdlib"`, "image: postgres:18", "WHERE id = $1", "postgres://postgres:secret@127.0.0.1:5432/my_blog?sslmode=disable"},
		{"mysql", `"github.com/go-sql-driver/mysql"`, "image: mysql:8.4", "WHERE id = ?", "mysql://root:secret@127.0.0.1:3306/my_blog"},
	} {
		t.Run(c.database, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "my-blog")
			data := starterData{Name: "my-blog", Module: "blog", TugVersion: "v0.1.0", Frontend: "react", Auth: true, Database: c.database}
			if err := writeStarter(root, data); err != nil {
				t.Fatal(err)
			}
			formatted(t, root)
			read := reader(t, root)
			if db := read("db.go"); !strings.Contains(db, c.driver) || strings.Contains(db, "sqlite") {
				t.Errorf("db.go isn't %s's:\n%s", c.database, db)
			}
			if !strings.Contains(read("users_db.go"), c.placeholder) {
				t.Errorf("users_db.go has no %q", c.placeholder)
			}
			// The database's name is the app's, as SQL takes it without quotes.
			if compose := read("compose.yaml"); !strings.Contains(compose, c.image) || !strings.Contains(compose, ": my_blog\n") {
				t.Errorf("compose.yaml:\n%s", compose)
			}
			if !strings.Contains(read(".env"), "\nDB_URL="+c.devURL+"\n") || !strings.Contains(read(".env.example"), "\nDB_URL="+c.devURL+"\n") {
				t.Errorf("the .env and .env.example don't name compose.yaml's database, %s", c.devURL)
			}
			if !strings.Contains(read("db_test.go"), `"`+c.devURL+`"`) {
				t.Error("the tests don't make their databases on compose.yaml's, without DB_URL")
			}
			for _, f := range []string{"main.go", "db.go", "users_db.go", "passkeys_db.go", "jobs_db.go", "throttles_db.go", "cache_db.go", "tokens_db.go", "broadcasts_db.go", "notifications_db.go", "migrations_db.go", "db_test.go", "compose.yaml", "Dockerfile", "README.md", ".env.example", ".gitignore"} {
				if got := read(f); strings.Contains(got, "[[") || strings.Contains(got, "app.db") || strings.Contains(got, "DB_PATH") {
					t.Errorf("%s has a placeholder left, or SQLite's file:\n%s", f, got)
				}
			}
		})
	}
	for name, want := range map[string]string{"blog": "blog", "My Blog!": "my_blog", "2048": "app_2048", "---": "app", strings.Repeat("b", 50): strings.Repeat("b", 40)} {
		if got := (starterData{Name: name}).DBName(); got != want {
			t.Errorf("%q's database is %q, want %q", name, got, want)
		}
	}
}

func TestNewTakesOneDatabaseAndOnlyWithAuth(t *testing.T) {
	checkout, _ := filepath.Abs("../..")
	for _, c := range []struct {
		flags []string
		want  string
	}{
		{[]string{"-postgres"}, "without -auth, the app has no database"},
		{[]string{"-auth", "-postgres", "-mysql"}, "-postgres or -mysql, not both"},
	} {
		dir := filepath.Join(t.TempDir(), "blog")
		err := runNew(append([]string{"-no-install", "-tug-dir", checkout, dir}, c.flags...))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: %v", c.flags, err)
		}
		if _, err := os.Stat(dir); err == nil {
			t.Errorf("%v: tug new made the app", c.flags)
		}
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

func TestNewMakesAnAppWithOneFrontend(t *testing.T) {
	checkout, _ := filepath.Abs("../..")
	dir := filepath.Join(t.TempDir(), "both")
	err := runNew([]string{"-no-install", "-vue", "-svelte", "-tug-dir", checkout, dir})
	if err == nil || !strings.Contains(err.Error(), "-vue or -svelte, not both") {
		t.Errorf("-vue with -svelte: %v", err)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Error("tug new made an app with two frontends")
	}
}

func TestNewWithSSRAddsTheAppOnTheServer(t *testing.T) {
	for _, frontend := range frontends {
		for _, auth := range []bool{false, true} {
			name := frontend
			if auth {
				name += " auth"
			}
			t.Run(name, func(t *testing.T) {
				root := filepath.Join(t.TempDir(), "blog")
				data := starterData{Name: "blog", Module: "blog", TugVersion: "v0.1.0", Frontend: frontend, Auth: auth, SSR: true}
				if auth {
					data.Database = "sqlite"
				}
				if err := writeStarter(root, data); err != nil {
					t.Fatal(err)
				}
				formatted(t, root)
				read := reader(t, root)
				for _, f := range onlySSR(data) {
					read(f)
				}
				for f, want := range map[string]string{
					"main.go":        "ssr.Gateway{DevServer: assets.DevServer",
					"package.json":   `"build": "vite build && vite build --ssr"`,
					"vite.config.ts": "input: 'resources/js/ssr." + data.Script() + "'",
					"app.html":       "{{ .InertiaHead }}",
					"Dockerfile":     "FROM gcr.io/distroless/nodejs24-debian12",
					".gitignore":     "/ssr/build/",
					".dockerignore":  "ssr/build",
					".env.example":   "SSR_URL=",
				} {
					if got := read(f); !strings.Contains(got, want) || strings.Contains(got, "[[") {
						t.Errorf("%s has no %q, or a placeholder left:\n%s", f, want, got)
					}
				}
			})
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
		// Vue's and Svelte's apps have the same Go as React's, so two
		// kinds of each build each of their layers, and both sides of
		// -ssr, rather than every kind again.
		{"Vue", []string{"-vue"}},
		{"Vue auth with SSR", []string{"-vue", "-auth", "-ssr"}},
		{"Svelte", []string{"-svelte"}},
		{"Svelte auth with SSR", []string{"-svelte", "-auth", "-ssr"}},
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
			if kind.name == "plain" {
				writesItsTexts(t, dir)
			}
			if kind.name == "auth" {
				migratesByItsCommand(t, dir)
			}
		})
	}
}

// writesItsTexts runs tug lang in the app in dir, and checks the file has
// tug's texts, from the app's run, and the name of the starter's form's
// field, from its Go.
func writesItsTexts(t *testing.T, dir string) {
	t.Helper()
	t.Chdir(dir)
	if err := runLang([]string{"vi"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "lang", "vi.json"))
	if err != nil {
		t.Fatal(err)
	}
	var texts map[string]string
	if err := json.Unmarshal(data, &texts); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{":field is required", "Not Found", "name"} {
		if _, ok := texts[want]; !ok {
			t.Errorf("lang/vi.json hasn't %q", want)
		}
	}
}

// TestANewAppOnPostgresOrMySQLPassesItsOwnTests makes an app on each of
// the databases on a server as a person would, which writes its types
// with none running, and runs its tests on the server TUG_TEST_POSTGRES or
// TUG_TEST_MYSQL names, as CI's do, or else skips them. Its frontend is
// the one the test above builds on SQLite.
func TestANewAppOnPostgresOrMySQLPassesItsOwnTests(t *testing.T) {
	if testing.Short() {
		t.Skip("installs the new app's packages")
	}
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("needs npm")
	}
	checkout, _ := filepath.Abs("../..")
	for _, db := range []struct{ flag, server string }{{"-postgres", "TUG_TEST_POSTGRES"}, {"-mysql", "TUG_TEST_MYSQL"}} {
		t.Run(db.flag[1:], func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "blog")
			if err := runNew([]string{dir, "-tug-dir", checkout, "-auth", db.flag}); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(dir, "resources/js/tug/pages.ts")); err != nil {
				t.Errorf("tug new didn't write the types: %v", err)
			}
			vet := exec.Command("go", "vet", "./...")
			vet.Dir = dir
			if out, err := vet.CombinedOutput(); err != nil {
				t.Fatalf("go vet: %v\n%s", err, out)
			}
			server := os.Getenv(db.server)
			if server == "" {
				t.Skipf("%s names no server for the app's tests to make their databases on", db.server)
			}
			test := exec.Command("go", "test", "./...")
			test.Dir = dir
			test.Env = append(os.Environ(), "DB_URL="+server)
			if out, err := test.CombinedOutput(); err != nil {
				t.Errorf("go test: %v\n%s", err, out)
			}
		})
	}
}

// migratesByItsCommand builds the auth app in dir, and runs its migrate
// command on a new database: the command runs the migrations itself, where
// every other run of the app has run them as it started.
func migratesByItsCommand(t *testing.T, dir string) {
	t.Helper()
	build := exec.Command("go", "build", "-o", "app", ".")
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	vars, err := readDotEnv(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	// app runs the app's binary with args, on the SQLite database at db.
	app := func(db string, args ...string) string {
		t.Helper()
		cmd := exec.Command(filepath.Join(dir, "app"), args...)
		cmd.Dir = dir
		cmd.Env = append(append(os.Environ(), vars...), "APP_URL=http://localhost:8080", "DB_PATH="+db)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("./app %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}
	db := filepath.Join(t.TempDir(), "app.db")
	if out := app(db, "migrate", "status"); !strings.Contains(out, "not run yet") || strings.Contains(out, "each of them run") {
		t.Errorf("migrate status of a new database:\n%s", out)
	}
	if out := app(db, "migrate"); !strings.Contains(out, "Ran ") {
		t.Errorf("migrate:\n%s", out)
	}
	if out := app(db, "migrate", "status"); !strings.Contains(out, "each of them run") {
		t.Errorf("migrate status once they've run:\n%s", out)
	}
	// Another command, as jobs, runs them as the app starts.
	if out := app(filepath.Join(t.TempDir(), "app.db"), "jobs"); !strings.Contains(out, "ran a migration") || !strings.Contains(out, "No job has failed for good.") {
		t.Errorf("jobs on a new database:\n%s", out)
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
	// Its address, as a deployed app has one, and tug dev gives it: the
	// auth starter's links, and its passkeys, need it.
	app.Env = append(append(os.Environ(), vars...), "ADDR="+addr, "APP_URL=http://"+addr)
	app.Stdout, app.Stderr = &out, &out
	if err := app.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		app.Process.Signal(os.Interrupt)
		app.Wait()
	}()

	var page string
	var header http.Header
	for deadline := time.Now().Add(30 * time.Second); !strings.Contains(page, `data-server-rendered="true"`); time.Sleep(100 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("no first visit came back rendered on the server; the last was\n%s\nand the app said\n%s", page, out.String())
		}
		if resp, err := http.Get("http://" + addr + "/"); err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			page, header = string(b), resp.Header
		}
	}

	// The page says what a browser may do with it, and its scripts carry
	// the nonce its policy runs scripts by: the template's, Vite's, and
	// those from the server's head. The page object is data, not a script.
	policy := header.Get("Content-Security-Policy")
	_, nonce, _ := strings.Cut(policy, "'nonce-")
	nonce, _, _ = strings.Cut(nonce, "'")
	if nonce == "" || header.Get("X-Frame-Options") != "SAMEORIGIN" {
		t.Errorf("the page's policy is %q, and its headers %v", policy, header)
	}
	scripts := 0
	for _, after := range strings.Split(page, "<script")[1:] {
		tag, _, _ := strings.Cut(after, ">")
		if strings.Contains(tag, `type="application/json"`) {
			continue
		}
		scripts++
		if !strings.Contains(tag, ` nonce="`+nonce+`"`) {
			t.Errorf("<script%s> hasn't the page's nonce, %s", tag, nonce)
		}
	}
	if scripts == 0 {
		t.Errorf("the page runs no scripts:\n%s", page)
	}
}
