package main

import (
	"bytes"
	"crypto/rand"
	"embed"
	"encoding/base64"
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

// starters are the apps tug new makes: starter, and with -auth, starter
// with starter-auth laid over it, whose files replace the ones of the same
// name and add the rest. Files ending in .tmpl are templates, filled in
// with the app's name and module between [[ and ]], which leaves the {{ }}
// of Go's own templates, such as app.html's, alone; the rest are copied as
// they are.
//
//go:embed all:starter all:starter-auth
var starters embed.FS

// notInAuth are the plain starter's files that an app with -auth has no
// use for, and leaves out: its layouts, in resources/js/layouts, take
// Layout.tsx's place.
var notInAuth = []string{"resources/js/Layout.tsx"}

// onlySSR are the files that only an app with -ssr has: the app on the
// server, and the directory its build goes in.
var onlySSR = []string{"resources/js/ssr.tsx", "ssr/.gitkeep"}

type starterData struct {
	Name       string // the directory's name
	Module     string // the Go module path
	TugVersion string // the tug module version go.mod requires
	TugDir     string // a tug checkout go.mod replaces it with, when tug isn't a release
	Auth       bool   // with accounts: starter-auth over starter
	SSR        bool   // with pages rendered on the server too
}

func runNew(args []string) error {
	flags := flag.NewFlagSet("new", flag.ContinueOnError)
	module := flags.String("module", "", "the app's Go module path (default: the directory's name)")
	tugDir := flags.String("tug-dir", "", "a checkout of tug to build the app against, rather than a release")
	noInstall := flags.Bool("no-install", false, "don't install the app's packages or write its types")
	withAuth := flags.Bool("auth", false, "with accounts: registering, verifying an email, logging in with two factors, resetting a password, and settings, with the users in SQLite")
	withSSR := flags.Bool("ssr", false, "with server-side rendering: a first visit's page renders on the server too, with Node, which runs beside the app")
	flags.Usage = func() {
		fmt.Fprint(flags.Output(), `usage: tug new [flags] <dir>

Makes a new tug app in dir: a Go server with an Inertia page, a form that
checks itself, a React frontend built by Vite, and a .env with a fresh
APP_KEY. With -auth, people register for accounts and verify their email,
log in, with a code from their phone too if they like, reset a forgotten
password by email, and change their profile, password and appearance in
settings; its frontend has Tailwind and shadcn/ui. With -ssr, a first
visit's page is rendered on the server as well as in the browser, by Node
running beside the app. Then it installs the Go and frontend packages and
writes the TypeScript types, so that "tug dev" runs it.

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
	data := starterData{Name: filepath.Base(abs), Module: *module, Auth: *withAuth, SSR: *withSSR}
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

	if *noInstall {
		// tug dev installs the npm packages and writes the types itself,
		// but the Go modules have to be there for the app to build.
		fmt.Printf("\nNext:\n\n  cd %s\n  go mod tidy\n  tug dev\n\n", dir)
		return nil
	}
	if err := install(abs); err != nil {
		return err
	}
	fmt.Printf("\nNext:\n\n  cd %s\n  tug dev\n\n", dir)
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
	dirs := []string{"starter"}
	if data.Auth {
		dirs = append(dirs, "starter-auth")
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
		for _, rel := range notInAuth {
			delete(files, rel)
		}
	}
	if !data.SSR {
		for _, rel := range onlySSR {
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
	key := make([]byte, 32)
	rand.Read(key)
	uses := "encrypts the sessions"
	if data.Auth {
		uses = "encrypts the sessions and two-factor secrets, and signs the links in mail"
	}
	env := "# Read by tug dev, and not committed. APP_KEY " + uses + ".\n" +
		"APP_KEY=base64:" + base64.StdEncoding.EncodeToString(key) + "\nAPP_DEBUG=true\n"
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
