package lang

import "strings"

// Dir is which way a language is written: "rtl" for one written right to
// left, as Arabic, Hebrew and Persian are, and "ltr" for the rest. The
// document around a page says it, <html dir="rtl">, and what the page
// lays out follows, as a CSS property that names a start and an end does.
// A tag may say its script, and then the script decides: "ar-Latn" is
// Arabic in Latin letters, left to right, and "pa-Arab" Punjabi in Arabic
// ones, right to left.
func Dir(locale string) string {
	l := strings.ToLower(strings.ReplaceAll(locale, "-", "_"))
	subtags := strings.Split(l, "_")
	for _, s := range subtags[1:] {
		// A subtag of four letters is the script, as BCP 47 writes a tag: a
		// region is two letters or three digits.
		if len(s) == 4 {
			if rightToLeftScripts[s] {
				return "rtl"
			}
			return "ltr"
		}
	}
	if rightToLeft[subtags[0]] {
		return "rtl"
	}
	return "ltr"
}

// Dir is which way w's language is written, as the package's Dir has it.
func (w Words) Dir() string {
	return Dir(w.locale)
}

// rightToLeft are the languages written right to left, by their language
// subtag: Arabic, Aramaic, Sorani Kurdish, Divehi, Persian, Hebrew, with
// its old tag, Kashmiri, Mazanderani, N'Ko, Dari, Pashto, Sindhi, Syriac,
// Uyghur, Urdu and Yiddish, with its old tags. Kurdish's "ku" isn't one:
// Kurmanji is written in Latin letters, and Sorani, "ckb", in Arabic.
var rightToLeft = map[string]bool{
	"ar": true, "arc": true, "ckb": true, "dv": true, "fa": true, "he": true, "iw": true,
	"ks": true, "mzn": true, "nqo": true, "prs": true, "ps": true, "sd": true, "syr": true,
	"ug": true, "ur": true, "yi": true, "ji": true,
}

// rightToLeftScripts are the scripts written right to left, by their
// subtag, for a tag that names one.
var rightToLeftScripts = map[string]bool{
	"adlm": true, "arab": true, "hebr": true, "mand": true, "nkoo": true,
	"rohg": true, "samr": true, "syrc": true, "thaa": true,
}
