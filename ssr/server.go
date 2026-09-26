package ssr

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// Server runs the app's SSR bundle with Node, in a process beside the app,
// for a Gateway to render with. Run starts it, and keeps it running until
// the app stops.
type Server struct {
	// Bundle is what `vite build --ssr` built: the entry, which starts
	// Inertia's SSR server, and the chunks it imports. It's usually an
	// embed.FS, through fs.Sub, so it's in the binary with the rest.
	Bundle fs.FS

	// Entry is the file in Bundle that starts the server. Default
	// "ssr.mjs".
	Entry string

	// Node is the program that runs the bundle: "node", found on PATH,
	// unless it names another, such as /nodejs/bin/node.
	Node string

	mu  sync.Mutex
	url string // the running server's, while it answers
}

// URL is where the server answers, while it does, and "" otherwise.
func (s *Server) URL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.url
}

func (s *Server) setURL(url string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.url = url
}

// Run writes the bundle to a directory of its own, and runs Node on it,
// with SSR_PORT set to a free port for it to answer on at 127.0.0.1. Once
// it answers, pages render there. If Node stops, Run starts it again, after
// a wait that grows while it keeps stopping. When ctx is done, it stops
// Node, and returns.
//
// Without a bundle, as before the first build, or without Node, Run logs
// that pages render in the browser, and returns nil: SSR is an extra, which
// the app can serve without. It's meant for tug's App.Go.
func (s *Server) Run(ctx context.Context) error {
	entry := cmp.Or(s.Entry, "ssr.mjs")
	if s.Bundle == nil {
		return nil
	}
	if _, err := fs.Stat(s.Bundle, entry); err != nil {
		slog.Info("ssr: no SSR bundle to run yet, so pages render in the browser; tug build makes one", "entry", entry)
		return nil
	}
	node, err := exec.LookPath(cmp.Or(s.Node, "node"))
	if err != nil {
		slog.Warn("ssr: Node isn't here to run the SSR bundle, so pages render in the browser", "err", err)
		return nil
	}
	dir, err := os.MkdirTemp("", "tug-ssr-")
	if err != nil {
		slog.Error("ssr: the SSR bundle has nowhere to go, so pages render in the browser", "err", err)
		return nil
	}
	defer os.RemoveAll(dir)
	if err := os.CopyFS(dir, s.Bundle); err != nil {
		slog.Error("ssr: the SSR bundle didn't copy out, so pages render in the browser", "err", err)
		return nil
	}

	wait := time.Second
	for {
		started := time.Now()
		err := s.run(ctx, node, filepath.Join(dir, entry))
		if ctx.Err() != nil {
			return nil
		}
		if time.Since(started) > time.Minute {
			wait = time.Second // it ran a while: this is a new trouble
		}
		slog.Error("ssr: the SSR server stopped, so pages render in the browser until it's back", "err", err, "restart_in", wait)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(wait):
		}
		wait = min(2*wait, time.Minute)
	}
}

// run runs Node on entry once, until it exits or ctx is done.
func (s *Server) run(ctx context.Context, node, entry string) error {
	port, err := freePort()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, node, entry)
	cmd.Dir = filepath.Dir(entry)
	cmd.Env = append(os.Environ(), "SSR_PORT="+strconv.Itoa(port), "NODE_ENV=production")
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	// Asked to stop, Node finishes the renders it has, and has five seconds
	// to before it's killed.
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 5 * time.Second
	stopWithApp(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	// exited is closed once Node has, with its error in waited.
	var waited error
	exited := make(chan struct{})
	go func() {
		waited = cmd.Wait()
		close(exited)
	}()

	url := "http://127.0.0.1:" + strconv.Itoa(port)
	if err := answers(ctx, url, exited); err != nil {
		cmd.Process.Kill()
		<-exited
		return cmp.Or(waited, err)
	}
	slog.Info("ssr: rendering pages with Node", "addr", url, "pid", cmd.Process.Pid)
	s.setURL(url)
	defer s.setURL("")
	<-exited
	return cmp.Or(waited, errors.New("Node exited"))
}

// answers waits for the server at url to answer its health check, for as
// long as it takes Node to start, which is seconds at most.
func answers(ctx context.Context, url string, exited <-chan struct{}) error {
	client := &http.Client{Timeout: time.Second}
	deadline := time.After(30 * time.Second)
	for {
		if resp, err := client.Get(url + "/health"); err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-exited:
			return errors.New("Node exited before its server answered")
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("the SSR server didn't answer at %s in 30 seconds", url)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// freePort is a port on 127.0.0.1 that nothing listens on: one the system
// hands out, and takes back at once, for Node to listen on.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
