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
	"strings"
	"text/template"
)

// starter is the app tug new makes. Files ending in .tmpl are templates,
// filled in with the app's name and module between [[ and ]], which leaves
// the {{ }} of Go's own templates, such as app.html's, alone; the rest are
// copied as they are.
//
//go:embed all:starter
var starter embed.FS

type starterData struct {
	Name       string // the directory's name
	Module     string // the Go module path
	TugVersion string // the tug module version go.mod requires
	TugDir     string // a tug checkout go.mod replaces it with, when tug isn't a release
}

func runNew(args []string) error {
	flags := flag.NewFlagSet("new", flag.ContinueOnError)
	module := flags.String("module", "", "the app's Go module path (default: the directory's name)")
	tugDir := flags.String("tug-dir", "", "a checkout of tug to build the app against, rather than a release")
	noInstall := flags.Bool("no-install", false, "don't install the app's packages or write its types")
	flags.Usage = func() {
		fmt.Fprint(flags.Output(), `usage: tug new [flags] <dir>

Makes a new tug app in dir: a Go server with an Inertia page, a form that
checks itself, a React frontend built by Vite, and a .env with a fresh
APP_KEY. Then it installs the Go and frontend packages and writes the
TypeScript types, so that "tug dev" runs it.

`)
		flags.PrintDefaults()
	}
	// The directory may come before the flags, as in "tug new blog -module x".
	var dir string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		dir, args = args[0], args[1:]
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if dir == "" && flags.NArg() > 0 {
		dir = flags.Arg(0)
	}
	if dir == "" {
		flags.Usage()
		return flag.ErrHelp
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if entries, err := os.ReadDir(abs); err == nil && len(entries) > 0 {
		return fmt.Errorf("%s isn't empty: tug new makes a directory of its own", dir)
	}
	data := starterData{Name: filepath.Base(abs), Module: *module}
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

	if !*noInstall {
		if err := install(abs); err != nil {
			return err
		}
	}
	fmt.Printf("\nNext:\n\n  cd %s\n  tug dev\n\n", dir)
	return nil
}

// tugModule works out which tug the app is built against: the release this
// tug is, or a checkout of it, as when tug was built from one.
func tugModule(data *starterData, dir string) error {
	if v := version(); dir == "" && release(v) {
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
	err := fs.WalkDir(starter, "starter", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		content, err := starter.ReadFile(path)
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(path, "starter/")
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
		target := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, content, 0o644)
	})
	if err != nil {
		return err
	}
	key := make([]byte, 32)
	rand.Read(key)
	env := "# Read by tug dev, and not committed. APP_KEY encrypts the sessions.\n" +
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
