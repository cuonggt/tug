package lang

import "testing"

func TestALanguageIsWrittenOneWayOrTheOther(t *testing.T) {
	for locale, want := range map[string]string{
		"en": "ltr", "vi": "ltr", "pt-BR": "ltr", "ku": "ltr",
		"ar": "rtl", "he": "rtl", "fa": "rtl", "ur": "rtl", "ckb": "rtl", "ps": "rtl",
		// A region doesn't change which way its language is written.
		"ar-EG": "rtl", "he-IL": "rtl", "fa-AF": "rtl",
		// A tag that says its script is written the script's way, whatever
		// the language usually is.
		"ar-Latn": "ltr", "ar-Latn-DZ": "ltr", "pa-Arab": "rtl", "uz-Arab-AF": "rtl",
		"sr-Cyrl-RS": "ltr",
		// A language the table doesn't name, and none at all, read as
		// English does: a document says something either way.
		"zz": "ltr", "": "ltr",
	} {
		if got := Dir(locale); got != want {
			t.Errorf("Dir(%q) = %q, want %q", locale, got, want)
		}
	}

	// The zero Words are English's, and a catalog's language says its own.
	var en Words
	if got := en.Dir(); got != "ltr" {
		t.Errorf("the zero Words are written %q", got)
	}
	c := catalog(t, "en", map[string]string{"ar.json": `{}`})
	if got := c.In("ar").Dir(); got != "rtl" {
		t.Errorf("Arabic is written %q", got)
	}
	if got := c.In("ar").Texts().Dir; got != "rtl" {
		t.Errorf("Arabic's texts say they're written %q", got)
	}
}
