package lang

import (
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

// catalog loads files, by name, with def as the default language.
func catalog(t *testing.T, def string, files map[string]string) *Catalog {
	t.Helper()
	fsys := fstest.MapFS{}
	for name, data := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(data)}
	}
	c, err := Load(fsys, def)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLoadReadsALanguageFromEachFile(t *testing.T) {
	c := catalog(t, "en", map[string]string{
		"vi.json":        `{"Post created": "Đã tạo bài viết"}`,
		"pt_br.json":     `{}`,
		".gitkeep":       "",
		"README.md":      "the languages",
		"drafts/fr.json": `{}`,
	})
	if got, want := c.Languages(), []string{"en", "pt-BR", "vi"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Languages() = %q, want %q", got, want)
	}
	if c.Default() != "en" {
		t.Errorf("Default() = %q", c.Default())
	}
	if got := c.In("vi").T("Post created"); got != "Đã tạo bài viết" {
		t.Errorf("vi says %q", got)
	}
}

func TestADefaultLanguageThatIsntEnglishHasAFileAndIsTheOnlyOneWithoutOthers(t *testing.T) {
	c := catalog(t, "vi", map[string]string{"vi.json": `{}`})
	if got := c.Languages(); !reflect.DeepEqual(got, []string{"vi"}) {
		t.Errorf("Languages() = %q, want vi alone: English isn't offered without its file", got)
	}
	if got := c.Match("en-US,en;q=0.9"); got != "vi" {
		t.Errorf("a browser in English gets %q, want the default", got)
	}
}

func TestLoadSaysWhatsWrongWithTheFiles(t *testing.T) {
	for name, tc := range map[string]struct {
		def   string
		files map[string]string
		want  string
	}{
		"no file for the default":  {"vi", map[string]string{"fr.json": `{}`}, "vi.json"},
		"a file not named for one": {"en", map[string]string{"english.json": `{}`}, "english.json"},
		"a file that isn't JSON":   {"en", map[string]string{"vi.json": `{"a": `}, "vi.json"},
		"a text that isn't text":   {"en", map[string]string{"vi.json": `{"a": 1}`}, "vi.json"},
		"two files for one":        {"en", map[string]string{"vi.json": `{}`, "VI.json": `{}`}, "vi"},
		"no default":               {"", nil, "default language"},
	} {
		fsys := fstest.MapFS{}
		for n, data := range tc.files {
			fsys[n] = &fstest.MapFile{Data: []byte(data)}
		}
		if _, err := Load(fsys, tc.def); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want an error that names %q", name, err, tc.want)
		}
	}
}

func TestATextALanguageDoesntHaveSaysItselfInEnglish(t *testing.T) {
	c := catalog(t, "en", map[string]string{"vi.json": `{"Post created": "Đã tạo bài viết", "Post deleted": ""}`})
	vi := c.In("vi")
	for text, want := range map[string]string{
		"Post created": "Đã tạo bài viết",
		"Post deleted": "Post deleted", // there, but not yet translated
		"Post saved":   "Post saved",
	} {
		if got := vi.T(text); got != want {
			t.Errorf("T(%q) = %q, want %q", text, got, want)
		}
	}
	if got := (Words{}).T("Post created"); got != "Post created" {
		t.Errorf("the zero Words say %q", got)
	}
	var none *Catalog
	if got := none.In("vi").T("Post created"); got != "Post created" || none.In("vi").Locale() != "en" {
		t.Errorf("a nil catalog says %q in %s", got, none.In("vi").Locale())
	}
}

func TestPlaceholdersAreFilledByName(t *testing.T) {
	var en Words
	for _, tc := range []struct {
		text string
		args []any
		want string
	}{
		{"Welcome, :name", []any{"name", "Ann"}, "Welcome, Ann"},
		{":field is required", []any{"field", "title"}, "title is required"},
		{":Field is required", []any{"field", "title"}, "Title is required"},
		{":FIELD is required", []any{"field", "title"}, "TITLE is required"},
		{":field and :fieldname", []any{"field", "a"}, "a and :fieldname"},
		{"at 10:30, :who", []any{"who", "Ann"}, "at 10:30, Ann"},
		{"a value can't name one: :name", []any{"name", ":other", "other", "x"}, "a value can't name one: :other"},
		{"ends with a colon:", []any{"name", "x"}, "ends with a colon:"},
		{":count items", []any{"count", 3}, "3 items"},
		{":name without its value", []any{"name"}, ":name without its value"},
		{"Chào :name", []any{"name", "Ánh"}, "Chào Ánh"},
		{":Name", []any{"name", "ánh"}, "Ánh"},
	} {
		if got := en.T(tc.text, tc.args...); got != tc.want {
			t.Errorf("T(%q, %v) = %q, want %q", tc.text, tc.args, got, tc.want)
		}
	}
}

