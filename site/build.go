package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const repo = "https://github.com/cuonggt/tug"

// config is what a build is of, and for.
type config struct {
	Root string // the tug checkout, with docs/ and README.md
	Out  string // where the site is written
	Base string // the path it's served under: "/", or "/tug/" on GitHub Pages
	URL  string // its address, base and all, without a slash at the end, or "" when it's not known
}

// marker is the file that says a directory is a site this program wrote,
// which it may empty to write the site again.
const marker = ".tug-site"

// A site is the guide, made into pages.
type site struct {
	cfg     config
	Repo    string
	Version string // the latest release's, from the README
	Parts   []part // the guide's, in its order
	Index   *page  // docs/README.md
	pages   map[string]*page
	names   []string // the pages', sorted

	assets   map[string]string // each file of assets/, by name, and its hash
	snippets map[string]string // the home page's code, by file name
	tmpl     map[string]*template.Template
}

var latestRelease = regexp.MustCompile(`\bv(\d+\.\d+\.\d+) is the latest release`)

// build writes the site, and returns what's wrong with the guide's links.
func build(cfg config) ([]string, error) {
	s, err := load(cfg)
	if err != nil {
		return nil, err
	}
	problems := s.check()
	return problems, s.write()
}

// load reads the guide and makes its pages, and reads the site's own
// templates, assets and code, from the current directory, site/.
func load(cfg config) (*site, error) {
	s := &site{cfg: cfg, Repo: repo, pages: map[string]*page{}}

	readme, err := os.ReadFile(filepath.Join(cfg.Root, "README.md"))
	if err != nil {
		return nil, fmt.Errorf("reading tug's README: %w", err)
	}
	if m := latestRelease.FindSubmatch(readme); m != nil {
		s.Version = "v" + string(m[1])
	}

	files, err := filepath.Glob(filepath.Join(cfg.Root, "docs", "*.md"))
	if err != nil {
		return nil, err
	}
	names := map[string]bool{}
	for _, f := range files {
		names[strings.TrimSuffix(filepath.Base(f), ".md")] = true
	}
	c := newConverter(cfg.Root, cfg.Base, names)
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".md")
		src, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		p, err := c.convert(name, src)
		if err != nil {
			return nil, err
		}
		s.pages[name] = p
		s.names = append(s.names, name)
	}
	slices.Sort(s.names)
	if s.Index = s.pages["README"]; s.Index == nil {
		return nil, errors.New("no docs/README.md, the guide's index, in " + cfg.Root)
	}
	s.Parts = s.Index.parts

	if err := s.loadOwn(); err != nil {
		return nil, err
	}
	return s, nil
}

// loadOwn reads what's the site's own: its templates, its assets, and the
// code its home page shows.
func (s *site) loadOwn() error {
	if _, err := os.Stat("templates/layout.html"); err != nil {
		return errors.New("no templates/layout.html here: run it in site/")
	}

	s.assets = map[string]string{}
	entries, err := os.ReadDir("assets")
	if err != nil {
		return err
	}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join("assets", e.Name()))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		s.assets[e.Name()] = hex.EncodeToString(sum[:4])
	}

	s.snippets = map[string]string{}
	snippets, err := filepath.Glob("snippets/*.txt")
	if err != nil {
		return err
	}
	for _, f := range snippets {
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		s.snippets[strings.TrimSuffix(filepath.Base(f), ".txt")] = string(b)
	}

	funcs := template.FuncMap{
		"href": func(path string) string { return s.cfg.Base + path },
		"asset": func(name string) (string, error) {
			hash, ok := s.assets[name]
			if !ok {
				return "", fmt.Errorf("no assets/%s", name)
			}
			return s.cfg.Base + "assets/" + name + "?v=" + hash, nil
		},
		"num": func(n int) string { return fmt.Sprintf("%02d", n) },
		// snippet is a file of snippets/ as a block of code, under a bar
		// with label, in the language of its extension.
		"snippet": func(name, label string) (template.HTML, error) {
			code, ok := s.snippets[name]
			if !ok {
				return "", fmt.Errorf("no snippets/%s.txt", name)
			}
			lang := strings.TrimPrefix(filepath.Ext(name), ".")
			return template.HTML(codeBlock(label, lang, code)), nil
		},
	}
	s.tmpl = map[string]*template.Template{}
	for _, name := range []string{"home", "guide", "doc", "404"} {
		t, err := template.New("").Funcs(funcs).ParseFiles(
			"templates/layout.html", "templates/icons.html", "templates/"+name+".html")
		if err != nil {
			return err
		}
		s.tmpl[name] = t
	}
	return nil
}

// check finds the guide's links to pages or headings that aren't there,
// and pages its index doesn't list.
func (s *site) check() []string {
	var problems []string
	listed := map[string]bool{"README": true, "roadmap": true}
	for _, p := range s.Parts {
		listed[p.Name] = true
	}
	for _, name := range s.names {
		p := s.pages[name]
		problems = append(problems, p.problems...)
		if !listed[name] {
			problems = append(problems, fmt.Sprintf("docs/%s.md isn't one of the parts docs/README.md lists", name))
		}
		for _, l := range p.links {
			to := s.pages[l.Page]
			switch {
			case to == nil:
				problems = append(problems, fmt.Sprintf("docs/%s.md: a link to %s, a page that isn't there", name, l.Dest))
			case l.ID != "" && !to.ids[l.ID]:
				problems = append(problems, fmt.Sprintf("docs/%s.md: a link to %s, a heading that isn't there", name, l.Dest))
			}
		}
	}
	return problems
}

