package main

import (
	"bytes"
	"fmt"
	"html"
	"html/template"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// A page is one of docs/, or a translation of one, made into HTML.
type page struct {
	Name        string        // the file's name without .md, as "forms", and "README" for the guide's index
	File        string        // the file, in the checkout, as docs/forms.md or docs/vi/forms.md
	Lang        *language     // what it's written in
	Path        string        // where it's served, under its language's path, as "docs/forms/"
	Title       string        // its first heading's text
	TitleHTML   template.HTML // and its HTML, code and all
	Description string        // its first paragraph's text, shortened, for search engines and previews
	Body        template.HTML // all after its first heading; for the index, up to its list of parts
	After       template.HTML // the index's, after its list of parts
	TOC         []heading     // its h2s and h3s

	Blob   string // the English's blob ID, which its translations name
	From   string // a translation's: the ID of the English's blob it was made from
	Behind bool   // a translation's English has changed since it was made

	ids      map[string]bool // its headings', for links to them
	sections []section       // what search finds in it
	links    []link          // to pages of the guide and their headings, checked once all are made
	problems []string        // found as it was made, as a link to a file that isn't there
	parts    []part          // the index's list of parts
}

// A heading is an entry of a page's table of contents.
type heading struct {
	Level int
	ID    string
	HTML  template.HTML
}

// A section is what search finds: a heading and the text under it, up to
// the next.
type section struct {
	Heading string
	ID      string
	Text    string
}

// A link is one from a page to a page of the guide, or to a heading in
// one, which has to be there.
type link struct {
	Dest string // as the page has it
	Page string // the page it's to, by name
	ID   string // and the heading, if it names one
	Own  bool   // to a page of its own directory: from a translation, another translation, not the English
}

// A part is one of the guide's, as its index lists them.
type part struct {
	Num   int
	Title string        // as the index names it
	Name  string        // the page's
	Path  string        // where it's served, under the site's base
	About template.HTML // what the index says it covers
}

// pagePath is where the page of docs/ named name is served, under its
// language's path: docs/forms.md at docs/forms/, and the index at docs/.
func pagePath(name string) string {
	if name == "README" {
		return "docs/"
	}
	return "docs/" + name + "/"
}

// relative is the link from the page at the path from to the one at to,
// both under docs/, as "../forms/": a page links to the pages of the
// language it's shown in, whatever path that language's pages are under,
// so the English is the same page in each language that hasn't
// translated it.
func relative(from, to string) string {
	rel := strings.TrimPrefix(to, "docs/")
	if from != "docs/" {
		rel = "../" + rel
	}
	if rel == "" {
		return "./"
	}
	return rel
}

// A converter makes the guide's pages HTML, or a language's translations
// of them.
type converter struct {
	root  string          // the checkout, for the files the guide links to
	dir   string          // the pages' directory in it: docs, or docs/<tag> for a language's
	names map[string]bool // the guide's pages, the English
	lang  *language       // what the pages are written in
	md    goldmark.Markdown
}

func newConverter(root, dir string, names map[string]bool, l *language) *converter {
	return &converter{
		root:  root,
		dir:   dir,
		names: names,
		lang:  l,
		md: goldmark.New(
			// GitHub's Markdown, as the guide is written to be read there.
			goldmark.WithExtensions(extension.GFM),
			goldmark.WithRendererOptions(
				gmhtml.WithUnsafe(),
				renderer.WithNodeRenderers(util.Prioritized(blocks{l}, 100)),
			),
		),
	}
}

// convert makes a page of the Markdown of the converter's <name>.md.
func (c *converter) convert(name string, src []byte) (*page, error) {
	p := &page{Name: name, File: c.dir + "/" + name + ".md", Lang: c.lang, Path: pagePath(name), ids: map[string]bool{}}
	doc := c.md.Parser().Parse(text.NewReader(src))

	// The index's list of parts, taken out before its links are the
	// site's, as a part is named by the file it links to.
	var list ast.Node
	if name == "README" {
		list = c.partsOf(p, doc, src)
	}

	slugs := map[string]int{}
	visit := func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Heading:
			id := uniqueSlug(slugs, plainText(n, src))
			n.SetAttributeString("id", []byte(id))
			p.ids[id] = true
		case *ast.Link:
			n.Destination = []byte(c.rewrite(p, string(n.Destination)))
		}
		return ast.WalkContinue, nil
	}
	if err := ast.Walk(doc, visit); err != nil {
		return nil, err
	}

	// The first heading is the page's title, which the page's template
	// writes, and its first paragraph what it's about.
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		if h, ok := n.(*ast.Heading); ok && h.Level == 1 {
			p.Title = squash(plainText(h, src))
			p.TitleHTML = c.inline(h, src)
			doc.RemoveChild(doc, h)
			break
		}
	}
	if p.Title == "" {
		return nil, fmt.Errorf("%s has no heading to be its title", p.File)
	}
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		if _, ok := n.(*ast.Paragraph); ok {
			p.Description = shorten(squash(plainText(n, src)), 160)
			break
		}
	}

	current := section{Heading: p.Title}
	var words strings.Builder
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		h, ok := n.(*ast.Heading)
		if !ok || h.Level > 3 {
			words.WriteString(plainText(n, src))
			words.WriteByte(' ')
			continue
		}
		current.Text = squash(words.String())
		p.sections = append(p.sections, current)
		words.Reset()
		current = section{Heading: squash(plainText(h, src)), ID: headingID(h)}
		p.TOC = append(p.TOC, heading{Level: h.Level, ID: headingID(h), HTML: c.inline(h, src)})
	}
	current.Text = squash(words.String())
	p.sections = append(p.sections, current)

	var body, after bytes.Buffer
	w := &body
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		if n == list {
			w = &after
			continue
		}
		if err := c.md.Renderer().Render(w, src, n); err != nil {
			return nil, err
		}
	}
	p.Body = template.HTML(body.String())
	p.After = template.HTML(after.String())
	return p, nil
}

