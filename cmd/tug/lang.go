package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/cuonggt/tug/internal/filetype"
	"github.com/cuonggt/tug/internal/label"
)

func runLang(args []string) error {
	flags := flag.NewFlagSet("lang", flag.ContinueOnError)
	check := flags.Bool("check", false, "write nothing, and say what's left to translate")
	flags.Usage = func() {
		fmt.Fprint(flags.Output(), `usage: tug lang <language>
       tug lang -check [language]

Writes lang/<language>.json, as lang/vi.json, with every text the app says
to a person, in English, for its translation: tug's own, as a form's
errors, the messages of the app's validate rules, the names of its forms'
fields, the file types its uploads take, the texts its Go gives T and
Choice, and those its pages give t and choice. A file that's there keeps
what it has, and gets the texts it hasn't. A text left empty is said in
English.

With -check, it writes nothing, and says what's left to translate in the
language named, or with none, in every file of lang/: a text the app says
that the file hasn't, one with no words, and a translation that loses a
placeholder of its English's, as :count, or says one the English hasn't.
Any of those fails it, for CI; a text the file has that the app no longer
says is a note, which doesn't. An English file, as en.json, words only
what it would have the app say otherwise, so its placeholders alone are
checked.

tug lang builds the app and runs it, as tug gen does, for tug's texts and
the app's rules, and reads the app's Go and its frontend for the rest.
`)
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 1 || !*check && flags.NArg() != 1 || flags.NArg() == 1 && !languageTag(flags.Arg(0)) {
		return errors.New("tug lang takes a language, as tug lang vi or tug lang pt-BR, and tug lang -check one or none")
	}
	if err := checkProject(); err != nil {
		return err
	}
	var files []string
	if *check {
		var err error
		if files, err = langFiles(".", flags.Arg(0)); err != nil {
			return err
		}
		if len(files) == 0 {
			fmt.Println("lang/ has no languages to check")
			return nil
		}
	}
	env, err := appEnv()
	if err != nil {
		return err
	}
	bin, err := buildApp(env, os.Stderr)
	if err != nil {
		return err
	}
	g, err := runForGen(env, bin)
	if err != nil {
		return err
	}
	own, err := appTexts(".")
	if err != nil {
		return err
	}
	said, err := pageTexts(filepath.Join("resources", "js"))
	if err != nil {
		return err
	}
	if *check {
		return checkLanguages(os.Stdout, files, slices.Concat(g.Texts, own, said))
	}
	path := filepath.Join("lang", flags.Arg(0)+".json")
	added, total, err := addTexts(path, slices.Concat(g.Texts, own, said))
	if err != nil {
		return err
	}
	if added == 0 {
		fmt.Printf("%s has every text already, %d of them\n", path, total)
	} else {
		fmt.Printf("wrote %s: %d texts to translate, of %d; one left empty is said in English\n", path, added, total)
	}
	return nil
}

// languageTag reports whether s reads as a language's tag, as vi or pt-BR:
// what a file of lang/ is named for.
func languageTag(s string) bool {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '-' || r == '_' })
	if len(parts) == 0 || len(parts[0]) < 2 || len(parts[0]) > 3 {
		return false
	}
	for _, p := range parts {
		for _, r := range p {
			if !('a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9') {
				return false
			}
		}
	}
	return true
}

// appTexts finds what the app's own Go, in dir, says to a person: the
// texts it gives T and Choice, the names of the fields its forms bind and
// check, by their label tags or their keys made into words, as validate
// and Bind name them, and the file types its upload tags name. Tests, and
// directories of dependencies or tools, aren't read.
func appTexts(dir string) ([]string, error) {
	var texts []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if path != dir && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				if sel, ok := n.Fun.(*ast.SelectorExpr); ok && (sel.Sel.Name == "T" || sel.Sel.Name == "Choice") && len(n.Args) > 0 {
					if s, ok := stringLit(n.Args[0]); ok {
						texts = append(texts, s)
					}
				}
			case *ast.StructType:
				texts = append(texts, fieldTexts(n)...)
			}
			return true
		})
		return nil
	})
	return texts, err
}

