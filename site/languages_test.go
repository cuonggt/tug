package main

import (
	"html/template"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// english is English, the guide's own language, as a page is written in.
func english() *language {
	return siteLanguages()[0]
}

// read is a file of a site written to out.
func read(t *testing.T, out, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestABlobsIDIsGits(t *testing.T) {
	for content, want := range map[string]string{
		"":        "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391",
		"hello\n": "ce013625030ba8dba906f756967f9e9ca394464a",
		// A checkout with CRLF line endings has git's LF ones in its blob.
		"hello\r\n": "ce013625030ba8dba906f756967f9e9ca394464a",
	} {
		if got := blobID([]byte(content)); got != want {
			t.Errorf("blobID(%q) = %s, want %s, as git hash-object has it", content, got, want)
		}
	}
}

func TestAPageLinksToPagesOfItsOwnLanguage(t *testing.T) {
	for _, c := range []struct{ from, to, want string }{
		{"docs/", "docs/forms/", "forms/"},
		{"docs/forms/", "docs/cli/", "../cli/"},
		{"docs/forms/", "docs/", "../"},
		{"docs/", "docs/", "./"},
		{"docs/forms/", "docs/forms/", "../forms/"},
	} {
		if got := relative(c.from, c.to); got != c.want {
			t.Errorf("relative(%q, %q) = %q, want %q", c.from, c.to, got, c.want)
		}
	}
}

func TestNumbersAreWrittenAsTheLanguageWritesThem(t *testing.T) {
	langs := siteLanguages()
	byTag := func(tag string) *language {
		i := slices.IndexFunc(langs, func(l *language) bool { return l.Tag == tag })
		return langs[i]
	}
	for _, c := range []struct {
		tag                 string
		visits, memory, p99 string
	}{
		{"en", "60,000", "19.9 MB", "4.38 ms"},
		{"vi", "60.000", "19,9 MB", "4,38 ms"},
		{"es", "60.000", "19,9 MB", "4,38 ms"},
		{"ja", "60,000", "19.9 MB", "4.38 ms"},
		{"zh-CN", "60,000", "19.9 MB", "4.38 ms"},
	} {
		l := byTag(c.tag)
		if got := l.thousands(60000); got != c.visits {
			t.Errorf("%s writes 60000 visits as %q, want %q", c.tag, got, c.visits)
		}
		if got := l.megabytes(19.89); got != c.memory {
			t.Errorf("%s writes 19.89 MiB as %q, want %q", c.tag, got, c.memory)
		}
		if got := l.millis(4.38); got != c.p99 {
			t.Errorf("%s writes 4.38 ms as %q, want %q", c.tag, got, c.p99)
		}
	}
}

func TestEveryTextATemplateMaySayIsSaid(t *testing.T) {
	l := english()
	tmpl := template.Must(template.New("").Funcs(template.FuncMap{"t": l.t, "tHTML": l.tHTML}).Parse(
		`{{t "Shown"}}{{if .X}}{{t "Only if"}}{{else}}{{tHTML "Or <b>else</b>"}}{{end}}` +
			`{{define "other"}}{{range .}}{{with .}}{{t "In a range, :n" "n" (t "Nested")}}{{end}}{{end}}{{end}}`))
	got := templateTexts(tmpl)
	slices.Sort(got)
	want := []string{"In a range, :n", "Nested", "Only if", "Or <b>else</b>", "Shown"}
	if !slices.Equal(got, want) {
		t.Errorf("the texts are %q, want %q, whether or not a page shows them", got, want)
	}
}

func TestALanguagesFileHasWordsForWhatTheSiteSaysInIt(t *testing.T) {
	en, vi := english(), &language{Tag: "vi", said: map[string]bool{}}
	en.said["Only English"] = true
	vi.said["Search"], vi.said["Copy"] = true, true
	vi.texts = map[string]string{"Search": "Tìm kiếm", "Gone": "Đã bỏ"}
	problems, notes := wordProblems([]*language{en, vi})
	if want := []string{`site/lang/vi.json has no words for "Copy"`}; !slices.Equal(problems, want) {
		t.Errorf("problems = %q, want %q", problems, want)
	}
	if want := []string{`site/lang/vi.json has words for "Gone", which the site doesn't say`}; !slices.Equal(notes, want) {
		t.Errorf("notes = %q, want %q", notes, want)
	}
}

func TestTextsWithMarkupEscapeTheirValues(t *testing.T) {
	l := english()
	got := l.tHTML("<code>:cmd</code> at :when", "cmd", "<script>", "when", template.HTML(`<time>now</time>`))
	if want := template.HTML("<code>&lt;script&gt;</code> at <time>now</time>"); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestSiteJSsWordsAreFoundInIt(t *testing.T) {
	texts := scriptTexts([]byte(`t('Pages') + t("Couldn't copy") + t(':count results', { count: n }) + split('x') + at('y')`))
	if want := []string{":count results", "Couldn't copy", "Pages"}; !slices.Equal(texts, want) {
		t.Errorf("site.js says %q, want %q", texts, want)
	}
}

// threeParts is an index of a guide of three parts, each a page.
const threeParts = "# The guide\n\nWhat it is.\n\n" +
	"1. [Getting started](getting-started.md): install tug.\n" +
	"2. [Forms](forms.md): validation.\n" +
	"3. [The CLI](cli.md): its commands.\n\n" +
	"[The roadmap](roadmap.md) has the rest.\n"

// translatedGuide writes a checkout whose guide has three parts and the
// roadmap in English, and a Vietnamese translation of its first part,
// which starts with start, and says text.
func translatedGuide(t *testing.T, start func(english []byte) string, text string) string {
	t.Helper()
	first := "# Getting started\n\nMake an app.\n\n## Install\n\nWith go install.\n"
	return guide(t, map[string]string{
		"README.md":                  "v0.39.0 is the latest release.\n",
		"docs/README.md":             threeParts,
		"docs/getting-started.md":    first,
		"docs/forms.md":              "# Forms\n\nValidation.\n\n## Validation\n\nTags.\n",
		"docs/cli.md":                "# The CLI\n\n## `tug dev`\n",
		"docs/roadmap.md":            "# Roadmap\n",
		"docs/vi/getting-started.md": start([]byte(first)) + text,
	})
}

// madeFrom starts a translation with the comment that names the English as
// it is.
func madeFrom(name string) func([]byte) string {
	return func(english []byte) string { return fromComment(name, blobID(english)) + "\n\n" }
}

const startedInVietnamese = "# Bắt đầu\n\nTạo một ứng dụng.\n\n## Cài đặt\n\n" +
	"Xem [biểu mẫu](../forms.md#validation), [lệnh tug dev](../cli.md#tug-dev) và [cài đặt](#cài-đặt).\n"

func TestATranslationTakesItsEnglishsPlace(t *testing.T) {
	root := translatedGuide(t, madeFrom("getting-started"), startedInVietnamese)
	out := t.TempDir()
	problems, notes, err := build(config{Root: root, Out: out, Base: "/tug/", URL: "https://example.com/tug"})
	if err != nil {
		t.Fatal(err)
	}
	// The site's own words are the real ones, which their own test checks.
	for _, p := range problems {
		if !strings.HasPrefix(p, "site/lang/") {
			t.Errorf("problem: %s", p)
		}
	}
	for _, n := range notes {
		if !strings.HasPrefix(n, "site/lang/") {
			t.Errorf("note: %s", n)
		}
	}

	page := read(t, out, "vi/docs/getting-started/index.html")
	for _, want := range []string{
		`<html lang="vi" data-base="/tug/vi/">`,
		`<h1>Bắt đầu</h1>`,
		`<h2 id="cài-đặt">Cài đặt</h2>`,
		`href="../forms/#validation"`,
		`href="../cli/#tug-dev"`,
		`<link rel="canonical" href="https://example.com/tug/vi/docs/getting-started/">`,
		`<link rel="alternate" hreflang="en" href="https://example.com/tug/docs/getting-started/">`,
		`<link rel="alternate" hreflang="vi" href="https://example.com/tug/vi/docs/getting-started/">`,
		`<link rel="alternate" hreflang="x-default" href="https://example.com/tug/docs/getting-started/">`,
		`href="https://github.com/cuonggt/tug/edit/main/docs/vi/getting-started.md"`,
		`<script type="application/json" id="words">`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the translation's page has no %s", want)
		}
	}
	if strings.Contains(page, "translated from") || strings.Contains(page, `class="note"`) {
		t.Error("the translation's page shows its comment, or a note")
	}
	if english := read(t, out, "docs/getting-started/index.html"); !strings.Contains(english,
		`<link rel="alternate" hreflang="vi" href="https://example.com/tug/vi/docs/getting-started/">`) {
		t.Error("the English page doesn't name its translation")
	}

	// A page that isn't translated is the English, which says so, and whose
	// English is canonical.
	forms := read(t, out, "vi/docs/forms/index.html")
	for _, want := range []string{
		`<html lang="vi"`,
		`<h1 lang="en">Forms</h1>`,
		`<div class="prose" lang="en">`,
		`<p class="note">`,
		`href="https://github.com/cuonggt/tug/new/main/docs/vi?filename=forms.md&amp;value=%3C%21--%20translated%20from%20docs%2Fforms.md%20at%20`,
		`<link rel="canonical" href="https://example.com/tug/docs/forms/">`,
	} {
		if !strings.Contains(forms, want) {
			t.Errorf("the page that isn't translated has no %s", want)
		}
	}
	if strings.Contains(forms, `rel="alternate"`) {
		t.Error("the page that isn't translated names alternates of its own")
	}
	// Its menu leads to the page in each language.
	for _, want := range []string{
		`<a href="/tug/docs/forms/" hreflang="en" lang="en">English</a>`,
		`<a href="/tug/vi/docs/forms/" hreflang="vi" lang="vi" aria-current="page">Tiếng Việt</a>`,
		`<a href="/tug/zh-cn/docs/forms/" hreflang="zh-CN" lang="zh-CN">简体中文</a>`,
	} {
		if !strings.Contains(forms, want) {
			t.Errorf("the menu of languages has no %s", want)
		}
	}

	if english := read(t, out, "docs/forms/index.html"); strings.Contains(english, `id="words"`) || !strings.Contains(english, `<html lang="en"`) {
		t.Error("an English page hands site.js words, or isn't in English")
	}
	sitemap := read(t, out, "sitemap.xml")
	for _, want := range []string{"https://example.com/tug/vi/</loc>", "https://example.com/tug/vi/docs/getting-started/</loc>"} {
		if !strings.Contains(sitemap, want) {
			t.Errorf("the sitemap has no %s", want)
		}
	}
	if strings.Contains(sitemap, "/vi/docs/forms/") {
		t.Error("the sitemap lists a page that isn't translated, as the English is")
	}
	if search := read(t, out, "vi/search.json"); !strings.Contains(search, `"Bắt đầu","docs/getting-started/"`) &&
		!strings.Contains(search, `"Getting started","docs/getting-started/"`) {
		t.Errorf("the Vietnamese search has no first part:\n%s", search)
	}

	// The one page for what isn't there is English, and its menu leads home.
	lost := read(t, out, "404.html")
	if !strings.Contains(lost, `<html lang="en"`) || !strings.Contains(lost, `<a href="/tug/ja/" hreflang="ja" lang="ja">日本語</a>`) {
		t.Error("the 404 page isn't English, or its menu doesn't lead to each language's home")
	}
}

func TestATranslationSaysWhichEnglishItWasMadeFrom(t *testing.T) {
	check := func(start func([]byte) string) ([]string, []string, *site) {
		t.Helper()
		s, err := load(config{Root: translatedGuide(t, start, startedInVietnamese), Base: "/"})
		if err != nil {
			t.Fatal(err)
		}
		problems, notes := s.check()
		return problems, notes, s
	}
	english := blobID([]byte("# Getting started\n\nMake an app.\n\n## Install\n\nWith go install.\n"))[:shortID]

	problems, _, _ := check(func([]byte) string { return "" })
	if want := "docs/vi/getting-started.md doesn't say which English it was made from: start it with <!-- translated from docs/getting-started.md at " + english + " -->"; !slices.Contains(problems, want) {
		t.Errorf("problems = %q, want %q", problems, want)
	}
	problems, _, _ = check(func([]byte) string { return "<!-- translated from docs/forms.md at " + english + " -->\n" })
	if !slices.ContainsFunc(problems, func(p string) bool {
		return strings.HasPrefix(p, "docs/vi/getting-started.md says it translates docs/forms.md")
	}) {
		t.Errorf("problems = %q, want one of the file it names", problems)
	}

	// The English has changed since: the page says so, and the check notes
	// what changed.
	problems, notes, s := check(func([]byte) string { return "<!-- translated from docs/getting-started.md at 0123456789ab -->\n\n" })
	if len(problems) > 0 {
		t.Errorf("problems: %q", problems)
	}
	if want := "docs/vi/getting-started.md was made from docs/getting-started.md at 0123456789ab, which has changed since: git diff 0123456789ab " + english; !slices.Contains(notes, want) {
		t.Errorf("notes = %q, want %q", notes, want)
	}
	vi := s.langs[slices.IndexFunc(s.langs, func(l *language) bool { return l.Tag == "vi" })]
	if !vi.pages["getting-started"].Behind {
		t.Error("the translation isn't behind its English")
	}
	out := t.TempDir()
	s.cfg.Out = out
	if err := s.write(); err != nil {
		t.Fatal(err)
	}
	if page := read(t, out, "vi/docs/getting-started/index.html"); !strings.Contains(page, `<p class="note">`) || !strings.Contains(page, `href="/docs/getting-started/" hreflang="en"`) {
		t.Error("the translation that's behind doesn't say so, with a link to the English")
	}
}

func TestATranslationsLinksLeadWhereGitHubsDo(t *testing.T) {
	root := translatedGuide(t, madeFrom("getting-started"), startedInVietnamese+
		"\n[a](forms.md) [b](../cli.md#nope) [c](#nope) [d](../../examples/nowhere.go)\n")
	if err := os.WriteFile(filepath.Join(root, "docs", "vi", "README.md"), []byte(
		fromComment("README", blobID([]byte(threeParts)))+"\n\n# Hướng dẫn\n\nNó là gì.\n\n"+
			"1. [Bắt đầu](../getting-started.md): cài tug.\n2. [Biểu mẫu](../forms.md): kiểm tra.\n3. [CLI](../cli.md): các lệnh.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := load(config{Root: root, Base: "/"})
	if err != nil {
		t.Fatal(err)
	}
	problems, _ := s.check()
	got := strings.Join(problems, "\n")
	for _, want := range []string{
		"docs/vi/getting-started.md: a link to forms.md, which isn't translated: link the English, ../forms.md, until it is",
		"docs/vi/getting-started.md: a link to ../cli.md#nope, a heading that isn't there",
		"docs/vi/getting-started.md: a link to #nope, a heading that isn't there",
		"docs/vi/getting-started.md: a link to ../../examples/nowhere.go, a file that isn't there",
		"docs/vi/README.md: a link to ../getting-started.md, the English, which docs/vi/getting-started.md translates: link getting-started.md",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("problems:\n%s\nwant %q", got, want)
		}
	}
	if strings.Contains(got, "forms.md#validation") || strings.Contains(got, "#cài-đặt") || strings.Contains(got, "list the parts") {
		t.Errorf("a link that leads somewhere, or an index with the English's parts, is a problem:\n%s", got)
	}

	// An index whose parts aren't the English's, in its order, is a problem.
	if err := os.WriteFile(filepath.Join(root, "docs", "vi", "README.md"), []byte(
		fromComment("README", blobID([]byte(threeParts)))+"\n\n# Hướng dẫn\n\nNó là gì.\n\n"+
			"1. [Biểu mẫu](../forms.md): kiểm tra.\n2. [Bắt đầu](getting-started.md)：cài tug.\n3. [CLI](../cli.md): các lệnh.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if s, err = load(config{Root: root, Base: "/"}); err != nil {
		t.Fatal(err)
	}
	problems, _ = s.check()
	if !slices.Contains(problems, "docs/vi/README.md doesn't list the parts docs/README.md does, in its order") {
		t.Errorf("problems = %q, want the parts out of order", problems)
	}
	vi := s.langs[slices.IndexFunc(s.langs, func(l *language) bool { return l.Tag == "vi" })]
	if about := string(vi.Parts[1].About); about != "cài tug." {
		t.Errorf("a part's about after a full-width colon is %q", about)
	}
}

func TestAPageInEnglishIsTheEnglishInEveryLanguage(t *testing.T) {
	root := translatedGuide(t, madeFrom("getting-started"), startedInVietnamese)
	out := t.TempDir()
	if _, _, err := build(config{Root: root, Out: out, Base: "/"}); err != nil {
		t.Fatal(err)
	}
	// The English's body, its links relative, is the same page in a
	// language that hasn't translated it.
	en, ja := read(t, out, "docs/forms/index.html"), read(t, out, "ja/docs/forms/index.html")
	body := func(page string) string {
		_, after, _ := strings.Cut(page, `<div class="prose"`)
		before, _, _ := strings.Cut(after, "</div>\n      <footer")
		_, body, _ := strings.Cut(before, ">")
		return body
	}
	if body(en) == "" || body(en) != body(ja) {
		t.Errorf("the English's body differs in Japanese:\n%s\n---\n%s", body(en), body(ja))
	}
}

func TestEachLanguageSaysTheHomePagesNumbersItsOwnWay(t *testing.T) {
	root := guide(t, map[string]string{
		"README.md":               "v0.39.0 is the latest release.\n",
		"docs/README.md":          threeParts,
		"docs/getting-started.md": "# Getting started\n",
		"docs/forms.md":           "# Forms\n",
		"docs/cli.md":             "# The CLI\n",
		"docs/roadmap.md":         "# Roadmap\n",
		"bench/results.json": `{"http": {"date": "2026-10-05", "machine": {"cpu": "Apple M1 Max", "cores": 10, "os": "macOS 27.0"}, "apps": [
			{"name": "tug", "visit": {"per_second": 60000, "p99_ms": 4.38}, "first_visit": {"per_second": 56000, "p99_ms": 5.09}}]}}`,
	})
	out := t.TempDir()
	if _, _, err := build(config{Root: root, Out: out, Base: "/"}); err != nil {
		t.Fatal(err)
	}
	if home := read(t, out, "vi/index.html"); !strings.Contains(home, `<span class="stat-value">60.000</span>`) || !strings.Contains(home, "4,38 ms") {
		t.Error("the Vietnamese home page writes its numbers as English does")
	}
	if home := read(t, out, "index.html"); !strings.Contains(home, `<span class="stat-value">60,000</span>`) {
		t.Error("the English home page writes its numbers as Vietnamese does")
	}
}
