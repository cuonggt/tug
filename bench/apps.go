package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cuonggt/tug/bench/page"
)

// An app is an app in apps/ that serves the page: setup.sh builds it for
// production, start.sh serves it on 127.0.0.1:$PORT, and versions.sh says
// what it runs on. There's one, tug's, as tug new makes an app.
type app struct {
	Dir  string // its directory in apps/
	Name string
	// Server is how it's served, for the tables.
	Server string
	// Deployed are the paths in its directory a deploy copies, for its
	// size.
	Deployed []string
}

var apps = []app{
	{Dir: "tug", Name: "tug", Server: "net/http, one process", Deployed: []string{".build/blog"}},
}

func (a app) dir() string { return filepath.Join("apps", a.Dir) }

func (a app) setup() error {
	cmd := exec.Command("./setup.sh")
	cmd.Dir = a.dir()
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

func (a app) versions() ([]string, error) {
	cmd := exec.Command("./versions.sh")
	cmd.Dir = a.dir()
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s: versions.sh: %w", a.Name, err)
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n"), nil
}

// A server is an app started by its start.sh, in a process group of its
// own, so that stopping it stops the workers it started too.
type server struct {
	app
	addr string
	cmd  *exec.Cmd
	log  *os.File
	exit chan error
}

func (a app) start() (*server, error) {
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	log, err := os.CreateTemp("", "bench-"+a.Dir+"-*.log")
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("./start.sh")
	cmd.Dir = a.dir()
	cmd.Env = append(os.Environ(), "PORT="+strconv.Itoa(port))
	cmd.Stdout, cmd.Stderr = log, log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		log.Close()
		return nil, fmt.Errorf("%s: start.sh: %w", a.Name, err)
	}
	s := &server{app: a, addr: "127.0.0.1:" + strconv.Itoa(port), cmd: cmd, log: log, exit: make(chan error, 1)}
	go func() { s.exit <- cmd.Wait() }()
	return s, nil
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// ready waits for the app to answer its page.
func (s *server) ready(timeout time.Duration) error {
	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case err := <-s.exit:
			return fmt.Errorf("%s stopped as it started (%v); its log:\n%s", s.Name, err, s.tail())
		default:
		}
		resp, err := client.Get("http://" + s.addr + "/posts/42")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("%s didn't answer in %v; its log:\n%s", s.Name, timeout, s.tail())
}

// stop stops the app's process group: asked first, then made to.
func (s *server) stop() {
	defer s.log.Close()
	syscall.Kill(-s.cmd.Process.Pid, syscall.SIGTERM)
	select {
	case <-s.exit:
	case <-time.After(15 * time.Second):
	}
	// What's left of the group, so the next run has the machine to itself.
	syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
}

// tail is the end of the app's log.
func (s *server) tail() string {
	b, _ := os.ReadFile(s.log.Name())
	if len(b) > 4000 {
		b = b[len(b)-4000:]
	}
	return string(b)
}

// requests checks the app answers a first visit and a visit with the page
// the benchmarks expect, and returns their requests as the load sends them:
// a first visit's as a browser sends it, with no cookies, and a visit's as
// Inertia's client sends one, with the version and the cookies the first
// visit set. Those go back whether or not they're marked Secure, as a
// browser sends them to the app where it's deployed, over HTTPS.
func (s *server) requests() (firstVisit, visit []byte, err error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	const path = "/posts/42"
	headers := []string{"Accept: text/html, application/xhtml+xml"}
	first, cookies, err := s.get(client, path, headers)
	if err != nil {
		return nil, nil, err
	}
	visitHeaders := append(slices.Clone(headers), "X-Requested-With: XMLHttpRequest", "X-Inertia: true", "X-Inertia-Version: "+first.Version)
	if len(cookies) > 0 {
		visitHeaders = append(visitHeaders, "Cookie: "+strings.Join(cookies, "; "))
	}
	if _, _, err := s.get(client, path, visitHeaders); err != nil {
		return nil, nil, err
	}
	return requestFor(s.addr, path, headers...), requestFor(s.addr, path, visitHeaders...), nil
}

// sentPage is the part of a page object the benchmarks check.
type sentPage struct {
	Component string     `json:"component"`
	Props     page.Props `json:"props"`
	URL       string     `json:"url"`
	Version   string     `json:"version"`
}

// pageScript is where a first visit's HTML has the page object, as Inertia
// v3 has it.
var pageScript = regexp.MustCompile(`(?s)<script data-page="app" type="application/json">(.*?)</script>`)

// get sends a GET of path with headers, each a "Name: value" line, and
// returns the page its answer has, and the cookies it set, as a Cookie
// header has them.
func (s *server) get(client *http.Client, path string, headers []string) (sentPage, []string, error) {
	req, _ := http.NewRequest("GET", "http://"+s.addr+path, nil)
	for _, h := range headers {
		name, value, _ := strings.Cut(h, ": ")
		req.Header.Set(name, value)
	}
	inertia := req.Header.Get("X-Inertia") != ""
	what := "a first visit"
	if inertia {
		what = "a visit"
	}
	resp, err := client.Do(req)
	if err != nil {
		return sentPage{}, nil, fmt.Errorf("%s, %s: %w", s.Name, what, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return sentPage{}, nil, fmt.Errorf("%s, %s: %w", s.Name, what, err)
	}
	if resp.StatusCode != http.StatusOK {
		return sentPage{}, nil, fmt.Errorf("%s answered %s with %s:\n%.2000s", s.Name, what, resp.Status, body)
	}
	if inertia {
		if resp.Header.Get("X-Inertia") != "true" || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
			return sentPage{}, nil, fmt.Errorf("%s answered %s without the page object: %v", s.Name, what, resp.Header)
		}
	} else if m := pageScript.FindSubmatch(body); m != nil {
		body = m[1]
	} else {
		return sentPage{}, nil, fmt.Errorf("%s answered %s with no page object:\n%.2000s", s.Name, what, body)
	}
	var p sentPage
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(&p); err != nil {
		return sentPage{}, nil, fmt.Errorf("%s, %s's page: %w", s.Name, what, err)
	}
	if want := page.For(42); p.Component != page.Component || !strings.HasSuffix(p.URL, "/posts/42") || !reflect.DeepEqual(p.Props, want) {
		return sentPage{}, nil, fmt.Errorf("%s answered %s with another page: %+v", s.Name, what, p)
	}
	var cookies []string
	for _, c := range resp.Cookies() {
		cookies = append(cookies, c.Name+"="+c.Value)
	}
	return p, cookies, nil
}
