package typegen

import (
	"fmt"
	"strings"

	"github.com/cuonggt/tug/lang"
)

// counts is how far the table of forms goes for each language. Every rule
// package lang has goes by the count itself, its last digit or its last
// two, and the counts it names, 0, 1, 2 and up to 4, are below 100: so the
// form a count of 100 or more takes is the one 100 + count % 100 takes,
// and two hundred counts say every form a language has.
//
// A table is a digit for each of them, as no language has ten forms: the
// most is six, which Arabic has.
const counts = 200

// langTS writes lang.ts: t() and choice(), which say a page's own words as
// Words.T and Words.Choice say the app's, from the texts the app shares
// with its pages, and the form each of languages gives a count, from
// package lang's own rules, so a page and the server say a count alike.
func langTS(languages []string) string {
	// Languages whose rules agree share a table: an app's own, and its
	// regions', mostly do.
	tables := map[string]string{}
	var entries []string
	for _, l := range languages {
		var table strings.Builder
		for n := range counts {
			table.WriteString(fmt.Sprint(lang.PluralIndex(l, n) % 10))
		}
		if _, ok := tables[normalized(l)]; ok {
			continue
		}
		tables[normalized(l)] = table.String()
		entries = append(entries, fmt.Sprintf("  %s: %s,", quote(normalized(l)), quote(table.String())))
	}

	var b strings.Builder
	b.WriteString(header)
	b.WriteString(`
/**
 * Texts are the words a page says, in the language of the request it came
 * with: the texts prop the app shares. words has the words for each text
 * the language has any for, and from names the language a text's words are
 * in where that isn't locale, whose rules give a count its form.
 */
export interface Texts {
  locale: string
  words: Record<string, string>
  from?: Record<string, string>
}

/** TextArgs are the values a text's placeholders are filled with, by name. */
export type TextArgs = Record<string, string | number>

`)
	b.WriteString(`// forms is the index of the form a count takes, by language, from the
// rules package lang has: a digit for each count below 100, and then one
// for each count from 100 up, which is the one 100 + count % 100 takes.
const forms: Record<string, string> = {
` + lines(entries) + `}

/**
 * t says text in the language texts is in, with its placeholders filled
 * from args: t(texts, 'Welcome, :name', { name: user.name }). A
 * placeholder whose name is capitalized fills with the value capitalized,
 * :Name, or in capitals, :NAME. A text the language has no words for says
 * itself, in English.
 */
export function t(texts: Texts, text: string, args?: TextArgs): string {
  return fill(wordsFor(texts, text)[0], args)
}

/**
 * choice says text for a count, as t says it: a text with forms, split by
 * |, says the form the language's rules have for the count, and :count is
 * the count. A form may name the counts it's for: '{0} no posts|{1} one
 * post|[2,*] :count posts'.
 */
export function choice(texts: Texts, text: string, count: number, args?: TextArgs): string {
  const [words, locale] = wordsFor(texts, text)
  return fill(choose(words, count, locale), { count, ...args })
}

// wordsFor is a text's words, and the language whose rules give a count
// its form: the one the words are in, or English for a text that says
// itself, as lookup has it in Go.
function wordsFor(texts: Texts, text: string): [string, string] {
  const words = texts.words[text]
  if (!words) {
    return [text, 'en']
  }
  return [words, texts.from?.[text] ?? texts.locale]
}

// fill puts args' values in text's placeholders: a colon, and a name of
// letters, digits and underscores that starts with a letter. A placeholder
// args has no value for stays as it is.
function fill(text: string, args?: TextArgs): string {
  if (!args || !text.includes(':')) {
    return text
  }
  return text.replace(/:([A-Za-z][A-Za-z0-9_]*)/g, (placeholder, name: string) => {
    const value = valueOf(args, name)
    return value === undefined ? placeholder : value
  })
}

// valueOf is a placeholder's value: its own, or its name's in lower case,
// capitalized as the placeholder is.
function valueOf(args: TextArgs, name: string): string | undefined {
  if (args[name] !== undefined) {
    return String(args[name])
  }
  const lower = args[name.toLowerCase()]
  if (lower === undefined) {
    return undefined
  }
  const value = String(lower)
  if (name.length > 1 && name === name.toUpperCase()) {
    return value.toUpperCase()
  }
  return value.replace(/^./u, (first) => first.toUpperCase())
}

// choose picks the form a count takes: a form that names its own counts
// first, and then the one the language's rules give the count.
function choose(text: string, count: number, locale: string): string {
  if (!text.includes('|')) {
    return text
  }
  const parts = text.split('|')
  for (let i = 0; i < parts.length; i++) {
    const [rest, named, takes] = interval(parts[i].trim(), count)
    if (takes) {
      return rest.trim()
    }
    if (named) {
      parts[i] = rest
    }
  }
  const form = indexOf(locale, count)
  return (parts[form] ?? parts[0]).trim()
}

// indexOf is the form the language's rules give a count: by the whole tag,
// then by its language, and the first form for a language they don't know.
function indexOf(locale: string, count: number): number {
  const n = Math.abs(Math.trunc(count))
  const tag = locale.toLowerCase().replace(/-/g, '_')
  const table = forms[tag] ?? forms[tag.split('_')[0]]
  if (!table) {
    return 0
  }
  return table.charCodeAt(n < 100 ? n : 100 + (n % 100)) - 48
}

// interval reads a form that names its counts, as '{0} none', '{1,2} a few'
// or '[2,*] :count': the form without them, whether it names any, and
// whether they take the count. In braces or brackets alike, two numbers are
// a range, either of which can be *, and one is itself.
function interval(form: string, count: number): [string, boolean, boolean] {
  if (form === '' || (form[0] !== '{' && form[0] !== '[')) {
    return [form, false, false]
  }
  const end = form.search(/[}\]]/)
  if (end < 0 || /[{[]/.test(form.slice(1, end))) {
    return [form, false, false]
  }
  const counts = form.slice(1, end)
  const rest = form.slice(end + 1)
  const comma = counts.indexOf(',')
  if (comma < 0) {
    const only = whole(counts.trim())
    return [rest, true, only !== undefined && only === count]
  }
  const from = counts.slice(0, comma).trim()
  const to = counts.slice(comma + 1).trim()
  const low = whole(from)
  const high = whole(to)
  if (from === '*' && high !== undefined) {
    return [rest, true, count <= high]
  }
  if (to === '*' && low !== undefined) {
    return [rest, true, count >= low]
  }
  if (low !== undefined && high !== undefined) {
    return [rest, true, low <= count && count <= high]
  }
  return [rest, true, false]
}

// whole reads a count as Go's strconv.Atoi does: a whole number, and
// nothing else.
function whole(s: string): number | undefined {
  return /^[+-]?\d+$/.test(s) ? Number(s) : undefined
}
`)
	return b.String()
}

// normalized is a language's tag as the forms table is keyed, as package
// lang keys its rules: in lower case, and with an underscore for a dash.
func normalized(tag string) string {
	return strings.ToLower(strings.ReplaceAll(tag, "-", "_"))
}