// partsOf reads the guide's parts from its index: the first numbered list,
// each item a link to the part's page, a colon, and what it covers. It
// returns the list, for the index's template to write as it likes.
func (c *converter) partsOf(p *page, doc ast.Node, src []byte) ast.Node {
	var list *ast.List
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		if l, ok := n.(*ast.List); ok && l.IsOrdered() {
			list = l
			break
		}
	}
	if list == nil {
		p.problems = append(p.problems, p.File+": no numbered list of the guide's parts")
		return nil
	}
	for item := list.FirstChild(); item != nil; item = item.NextSibling() {
		var first *ast.Link
		_ = ast.Walk(item, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if l, ok := n.(*ast.Link); ok && entering {
				first = l
				return ast.WalkStop, nil
			}
			return ast.WalkContinue, nil
		})
		if first == nil {
			p.problems = append(p.problems, p.File+": a part of the guide that isn't a link to its page")
			continue
		}
		dest, _, _ := strings.Cut(string(first.Destination), "#")
		name := strings.TrimSuffix(path.Base(dest), ".md")
		if !c.names[name] || path.Ext(dest) != ".md" {
			p.problems = append(p.problems, fmt.Sprintf("%s: the part %q links to %s, which isn't a page of docs/", p.File, squash(plainText(first, src)), dest))
			continue
		}

		// What's after the link is what the part covers, from its colon on.
		about := ast.NewParagraph()
		for n := first.NextSibling(); n != nil; {
			next := n.NextSibling()
			about.AppendChild(about, n)
			n = next
		}
		// Chinese and Japanese write the colon full width.
		if t, ok := about.FirstChild().(*ast.Text); ok {
			v := t.Value(src)
			trimmed := bytes.TrimLeft(v, ": ：")
			t.Segment = t.Segment.WithStart(t.Segment.Start + len(v) - len(trimmed))
		}
		unlink(about)

		p.parts = append(p.parts, part{
			Num:   len(p.parts) + 1,
			Title: squash(plainText(first, src)),
			Name:  name,
			Path:  pagePath(name),
			About: c.inline(about, src),
		})
	}
	return list
}

