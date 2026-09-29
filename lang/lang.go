// Package lang says an app's texts in its languages: what tug says to a
// person, as a form's errors, and what the app does, as its flash
// messages and mail. It has no import of tug.
//
// A language is a JSON file of texts, each under what it says in English,
// as Laravel's lang/vi.json is:
//
//	{
//	  ":field is required": "Vui lòng nhập :field",
//	  "Post created": "Đã tạo bài viết"
//	}
//
// The English is the key, and stays in the code where it's said, so a text
// a language doesn't have says itself, in English. Load reads a directory
// of them, which the app embeds, into a Catalog:
//
//	//go:embed all:lang
//	var langFiles embed.FS
//
//	words, err := fs.Sub(langFiles, "lang")
//	catalog, err := lang.Load(words, "en")
//
// Texts are said in one language, with Catalog.In, their :name placeholders
// filled by name: catalog.In("vi").T("Welcome, :name", "name", user.Name).
// A text with forms for a count, split by |, is said with Choice, which
// picks the form the language's rules have for the count, as Laravel's
// trans_choice does: ":count post|:count posts".
package lang

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Catalog is an app's languages: its default, and those it has a file
// for, each with its texts. A nil Catalog speaks English alone.
type Catalog struct {
	def   string                       // the default language, as a tag, as "en" or "pt-BR"
	langs []string                     // the default, then the rest, sorted
	texts map[string]map[string]string // by the lower case tag, the English and the language's
}

// Load reads the languages in fsys: a file each, named for its language as
// vi.json or pt-BR.json, of texts under their English. def is the app's
// default language, which a request gets when neither the app nor the
// browser picks another the catalog has. It has a file, unless it's
// English, whose texts are their own keys. Files that aren't JSON, as a
// .gitkeep, are passed over, and a nil fsys has none.
func Load(fsys fs.FS, def string) (*Catalog, error) {
	c := &Catalog{def: canonical(def), texts: map[string]map[string]string{}}
	if !validTag(c.def) {
		return nil, fmt.Errorf("lang: the default language is %q, which isn't one, such as en or vi", def)
	}
	var entries []fs.DirEntry
	if fsys != nil {
		var err error
		if entries, err = fs.ReadDir(fsys, "."); err != nil {
			return nil, fmt.Errorf("lang: %w", err)
		}
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") || path.Ext(name) != ".json" {
			continue
		}
		tag := canonical(strings.TrimSuffix(name, ".json"))
		if !validTag(tag) {
			return nil, fmt.Errorf("lang: %s isn't named for a language, as vi.json or pt-BR.json are", name)
		}
		if _, ok := c.texts[strings.ToLower(tag)]; ok {
			return nil, fmt.Errorf("lang: two files are for %s", tag)
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("lang: %w", err)
		}
		var texts map[string]string
		if err := json.Unmarshal(data, &texts); err != nil {
			return nil, fmt.Errorf("lang: %s isn't an object of texts, each a string: %w", name, err)
		}
		c.texts[strings.ToLower(tag)] = texts
		c.langs = append(c.langs, tag)
	}
	if _, ok := c.texts[strings.ToLower(c.def)]; !ok && base(c.def) != "en" {
		return nil, fmt.Errorf("lang: the default language is %s, and there's no %s.json", c.def, c.def)
	}
	c.langs = slices.DeleteFunc(c.langs, func(l string) bool { return strings.EqualFold(l, c.def) })
	slices.Sort(c.langs)
	c.langs = append([]string{c.def}, c.langs...)
	return c, nil
}

// Default returns the app's default language: English, for a nil Catalog.
func (c *Catalog) Default() string {
	if c == nil {
		return "en"
	}
	return c.def
}

// Languages returns the languages the catalog has: the default first, then
// those with a file.
func (c *Catalog) Languages() []string {
	if c == nil {
		return []string{"en"}
	}
	return slices.Clone(c.langs)
}

// Find returns the catalog's language for tag, whatever its case, as a
// user's choice names one: the same language, or else its language
// without the region, "vi" for "vi-VN", or else one of that language's
// regions, "pt-BR" for "pt". It reports false when the catalog has none.
func (c *Catalog) Find(tag string) (string, bool) {
	tag = strings.ToLower(canonical(tag))
	if tag == "" {
		return "", false
	}
	langs := c.Languages()
	for _, l := range langs {
		if strings.ToLower(l) == tag {
			return l, true
		}
	}
	for _, l := range langs {
		if strings.ToLower(l) == base(tag) {
			return l, true
		}
	}
	for _, l := range langs {
		if strings.ToLower(base(l)) == base(tag) {
			return l, true
		}
	}
	return "", false
}

// Match returns the catalog's best language for a browser's
// Accept-Language, as "vi-VN,vi;q=0.9,en;q=0.8": the first, by weight, it
// has, as Find finds one, or else the default.
func (c *Catalog) Match(acceptLanguage string) string {
	for _, tag := range accepted(acceptLanguage) {
		if tag == "*" {
			break
		}
		if l, ok := c.Find(tag); ok {
			return l
		}
	}
	return c.Default()
}

// maxAccepted is how many of Accept-Language's languages are read: a
// person's browser names a few, and a header of thousands is someone
// else's.
const maxAccepted = 32

