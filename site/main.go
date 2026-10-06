// Command site makes tug's website of its guide: each page of docs/, in
// the order docs/README.md lists them, a home page, and an index search
// reads, as static files, for GitHub Pages, in English and in each of the
// site's other languages, under a path of its own, as /vi/. Run it in
// site/:
//
//	go run .                          # writes the site to dist/
//	go run . -serve 127.0.0.1:8090    # serves it, made again as docs/ or the site changes
//	go run . -check                   # fails on a link to a page or heading that isn't there
//
// A language's words, the site's own, are in lang/<tag>.json, under the
// English, as an app's are, and -check fails on one the site says that a
// file has no words for. A page of the guide translated into it is
// docs/<tag>/<name>.md, which starts with the comment that says which
// English it was made from, by git's ID of its blob, as -check says it:
//
//	<!-- translated from docs/forms.md at 1f2e3d4c5b6a -->
//
// Its links to the pages translated with it are to their names, as
// forms.md, and to the rest, the English, as ../forms.md. A page that
// isn't translated is the English, which says so, and one whose English
// has changed since it was made says it may be out of date, and -check
// notes it, with the git diff of what changed.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func main() {
	var cfg config
	flag.StringVar(&cfg.Root, "root", "..", "the tug checkout, with docs/ and README.md")
	flag.StringVar(&cfg.Out, "out", "dist", "the directory the site is written to")
	flag.StringVar(&cfg.Base, "base", "/", "the path the site is served under, as /tug/ on GitHub Pages")
	flag.StringVar(&cfg.URL, "url", "", "the site's address, as https://cuonggt.github.io/tug, for canonical links and the sitemap")
	serve := flag.String("serve", "", "serve the site at this address, as 127.0.0.1:8090, made again as its sources change")
	check := flag.Bool("check", false, "fail on a link to a page or heading that isn't there")
	flag.Parse()
	log.SetFlags(0)
	log.SetPrefix("site: ")

	cfg.Base = "/" + strings.Trim(cfg.Base, "/") + "/"
	if cfg.Base == "//" {
		cfg.Base = "/"
	}
	cfg.URL = strings.TrimRight(cfg.URL, "/")

	if *serve != "" {
		log.Fatal(serveSite(*serve, cfg))
	}
	problems, notes, err := build(cfg)
	for _, n := range notes {
		log.Print(n)
	}
	for _, p := range problems {
		log.Print(p)
	}
	if err != nil {
		log.Fatal(err)
	}
	if *check && len(problems) > 0 {
		log.Fatalf("%d problems with the guide's links, its translations or the site's words", len(problems))
	}
	log.Printf("wrote %s", cfg.Out)
}

// serveSite serves the site from a directory of its own, written again
// before a request when a source has changed since it was last written.
func serveSite(addr string, cfg config) error {
	dir, err := os.MkdirTemp("", "tug-site-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	cfg.Out = dir

	var (
		mu    sync.RWMutex
		built string
	)
	rebuild := func() error {
		mu.Lock()
		defer mu.Unlock()
		now, err := fingerprint(cfg.Root)
		if err != nil || now == built {
			return err
		}
		began := time.Now()
		problems, notes, err := build(cfg)
		for _, n := range notes {
			log.Print(n)
		}
		for _, p := range problems {
			log.Print(p)
		}
		if err != nil {
			return err
		}
		built = now
		log.Printf("built in %v", time.Since(began).Round(time.Millisecond))
		return nil
	}
	if err := rebuild(); err != nil {
		return err
	}

	files := http.FileServer(http.Dir(dir))
	site := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := rebuild(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		mu.RLock()
		defer mu.RUnlock()
		// What isn't there is the 404 page, as GitHub Pages has it.
		name := filepath.Join(dir, filepath.FromSlash(path.Clean("/"+r.URL.Path)))
		if _, err := os.Stat(name); errors.Is(err, fs.ErrNotExist) {
			page, err := os.ReadFile(filepath.Join(dir, "404.html"))
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write(page)
			return
		}
		files.ServeHTTP(w, r)
	})

	mux := http.NewServeMux()
	if cfg.Base == "/" {
		mux.Handle("/", site)
	} else {
		mux.Handle(cfg.Base, http.StripPrefix(strings.TrimSuffix(cfg.Base, "/"), site))
		mux.Handle("/{$}", http.RedirectHandler(cfg.Base, http.StatusFound))
	}
	log.Printf("serving http://%s%s", addr, cfg.Base)
	return http.ListenAndServe(addr, mux)
}

// fingerprint is the names, sizes and times of the site's sources: the
// guide and its translations, the README and the benchmarks' results, and
// the site's own files and words.
func fingerprint(root string) (string, error) {
	var b strings.Builder
	add := func(dir string) error {
		return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if errors.Is(err, fs.ErrNotExist) {
				return nil // as results.json, before the benchmarks first run
			}
			if err != nil || d.IsDir() {
				return err
			}
			fi, err := d.Info()
			if err != nil {
				return err
			}
			fmt.Fprintf(&b, "%s %d %d\n", p, fi.Size(), fi.ModTime().UnixNano())
			return nil
		})
	}
	for _, dir := range []string{filepath.Join(root, "docs"), filepath.Join(root, "README.md"), filepath.Join(root, "bench", "results.json"), "templates", "assets", "snippets", "lang"} {
		if err := add(dir); err != nil {
			return "", err
		}
	}
	return b.String(), nil
}
