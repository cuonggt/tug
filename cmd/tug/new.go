package main

import (
	"bytes"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"text/template"
)

// starters are the apps tug new makes, in layers, each laid over the ones
// before it, whose files it replaces where they have the same name: the
// plain app, in starter, and its frontend's, in react, vue or svelte; then
// with -auth, the app with accounts, in starter-auth, its frontend's, in
// react-auth, vue-auth or svelte-auth, and its database's, in sqlite,
// postgres or mysql: the SQL, which is all the Go that differs between
// them. starter and starter-auth have the Go and what every frontend uses.
// Files ending in .tmpl are templates, filled in with starterData between
// [[ and ]], which leaves the {{ }} of Go's own templates, such as
// app.html's, alone; the rest are copied as they are.
//
//go:embed all:starter all:starter-auth all:react all:react-auth all:vue all:vue-auth all:svelte all:svelte-auth all:sqlite all:postgres all:mysql
var starters embed.FS

// notInAuth are the plain app's files that an app with -auth has no use
// for, and leaves out: its layouts, in resources/js/layouts, take the
// plain Layout's place.
func notInAuth(data starterData) []string {
	return []string{"resources/js/Layout." + data.Component()}
}

// onlySSR are the files that only an app with -ssr has: the app on the
// server, and the directory its build goes in.
func onlySSR(data starterData) []string {
	return []string{"resources/js/ssr." + data.Script(), "ssr/.gitkeep"}
}

type starterData struct {
	Name       string // the directory's name
	Module     string // the Go module path
	TugVersion string // the tug module version go.mod requires
	TugDir     string // a tug checkout go.mod replaces it with, when tug isn't a release
	Frontend   string // react, vue or svelte: the layers of the frontend's own files
	Auth       bool   // with accounts: starter-auth over starter
	Database   string // with accounts, sqlite, postgres or mysql: the layer of its SQL
	SSR        bool   // with pages rendered on the server too
}

// React, Vue and Svelte say which frontend the app has, for the
// templates' conditions, as [[ if .React ]].
func (d starterData) React() bool  { return d.Frontend == "react" }
func (d starterData) Vue() bool    { return d.Frontend == "vue" }
func (d starterData) Svelte() bool { return d.Frontend == "svelte" }

// SQLite, Postgres and MySQL say which database an app with accounts
// keeps them in, as [[ if .SQLite ]].
func (d starterData) SQLite() bool   { return d.Database == "sqlite" }
func (d starterData) Postgres() bool { return d.Database == "postgres" }
func (d starterData) MySQL() bool    { return d.Database == "mysql" }

// DBName is the name of the app's database on Postgres or MySQL, as
// compose.yaml makes it: the app's name, as a name SQL takes without
// quotes, "my_blog" for my-blog.
func (d starterData) DBName() string {
	name := strings.Trim(nonIdentifier.ReplaceAllString(strings.ToLower(d.Name), "_"), "_")
	if name == "" || name[0] >= '0' && name[0] <= '9' {
		name = "app_" + name
	}
	// With room for the tests' own databases' names, as blog_test_5f3a9c0e,
	// in the 63 bytes Postgres keeps of a name.
	return strings.TrimRight(name[:min(len(name), 40)], "_")
}

var nonIdentifier = regexp.MustCompile(`[^a-z0-9_]+`)

// DevURL is where the app's database is in development: compose.yaml's
// Postgres or MySQL, on 127.0.0.1, which the .env tug new writes names,
// and the tests make their databases on.
func (d starterData) DevURL() string {
	if d.MySQL() {
		return "mysql://root:secret@127.0.0.1:3306/" + d.DBName()
	}
	return "postgres://postgres:secret@127.0.0.1:5432/" + d.DBName() + "?sslmode=disable"
}

// Framework is the frontend's name, as a sentence has it.
func (d starterData) Framework() string {
	return map[string]string{"react": "React", "vue": "Vue", "svelte": "Svelte"}[d.Frontend]
}

// Script is the extension of the frontend's app.tsx or app.ts, and its
// ssr's: TSX in React, TypeScript in the others.
func (d starterData) Script() string {
	if d.React() {
		return "tsx"
	}
	return "ts"
}

// Component is the extension of a page's file: Home.tsx, Home.vue or
// Home.svelte.
func (d starterData) Component() string {
	if d.React() {
		return "tsx"
	}
	return d.Frontend
}

// Adapter is the npm package of Inertia's adapter for the frontend.
func (d starterData) Adapter() string {
	return map[string]string{"react": "@inertiajs/react", "vue": "@inertiajs/vue3", "svelte": "@inertiajs/svelte"}[d.Frontend]
}

