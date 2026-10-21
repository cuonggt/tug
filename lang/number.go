package lang

import (
	"strconv"
	"strings"
)

// Number says n as the language writes a whole number, its thousands
// apart: 1234 is "1,234" in English and "1.234" in Vietnamese. It's for
// the numbers Go says itself, in a mail, a log line or a command's
// output, and Choice says a text's :count with it. A page says a number
// with the number() tug gen writes, the browser's own Intl, which knows
// every language.
func (w Words) Number(n int) string {
	return grouped(strconv.Itoa(n), numbersOf(w.locale).group)
}

// Decimal says n with digits after the point, as the language writes a
// fraction: Decimal(19.9, 1) is "19.9" in English and "19,9" in Spanish.
func (w Words) Decimal(n float64, digits int) string {
	numbers := numbersOf(w.locale)
	whole, fraction, ok := strings.Cut(strconv.FormatFloat(n, 'f', digits, 64), ".")
	whole = grouped(whole, numbers.group)
	if !ok {
		return whole
	}
	return whole + numbers.point + fraction
}

// numbers is what a language writes between a number's thousands, and
// before its fraction.
type numbers struct{ group, point string }

// numbersOf is the language's, as numberRules has them: by the whole tag,
// then by its language, as the forms of a count are found.
func numbersOf(locale string) numbers {
	l := strings.ToLower(strings.ReplaceAll(locale, "-", "_"))
	n, ok := numberRules[l]
	if !ok {
		lang, _, _ := strings.Cut(l, "_")
		n, ok = numberRules[lang]
	}
	if !ok {
		return numbers{group: ",", point: "."}
	}
	return n
}

// grouped puts group between a number's thousands, from the right, past
// the sign a number may start with.
func grouped(digits, group string) string {
	start := 0
	if digits != "" && (digits[0] == '-' || digits[0] == '+') {
		start = 1
	}
	rest := digits[start:]
	if group == "" || len(rest) < 4 {
		return digits
	}
	var b strings.Builder
	b.WriteString(digits[:start])
	for i := range len(rest) {
		if i > 0 && (len(rest)-i)%3 == 0 {
			b.WriteString(group)
		}
		b.WriteByte(rest[i])
	}
	return b.String()
}

// numberRules are what each language puts between a number's thousands
// and before its fraction, as CLDR has them. A language they don't name
// writes its numbers as English does: left out are the languages whose
// digits aren't the Latin ones Go writes, as Arabic's, and those whose
// thousands go in twos past the first three, as Hindi's lakh and crore,
// which a separator alone can't say. A page says those right all the
// same, as the browser's Intl has every language's.
var numberRules = func() map[string]numbers {
	rules := map[string]numbers{}
	add := func(group, point string, langs ...string) {
		for _, l := range langs {
			rules[l] = numbers{group: group, point: point}
		}
	}
	// A space between the thousands is a space that doesn't break, so a
	// number stays on one line; French's is the narrow one, as CLDR has it.
	const (
		nbsp   = " "
		narrow = " "
	)
	add(",", ".", "en", "es_mx", "he", "ja", "km", "ko", "lo", "ms", "my", "sw", "th", "zh", "zu")
	add(".", ",", "af", "ca", "da", "de", "el", "es", "gl", "hr", "id", "is", "it", "nl",
		"pt", "ro", "sl", "sr", "tr", "vi")
	add(nbsp, ",", "bg", "cs", "et", "fi", "hu", "lt", "lv", "nb", "nn", "no", "pl", "ru", "sk", "sv", "uk")
	add(narrow, ",", "fr")
	return rules
}()
