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
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"text/template/parse"
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

// A site is the guide, made into pages, in each of the site's languages.
type site struct {
	cfg     config
	Repo    string
	Version string        // the latest release's, from the README
	langs   []*language   // the site's, English first
	en      *language     // the guide's own
	names   []string      // the guide's pages', sorted
	results *benchResults // bench/results.json, when there is one

	assets      map[string]string // each file of assets/, by name, and its hash
	snippets    map[string]string // the home page's code, by file name
	scriptTexts []string          // what site.js says, which a page hands it in its language
	tmpl        map[*language]map[string]*template.Template
}

var latestRelease = regexp.MustCompile(`\bv(\d+\.\d+\.\d+) is the latest release`)

// build writes the site, and returns what's wrong with the guide's links,
// its translations, and the site's words, which -check fails on, and
// notes of what's out of date, as a translation whose English has changed
// since it was made.
func build(cfg config) (problems, notes []string, err error) {
	s, err := load(cfg)
	if err != nil {
		return nil, nil, err
	}
	if err := s.write(); err != nil {
		return nil, nil, err
	}
	problems, notes = s.check()
	// What the site says in each language is known once it's written.
	missing, unused := wordProblems(s.langs)
	return append(problems, missing...), append(notes, unused...), nil
}

// load reads the guide and makes its pages, and each language's
// translations of them, and reads the site's own words, templates, assets
// and code, from the current directory, site/.
func load(cfg config) (*site, error) {
	s := &site{cfg: cfg, Repo: repo}

	readme, err := os.ReadFile(filepath.Join(cfg.Root, "README.md"))
	if err != nil {
		return nil, fmt.Errorf("reading tug's README: %w", err)
	}
	if m := latestRelease.FindSubmatch(readme); m != nil {
		s.Version = "v" + string(m[1])
	}
	if s.langs, err = loadLanguages("lang"); err != nil {
		return nil, err
	}
	s.en = s.langs[0]

	files, err := filepath.Glob(filepath.Join(cfg.Root, "docs", "*.md"))
	if err != nil {
		return nil, err
	}
	names := map[string]bool{}
	for _, f := range files {
		names[strings.TrimSuffix(filepath.Base(f), ".md")] = true
	}
	c := newConverter(cfg.Root, "docs", names, s.en)
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
		p.Blob = blobID(src)
		s.en.pages[name] = p
		s.names = append(s.names, name)
	}
	slices.Sort(s.names)
	if s.en.Index = s.en.pages["README"]; s.en.Index == nil {
		return nil, errors.New("no docs/README.md, the guide's index, in " + cfg.Root)
	}
	s.en.Parts = s.en.Index.parts
	for _, l := range s.langs[1:] {
		if err := s.loadTranslations(l, names); err != nil {
			return nil, err
		}
	}

	if s.results, err = readBenchResults(cfg.Root); err != nil {
		return nil, err
	}
	if err := s.loadOwn(); err != nil {
		return nil, err
	}
	return s, nil
}

