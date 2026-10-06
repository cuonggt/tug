package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/cuonggt/tug/lang"
)

// A language is one the site is in. English is the guide's own, at the
// site's root. Another's pages are under a path of its own, saying the
// site's words from lang/<tag>.json, through tug's own package lang, as
// an app says its words, and each page of the guide translated into it
// in docs/<tag>/ takes the English's place; the rest are the English.
type language struct {
	Tag  string // as BCP 47 writes it, as zh-CN: <html lang>'s, its file in lang/, and its directory of docs/
	Name string // in itself, for the menu that picks one
	Path string // where its pages are, under the site's base: "" for English, else its tag in lower case, as "zh-cn/"

	group, point string // what its numbers have between their thousands, and before a fraction

	words lang.Words
	texts map[string]string // its file's: the English, and its words, which check looks for what the site said in
	said  map[string]bool   // what the site said in it, as its pages were written

	pages map[string]*page // the guide in it, by name: its translation, or else the English
	own   []*page          // its translations, as docs/<tag>/ has them
	Index *page            // the guide's index, in it or the English
	Parts []part           // the guide's parts, as that index names them
}

// siteLanguages are the languages the site is in, English first, then as
// the menu lists them. Another needs its words in lang/ and its line here.
func siteLanguages() []*language {
	langs := []*language{
		{Tag: "en", Name: "English", group: ",", point: "."},
		{Tag: "es", Name: "Español", group: ".", point: ","},
		{Tag: "ja", Name: "日本語", group: ",", point: "."},
		{Tag: "vi", Name: "Tiếng Việt", group: ".", point: ","},
		{Tag: "zh-CN", Name: "简体中文", group: ",", point: "."},
	}
	for _, l := range langs {
		if l.Tag != "en" {
			l.Path = strings.ToLower(l.Tag) + "/"
		}
		l.said = map[string]bool{}
		l.pages = map[string]*page{}
	}
	return langs
}

// loadLanguages reads the site's words, from a file of dir for each of its
// languages but English, whose words are their own.
func loadLanguages(dir string) ([]*language, error) {
	catalog, err := lang.Load(os.DirFS(dir), "en")
	if err != nil {
		return nil, err
	}
	langs := siteLanguages()
	files := catalog.Languages()[1:]
	for _, tag := range files {
		if !slices.ContainsFunc(langs, func(l *language) bool { return l.Tag == tag }) {
			return nil, fmt.Errorf("%s/%s.json is in a language the site isn't: add it to siteLanguages, in languages.go", dir, tag)
		}
	}
	for _, l := range langs {
		l.words = catalog.In(l.Tag)
		if l.Tag == "en" {
			continue
		}
		if !slices.Contains(files, l.Tag) {
			return nil, fmt.Errorf("no %s/%s.json, the site's words in %s", dir, l.Tag, l.Name)
		}
		b, err := os.ReadFile(filepath.Join(dir, l.Tag+".json"))
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(b, &l.texts); err != nil {
			return nil, fmt.Errorf("%s/%s.json: %w", dir, l.Tag, err)
		}
	}
	return langs, nil
}

// Short is the language's tag without its region, in capitals, as the
// menu's button shows the language the page is in.
func (l *language) Short() string {
	short, _, _ := strings.Cut(l.Tag, "-")
	return strings.ToUpper(short)
}

// t says text in l, its :name placeholders filled from args, which come
// in pairs, as lang's Words.T takes them, and keeps it among what the site
// said in l, which check looks for in l's file.
func (l *language) t(text string, args ...any) string {
	l.said[text] = true
	return l.words.T(text, args...)
}

// tHTML is t for a text with markup in it, as <code>: the text is the
// site's own, and the values that fill it are escaped, but for those that
// are HTML already.
func (l *language) tHTML(text string, args ...any) template.HTML {
	filled := slices.Clone(args)
	for i := 1; i < len(filled); i += 2 {
		if h, ok := filled[i].(template.HTML); ok {
			filled[i] = string(h)
		} else {
			filled[i] = html.EscapeString(fmt.Sprint(filled[i]))
		}
	}
	return template.HTML(l.t(text, filled...))
}

// thousands is n rounded, with the language's separator between its
// thousands.
func (l *language) thousands(n float64) string {
	s := strconv.FormatInt(int64(n+0.5), 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + l.group + s[i:]
	}
	return s
}

// decimal is n to so many digits after the language's point.
func (l *language) decimal(n float64, digits int) string {
	return strings.Replace(strconv.FormatFloat(n, 'f', digits, 64), ".", l.point, 1)
}

// translated says whether the guide's page of that name is in l, rather
// than the English in its place.
func (l *language) translated(name string) bool {
	p := l.pages[name]
	return p != nil && p.Lang == l
}

// wordProblems finds what the site said in a language that its file has
// no words for, which would be said in English in its pages, and notes
// the words a file has that the site no longer says.
func wordProblems(langs []*language) (problems, notes []string) {
	for _, l := range langs {
		if l.Tag == "en" {
			continue
		}
		for _, text := range slices.Sorted(maps.Keys(l.said)) {
			if _, ok := l.texts[text]; !ok {
				problems = append(problems, fmt.Sprintf("site/lang/%s.json has no words for %q", l.Tag, text))
			}
		}
		for _, text := range slices.Sorted(maps.Keys(l.texts)) {
			if !l.said[text] {
				notes = append(notes, fmt.Sprintf("site/lang/%s.json has words for %q, which the site doesn't say", l.Tag, text))
			}
		}
	}
	return problems, notes
}

// blobID is git's ID of a file's content, its blob's, by which a
// translation names the English it was made from, and which git takes:
// git diff <then> <now> is what the English has changed since. A checkout
// with CRLF line endings has the blob's LF ones, as git keeps them.
func blobID(content []byte) string {
	content = bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(content))
	h.Write(content)
	return hex.EncodeToString(h.Sum(nil))
}

// shortID is as much of a blob's ID as a translation says, and a note
// shows: enough that git takes it as that blob's alone.
const shortID = 12

// translatedFrom is the comment a translation starts with, which names
// the English it was made from, by its file and its blob's ID: GitHub
// shows no comment, and the site takes it out.
var translatedFrom = regexp.MustCompile(`\A\s*<!--\s*translated from (\S+) at ([0-9a-f]{7,40})\s*-->[ \t]*\n?`)

// fromComment is the comment that starts a translation of the English
// page named name, whose blob is blob.
func fromComment(name, blob string) string {
	return fmt.Sprintf("<!-- translated from docs/%s.md at %s -->", name, blob[:shortID])
}