// stringLit is the string a literal in the source says.
func stringLit(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	return s, err == nil
}

// fieldTexts are the texts a struct's fields bring to messages: the names
// of those a form binds or checks, and of those they're compared with, and
// the file types their uploads take.
func fieldTexts(st *ast.StructType) []string {
	var texts []string
	tags := map[string]reflect.StructTag{} // by the field's Go name
	for _, f := range st.Fields.List {
		tag := structTag(f)
		for _, name := range f.Names {
			tags[name.Name] = tag
		}
	}
	for _, f := range st.Fields.List {
		tag := structTag(f)
		rules, checked := tag.Lookup("validate")
		_, form := tag.Lookup("form")
		_, query := tag.Lookup("query")
		_, path := tag.Lookup("path")
		if !checked && !form && !query && !path {
			continue
		}
		for _, name := range f.Names {
			texts = append(texts, fieldNames(name.Name, tag)...)
		}
		for rule := range strings.SplitSeq(rules, ",") {
			tagName, param, _ := strings.Cut(rule, "=")
			switch tagName {
			case "file_type":
				types := strings.Fields(param)
				if len(types) > 0 && !slices.ContainsFunc(types, func(t string) bool { return !filetype.Known(t) }) {
					texts = append(texts, filetype.Names(types))
				}
			case "eqfield", "nefield":
				if other, ok := tags[param]; ok {
					texts = append(texts, fieldNames(param, other)...)
				}
			}
		}
	}
	return texts
}

// fieldNames are the names a message gives the field goName, with tag: its
// label, or else its keys made into words, as the body, the query or the
// path has them.
func fieldNames(goName string, tag reflect.StructTag) []string {
	if l := tag.Get("label"); l != "" {
		return []string{l}
	}
	var names []string
	key, _, _ := strings.Cut(tag.Get("json"), ",")
	if key == "" {
		key = tag.Get("form")
	}
	switch key {
	case "-":
	case "":
		names = append(names, label.Readable(goName))
	default:
		names = append(names, label.Readable(key))
	}
	for _, k := range []string{tag.Get("query"), tag.Get("path")} {
		if k != "" {
			names = append(names, label.Readable(k))
		}
	}
	return names
}

func structTag(f *ast.Field) reflect.StructTag {
	if f.Tag == nil {
		return ""
	}
	s, _ := strconv.Unquote(f.Tag.Value)
	return reflect.StructTag(s)
}

// langFiles are the language files -check reads, in dir's lang/: the one
// named, or with none, every one there, by its name, as lang's Load reads
// them. One named that isn't there is an error that says what makes it.
func langFiles(dir, named string) ([]string, error) {
	if named != "" {
		path := filepath.Join(dir, "lang", named+".json")
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%s isn't there: tug lang %s makes it", filepath.Join("lang", named+".json"), named)
		} else if err != nil {
			return nil, err
		}
		return []string{path}, nil
	}
	paths, err := filepath.Glob(filepath.Join(dir, "lang", "*.json"))
	if err != nil {
		return nil, err
	}
	var files []string
	for _, path := range paths {
		if languageTag(strings.TrimSuffix(filepath.Base(path), ".json")) {
			files = append(files, path)
		}
	}
	return files, nil
}

// checked is what -check found in a language's file: the problems, which
// fail it, and the notes, which don't, each said of a text, in its English's
// order, and how many texts are left to translate, and to fix.
type checked struct {
	problems, notes []string
	translate, fix  int
	said            int // the texts the app says
}