// AppearanceFile is the auth starter's file that keeps the appearance a
// user chose, in the browser, which the files every frontend shares point
// to.
func (d starterData) AppearanceFile() string {
	return map[string]string{
		"react":  "resources/js/hooks/use-appearance.ts",
		"vue":    "resources/js/composables/useAppearance.ts",
		"svelte": "resources/js/lib/appearance.svelte.ts",
	}[d.Frontend]
}

func runNew(args []string) error {
	flags := flag.NewFlagSet("new", flag.ContinueOnError)
	module := flags.String("module", "", "the app's Go module path (default: the directory's name)")
	tugDir := flags.String("tug-dir", "", "a checkout of tug to build the app against, rather than a release")
	noInstall := flags.Bool("no-install", false, "don't install the app's packages or write its types")
	withAuth := flags.Bool("auth", false, "with accounts: registering, verifying an email, logging in with two factors or a passkey, resetting a password, and settings, with the users in SQLite")
	withPostgres := flags.Bool("postgres", false, "with -auth, the users in Postgres, rather than SQLite")
	withMySQL := flags.Bool("mysql", false, "with -auth, the users in MySQL, rather than SQLite")
	withSSR := flags.Bool("ssr", false, "with server-side rendering: a first visit's page renders on the server too, with Node, which runs beside the app")
	withVue := flags.Bool("vue", false, "with a Vue frontend, rather than React's")
	withSvelte := flags.Bool("svelte", false, "with a Svelte frontend, rather than React's")
	flags.Usage = func() {
		fmt.Fprint(flags.Output(), `usage: tug new [flags] <dir>

Makes a new tug app in dir: a Go server with an Inertia page, a form that
checks itself, a React frontend built by Vite, or with -vue or -svelte a
Vue or Svelte one, and a .env with a fresh APP_KEY. With -auth, people
register for accounts and verify their email, log in, with a code from
their phone too if they like, or with a passkey, reset a forgotten
password by email, and change their profile and photo, password and
appearance in settings; its frontend has Tailwind and shadcn's
components, and its users are in SQLite, or with -postgres or -mysql, in
Postgres or MySQL, which its compose.yaml runs for development. With
-ssr, a first visit's page is rendered on the server as well as in the
browser, by Node running beside the app. Then it installs the Go and
frontend packages and writes the TypeScript types, so that "tug dev"
runs it.

`)
		flags.PrintDefaults()
	}
	// The flag package stops at the first argument that isn't a flag, and
	// the directory can come anywhere among them: "tug new blog -auth" as
	// well as "tug new -auth blog".
	var dirs []string
	for {
		if err := flags.Parse(args); err != nil {
			return err
		}
		if flags.NArg() == 0 {
			break
		}
		dirs = append(dirs, flags.Arg(0))
		args = flags.Args()[1:]
	}
	if len(dirs) != 1 {
		flags.Usage()
		return flag.ErrHelp
	}
	dir := dirs[0]

	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if entries, err := os.ReadDir(abs); err == nil && len(entries) > 0 {
		return fmt.Errorf("%s isn't empty: tug new makes a directory of its own", dir)
	}
	data := starterData{Name: filepath.Base(abs), Module: *module, Frontend: "react", Auth: *withAuth, SSR: *withSSR}
	switch {
	case *withVue && *withSvelte:
		return errors.New("an app has one frontend: -vue or -svelte, not both")
	case *withVue:
		data.Frontend = "vue"
	case *withSvelte:
		data.Frontend = "svelte"
	}
	switch {
	case (*withPostgres || *withMySQL) && !*withAuth:
		return errors.New("-postgres and -mysql are where -auth keeps its users: without -auth, the app has no database")
	case *withPostgres && *withMySQL:
		return errors.New("an app has one database: -postgres or -mysql, not both")
	case *withPostgres:
		data.Database = "postgres"
	case *withMySQL:
		data.Database = "mysql"
	case *withAuth:
		data.Database = "sqlite"
	}
	if data.Module == "" {
		data.Module = data.Name
	}
	if err := tugModule(&data, *tugDir); err != nil {
		return err
	}
	if err := writeStarter(abs, data); err != nil {
		return err
	}
	fmt.Printf("tug new: made %s\n", dir)

	next := []string{"cd " + dir}
	if *noInstall {
		// tug dev installs the npm packages and writes the types itself,
		// but the Go modules have to be there for the app to build.
		next = append(next, "go mod tidy")
	} else if err := install(abs); err != nil {
		return err
	}
	if data.Postgres() || data.MySQL() {
		// A database outlives any one run of the app, so tug dev leaves it
		// to compose.
		next = append(next, "docker compose up -d")
	}
	fmt.Printf("\nNext:\n\n  %s\n  tug dev\n\n", strings.Join(next, "\n  "))
	return nil
}