// A view is what a page's template is given.
type view struct {
	*site
	Kind        string // the template: home, guide, doc or 404
	Title       string
	Description string
	Path        string // the page's, under the site's base
	Canonical   string // its address, when the site's is known
	Page        *page
	Part        *part // the page's part of the guide, when it's one
	Prev, Next  *part
}

// write writes the site to its directory, emptying it first.
func (s *site) write() error {
	out := s.cfg.Out
	if err := clean(out); err != nil {
		return err
	}
	put := func(name string, b []byte) error {
		file := filepath.Join(out, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			return err
		}
		return os.WriteFile(file, b, 0o644)
	}
	render := func(file string, v view) error {
		if s.cfg.URL != "" && v.Kind != "404" {
			v.Canonical = s.cfg.URL + "/" + v.Path
		}
		var b bytes.Buffer
		if err := s.tmpl[v.Kind].ExecuteTemplate(&b, "layout", v); err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
		return put(file, b.Bytes())
	}

	home := view{
		site:        s,
		Kind:        "home",
		Title:       "tug: Go handlers, Inertia pages, no API in between",
		Description: s.Index.Description,
	}
	if err := render("index.html", home); err != nil {
		return err
	}

	index := &part{Title: s.Index.Title, Name: "README", Path: s.Index.Path}
	guide := view{
		site:        s,
		Kind:        "guide",
		Title:       s.Index.Title + " · tug",
		Description: s.Index.Description,
		Path:        s.Index.Path,
		Page:        s.Index,
	}
	if len(s.Parts) > 0 {
		guide.Next = &s.Parts[0]
	}
	if err := render("docs/index.html", guide); err != nil {
		return err
	}

	for _, name := range s.names {
		p := s.pages[name]
		if name == "README" {
			continue
		}
		v := view{
			site:        s,
			Kind:        "doc",
			Title:       p.Title + " · tug",
			Description: p.Description,
			Path:        p.Path,
			Page:        p,
		}
		for i := range s.Parts {
			if s.Parts[i].Name != name {
				continue
			}
			v.Part = &s.Parts[i]
			v.Prev = index
			if i > 0 {
				v.Prev = &s.Parts[i-1]
			}
			if i+1 < len(s.Parts) {
				v.Next = &s.Parts[i+1]
			}
		}
		if err := render(p.Path+"index.html", v); err != nil {
			return err
		}
	}

	notFound := view{site: s, Kind: "404", Title: "Not found · tug", Description: "There's no page here."}
	if err := render("404.html", notFound); err != nil {
		return err
	}

	search, err := s.searchIndex()
	if err != nil {
		return err
	}
	if err := put("search.json", search); err != nil {
		return err
	}
	for name := range s.assets {
		b, err := os.ReadFile(filepath.Join("assets", name))
		if err != nil {
			return err
		}
		if err := put("assets/"+name, b); err != nil {
			return err
		}
	}
	if s.cfg.URL != "" {
		if err := put("sitemap.xml", s.sitemap()); err != nil {
			return err
		}
		robots := "User-agent: *\nAllow: /\n\nSitemap: " + s.cfg.URL + "/sitemap.xml\n"
		if err := put("robots.txt", []byte(robots)); err != nil {
			return err
		}
	}
	// GitHub Pages leaves a site with this file as it is, rather than run
	// Jekyll on it.
	if err := put(".nojekyll", nil); err != nil {
		return err
	}
	return put(marker, nil)
}

// clean empties the directory the site goes in, if it's one this program
// wrote, or makes it.
func clean(dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return os.MkdirAll(dir, 0o755)
	}
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		if _, err := os.Stat(filepath.Join(dir, marker)); err != nil {
			return fmt.Errorf("%s has files that aren't a site's: empty it, or give another -out", dir)
		}
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	}
	return os.MkdirAll(dir, 0o755)
}

// searchIndex is what site.js searches: each page's title and path, and
// each section's heading, ID and text, as arrays rather than objects, as
// it's fetched whole. The roadmap's sections are their headings and a
// little of their text, so a search finds the guide before the history.
func (s *site) searchIndex() ([]byte, error) {
	var idx struct {
		Pages    [][2]string `json:"pages"`
		Sections [][4]any    `json:"sections"`
	}
	add := func(p *page, title string) {
		n := len(idx.Pages)
		idx.Pages = append(idx.Pages, [2]string{title, p.Path})
		for _, sec := range p.sections {
			text := sec.Text
			if p.Name == "roadmap" {
				text = shorten(text, 200)
			}
			if sec.ID == "" && text == "" {
				continue
			}
			idx.Sections = append(idx.Sections, [4]any{n, sec.Heading, sec.ID, text})
		}
	}
	add(s.Index, s.Index.Title)
	for _, part := range s.Parts {
		if p := s.pages[part.Name]; p != nil {
			add(p, part.Title)
		}
	}
	for _, name := range s.names {
		if listedPart(s.Parts, name) || name == "README" {
			continue
		}
		add(s.pages[name], s.pages[name].Title)
	}
	return json.Marshal(idx)
}

func listedPart(parts []part, name string) bool {
	for _, p := range parts {
		if p.Name == name {
			return true
		}
	}
	return false
}

func (s *site) sitemap() []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	loc := func(path string) {
		fmt.Fprintf(&b, "  <url><loc>%s/%s</loc></url>\n", s.cfg.URL, path)
	}
	loc("")
	for _, name := range s.names {
		loc(s.pages[name].Path)
	}
	b.WriteString("</urlset>\n")
	return b.Bytes()
}
