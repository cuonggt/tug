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
	"io/fs"
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
	flags.Usage = func() {
		fmt.Fprint(flags.Output(), `usage: tug lang <language>

Writes lang/<language>.json, as lang/vi.json, with every text the app says
to a person, in English, for its translation: tug's own, as a form's
errors, the messages of the app's validate rules, the names of its forms'
fields, the file types its uploads take, the texts its Go gives T and
Choice, and those its pages give t and choice. A file that's there keeps
what it has, and gets the texts it hasn't. A text left empty is said in
English.

tug lang builds the app and runs it, as tug gen does, for tug's texts and
the app's rules, and reads the app's Go and its frontend for the rest.
`)
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 || !languageTag(flags.Arg(0)) {
		return errors.New("tug lang takes a language, as tug lang vi or tug lang pt-BR")
	}
	if err := checkProject(); err != nil {
		return err
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

// pageTexts are the texts a frontend's pages say: the literal a t() or a
// choice() call takes first, in the files under dir, which is
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

// saidInPages finds a t() or a choice() call's text: a JavaScript string
// in any of its quotes, which a group each reads, as a regexp here can't
// say "the quote it opened with"; and nothing of a call on something
// else, as format( or $t(, which the character before the name rules out.
var saidInPages = regexp.MustCompile(`(?:^|[^\w$.])(?:t|choice)\(\s*(?:` +
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