// unlink puts each link's text in the link's place, for a part's card,
// which is a link itself.
func unlink(n ast.Node) {
	for c := n.FirstChild(); c != nil; {
		next := c.NextSibling()
		if l, ok := c.(*ast.Link); ok {
			for t := l.FirstChild(); t != nil; {
				tn := t.NextSibling()
				n.InsertBefore(n, l, t)
				t = tn
			}
			n.RemoveChild(n, l)
		} else {
			unlink(c)
		}
		c = next
	}
}

// inline is the HTML of a node's children, as a heading's, without the
// element around them.
func (c *converter) inline(n ast.Node, src []byte) template.HTML {
	var b bytes.Buffer
	for ch := n.FirstChild(); ch != nil; ch = ch.NextSibling() {
		_ = c.md.Renderer().Render(&b, src, ch)
	}
	return template.HTML(strings.TrimSpace(b.String()))
}

// rewrite makes a link of the guide's one of the site's: a page of docs/
// links to its page in the language the page is shown in, and a file of
// the checkout's, as an example's code, to it on GitHub. A translation
// links to another as docs/<tag>/ has it, and to a page it hasn't
// translated as the English, ../forms.md, so that its links lead to the
// same pages on GitHub.
func (c *converter) rewrite(p *page, dest string) string {
	if strings.HasPrefix(dest, "#") {
		p.links = append(p.links, link{Dest: dest, Page: p.Name, ID: dest[1:], Own: true})
		return dest
	}
	if u, err := url.Parse(dest); dest == "" || err != nil || u.Scheme != "" || u.Host != "" {
		return dest
	}
	file, frag, _ := strings.Cut(dest, "#")
	target := path.Clean(path.Join(c.dir, file))
	for _, dir := range []string{c.dir, "docs"} {
		name, ok := strings.CutPrefix(target, dir+"/")
		if !ok || path.Ext(name) != ".md" || strings.Contains(name, "/") {
			continue
		}
		name = strings.TrimSuffix(name, ".md")
		p.links = append(p.links, link{Dest: dest, Page: name, ID: frag, Own: dir == c.dir})
		href := relative(p.Path, pagePath(name))
		if frag != "" {
			href += "#" + frag
		}
		return href
	}
	if target == ".." || strings.HasPrefix(target, "../") {
		p.problems = append(p.problems, fmt.Sprintf("%s: a link to %s, outside the checkout", p.File, dest))
		return dest
	}
	kind := "blob"
	fi, err := os.Stat(filepath.Join(c.root, filepath.FromSlash(target)))
	switch {
	case err != nil:
		p.problems = append(p.problems, fmt.Sprintf("%s: a link to %s, a file that isn't there", p.File, dest))
	case fi.IsDir():
		kind = "tree"
	}
	href := repo + "/" + kind + "/main/" + target
	if frag != "" {
		href += "#" + frag
	}
	return href
}

// githubSlug is the ID GitHub gives a heading, as github-slugger makes it,
// so that the guide's links to headings work on the site as they do on
// GitHub: lower case, a space a hyphen, and only letters, numbers, marks,
// hyphens and underscores kept.
func githubSlug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r == ' ':
			b.WriteByte('-')
		case r == '-', unicode.IsLetter(r), unicode.IsNumber(r), unicode.In(r, unicode.M, unicode.Pc):
			b.WriteRune(r)
		}
	}
	return b.String()
}

