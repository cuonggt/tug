package main

import (
	"fmt"
	"html"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// langNames are the names the bar over a block of code gives the guide's
// languages.
var langNames = map[string]string{
	"go":         "Go",
	"sh":         "Shell",
	"bash":       "Shell",
	"tsx":        "TSX",
	"ts":         "TypeScript",
	"sql":        "SQL",
	"json":       "JSON",
	"dockerfile": "Dockerfile",
	"html":       "HTML",
	"yaml":       "YAML",
	"vue":        "Vue",
	"svelte":     "Svelte",
}

func langName(lang string) string {
	if name, ok := langNames[lang]; ok {
		return name
	}
	return lang
}

// copyButton copies the code of the block it's in, by site.js.
const copyButton = `<button type="button" class="copy" data-copy>` +
	`<svg class="i" viewBox="0 0 24 24" aria-hidden="true"><rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15V6a2 2 0 0 1 2-2h9"/></svg>` +
	`<span data-copy-label>Copy</span></button>`

// codeBlock writes a block of code in its language's colors, under a bar
// with its label, a language or a file's name, and a button to copy it.
func codeBlock(label, lang, code string) string {
	var b strings.Builder
	b.WriteString(`<div class="code"><div class="code-bar">`)
	fmt.Fprintf(&b, `<span class="code-label">%s</span>`, html.EscapeString(label))
	b.WriteString(copyButton)
	b.WriteString(`</div><pre><code>`)
	writeTokens(&b, lang, strings.TrimRight(code, "\n"))
	b.WriteString("</code></pre></div>\n")
	return b.String()
}

// writeTokens writes code as spans of the few classes site.css colors,
// from chroma's tokens, or as it is in a language chroma doesn't know.
func writeTokens(b *strings.Builder, lang, code string) {
	lexer := lexers.Get(lang)
	if lang == "" || lexer == nil {
		b.WriteString(html.EscapeString(code))
		return
	}
	tokens, err := chroma.Coalesce(lexer).Tokenise(nil, code)
	if err != nil {
		b.WriteString(html.EscapeString(code))
		return
	}
	for _, t := range tokens.Tokens() {
		class := tokenClass(t.Type)
		if class == "" {
			b.WriteString(html.EscapeString(t.Value))
			continue
		}
		fmt.Fprintf(b, `<span class="%s">%s</span>`, class, html.EscapeString(t.Value))
	}
}

// tokenClass is the class of a token's color: keywords, types, functions,
// strings, numbers, comments, a shell's prompt, and markup's tags and
// attributes. The rest is the text's color.
func tokenClass(t chroma.TokenType) string {
	switch {
	case t == chroma.KeywordType || t == chroma.NameClass:
		return "t"
	case t.InCategory(chroma.Keyword):
		return "k"
	case t.InCategory(chroma.Comment):
		return "c"
	case t.InSubCategory(chroma.LiteralString):
		return "s"
	case t.InSubCategory(chroma.LiteralNumber), t == chroma.NameConstant:
		return "n"
	case t == chroma.NameFunction, t == chroma.NameFunctionMagic:
		return "f"
	case t == chroma.NameBuiltin, t == chroma.NameBuiltinPseudo:
		return "b"
	case t == chroma.NameTag:
		return "g"
	case t == chroma.NameAttribute:
		return "a"
	case t == chroma.GenericPrompt:
		return "p"
	}
	return ""
}