// loadTranslations reads l's translations of the guide's pages, from
// docs/<tag>/, each in its English's place among l's pages, which are the
// English where there's none. A translation starts with the comment that
// says which English it was made from, and is behind once that English
// has changed.
func (s *site) loadTranslations(l *language, names map[string]bool) error {
	maps.Copy(l.pages, s.en.pages)
	dir := "docs/" + l.Tag
	files, err := filepath.Glob(filepath.Join(s.cfg.Root, filepath.FromSlash(dir), "*.md"))
	if err != nil {
		return err
	}
	c := newConverter(s.cfg.Root, dir, names, l)
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".md")
		src, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		var said, from string
		m := translatedFrom.FindSubmatchIndex(src)
		if m != nil {
			said, from = string(src[m[2]:m[3]]), string(src[m[4]:m[5]])
			src = src[m[1]:]
		}
		p, err := c.convert(name, src)
		if err != nil {
			return err
		}
		l.own = append(l.own, p)
		en := s.en.pages[name]
		if en == nil {
			continue // check says it translates nothing
		}
		l.pages[name] = p
		start := fromComment(name, en.Blob)
		switch {
		case m == nil:
			p.problems = append(p.problems, fmt.Sprintf("%s doesn't say which English it was made from: start it with %s", p.File, start))
		case said != "docs/"+name+".md":
			p.problems = append(p.problems, fmt.Sprintf("%s says it translates %s: start it with %s", p.File, said, start))
		default:
			p.From = from
			p.Behind = !strings.HasPrefix(en.Blob, from)
		}
	}
	l.Index = l.pages["README"]
	l.Parts = l.Index.parts
	return nil
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
		if e.Name() == "site.js" {
			s.scriptTexts = scriptTexts(b)
		}
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

	// Each language's templates: their links lead to its pages, and they
	// say their words in it.
	s.tmpl = map[*language]map[string]*template.Template{}
	for _, l := range s.langs {
		funcs := template.FuncMap{
			"href": func(path string) string { return s.cfg.Base + l.Path + path },
			"asset": func(name string) (string, error) {
				hash, ok := s.assets[name]
				if !ok {
					return "", fmt.Errorf("no assets/%s", name)
				}
				return s.cfg.Base + "assets/" + name + "?v=" + hash, nil
			},
			"num":   func(n int) string { return fmt.Sprintf("%02d", n) },
			"t":     l.t,
			"tHTML": l.tHTML,
			// snippet is a file of snippets/ as a block of code, under a bar
			// with label, in the language of its extension.
			"snippet": func(name, label string) (template.HTML, error) {
				code, ok := s.snippets[name]
				if !ok {
					return "", fmt.Errorf("no snippets/%s.txt", name)
				}
				lang := strings.TrimPrefix(filepath.Ext(name), ".")
				return template.HTML(codeBlock(label, lang, code, l)), nil
			},
		}
		s.tmpl[l] = map[string]*template.Template{}
		for _, name := range []string{"home", "guide", "doc", "404"} {
			t, err := template.New("").Funcs(funcs).ParseFiles(
				"templates/layout.html", "templates/icons.html", "templates/"+name+".html")
			if err != nil {
				return err
			}
			s.tmpl[l][name] = t
			// What a template may say is said, whether or not a page shows
			// it yet, as the note on a translation that's behind; but the
			// 404 page is English's alone.
			if l == s.en || name != "404" {
				for _, text := range templateTexts(t) {
					l.said[text] = true
				}
			}
		}
	}
	return nil
}

// templateTexts lists what t and the templates it has say through t and
// tHTML: each literal they're handed.
func templateTexts(t *template.Template) []string {
	var texts []string
	var walk func(n parse.Node)
	walk = func(n parse.Node) {
		switch n := n.(type) {
		case *parse.ListNode:
			if n != nil {
				for _, c := range n.Nodes {
					walk(c)
				}
			}
		case *parse.ActionNode:
			walk(n.Pipe)
		case *parse.PipeNode:
			if n != nil {
				for _, c := range n.Cmds {
					walk(c)
				}
			}
		case *parse.CommandNode:
			if len(n.Args) > 1 {
				fn, isFn := n.Args[0].(*parse.IdentifierNode)
				text, isText := n.Args[1].(*parse.StringNode)
				if isFn && isText && (fn.Ident == "t" || fn.Ident == "tHTML") {
					texts = append(texts, text.Text)
				}
			}
			for _, arg := range n.Args {
				walk(arg)
			}
		case *parse.IfNode:
			walk(n.Pipe)
			walk(n.List)
			walk(n.ElseList)
		case *parse.RangeNode:
			walk(n.Pipe)
			walk(n.List)
			walk(n.ElseList)
		case *parse.WithNode:
			walk(n.Pipe)
			walk(n.List)
			walk(n.ElseList)
		case *parse.TemplateNode:
			walk(n.Pipe)
		}
	}
	for _, tt := range t.Templates() {
		if tt.Tree != nil {
			walk(tt.Tree.Root)
		}
	}
	return texts
}

// scriptText is what site.js says: a literal it hands its t, which finds
// its words in the page's language.
var scriptText = regexp.MustCompile(`\bt\((?:'([^'\\]*)'|"([^"\\]*)")`)

// scriptTexts lists what site.js says, sorted.
func scriptTexts(js []byte) []string {
	var texts []string
	for _, m := range scriptText.FindAllSubmatch(js, -1) {
		texts = append(texts, string(m[1])+string(m[2]))
	}
	slices.Sort(texts)
	return slices.Compact(texts)
}