// accepted lists the languages of an Accept-Language header by weight,
// heaviest first, and those of the same weight in the header's order,
// leaving out those it refuses, with q=0.
func accepted(header string) []string {
	type weighted struct {
		tag string
		q   float64
	}
	var ws []weighted
	for part := range strings.SplitSeq(header, ",") {
		if len(ws) == maxAccepted {
			break
		}
		tag, params, _ := strings.Cut(part, ";")
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		q := 1.0
		for p := range strings.SplitSeq(params, ";") {
			k, v, ok := strings.Cut(p, "=")
			if ok && strings.EqualFold(strings.TrimSpace(k), "q") {
				if q, _ = strconv.ParseFloat(strings.TrimSpace(v), 64); q > 1 {
					q = 0 // not a weight
				}
			}
		}
		if q > 0 {
			ws = append(ws, weighted{tag, q})
		}
	}
	slices.SortStableFunc(ws, func(a, b weighted) int { return cmp.Compare(b.q, a.q) })
	tags := make([]string, len(ws))
	for i, w := range ws {
		tags[i] = w.tag
	}
	return tags
}

// In returns the words of one of the catalog's languages, as Find finds
// it, or of the default when it has none, for what has no request to say
// it in, as a job that makes a mail in the language its user picked.
func (c *Catalog) In(locale string) Words {
	l, ok := c.Find(locale)
	if !ok {
		l = c.Default()
	}
	return Words{c: c, locale: l}
}

// Words are the texts of one language. The zero Words are English's.
type Words struct {
	c      *Catalog
	locale string
}

// Locale returns the language, as "vi" or "pt-BR".
func (w Words) Locale() string {
	return cmp.Or(w.locale, "en")
}

// T says text in the language, with its placeholders, :name, filled from
// args, which come in pairs, as slog takes them: T("Welcome, :name",
// "name", user.Name). A placeholder whose name is capitalized fills with
// the value capitalized, :Name, or in capitals, :NAME; one args has no
// value for stays as it is. A text the language has no words for says
// itself, in English.
func (w Words) T(text string, args ...any) string {
	s, _ := w.lookup(text)
	return fill(s, args)
}

// Choice says text for a count, n, as T does: a text with forms, split by
// |, as ":count post|:count posts", says the form the language's rules
// have for n, and :count is n. A form may name the counts it's for, as
// Laravel's do: "{0} no posts|{1} one post|[2,*] :count posts".
func (w Words) Choice(text string, n int, args ...any) string {
	s, locale := w.lookup(text)
	return fill(choose(s, n, locale), append([]any{"count", n}, args...))
}

// lookup finds text's words: the language's, its language without the
// region's, English's own when the app has an en.json, or else the text
// itself. It returns the language they're in, whose rules pick a form.
func (w Words) lookup(text string) (string, string) {
	if w.c != nil {
		for _, l := range []string{w.locale, base(w.locale), "en"} {
			if s := w.c.texts[strings.ToLower(l)][text]; s != "" {
				return s, l
			}
		}
	}
	return text, "en"
}

// fill puts args' values in text's placeholders: a colon, and a name of
// letters, digits and underscores, that starts with a letter. What it
// puts is never read again for placeholders, so a value can't name one.
func fill(text string, args []any) string {
	if len(args) < 2 || !strings.Contains(text, ":") {
		return text
	}
	values := make(map[string]string, len(args)/2)
	for i := 0; i+1 < len(args); i += 2 {
		values[fmt.Sprint(args[i])] = fmt.Sprint(args[i+1])
	}
	var b strings.Builder
	for i := 0; i < len(text); {
		if text[i] != ':' || i+1 == len(text) || !isLetter(text[i+1]) {
			b.WriteByte(text[i])
			i++
			continue
		}
		j := i + 1
		for j < len(text) && (isLetter(text[j]) || '0' <= text[j] && text[j] <= '9' || text[j] == '_') {
			j++
		}
		if v, ok := value(values, text[i+1:j]); ok {
			b.WriteString(v)
		} else {
			b.WriteString(text[i:j])
		}
		i = j
	}
	return b.String()
}

// value is a placeholder's value: its own, or its name's in lower case,
// capitalized as the placeholder is.
func value(values map[string]string, name string) (string, bool) {
	if v, ok := values[name]; ok {
		return v, true
	}
	v, ok := values[strings.ToLower(name)]
	if !ok {
		return "", false
	}
	if len(name) > 1 && name == strings.ToUpper(name) {
		return strings.ToUpper(v), true
	}
	r, size := utf8.DecodeRuneInString(v)
	return strings.ToUpper(string(r)) + v[size:], true
}

func isLetter(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}

// canonical writes a language's tag as BCP 47 has it: the language in
// lower case, a region in upper case, and a script capitalized, with
// hyphens, so that pt_br is pt-BR.
func canonical(tag string) string {
	parts := strings.FieldsFunc(strings.TrimSpace(tag), func(r rune) bool { return r == '-' || r == '_' })
	for i, p := range parts {
		switch {
		case i == 0:
			parts[i] = strings.ToLower(p)
		case len(p) == 2:
			parts[i] = strings.ToUpper(p)
		case len(p) == 4:
			parts[i] = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
		default:
			parts[i] = strings.ToLower(p)
		}
	}
	return strings.Join(parts, "-")
}

// validTag reports whether tag reads as a language's: two or three letters,
// then subtags of letters and digits.
func validTag(tag string) bool {
	parts := strings.Split(tag, "-")
	if len(parts[0]) < 2 || len(parts[0]) > 3 {
		return false
	}
	for i, p := range parts {
		if p == "" || len(p) > 8 {
			return false
		}
		for j := range len(p) {
			if !isLetter(p[j]) && (i == 0 || p[j] < '0' || p[j] > '9') {
				return false
			}
		}
	}
	return true
}

// base is a tag's language, without its region or script: "pt" for
// "pt-BR".
func base(tag string) string {
	l, _, _ := strings.Cut(tag, "-")
	return strings.ToLower(l)
}
