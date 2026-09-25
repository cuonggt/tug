package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func runBuild(args []string) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	out := fs.String("o", "", "the binary to write (default: the directory's name)")
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), `usage: tug build [-o binary]

Builds the app into one binary with its frontend in it: it writes the
TypeScript types (tug gen), type-checks the frontend when package.json
has a typecheck script, builds it with Vite, and builds the Go with the
build embedded, static (CGO_ENABLED=0) and stripped.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := checkProject(); err != nil {
		return err
	}
	env, err := appEnv()
	if err != nil {
		return err
	}
	if *out == "" {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		*out = filepath.Base(wd)
	}

	fmt.Println("tug build: the types")
	bin, err := buildApp(env, os.Stderr)
	if err != nil {
		return err
	}
	if _, err := generate(env, bin); err != nil {
		return err
	}
	pm := packageManager()
	if hasScript("typecheck") {
		fmt.Println("tug build: type-checking the frontend")
		if err := run(env, pm, "run", "typecheck"); err != nil {
			return err
		}
	}
	fmt.Println("tug build: the frontend")
	if err := run(env, pm, "run", "build"); err != nil {
		return err
	}
	fmt.Println("tug build: the binary")
	if err := run(append(env, "CGO_ENABLED=0"), "go", "build", "-trimpath", "-ldflags=-s -w", "-o", *out, "."); err != nil {
		return err
	}
	fi, err := os.Stat(*out)
	if err != nil {
		return err
	}
	fmt.Printf("tug build: %s, %.1f MB, frontend included\n", *out, float64(fi.Size())/(1<<20))
	return nil
}
