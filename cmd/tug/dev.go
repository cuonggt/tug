package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
)

func runDev(args []string) error {
	flags := flag.NewFlagSet("dev", flag.ContinueOnError)
	flags.Usage = func() {
		fmt.Fprint(flags.Output(), `usage: tug dev

Runs the app for development: Vite's dev server, which reloads the
frontend as it changes, and the Go server, which tug rebuilds and restarts
when a Go file, go.mod or a template changes, writing the TypeScript types
again and reloading the browser. The app's .env is read first, and the Go
server listens on 127.0.0.1:8080 unless ADDR or PORT say otherwise.
`)
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if err := checkProject(); err != nil {
		return err
	}
	env, err := appEnv()
	if err != nil {
		return err
	}

	var mu sync.Mutex
	tugLabel, appLabel, viteLabel := labels(isTerminal(os.Stdout))
	say := func(format string, a ...any) {
		mu.Lock()
		defer mu.Unlock()
		fmt.Printf(tugLabel+" "+format+"\n", a...)
	}
	appOut := &prefixed{mu: &mu, w: os.Stdout, label: appLabel}

	pm := packageManager()
	if _, err := os.Stat("node_modules"); err != nil {
		say("installing the frontend's packages with %s", pm)
		if err := run(env, pm, "install"); err != nil {
			return err
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	vite, err := startProc(env, &prefixed{mu: &mu, w: os.Stdout, label: viteLabel}, pm, "run", "dev")
	if err != nil {
		return err
	}
	defer vite.stop()

	addr, err := devAddr(env, say)
	if err != nil {
		return err
	}
	// TUG_DEV tells the app that Vite's dev server is its, which renders
	// its pages on the server too, so it needs no Node process of its own.
	env = append(env, "ADDR="+addr, "TUG_DEV=1")
	changes := watch(ctx, ".", 300*time.Millisecond)
	var app *proc
	defer func() { app.stop() }()

	for {
		began := time.Now()
		bin, err := buildApp(env, appOut)
		if err != nil {
			say("the build failed: fix it, and tug builds again when it's saved")
		} else {
			if changed, err := generate(env, bin); err != nil {
				say("the TypeScript types weren't written: %v", err)
			} else if len(changed) > 0 {
				say("wrote %s", strings.Join(changed, ", "))
			}
			app.stop()
			if app, err = startProc(env, appOut, bin); err != nil {
				say("the app didn't start: %v", err)
			} else if listening(addr, app, 10*time.Second) {
				say("serving http://%s, built in %v", addr, time.Since(began).Round(time.Millisecond))
				reloadBrowser()
			}
		}

		select {
		case <-ctx.Done():
			say("stopping")
			return nil
		case files := <-changes:
			say("%s changed", describe(files))
		case <-vite.done:
			say("Vite stopped: %v", vite.err)
			return vite.err
		case <-app.exited():
			say("the app stopped (%v): tug starts it again when a file is saved", app.err)
			select {
			case <-ctx.Done():
				return nil
			case files := <-changes:
				say("%s changed", describe(files))
			}
		}
	}
}

// devAddr is where the app listens under tug dev: where ADDR or PORT say,
// as tug.ConfigFromEnv reads them, or else 127.0.0.1:8080, or the next
// port that's free when that one isn't, as `php artisan serve` does.
func devAddr(env []string, say func(string, ...any)) (string, error) {
	vars := map[string]string{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		vars[k] = v
	}
	addr := vars["ADDR"]
	if addr == "" && vars["PORT"] != "" {
		addr = ":" + vars["PORT"]
	}
	if addr != "" {
		if !free(addr) {
			return "", fmt.Errorf("something else is listening on %s: stop it, or change ADDR or PORT", addr)
		}
		if strings.HasPrefix(addr, ":") {
			addr = "127.0.0.1" + addr // an address to dial, and show
		}
		return addr, nil
	}
	for port := 8080; port < 8100; port++ {
		addr = fmt.Sprintf("127.0.0.1:%d", port)
		if free(addr) {
			if port != 8080 {
				say("127.0.0.1:8080 is taken, so the app is on %s", addr)
			}
			return addr, nil
		}
	}
	return "", errors.New("ports 8080 to 8099 are all taken: set ADDR to one that isn't")
}

// free reports whether nothing listens on addr yet.
func free(addr string) bool {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// listening waits for the app to listen at addr, and gives up if it exits.
func listening(addr string, app *proc, within time.Duration) bool {
	for deadline := time.Now().Add(within); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		select {
		case <-app.exited():
			return false
		default:
		}
		if conn, err := net.Dial("tcp", addr); err == nil {
			conn.Close()
			return true
		}
	}
	return false
}

// reloadBrowser touches the file the tug plugin in vite.config.ts watches,
// which has Vite reload the browser: the Go server has changed.
func reloadBrowser() {
	os.WriteFile(filepath.Join(work, "reload"), []byte(time.Now().Format(time.RFC3339Nano)), 0o644)
}

func describe(files []string) string {
	if len(files) == 1 {
		return files[0]
	}
	return fmt.Sprintf("%s and %d more", files[0], len(files)-1)
}

// proc is a process tug dev runs, in a process group of its own.
type proc struct {
	cmd  *exec.Cmd
	done chan struct{}
	err  error
}

func startProc(env []string, out io.Writer, name string, args ...string) (*proc, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = out, out
	ownGroup(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &proc{cmd: cmd, done: make(chan struct{})}
	go func() {
		p.err = cmd.Wait()
		close(p.done)
	}()
	return p, nil
}

// exited is closed when the process has exited; for no process, never.
func (p *proc) exited() <-chan struct{} {
	if p == nil {
		return nil
	}
	return p.done
}

// stop asks the process and what it started to stop, and makes them after
// five seconds.
func (p *proc) stop() {
	if p == nil {
		return
	}
	select {
	case <-p.done:
		return
	default:
	}
	signalGroup(p.cmd, sigterm)
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
		signalGroup(p.cmd, sigkill)
		<-p.done
	}
}

// watch sends the files that changed, every so often, among those that
// changing the Go server takes a rebuild for: Go, go.mod and go.sum, and
// the templates Go embeds. Vite watches the frontend itself.
func watch(ctx context.Context, root string, every time.Duration) <-chan []string {
	changes := make(chan []string)
	go func() {
		last := snapshot(root)
		tick := time.NewTicker(every)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
			now := snapshot(root)
			changed := diff(last, now)
			if len(changed) == 0 {
				continue
			}
			// An editor saving several files, or a formatter after it:
			// let it finish, and build once.
			time.Sleep(100 * time.Millisecond)
			last = snapshot(root)
			select {
			case changes <- changed:
			case <-ctx.Done():
				return
			}
		}
	}()
	return changes
}

type stamp struct {
	mod  time.Time
	size int64
}

// skipDirs are directories the Go server's build doesn't read, or that
// tug and Vite write to.
var skipDirs = []string{"node_modules", "vendor", "testdata", "public"}

func snapshot(root string) map[string]stamp {
	files := map[string]stamp{}
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || slices.Contains(skipDirs, name)) {
				return filepath.SkipDir
			}
			return nil
		}
		if !watched(name) {
			return nil
		}
		if info, err := d.Info(); err == nil {
			files[path] = stamp{info.ModTime(), info.Size()}
		}
		return nil
	})
	return files
}

func watched(name string) bool {
	switch filepath.Ext(name) {
	case ".go", ".html", ".tmpl", ".gohtml":
		return true
	}
	return name == "go.mod" || name == "go.sum"
}

func diff(before, after map[string]stamp) []string {
	var changed []string
	for path, s := range after {
		if old, ok := before[path]; !ok || old != s {
			changed = append(changed, path)
		}
	}
	for path := range before {
		if _, ok := after[path]; !ok {
			changed = append(changed, path)
		}
	}
	slices.Sort(changed)
	return changed
}
