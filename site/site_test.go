package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHeadingsGetTheIDsGitHubGivesThem(t *testing.T) {
	for heading, want := range map[string]string{
		"`tug dev`":                  "tug-dev",
		"What's in the app":          "whats-in-the-app",
		"M1 · HTTP core — done":      "m1--http-core--done",
		"Rotating `APP_KEY`":         "rotating-app_key",
		"`.env` and the environment": "env-and-the-environment",
		"Numbers past JavaScript's":  "numbers-past-javascripts",
		"Tiếng Việt, đã dịch":        "tiếng-việt-đã-dịch",
	} {
		if got := githubSlug(heading); got != want {
			t.Errorf("githubSlug(%q) = %q, want %q", heading, got, want)
		}
	}
}

func TestAHeadingThatsTakenGetsANumber(t *testing.T) {
	seen := map[string]int{}
	var got []string
	for _, h := range []string{"Notes", "Notes", "Notes-1", "Notes"} {
		got = append(got, uniqueSlug(seen, h))
	}
	want := []string{"notes", "notes-1", "notes-1-1", "notes-2"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("got %v, want %v, as github-slugger has them", got, want)
	}
}

// guide writes a checkout with the given pages of docs/ and a README, for
// load to read.
func guide(t *testing.T, pages map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, text := range pages {
		file := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const index = "# The guide\n\nWhat it is.\n\n" +
	"1. [Getting started](getting-started.md): install tug, and make\n   an app with `tug new`.\n" +
	"2. [Forms](forms.md): validation, with [a link](cli.md) in it.\n\n" +
	"[The roadmap](roadmap.md) has the rest.\n"

func TestLinksBetweenPagesAreLinksBetweenTheSitesPages(t *testing.T) {
	root := guide(t, map[string]string{
		"examples/api/main.go": "package main\n",
		"README.md":            "v0.39.0 is the latest release.\n",
	})
	c := newConverter(root, "/tug/", map[string]bool{"forms": true, "cli": true})
	p, err := c.convert("forms", []byte("# Forms\n\n"+
		"[a](cli.md) [b](cli.md#tug-dev) [c](#here) [d](https://inertiajs.com) "+
		"[e](../examples/api/main.go) [f](../examples/api) [g](../nowhere.go)\n\n## Here\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`href="/tug/docs/cli/"`,
		`href="/tug/docs/cli/#tug-dev"`,
		`href="#here"`,
		`href="https://inertiajs.com"`,
		`href="https://github.com/cuonggt/tug/blob/main/examples/api/main.go"`,
		`href="https://github.com/cuonggt/tug/tree/main/examples/api"`,
	} {
		if !strings.Contains(string(p.Body), want) {
			t.Errorf("the page has no %s:\n%s", want, p.Body)
		}
	}
	if len(p.problems) != 1 || !strings.Contains(p.problems[0], "../nowhere.go, a file that isn't there") {
		t.Errorf("problems = %q, want the link to a file that isn't there", p.problems)
	}
}

func TestTheIndexListsTheGuidesPartsInItsOrder(t *testing.T) {
	c := newConverter(".", "/", map[string]bool{"README": true, "getting-started": true, "forms": true, "cli": true, "roadmap": true})
	p, err := c.convert("README", []byte(index))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.parts) != 2 {
		t.Fatalf("parts = %+v, want two", p.parts)
	}
	first, second := p.parts[0], p.parts[1]
	if first.Num != 1 || first.Title != "Getting started" || first.Path != "docs/getting-started/" {
		t.Errorf("the first part is %+v", first)
	}
	if got := string(first.About); got != "install tug, and make\nan app with <code>tug new</code>." {
		t.Errorf("the first part's about is %q, without the colon and with its code", got)
	}
	// A part's card is a link, which a link can't be inside.
	if strings.Contains(string(second.About), "<a ") || !strings.Contains(string(second.About), "a link") {
		t.Errorf("the second part's about is %q, its link's text without the link", second.About)
	}
	if strings.Contains(string(p.Body), "Getting started") || !strings.Contains(string(p.After), "/docs/roadmap/") {
		t.Errorf("the list isn't between the index's body and what's after it:\n%s\n---\n%s", p.Body, p.After)
	}
}

func TestALinkToAHeadingThatIsntThereIsAProblem(t *testing.T) {
	root := guide(t, map[string]string{
		"README.md":               "v0.39.0 is the latest release.\n",
		"docs/README.md":          index,
		"docs/getting-started.md": "# Getting started\n\nSee [the CLI](cli.md#tug-dev) and [forms](forms.md#nope).\n",
		"docs/forms.md":           "# Forms\n\n## Validation\n",
		"docs/cli.md":             "# The CLI\n\n## `tug dev`\n",
		"docs/roadmap.md":         "# Roadmap\n",
		"docs/stray.md":           "# Stray\n",
	})
	s, err := load(config{Root: root, Base: "/"})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(s.check(), "\n")
	for _, want := range []string{
		"docs/getting-started.md: a link to forms.md#nope, a heading that isn't there",
		"docs/stray.md isn't one of the parts docs/README.md lists",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("problems:\n%s\nwant %q", got, want)
		}
	}
	if strings.Contains(got, "cli.md#tug-dev") {
		t.Errorf("a link to a heading that's there is a problem:\n%s", got)
	}
}

func TestEveryLinkInTheGuideLeadsSomewhere(t *testing.T) {
	s, err := load(config{Root: "..", Base: "/"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range s.check() {
		t.Error(p)
	}
	if len(s.Parts) == 0 || s.Version == "" {
		t.Errorf("the guide has %d parts and the version %q", len(s.Parts), s.Version)
	}
}

func TestCodeIsColoredAndEscaped(t *testing.T) {
	got := codeBlock("Go", "go", "if a < b {\n\treturn \"<b>\"\n}\n")
	for _, want := range []string{`<span class="k">if</span>`, `&lt;`, `&#34;&lt;b&gt;&#34;`, `<span class="code-label">Go</span>`} {
		if !strings.Contains(got, want) {
			t.Errorf("no %s in\n%s", want, got)
		}
	}
	if plain := codeBlock("", "", "<script>"); !strings.Contains(plain, "&lt;script&gt;") {
		t.Errorf("code in no language isn't escaped:\n%s", plain)
	}
}

func TestTheSiteIsWrittenAndFindable(t *testing.T) {
	out := t.TempDir()
	problems, err := build(config{Root: "..", Out: out, Base: "/tug/", URL: "https://cuonggt.github.io/tug"})
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) > 0 {
		t.Errorf("problems: %q", problems)
	}
	for _, file := range []string{"index.html", "docs/index.html", "docs/forms/index.html", "docs/roadmap/index.html", "404.html", "sitemap.xml", "robots.txt", "assets/site.css"} {
		if _, err := os.Stat(filepath.Join(out, file)); err != nil {
			t.Errorf("no %s: %v", file, err)
		}
	}

	page, _ := os.ReadFile(filepath.Join(out, "docs/forms/index.html"))
	for _, want := range []string{
		`<link rel="canonical" href="https://cuonggt.github.io/tug/docs/forms/">`,
		`href="/tug/docs/forms/" aria-current="page"`,
		`<title>Forms and sessions · tug</title>`,
	} {
		if !strings.Contains(string(page), want) {
			t.Errorf("the forms page has no %s", want)
		}
	}

	var idx struct {
		Pages    [][2]string `json:"pages"`
		Sections [][4]any    `json:"sections"`
	}
	b, _ := os.ReadFile(filepath.Join(out, "search.json"))
	if err := json.Unmarshal(b, &idx); err != nil {
		t.Fatal(err)
	}
	if idx.Pages[0][1] != "docs/" || idx.Pages[1][1] != "docs/getting-started/" {
		t.Errorf("search's pages start %q, want the index, then the guide's first part", idx.Pages[:2])
	}
}

func TestAnotherDirectoryIsntEmptiedForTheSite(t *testing.T) {
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "notes.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := build(config{Root: "..", Out: out, Base: "/"}); err == nil {
		t.Fatal("the site was written over a directory of someone else's files")
	}
	if _, err := os.Stat(filepath.Join(out, "notes.txt")); err != nil {
		t.Errorf("the directory's file is gone: %v", err)
	}
}

func TestTheHomePageShowsTheBenchmarksResults(t *testing.T) {
	pages := map[string]string{
		"README.md":               "v0.39.0 is the latest release.\n",
		"docs/README.md":          index,
		"docs/getting-started.md": "# Getting started\n",
		"docs/forms.md":           "# Forms\n",
		"docs/cli.md":             "# The CLI\n",
		"docs/roadmap.md":         "# Roadmap\n",
	}
	without := guide(t, pages)
	pages["bench/results.json"] = `{
		"go": {
			"router": [{"name": "tug", "ns_op": 233.1}, {"name": "Gin", "ns_op": 93.2}],
			"visit": [{"name": "tug, App", "ns_op": 8300}, {"name": "gonertia, ServeMux", "ns_op": 10100}]
		},
		"http": {"date": "2026-10-05", "machine": {"cpu": "Apple M1 Max", "cores": 10, "os": "macOS 27.0"}, "apps": [
			{"name": "tug", "language": "Go", "visit": {"per_second": 60000, "p99_ms": 2.1}, "first_visit": {"per_second": 50000}},
			{"name": "Rails", "language": "Ruby", "visit": {"per_second": 3000, "p99_ms": 48}, "first_visit": {"per_second": 2500}},
			{"name": "Laravel", "language": "PHP", "visit": {"per_second": 7500, "p99_ms": 21}, "first_visit": {"per_second": 6000}}
		]}
	}`
	with := guide(t, pages)

	home := func(root string) string {
		t.Helper()
		out := t.TempDir()
		if _, err := build(config{Root: root, Out: out, Base: "/"}); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(out, "index.html"))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	got := home(with)
	for _, want := range []string{
		// The headline, from the fastest of the others and the slowest.
		"The same page, 8.0 to 20 times the requests a second.",
		"tug answers 8.0 times as many visits a second as Laravel, the fastest of the others, and 20 times as many as Rails.",
		"served by tug and by Laravel and Rails,",
		// tug's bar is the longest, and the others are their share of it.
		`<tr class="is-tug">`, `style="--w: 100.0%"`, `style="--w: 12.5%"`, "60,000",
		// The adapters, by time, tug's App with its note, and the routers.
		`<span class="bar-name">tug</span><span class="bar-note">App</span>`, "8.3 µs", "10.1 µs",
		"What a router adds", "93 ns", "233 ns",
		`Measured <time datetime="2026-10-05">2026-10-05</time> on Apple M1 Max, 10 cores, macOS 27.0.`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the home page has no %s", want)
		}
	}
	if strings.Index(got, "Laravel</span>") > strings.Index(got, "Rails</span>") {
		t.Error("the apps aren't in the order of their visits a second")
	}
	if strings.Index(got, "93 ns") > strings.Index(got, "233 ns") {
		t.Error("the routers aren't the fastest first")
	}
	if got := home(without); strings.Contains(got, "bench-title") {
		t.Error("a checkout with no results has a section of them")
	}

	// Why Go is there either way, its table's Go row marked, and the
	// sections' backgrounds still alternate.
	for _, got := range []string{got, home(without)} {
		if !strings.Contains(got, `<h2 id="why-title">`) || !strings.Contains(got, `<tr class="is-go"><th scope="row">Go <span>net/http, Gin</span></th>`) {
			t.Error("the home page has no Why Go, or no Go row in its table")
		}
	}
	if !strings.Contains(got, `<section class="section" aria-labelledby="guide-title">`) ||
		!strings.Contains(home(without), `<section class="section section-alt" aria-labelledby="guide-title">`) {
		t.Error("the guide's section doesn't take the background the section before it leaves")
	}
}