// tugModule works out which tug the app is built against: the version this
// tug is, when that's one the go command can fetch, or a checkout of it, as
// when tug was built from one.
func tugModule(data *starterData, dir string) error {
	if v := version(); dir == "" && (release(v) || fetched()) {
		data.TugVersion = v
		return nil
	}
	if dir == "" {
		dir = checkoutDir()
	}
	if dir == "" {
		return errors.New("this tug isn't a release: pass -tug-dir with the checkout of tug to build the app against")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	mod, err := os.ReadFile(filepath.Join(abs, "go.mod"))
	if err != nil || !bytes.HasPrefix(mod, []byte("module github.com/cuonggt/tug\n")) {
		return fmt.Errorf("%s isn't a checkout of tug", dir)
	}
	data.TugVersion, data.TugDir = "v0.0.0", abs
	return nil
}

// release reports whether v is a released version of tug, rather than a
// build from a checkout: "(devel)", or since Go 1.24 a pseudo-version such
// as v0.0.0-20260925051818-b571d11c4df5, "+dirty" when it had changes.
func release(v string) bool {
	return strings.HasPrefix(v, "v") && !strings.Contains(v, "+") && !pseudoVersion.MatchString(v)
}

var pseudoVersion = regexp.MustCompile(`\d{14}-[0-9a-f]{12}$`)

// fetched reports whether this tug was built from a module the go command
// fetched, as go install github.com/cuonggt/tug/cmd/tug@latest does, which
// its checksum says. Its version is one an app can require, even a
// pseudo-version: the commit is published.
func fetched() bool {
	info, ok := debug.ReadBuildInfo()
	return ok && info.Main.Sum != ""
}

// checkoutDir is the checkout this tug was built from, which its own
// source path says, unless it was built with -trimpath.
func checkoutDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok || !filepath.IsAbs(file) {
		return ""
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(file))) // cmd/tug/new.go
}

func writeStarter(root string, data starterData) error {
	dirs := []string{"starter", data.Frontend}
	if data.Auth {
		dirs = append(dirs, "starter-auth", data.Frontend+"-auth", data.Database)
	}
	files := map[string][]byte{} // by their path in the app
	for _, dir := range dirs {
		err := fs.WalkDir(starters, dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			content, err := starters.ReadFile(path)
			if err != nil {
				return err
			}
			rel := strings.TrimPrefix(path, dir+"/")
			if name, ok := strings.CutSuffix(rel, ".tmpl"); ok {
				tmpl, err := template.New(rel).Delims("[[", "]]").Option("missingkey=error").Parse(string(content))
				if err != nil {
					return err
				}
				var b bytes.Buffer
				if err := tmpl.Execute(&b, data); err != nil {
					return err
				}
				rel, content = name, b.Bytes()
			}
			files[rel] = content
			return nil
		})
		if err != nil {
			return err
		}
	}
	if data.Auth {
		for _, rel := range notInAuth(data) {
			delete(files, rel)
		}
	}
	if !data.SSR {
		for _, rel := range onlySSR(data) {
			delete(files, rel)
		}
	}
	for rel, content := range files {
		target := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, content, 0o644); err != nil {
			return err
		}
	}
	uses := "encrypts the sessions"
	if data.Auth {
		uses = "encrypts the sessions and two-factor secrets, and signs the links in mail and to photos"
	}
	env := "# Read by tug dev, and not committed. APP_KEY " + uses + ".\n" +
		"APP_KEY=" + newKey() + "\nAPP_DEBUG=true\n"
	if data.Postgres() || data.MySQL() {
		env += "# The database compose.yaml runs: docker compose up -d.\nDB_URL=" + data.DevURL() + "\n"
	}
	return os.WriteFile(filepath.Join(root, ".env"), []byte(env), 0o600)
}

// install gets the app ready to run: its Go modules, its frontend's
// packages, and the TypeScript types of its pages and routes.
func install(root string) error {
	if err := runIn(root, "go", "mod", "tidy"); err != nil {
		return err
	}
	if err := runIn(root, "npm", "install", "--no-audit", "--no-fund"); err != nil {
		return err
	}
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer os.Chdir(wd)
	if err := os.Chdir(root); err != nil {
		return err
	}
	if err := os.MkdirAll(work, 0o755); err != nil {
		return err
	}
	env, err := appEnv()
	if err != nil {
		return err
	}
	bin, err := buildApp(env, os.Stderr)
	if err != nil {
		return err
	}
	_, err = generate(env, bin)
	return err
}

func runIn(dir, name string, args ...string) error {
	fmt.Printf("tug new: %s %s\n", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}
