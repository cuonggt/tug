package lang

import (
	"strconv"
	"strings"
)

// choose picks the form of text for the count n, in locale: the first
// whose own counts, {0} or [2,*], take n, or else the one locale's rules
// have for n, or the first when there are fewer forms than that.
func choose(text string, n int, locale string) string {
	if !strings.Contains(text, "|") {
		return text
	}
	forms := strings.Split(text, "|")
	for i, f := range forms {
		rest, named, takes := interval(strings.TrimSpace(f), n)
		if takes {
			return strings.TrimSpace(rest)
		}
		if named {
			forms[i] = rest
		}
	}
	i := pluralIndex(locale, n)
	if i >= len(forms) {
		i = 0
	}
	return strings.TrimSpace(forms[i])
}

// interval reads a form that names its counts, as "{0} none", "{1,2} a
// few" or "[2,*] :count": it returns the form without them, whether it
// names any, and whether they take n. In braces or brackets alike, two
// numbers are a range, either of which can be *, and one is itself.
func interval(form string, n int) (string, bool, bool) {
	if form == "" || (form[0] != '{' && form[0] != '[') {
		return form, false, false
	}
	end := strings.IndexAny(form, "}]")
	if end < 0 || strings.ContainsAny(form[1:end], "{[") {
		return form, false, false
	}
	counts, rest := form[1:end], form[end+1:]
	from, to, isRange := strings.Cut(counts, ",")
	if !isRange {
		c, err := strconv.Atoi(strings.TrimSpace(counts))
		return rest, true, err == nil && c == n
	}
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	lo, loErr := strconv.Atoi(from)
	hi, hiErr := strconv.Atoi(to)
	switch {
	case from == "*" && hiErr == nil:
		return rest, true, n <= hi
	case to == "*" && loErr == nil:
		return rest, true, n >= lo
	case loErr == nil && hiErr == nil:
		return rest, true, lo <= n && n <= hi
	}
	return rest, true, false
}

// pluralIndex is the form a language's rules have for n, from 0: the rules
// Laravel's MessageSelector has, for the languages it names, looked up by
// the whole tag, as pt_BR's differ from pt's, and else by its language.
// A language it doesn't name has one form.
func pluralIndex(locale string, n int) int {
	if n < 0 {
		n = -n
	}
	l := strings.ToLower(strings.ReplaceAll(locale, "-", "_"))
	rule, ok := pluralRules[l]
	if !ok {
		lang, _, _ := strings.Cut(l, "_")
		rule, ok = pluralRules[lang]
	}
	if !ok {
		return 0
	}
	return rule(n)
}

// pluralRules are the rules of forms, by language.
var pluralRules = func() map[string]func(int) int {
	rules := map[string]func(int) int{}
	add := func(rule func(int) int, langs ...string) {
		for _, l := range langs {
			rules[l] = rule
		}
	}
	pick := func(b bool, yes, no int) int {
		if b {
			return yes
		}
		return no
	}
	add(func(n int) int { return 0 },
		"az", "bo", "dz", "id", "ja", "jv", "ka", "km", "kn", "ko", "ms", "th", "tr", "vi", "zh")
	add(func(n int) int { return pick(n == 1, 0, 1) },
		"af", "bn", "bg", "ca", "da", "de", "el", "en", "eo", "es", "et", "eu", "fa", "fi", "fo", "fur", "fy",
		"gl", "gu", "ha", "he", "hu", "is", "it", "ku", "lb", "ml", "mn", "mr", "nah", "nb", "ne", "nl", "nn",
		"no", "oc", "om", "or", "pa", "pap", "ps", "pt", "so", "sq", "sv", "sw", "ta", "te", "tk", "ur", "zu")
	add(func(n int) int { return pick(n == 0 || n == 1, 0, 1) },
		"am", "bh", "fil", "fr", "gun", "hi", "hy", "ln", "mg", "nso", "pt_br", "ti", "wa", "xbr")
	add(func(n int) int {
		switch {
		case n%10 == 1 && n%100 != 11:
			return 0
		case n%10 >= 2 && n%10 <= 4 && (n%100 < 10 || n%100 >= 20):
			return 1
		}
		return 2
	}, "be", "bs", "hr", "ru", "sh", "sr", "uk")
	add(func(n int) int {
		switch {
		case n == 1:
			return 0
		case n >= 2 && n <= 4:
			return 1
		}
		return 2
	}, "cs", "sk")
	add(func(n int) int {
		switch n {
		case 1:
			return 0
		case 2:
			return 1
		}
		return 2
	}, "ga")
	add(func(n int) int {
		switch {
		case n%10 == 1 && n%100 != 11:
			return 0
		case n%10 >= 2 && (n%100 < 10 || n%100 >= 20):
			return 1
		}
		return 2
	}, "lt")
	add(func(n int) int {
		switch n % 100 {
		case 1:
			return 0
		case 2:
			return 1
		case 3, 4:
			return 2
		}
		return 3
	}, "sl")
	add(func(n int) int { return pick(n%10 == 1, 0, 1) }, "mk")
	add(func(n int) int {
		switch {
		case n == 1:
			return 0
		case n == 0 || (n%100 > 1 && n%100 < 11):
			return 1
		case n%100 > 10 && n%100 < 20:
			return 2
		}
		return 3
	}, "mt")
	add(func(n int) int {
		switch {
		case n == 0:
			return 0
		case n%10 == 1 && n%100 != 11:
			return 1
		}
		return 2
	}, "lv")
	add(func(n int) int {
		switch {
		case n == 1:
			return 0
		case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
			return 1
		}
		return 2
	}, "pl")
	add(func(n int) int {
		switch n {
		case 1:
			return 0
		case 2:
			return 1
		case 8, 11:
			return 2
		}
		return 3
	}, "cy")
	add(func(n int) int {
		switch {
		case n == 1:
			return 0
		case n == 0 || (n%100 > 0 && n%100 < 20):
			return 1
		}
		return 2
	}, "ro")
	add(func(n int) int {
		switch {
		case n == 0:
			return 0
		case n == 1:
			return 1
		case n == 2:
			return 2
		case n%100 >= 3 && n%100 <= 10:
			return 3
		case n%100 >= 11 && n%100 <= 99:
			return 4
		}
		return 5
	}, "ar")
	return rules
}()