// check finds the guide's links to pages or headings that aren't there,
// and pages its index doesn't list, and what's wrong with its
// translations, and notes those whose English has changed since they were
// made.
func (s *site) check() (problems, notes []string) {
	listed := map[string]bool{"README": true, "roadmap": true, "benchmarks": true}
	for _, p := range s.en.Parts {
		listed[p.Name] = true
	}
	for _, l := range s.langs {
		own := l.own
		if l == s.en {
			own = nil
			for _, name := range s.names {
				own = append(own, s.en.pages[name])
			}
		}
		for _, p := range own {
			problems = append(problems, p.problems...)
			en := s.en.pages[p.Name]
			switch {
			case en == nil:
				problems = append(problems, fmt.Sprintf("%s translates docs/%s.md, which isn't there", p.File, p.Name))
				continue
			case l == s.en && !listed[p.Name]:
				problems = append(problems, fmt.Sprintf("%s isn't one of the parts docs/README.md lists", p.File))
			case p.Behind:
				notes = append(notes, fmt.Sprintf("%s was made from %s at %s, which has changed since: git diff %s %s",
					p.File, en.File, p.From, p.From, en.Blob[:shortID]))
			}
			problems = append(problems, linkProblems(l, p)...)
		}
		if l != s.en && l.translated("README") &&
			!slices.EqualFunc(l.Parts, s.en.Parts, func(a, b part) bool { return a.Name == b.Name }) {
			problems = append(problems, fmt.Sprintf("docs/%s/README.md doesn't list the parts docs/README.md does, in its order", l.Tag))
		}
	}
	return problems, notes
}

// linkProblems finds p's links, in l, to a page or a heading that isn't
// there. A translation links to another translation by its name, and to
// the English where l has none, as GitHub has the files.
func linkProblems(l *language, p *page) []string {
	var problems []string
	for _, ln := range p.links {
		to := l.pages[ln.Page]
		switch {
		case to == nil:
			problems = append(problems, fmt.Sprintf("%s: a link to %s, a page that isn't there", p.File, ln.Dest))
		case ln.Own && to.Lang != l:
			problems = append(problems, fmt.Sprintf("%s: a link to %s, which isn't translated: link the English, ../%s.md, until it is", p.File, ln.Dest, ln.Page))
		case !ln.Own && to.Lang == l:
			problems = append(problems, fmt.Sprintf("%s: a link to %s, the English, which %s translates: link %s.md", p.File, ln.Dest, to.File, ln.Page))
		case ln.ID != "" && !to.ids[ln.ID]:
			problems = append(problems, fmt.Sprintf("%s: a link to %s, a heading that isn't there", p.File, ln.Dest))
		}
	}
	return problems
}

// A view is what a page's template is given.
type view struct {
	*site
	Lang        *language // the language the page is in
	Kind        string    // the template: home, guide, doc or 404
	Title       string
	Description string
	Path        string            // the page's, under its language's path
	Canonical   string            // its address, when the site's is known: the English's, for a page that isn't translated
	Alternates  []alternate       // its address in each language it's in, when it's in more than one, for search engines
	Langs       []alternate       // the page in each of the site's languages, for the menu that picks one
	Words       map[string]string // what site.js says, in the page's language, but English
	Page        *page
	Parts       []part // the guide's, as the index in the page's language names them
	Part        *part  // the page's part of the guide, when it's one
	Prev, Next  *part
	Bench       *bench
	Fallback    bool   // the page isn't translated into its language, so it's the English
	English     string // the page in English, which a translation behind it links to
	Source      string // where the page is edited on GitHub, or, when it's the English, translated
}

// An alternate is a page in one of the site's languages.
type alternate struct {
	Tag, Name, URL string
	Current        bool // it's the page's own language
}

