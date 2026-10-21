package lang

import "testing"

func TestANumberIsWrittenAsItsLanguageWritesOne(t *testing.T) {
	c := catalog(t, "en", map[string]string{
		"vi.json":    `{}`,
		"fr.json":    `{}`,
		"ru.json":    `{}`,
		"ja.json":    `{}`,
		"pt_br.json": `{}`,
		"es_mx.json": `{}`,
		"ar.json":    `{}`,
	})
	for _, tc := range []struct {
		locale string
		n      int
		want   string
	}{
		{"en", 0, "0"}, {"en", 999, "999"}, {"en", 1234, "1,234"}, {"en", 1234567, "1,234,567"},
		{"en", -1234, "-1,234"},
		{"vi", 1234567, "1.234.567"},
		{"ja", 1234567, "1,234,567"},
		{"fr", 1234567, "1 234 567"},
		{"ru", 1234567, "1 234 567"},
		// A region takes its language's, and one of its own wins.
		{"pt-BR", 1234, "1.234"},
		{"es-MX", 1234, "1,234"},
		// A language the rules don't name writes its numbers as English
		// does, as its own digits aren't the ones Go writes.
		{"ar", 1234, "1,234"},
	} {
		if got := c.In(tc.locale).Number(tc.n); got != tc.want {
			t.Errorf("%s, %d: %q, want %q", tc.locale, tc.n, got, tc.want)
		}
	}
	// The zero Words are English's.
	var en Words
	if got := en.Number(1234); got != "1,234" {
		t.Errorf("the zero Words: %q", got)
	}
}

func TestADecimalTakesItsLanguagesPoint(t *testing.T) {
	c := catalog(t, "en", map[string]string{"es.json": `{}`, "vi.json": `{}`})
	for _, tc := range []struct {
		locale string
		n      float64
		digits int
		want   string
	}{
		{"en", 19.9, 1, "19.9"},
		{"es", 19.9, 1, "19,9"},
		{"es", 1234.5, 2, "1.234,50"},
		{"vi", 1234.567, 0, "1.235"},
		{"en", -0.5, 1, "-0.5"},
	} {
		if got := c.In(tc.locale).Decimal(tc.n, tc.digits); got != tc.want {
			t.Errorf("%s, %v: %q, want %q", tc.locale, tc.n, got, tc.want)
		}
	}
}

func TestACountIsSaidAThousandApart(t *testing.T) {
	c := catalog(t, "en", map[string]string{
		"vi.json": `{":count post|:count posts": ":count bài viết"}`,
	})
	if got := c.In("vi").Choice(":count post|:count posts", 1234); got != "1.234 bài viết" {
		t.Errorf("Vietnamese says %q", got)
	}
	if got := c.In("en").Choice(":count post|:count posts", 1234); got != "1,234 posts" {
		t.Errorf("English says %q", got)
	}
	// A value of the app's own is written as it comes: a year or an ID is
	// not a quantity.
	if got := c.In("vi").T("Since :year", "year", 2026); got != "Since 2026" {
		t.Errorf("a value of the app's: %q", got)
	}
}