// checkTexts checks the language file at path against texts, those the app
// says: a text the file hasn't, one with no words, and a translation whose
// placeholders aren't its English's are problems, and a text the file has
// that the app no longer says is a note. An English file words only what it
// would have the app say otherwise, so a text it hasn't, or leaves empty,
// is the English, as the app has it, and its placeholders alone are checked.
func checkTexts(path string, texts []string) (checked, error) {
	have, err := readTexts(path)
	if err != nil {
		return checked{}, err
	}
	tag := strings.FieldsFunc(strings.TrimSuffix(filepath.Base(path), ".json"), func(r rune) bool { return r == '-' || r == '_' })
	english := len(tag) > 0 && strings.EqualFold(tag[0], "en")
	said := map[string]bool{}
	for _, t := range texts {
		if t != "" {
			said[t] = true
		}
	}
	c := checked{said: len(said)}
	for _, text := range slices.Sorted(maps.Keys(said)) {
		words, ok := have[text]
		switch {
		case !ok && !english:
			c.problems = append(c.problems, fmt.Sprintf("%q isn't there", text))
			c.translate++
		case words == "" && !english:
			c.problems = append(c.problems, fmt.Sprintf("%q has no words", text))
			c.translate++
		case words != "":
			if wrong := placeholdersOf(text, words); wrong != "" {
				c.problems = append(c.problems, fmt.Sprintf("%q %s", text, wrong))
				c.fix++
			}
		}
	}
	for _, text := range slices.Sorted(maps.Keys(have)) {
		if !said[text] {
			c.notes = append(c.notes, fmt.Sprintf("%q is said nowhere now, for its translator to delete", text))
		}
	}
	return c, nil
}

// placeholdersOf says how a translation's placeholders aren't its English's:
// one it loses, whose value it doesn't say, and one it says that the English
// hasn't, which it says as written; or "" when they're the same. A choice's
// translation may say :count, which Choice fills, whether or not its English
// does.
func placeholdersOf(english, words string) string {
	want, got := placeholders(english), placeholders(words)
	choice := strings.Contains(english, "|")
	var lost, made []string
	for _, name := range slices.Sorted(maps.Keys(want)) {
		if !got[name] {
			lost = append(lost, ":"+name)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(got)) {
		if !want[name] && !(choice && name == "count") {
			made = append(made, ":"+name)
		}
	}
	var says []string
	if len(lost) > 0 {
		says = append(says, "loses "+strings.Join(lost, " and "))
	}
	if len(made) > 0 {
		says = append(says, "says "+strings.Join(made, " and ")+", which the English doesn't")
	}
	return strings.Join(says, ", and ")
}

// placeholders are the names of a text's placeholders, as package lang's
// fill reads them: a colon, a letter, and letters, digits and underscores,
// :name, :Name and :NAME each name.
func placeholders(text string) map[string]bool {
	names := map[string]bool{}
	letter := func(c byte) bool { return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' }
	for i := 0; i < len(text); i++ {
		if text[i] != ':' || i+1 == len(text) || !letter(text[i+1]) {
			continue
		}
		j := i + 1
		for j < len(text) && (letter(text[j]) || '0' <= text[j] && text[j] <= '9' || text[j] == '_') {
			j++
		}
		names[strings.ToLower(text[i+1:j])] = true
		i = j - 1
	}
	return names
}

// checkLanguages checks each of files against texts, and says what it found
// in w, a line a problem or a note, and a line a file of what's left; any
// problem is its error, for the status a CI run fails by.
func checkLanguages(w io.Writer, files, texts []string) error {
	var translate, fix int
	for _, path := range files {
		c, err := checkTexts(path, texts)
		if err != nil {
			return err
		}
		name := filepath.Join("lang", filepath.Base(path))
		for _, line := range slices.Concat(c.problems, c.notes) {
			fmt.Fprintf(w, "%s: %s\n", name, line)
		}
		switch {
		case c.translate == 0 && c.fix == 0:
			fmt.Fprintf(w, "%s: every text translated, %d of them\n", name, c.said)
		case c.fix == 0:
			fmt.Fprintf(w, "%s: %d of %d texts to translate\n", name, c.translate, c.said)
		case c.translate == 0:
			fmt.Fprintf(w, "%s: %d of %d texts to fix\n", name, c.fix, c.said)
		default:
			fmt.Fprintf(w, "%s: %d of %d texts to translate, and %d to fix\n", name, c.translate, c.said, c.fix)
		}
		translate, fix = translate+c.translate, fix+c.fix
	}
	switch {
	case translate == 0 && fix == 0:
		return nil
	case fix == 0:
		return fmt.Errorf("%s to translate", several(translate, "text", "texts"))
	case translate == 0:
		return fmt.Errorf("%s to fix", several(fix, "text", "texts"))
	default:
		return fmt.Errorf("%s to translate, and %d to fix", several(translate, "text", "texts"), fix)
	}
}

// readTexts reads the language file at path, an object of texts, each a
// string; a file of null is one of none.
func readTexts(path string) (map[string]string, error) {
	have := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &have); err != nil {
		return nil, fmt.Errorf("%s isn't an object of texts, each a string: %w", path, err)
	}
	if have == nil {
		have = map[string]string{}
	}
	return have, nil
}

// addTexts adds the texts that the language file at path hasn't got, each
// with no words yet, which says it in English, and keeps those it has, as
// they are. It writes the file, sorted, only when it adds some, and returns
// how many it added, and how many the file has.
func addTexts(path string, texts []string) (int, int, error) {
	have := map[string]string{}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &have); err != nil {
			return 0, 0, fmt.Errorf("%s isn't an object of texts, each a string: %w", path, err)
		}
		if have == nil {
			have = map[string]string{} // the file is null, which reads as no texts
		}
	case !errors.Is(err, fs.ErrNotExist):
		return 0, 0, err
	}
	added := 0
	for _, t := range texts {
		if _, ok := have[t]; !ok && t != "" {
			have[t] = ""
			added++
		}
	}
	if added == 0 && err == nil {
		return 0, len(have), nil
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false) // a translator reads <, > and & as they are
	enc.SetIndent("", "  ")
	if err := enc.Encode(have); err != nil {
		return 0, 0, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, 0, err
	}
	return added, len(have), os.WriteFile(path, b.Bytes(), 0o644)
}

