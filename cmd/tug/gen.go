package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func runGen(args []string) error {
	fs := flag.NewFlagSet("gen", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), `usage: tug gen

Writes the TypeScript of the app's pages and named routes to %s:
pages.ts, with the props of each page declared with tug.Page, and
routes.ts, with route() to build the path of each named route.

tug gen builds the app and runs it, and the app's Run writes the types
instead of serving: whatever main does before Run, it does for tug gen too.
`, genDir)
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
	bin, err := buildApp(env, os.Stderr)
	if err != nil {
		return err
	}
	changed, err := generate(env, bin)
	if err != nil {
		return err
	}
	if len(changed) == 0 {
		fmt.Println("the types are up to date")
	}
	for _, f := range changed {
		fmt.Println("wrote", f)
	}
	return nil
}

// buildApp builds the app's binary into the work directory.
func buildApp(env []string, out io.Writer) (string, error) {
	bin := filepath.Join(work, "app")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Run(); err != nil {
		return "", errors.New("the app doesn't build")
	}
	return filepath.Abs(bin)
}

// generate runs the app to have it write its TypeScript, and writes what's
// changed of it into genDir, returning the files it wrote.
func generate(env []string, bin string) ([]string, error) {
	out, err := filepath.Abs(filepath.Join(work, "gen.json"))
	if err != nil {
		return nil, err
	}
	os.Remove(out)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var logs bytes.Buffer
	cmd := exec.CommandContext(ctx, bin)
	cmd.Env = append(env, "TUG_GEN="+out)
	cmd.Stdout, cmd.Stderr = &logs, &logs
	runErr := cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, errors.New("the app ran for 30s without writing its types: does main call app.Run?")
	}
	data, err := os.ReadFile(out)
	if err != nil {
		if runErr != nil {
			return nil, fmt.Errorf("the app stopped before writing its types (%v):\n%s", runErr, strings.TrimSpace(logs.String()))
		}
		return nil, errors.New("the app didn't write its types: does main call app.Run?")
	}
	var ts struct{ Pages, Routes string }
	if err := json.Unmarshal(data, &ts); err != nil {
		return nil, err
	}

	var changed []string
	for name, content := range map[string]string{"pages.ts": ts.Pages, "routes.ts": ts.Routes} {
		path := filepath.Join(genDir, name)
		ok, err := writeIfChanged(path, []byte(content))
		if err != nil {
			return nil, err
		}
		if ok {
			changed = append(changed, path)
		}
	}
	return changed, nil
}