// write writes the site to its directory, emptying it first: each
// language's pages, under its path, and its index for search.
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
		var b bytes.Buffer
		if err := s.tmpl[v.Lang][v.Kind].ExecuteTemplate(&b, "layout", v); err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
		return put(file, b.Bytes())
	}

	for _, l := range s.langs {
		words := s.scriptWords(l)
		at := func(kind, path string, p *page) view {
			v := view{site: s, Lang: l, Kind: kind, Path: path, Page: p, Parts: l.Parts, Words: words}
			s.place(&v)
			return v
		}

		home := at("home", "", nil)
		home.Title = l.t("tug: Go handlers, Inertia pages, no API in between")
		home.Description = l.Index.Description
		home.Bench = benchOf(s.results, l)
		if err := render(l.Path+"index.html", home); err != nil {
			return err
		}

		guide := at("guide", l.Index.Path, l.Index)
		guide.Title = l.Index.Title + " · tug"
		guide.Description = l.Index.Description
		if len(l.Parts) > 0 {
			guide.Next = &l.Parts[0]
		}
		if err := render(l.Path+"docs/index.html", guide); err != nil {
			return err
		}

		index := &part{Title: l.Index.Title, Name: "README", Path: l.Index.Path}
		for _, name := range s.names {
			if name == "README" {
				continue
			}
			p := l.pages[name]
			v := at("doc", p.Path, p)
			v.Title = p.Title + " · tug"
			v.Description = p.Description
			for i := range l.Parts {
				if l.Parts[i].Name != name {
					continue
				}
				v.Part = &l.Parts[i]
				v.Prev = index
				if i > 0 {
					v.Prev = &l.Parts[i-1]
				}
				if i+1 < len(l.Parts) {
					v.Next = &l.Parts[i+1]
				}
			}
			if err := render(l.Path+p.Path+"index.html", v); err != nil {
				return err
			}
		}

		search, err := s.searchIndex(l)
		if err != nil {
			return err
		}
		if err := put(l.Path+"search.json", search); err != nil {
			return err
		}
	}

	// GitHub Pages has one page for a path that isn't there, whatever its
	// language: the English, whose menu leads to each language's home.
	notFound := view{site: s, Lang: s.en, Kind: "404", Title: s.en.t("Not found · tug"), Description: s.en.t("There's no page here."), Parts: s.en.Parts}
	for _, l := range s.langs {
		notFound.Langs = append(notFound.Langs, alternate{Tag: l.Tag, Name: l.Name, URL: s.cfg.Base + l.Path, Current: l == s.en})
	}
	if err := render("404.html", notFound); err != nil {
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

// place says where v's page is: its link in each of the site's languages,
// for the menu, where it's edited, or translated, and, when the site's
// address is known, its canonical address and its alternates. The home
// page is in every language, as the site's words are, and a page of the
// guide in those that have translated it; the English is canonical for
// the rest.
func (s *site) place(v *view) {
	in := func(l *language) bool { return v.Page == nil || l.translated(v.Page.Name) }
	v.Fallback = !in(v.Lang)
	for _, l := range s.langs {
		v.Langs = append(v.Langs, alternate{Tag: l.Tag, Name: l.Name, URL: s.cfg.Base + l.Path + v.Path, Current: l == v.Lang})
	}
	if p := v.Page; p != nil {
		v.English = s.cfg.Base + v.Path
		v.Source = s.Repo + "/edit/main/" + p.File
		if v.Fallback {
			// GitHub's page for a new file, named, and started with the
			// comment that says which English it's made from. A space is
			// %20, not the query's +, which html/template would write as
			// &#43;.
			q := url.Values{"filename": {p.Name + ".md"}, "value": {fromComment(p.Name, p.Blob) + "\n\n"}}
			v.Source = s.Repo + "/new/main/docs/" + v.Lang.Tag + "?" + strings.ReplaceAll(q.Encode(), "+", "%20")
		}
	}
	if s.cfg.URL == "" {
		return
	}
	if v.Fallback {
		v.Canonical = s.cfg.URL + "/" + v.Path
		return
	}
	v.Canonical = s.cfg.URL + "/" + v.Lang.Path + v.Path
	for _, l := range s.langs {
		if in(l) {
			v.Alternates = append(v.Alternates, alternate{Tag: l.Tag, Name: l.Name, URL: s.cfg.URL + "/" + l.Path + v.Path})
		}
	}
	if len(v.Alternates) < 2 {
		v.Alternates = nil
		return
	}
	v.Alternates = append(v.Alternates, alternate{Tag: "x-default", URL: s.cfg.URL + "/" + v.Path})
}

// scriptWords are what site.js says, in l, for its pages to hand it; it
// says English itself.
func (s *site) scriptWords(l *language) map[string]string {
	if l == s.en {
		return nil
	}
	words := map[string]string{}
	for _, text := range s.scriptTexts {
		words[text] = l.t(text)
	}
	return words
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

// searchIndex is what site.js searches in l's pages: each page's title
// and path, under l's, and each section's heading, ID and text, as arrays
// rather than objects, as it's fetched whole. The roadmap's sections are
// their headings and a little of their text, so a search finds the guide
// before the history.
func (s *site) searchIndex(l *language) ([]byte, error) {
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
	add(l.Index, l.Index.Title)
	for _, part := range l.Parts {
		if p := l.pages[part.Name]; p != nil {
			add(p, part.Title)
		}
	}
	for _, name := range s.names {
		if listedPart(l.Parts, name) || name == "README" {
			continue
		}
		add(l.pages[name], l.pages[name].Title)
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

// sitemap lists each language's home page, and the guide's pages it has:
// the English, and each translation.
func (s *site) sitemap() []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	loc := func(path string) {
		fmt.Fprintf(&b, "  <url><loc>%s/%s</loc></url>\n", s.cfg.URL, path)
	}
	for _, l := range s.langs {
		loc(l.Path)
		for _, name := range s.names {
			if l.translated(name) {
				loc(l.Path + l.pages[name].Path)
			}
		}
	}
	b.WriteString("</urlset>\n")
	return b.Bytes()
}