func TestChoiceSaysTheFormALanguagesRulesHaveForTheCount(t *testing.T) {
	c := catalog(t, "en", map[string]string{
		"vi.json": `{":count post|:count posts": ":count bài viết"}`,
		"ru.json": `{":count post|:count posts": ":count пост|:count поста|:count постов"}`,
		"fr.json": `{":count post|:count posts": ":count article|:count articles"}`,
	})
	text := ":count post|:count posts"
	for _, tc := range []struct {
		locale string
		n      int
		want   string
	}{
		{"en", 0, "0 posts"}, {"en", 1, "1 post"}, {"en", 2, "2 posts"},
		{"vi", 1, "1 bài viết"}, {"vi", 5, "5 bài viết"},
		{"ru", 1, "1 пост"}, {"ru", 3, "3 поста"}, {"ru", 5, "5 постов"},
		{"ru", 11, "11 постов"}, {"ru", 21, "21 пост"}, {"ru", 22, "22 поста"},
		{"fr", 0, "0 article"}, {"fr", 1, "1 article"}, {"fr", 2, "2 articles"},
	} {
		if got := c.In(tc.locale).Choice(text, tc.n); got != tc.want {
			t.Errorf("%s, %d: %q, want %q", tc.locale, tc.n, got, tc.want)
		}
	}
	// A text Vietnamese doesn't have is English's, with English's rules:
	// Vietnamese's one form would say "5 comment".
	if got := c.In("vi").Choice(":count comment|:count comments", 5); got != "5 comments" {
		t.Errorf("an English fallback: %q", got)
	}
}

func TestAFormCanNameTheCountsItsFor(t *testing.T) {
	var en Words
	text := "{0} no posts|{1} one post|[2,19] :count posts|[20,*] lots of posts"
	for n, want := range map[int]string{0: "no posts", 1: "one post", 7: "7 posts", 20: "lots of posts", 500: "lots of posts"} {
		if got := en.Choice(text, n); got != want {
			t.Errorf("%d: %q, want %q", n, got, want)
		}
	}
	// A form's counts are read in order, before the rules.
	if got := en.Choice("{0} none|:count item|:count items", 0); got != "none" {
		t.Errorf("with a form for 0: %q", got)
	}
}

func TestMatchPicksTheBrowsersFirstLanguageTheAppHas(t *testing.T) {
	c := catalog(t, "en", map[string]string{"vi.json": `{}`, "pt-BR.json": `{}`})
	for header, want := range map[string]string{
		"vi-VN,vi;q=0.9,en;q=0.8":   "vi",
		"fr-FR, fr;q=0.9, en;q=0.5": "en",
		"fr":                        "en",
		"en-GB":                     "en",
		"pt":                        "pt-BR",
		"pt-PT":                     "pt-BR",
		"en;q=0.5, vi;q=0.9":        "vi",
		"vi;q=0, en":                "en",
		"vi;q=abc, pt":              "pt-BR",
		"vi;q=2":                    "en",
		"*":                         "en",
		"de, *;q=0.5":               "en",
		"":                          "en",
		",,;q=,":                    "en",
		"VI":                        "vi",
	} {
		if got := c.Match(header); got != want {
			t.Errorf("Match(%q) = %q, want %q", header, got, want)
		}
	}
}

func TestFindAndInResolveALanguageTheCatalogHas(t *testing.T) {
	c := catalog(t, "en", map[string]string{"vi.json": `{}`, "pt-BR.json": `{}`})
	for tag, want := range map[string]string{"vi": "vi", "VI-vn": "vi", "pt_br": "pt-BR", "pt": "pt-BR", "en-US": "en"} {
		if got, ok := c.Find(tag); !ok || got != want {
			t.Errorf("Find(%q) = %q, %v; want %q", tag, got, ok, want)
		}
	}
	if got, ok := c.Find("fr"); ok {
		t.Errorf("Find(fr) = %q, want none", got)
	}
	if got := c.In("fr").Locale(); got != "en" {
		t.Errorf("In(fr) is %q, want the default", got)
	}
}

func TestEnglishsOwnFileAndARegionsLanguageAreWhereAMissingTextComesFrom(t *testing.T) {
	c := catalog(t, "en", map[string]string{
		"en.json":    `{":field is required": "Please fill in :field"}`,
		"pt.json":    `{"Post created": "Artigo criado", "Post saved": "Artigo guardado"}`,
		"pt-BR.json": `{"Post saved": "Artigo salvo"}`,
	})
	if got := c.In("pt-BR").T("Post saved"); got != "Artigo salvo" {
		t.Errorf("pt-BR's own: %q", got)
	}
	if got := c.In("pt-BR").T("Post created"); got != "Artigo criado" {
		t.Errorf("pt-BR, from pt: %q", got)
	}
	if got := c.In("pt-BR").T(":field is required", "field", "title"); got != "Please fill in title" {
		t.Errorf("pt-BR, from the app's English: %q", got)
	}
}
