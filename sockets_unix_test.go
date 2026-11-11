//go:build unix

package tug

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// The app systemd hands sockets to runs in a process of its own, this test
// binary again: the descriptors it's handed are at 3 and 4 in the process
// that takes them, which in this one are the runtime's own. TestMain runs
// socketsApp there in place of the tests.

func TestMain(m *testing.M) {
	if os.Getenv("TUG_TEST_SOCKETS_APP") != "" {
		os.Exit(socketsApp())
	}
	os.Exit(m.Run())
}

// socketsApp is the app the tests start as systemd starts a service: its
// Config from the environment, / answering with TUG_TEST_SOCKETS_APP,
// which of the app's processes it is, and /env with systemd's variables,
// which Run unsets.
func socketsApp() int {
	app := New()
	process := os.Getenv("TUG_TEST_SOCKETS_APP")
	app.Get("/", func(c *Ctx) error { return c.String(http.StatusOK, process) })
	app.Get("/env", func(c *Ctx) error {
		return c.String(http.StatusOK, os.Getenv("LISTEN_PID")+os.Getenv("LISTEN_FDS")+os.Getenv("LISTEN_FDNAMES"))
	})
	if err := app.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// systemdSocket is a socket held as systemd holds a .socket unit's, open
// across the app's restarts, for each of its processes to be handed: its
// address, and the descriptor to hand on.
func systemdSocket(t *testing.T) (addr string, f *os.File) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close() // the copy File makes holds the socket open
	if f, err = ln.(*net.TCPListener).File(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return ln.Addr().String(), f
}

// systemdStarts starts the app as systemd starts a service with sockets:
// at the descriptors from 3, with their names in LISTEN_FDNAMES, and
// LISTEN_PID the app's own, through a shell whose exec keeps its process
// ID, with env after the rest. The app's log, and the error it stops with,
// go to the buffer it returns.
func systemdStarts(t *testing.T, sockets []*os.File, names []string, env ...string) (*exec.Cmd, *syncBuffer) {
	t.Helper()
	cmd := exec.Command("sh", "-c", `export LISTEN_PID=$$ && exec "$0"`, os.Args[0])
	cmd.Env = append(os.Environ(), "TUG_TEST_SOCKETS_APP=1",
		"ADDR=", "PORT=", "APP_URL=", "TLS_CERT=", "TLS_KEY=", "HTTP_ADDR=", "TUG_DEV=",
		"LISTEN_FDS="+strconv.Itoa(len(sockets)), "LISTEN_FDNAMES="+strings.Join(names, ":"))
	cmd.Env = append(cmd.Env, env...)
	cmd.ExtraFiles = sockets
	out := new(syncBuffer)
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// An app a test failed before it stopped.
	t.Cleanup(func() { cmd.Process.Kill() })
	return cmd, out
}

// stops stops the app as systemd stops a service, by SIGTERM, and waits
// for it to have shut down.
func stops(t *testing.T, app *exec.Cmd, out *syncBuffer) {
	t.Helper()
	app.Process.Signal(syscall.SIGTERM)
	if err := app.Wait(); err != nil {
		t.Fatalf("the app stopped with %v:\n%s", err, out)
	}
}

// bodyOf is res's body, which it closes.
func bodyOf(t *testing.T, res *http.Response) string {
	t.Helper()
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRunServesHTTPSAndHTTPOnTheSocketsSystemdNamesThem(t *testing.T) {
	certFile, keyFile, cert := writeCert(t, t.TempDir(), "example.com")
	plainAddr, plain := systemdSocket(t)
	secureAddr, secure := systemdSocket(t)
	// Two units' sockets come in no order systemd keeps, here http's
	// first, and their names say which is which.
	app, out := systemdStarts(t, []*os.File{plain, secure}, []string{"http", "https"}, "TLS_CERT="+certFile, "TLS_KEY="+keyFile)

	// The sockets listen from before the app starts, so these wait for it
	// rather than being refused.
	client := trusting(cert, "example.com")
	client.Timeout = time.Minute
	res, err := client.Get("https://" + secureAddr + "/")
	if err != nil {
		t.Fatalf("HTTPS on the socket named https: %v\n%s", err, out)
	}
	if body := bodyOf(t, res); res.StatusCode != http.StatusOK || body != "1" || res.ProtoMajor != 2 {
		t.Errorf("HTTPS: %s %d %q, want HTTP/2's 200 and the app's page", res.Proto, res.StatusCode, body)
	}

	req, _ := http.NewRequest(http.MethodGet, "http://"+plainAddr+"/posts?page=2", nil)
	req.Host = "example.com"
	shown := &http.Client{Timeout: time.Minute, CheckRedirect: noRedirects.CheckRedirect}
	res, err = shown.Do(req)
	if err != nil {
		t.Fatalf("HTTP on the socket named http: %v\n%s", err, out)
	}
	res.Body.Close()
	_, securePort, _ := net.SplitHostPort(secureAddr)
	if want := "https://example.com:" + securePort + "/posts?page=2"; res.StatusCode != http.StatusPermanentRedirect || res.Header.Get("Location") != want {
		t.Errorf("HTTP: %d to %q, want a 308 to %s, HTTPS's socket", res.StatusCode, res.Header.Get("Location"), want)
	}

	// What the app starts, as the SSR's Node, isn't told of the sockets.
	res, err = client.Get("https://" + secureAddr + "/env")
	if err != nil {
		t.Fatal(err)
	}
	if env := bodyOf(t, res); env != "" {
		t.Errorf("systemd's variables are left in the app's environment: %q", env)
	}

	stops(t, app, out)
	for _, want := range []string{
		"listening addr=" + secureAddr + " scheme=https from=systemd",
		"listening addr=" + plainAddr + " scheme=http from=systemd",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the log doesn't say %q:\n%s", want, out)
		}
	}
}

func TestARestartOnSystemdsSocketRefusesNoRequest(t *testing.T) {
	addr, socket := systemdSocket(t)
	first, firstOut := systemdStarts(t, []*os.File{socket}, nil)

	// A stream of visitors, each on a connection of its own, as a browser
	// arriving opens one, through the restart.
	client := &http.Client{Timeout: time.Minute, Transport: &http.Transport{DisableKeepAlives: true}}
	var (
		mu      sync.Mutex
		served  = map[string]int{} // by the app's process
		refused []error
		stop    = make(chan struct{})
		sending sync.WaitGroup
	)
	for range 4 {
		sending.Go(func() {
			for {
				select {
				case <-stop:
					return
				case <-time.After(5 * time.Millisecond):
				}
				res, err := client.Get("http://" + addr + "/")
				var body []byte
				if err == nil {
					body, err = io.ReadAll(res.Body)
					res.Body.Close()
				}
				mu.Lock()
				if err != nil {
					refused = append(refused, err)
				} else {
					served[string(body)]++
				}
				mu.Unlock()
			}
		})
	}
	until := func(process string) {
		t.Helper()
		for deadline := time.Now().Add(time.Minute); ; time.Sleep(10 * time.Millisecond) {
			mu.Lock()
			n := served[process]
			mu.Unlock()
			if n >= 20 {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("process %s of the app served %d requests in a minute", process, n)
			}
		}
	}

	until("1")
	stops(t, first, firstOut) // as systemctl restart does: the old process stopped, then the new one started
	second, secondOut := systemdStarts(t, []*os.File{socket}, nil, "TUG_TEST_SOCKETS_APP=2")
	until("2")
	close(stop)
	sending.Wait()
	stops(t, second, secondOut)

	if len(refused) > 0 {
		t.Errorf("%d of %d requests failed as the app restarted, the first with %v", len(refused), len(refused)+served["1"]+served["2"], refused[0])
	}
}

func TestSocketsThatDontFitStopTheApp(t *testing.T) {
	certFile, keyFile, _ := writeCert(t, t.TempDir(), "example.com")
	_, one := systemdSocket(t)
	_, two := systemdSocket(t)
	for _, tc := range []struct {
		sockets []*os.File
		env     []string
		want    string
	}{
		{[]*os.File{one}, []string{"TLS_CERT=" + certFile, "TLS_KEY=" + keyFile}, "systemd passed the app 1 socket, and with TLS it serves HTTPS and HTTP on two"},
		{[]*os.File{one, two}, nil, "systemd passed the app 2 sockets, and without TLS it serves plain HTTP on one"},
	} {
		app, out := systemdStarts(t, tc.sockets, nil, tc.env...)
		if err := app.Wait(); err == nil || !strings.Contains(out.String(), tc.want) {
			t.Errorf("%d sockets: %v, want the app to stop, saying %q:\n%s", len(tc.sockets), err, tc.want, out)
		}
	}
}

func TestSocketsForAnotherProcessAreLeftAndTheAppListensItself(t *testing.T) {
	captureLog(t)
	// A parent's variables, handed down: this process's descriptors 3 and
	// 4 are no sockets of systemd's.
	t.Setenv("LISTEN_PID", strconv.Itoa(os.Getppid()))
	t.Setenv("LISTEN_FDS", "2")
	t.Setenv("LISTEN_FDNAMES", "https:http")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	app := New(Config{Addr: addr})
	app.Get("/", text("home"))
	done := make(chan error, 1)
	go func() { done <- app.Run() }()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if res, err := http.Get("http://" + addr + "/"); err == nil {
			res.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the app didn't listen on its own address")
		}
	}
	syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the app didn't stop")
	}
	if os.Getenv("LISTEN_FDS") != "2" {
		t.Error("another process's variables were unset")
	}
}