// uniqueSlug is a heading's ID among a page's: a second "foo" is "foo-1",
// as GitHub has it.
func uniqueSlug(seen map[string]int, s string) string {
	slug := githubSlug(s)
	id := slug
	for {
		if _, taken := seen[id]; !taken {
			break
		}
		seen[slug]++
		id = fmt.Sprintf("%s-%d", slug, seen[slug])
	}
	seen[id] = 0
	return id
}

func headingID(h *ast.Heading) string {
	v, _ := h.AttributeString("id")
	b, _ := v.([]byte)
	return string(b)
}

// plainText is a node's text as a reader sees it: code's and links' text
// in, the Markdown and HTML out.
func plainText(n ast.Node, src []byte) string {
	var b strings.Builder
	var walk func(n ast.Node)
	walk = func(n ast.Node) {
		switch n := n.(type) {
		case *ast.Text:
			b.Write(n.Value(src))
			if n.SoftLineBreak() || n.HardLineBreak() {
				b.WriteByte(' ')
			}
			return
		case *ast.String:
			b.Write(n.Value)
			return
		case *ast.AutoLink:
			b.Write(n.Label(src))
			return
		case *ast.RawHTML, *ast.HTMLBlock:
			return
		case *ast.FencedCodeBlock, *ast.CodeBlock:
			lines := n.Lines()
			for i := range lines.Len() {
				seg := lines.At(i)
				b.Write(seg.Value(src))
			}
			b.WriteByte(' ')
			return
		}
		for ch := n.FirstChild(); ch != nil; ch = ch.NextSibling() {
			walk(ch)
		}
		if n.Type() == ast.TypeBlock {
			b.WriteByte(' ')
		}
	}
	walk(n)
	return b.String()
}

// squash puts one space where text has any run of them.
func squash(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// shorten cuts text at a word to n characters at most, with an ellipsis.
func shorten(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	cut := string(r[:n-1])
	if i := strings.LastIndexByte(cut, ' '); i > n/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,;:.") + "…"
}

// blocks writes the guide's headings, code and tables as the site styles
// them, saying what they say in the language the page is written in.
type blocks struct{ lang *language }

func (b blocks) RegisterFuncs(r renderer.NodeRendererFuncRegisterer) {
	r.Register(ast.KindHeading, b.renderHeading)
	r.Register(ast.KindFencedCodeBlock, b.renderCode)
	r.Register(ast.KindCodeBlock, b.renderCode)
	r.Register(extast.KindTable, renderTable)
}

// renderHeading writes a heading with a link to it beside it, outside it,
// as GitHub does, so a screen reader's list of headings has the headings'
// own words.
func (b blocks) renderHeading(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	h := node.(*ast.Heading)
	id := html.EscapeString(headingID(h))
	if entering {
		fmt.Fprintf(w, `<div class="heading heading-%d"><h%d id="%s">`, h.Level, h.Level, id)
		return ast.WalkContinue, nil
	}
	label := b.lang.t("Link to the section :heading", "heading", squash(plainText(h, src)))
	fmt.Fprintf(w, "</h%d><a class=\"anchor\" href=\"#%s\" aria-label=\"%s\"></a></div>\n", h.Level, id, html.EscapeString(label))
	return ast.WalkContinue, nil
}

func (b blocks) renderCode(w util.BufWriter, src []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	var lang string
	if f, ok := node.(*ast.FencedCodeBlock); ok {
		lang = string(f.Language(src))
	}
	var code strings.Builder
	lines := node.Lines()
	for i := range lines.Len() {
		seg := lines.At(i)
		code.Write(seg.Value(src))
	}
	_, _ = w.WriteString(codeBlock(langName(lang), lang, code.String(), b.lang))
	return ast.WalkSkipChildren, nil
}

// renderTable puts a table in a box of its own, which scrolls sideways
// when the table is wider than the page.
func renderTable(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("<div class=\"table\"><table>\n")
	} else {
		_, _ = w.WriteString("</table></div>\n")
	}
	return ast.WalkContinue, nil
}