// pageTexts are the texts a frontend's pages say: the literal a t(), a
// text() or a choice() call takes first, in the files under dir, which is
// resources/js. A page says them as the Go says its own, through the
// TypeScript tug gen writes, so one file is a language, both sides of it.
// A text made as the page runs, of a variable, is the translator's to add,
// as in Go.
func pageTexts(dir string) ([]string, error) {
	var texts []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		switch {
		case errors.Is(err, fs.ErrNotExist) && path == dir:
			return nil // an app whose frontend is elsewhere, or none
		case err != nil:
			return err
		case d.IsDir():
			name := d.Name()
			// tug's own is written, not said, and node_modules is nobody's
			// to translate.
			if path != dir && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "tug") {
				return filepath.SkipDir
			}
			return nil
		case !slices.Contains(pageFiles, filepath.Ext(path)):
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range saidInPages.FindAllSubmatch(source, -1) {
			// One of the quotes matched: the others took no part, and are
			// nil rather than empty.
			for _, said := range m[1:] {
				if said == nil {
					continue
				}
				if text, ok := unquoteJS(string(said)); ok {
					texts = append(texts, text)
				}
			}
		}
		return nil
	})
	return texts, err
}

// pageFiles are the files a page's words are in, by extension.
var pageFiles = []string{".ts", ".tsx", ".js", ".jsx", ".vue", ".svelte"}

// saidInPages finds a t(), a text() or a choice() call's text: a JavaScript string
// in any of its quotes, which a group each reads, as a regexp here can't
// say "the quote it opened with"; and nothing of a call on something
// else, as format( or $t(, which the character before the name rules out.
var saidInPages = regexp.MustCompile(`(?:^|[^\w$.])(?:t|text|choice)\(\s*(?:` +
	`'((?:[^'\\]|\\.)*)'|` +
	`"((?:[^"\\]|\\.)*)"|` +
	"`((?:[^`" + `\\]|\\.)*)` + "`" + `)`)

// unquoteJS reads a JavaScript string's own escapes, the ones a text has
// any use for, and says whether the string is one to keep: an empty text,
// or one with a ${} in it, which is said as the page runs, isn't.
func unquoteJS(s string) (string, bool) {
	if s == "" || strings.Contains(s, "${") {
		return "", false
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		default: // \\, \' and \" say themselves, as does anything else
			b.WriteByte(s[i])
		}
	}
	return b.String(), true
}
