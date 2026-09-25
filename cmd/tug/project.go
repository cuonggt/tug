package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// work is where tug keeps what it makes while it works on an app: the
// binary it builds, the TypeScript it asks the app for, and the file that
// tells Vite to reload the browser. The app's .gitignore leaves it out.
const work = ".tug"

// genDir is where tug gen writes the TypeScript.
const genDir = "resources/js/tug"

// checkProject reports what's missing, when the current directory isn't a
// tug app's: its main package, and its package.json. The directory needn't
// have a go.mod of its own, as an app inside another module doesn't.
func checkProject() error {
	goFiles, _ := filepath.Glob("*.go")
	if len(goFiles) == 0 {
		return errors.New("there's no Go here: run tug in the app's directory, or make an app with tug new")
	}
	if _, err := os.Stat("package.json"); err != nil {
		return errors.New("there's no package.json here: run tug in the app's directory, or make an app with tug new")
	}
	return os.MkdirAll(work, 0o755)
}

// appEnv is the environment the app runs with: tug's own, with the app's
// .env underneath it, as Laravel reads one.
func appEnv() ([]string, error) {
	env := os.Environ()
	set := map[string]bool{}
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		set[k] = true
	}
	vars, err := readDotEnv(".env")
	if err != nil {
		return nil, err
	}
	for _, kv := range vars {
		if k, _, _ := strings.Cut(kv, "="); !set[k] {
			env = append(env, kv)
			set[k] = true
		}
	}
	return env, nil
}

// readDotEnv reads KEY=value lines, skipping blanks and # comments, and a
// value's surrounding quotes. A file that isn't there is empty.
func readDotEnv(path string) ([]string, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var vars []string
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(strings.TrimPrefix(line, "export "), "=")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, fmt.Errorf("%s:%d: %q isn't KEY=value", path, n, line)
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
		vars = append(vars, strings.TrimSpace(k)+"="+v)
	}
	return vars, sc.Err()
}

// packageManager is the one the app's lockfile says it uses, npm unless
// another's lockfile is there.
func packageManager() string {
	for lock, pm := range map[string]string{"bun.lock": "bun", "bun.lockb": "bun", "pnpm-lock.yaml": "pnpm", "yarn.lock": "yarn"} {
		if _, err := os.Stat(lock); err == nil {
			return pm
		}
	}
	return "npm"
}

// hasScript reports whether package.json has a script called name.
func hasScript(name string) bool {
	data, err := os.ReadFile("package.json")
	return err == nil && bytes.Contains(data, []byte(`"`+name+`":`))
}

// run runs a command, its output going to tug's, and says which command
// failed when one does.
func run(env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

// writeIfChanged writes data to path unless it's there already, so that
// an unchanged file doesn't set off Vite, or an editor.
func writeIfChanged(path string, data []byte) (bool, error) {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, data, 0o644)
}

// prefixed writes lines to w, each after a label, as the processes tug dev
// runs share one terminal.
type prefixed struct {
	mu    *sync.Mutex
	w     io.Writer
	label string
	buf   []byte
}

func (p *prefixed) Write(b []byte) (int, error) {
	p.buf = append(p.buf, b...)
	for {
		i := bytes.IndexByte(p.buf, '\n')
		if i < 0 {
			return len(b), nil
		}
		p.mu.Lock()
		fmt.Fprintf(p.w, "%s %s\n", p.label, p.buf[:i])
		p.mu.Unlock()
		p.buf = p.buf[i+1:]
	}
}

// labels are the labels of tug dev's output, colored on a terminal.
func labels(color bool) (tug, app, vite string) {
	tug, app, vite = "tug  │", "app  │", "vite │"
	if color {
		tug, app, vite = "\x1b[33m"+tug+"\x1b[0m", "\x1b[36m"+app+"\x1b[0m", "\x1b[35m"+vite+"\x1b[0m"
	}
	return tug, app, vite
}

// isTerminal reports whether f is a terminal, for color.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0 && os.Getenv("NO_COLOR") == ""
}
