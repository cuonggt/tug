package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// write puts files, by path under dir, in dir.
func write(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLangFindsWhatTheAppsGoSaysToAPerson(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{
		"main.go": "package main\n\n" +
			"type PostInput struct {\n" +
			"\tTitle    string `json:\"title\" validate:\"required\"`\n" +
			"\tStartsAt string `json:\"starts_at\" validate:\"required\"`\n" +
			"\tEmail    string `json:\"email\" label:\"email address\" validate:\"required\"`\n" +
			"\tConfirm  string `json:\"password_confirmation\" validate:\"eqfield=Password\"`\n" +
			"\tPassword string\n" +
			"\tPage     int    `query:\"page\"`\n" +
			"\tSecret   string `json:\"-\" validate:\"required\"`\n" +
			"\tPhoto    any    `form:\"photo\" validate:\"file_type=image/png image/jpeg\"`\n" +
			"\tShown    string `json:\"shown\"` // a prop, never bound or checked\n" +
			"}\n\n" +
			"func handle(c ctx, n int) {\n" +
			"\tc.Flash(\"success\", c.T(\"Post created\"))\n" +
			"\tc.Flash(\"success\", c.Choice(`:count post deleted|:count posts deleted`, n))\n" +
			"\tc.T(\"Welcome, :name\", \"name\", \"Ann\")\n" +
			"\tc.T(title)\n" +
			"}\n",
		"jobs/mail.go":            "package jobs\n\nfunc mail(w words) string { return w.In(locale).T(\"Your receipt\") }\n",
		"main_test.go":            "package main\n\nfunc testing() { c.T(\"only in a test\") }\n",
		"node_modules/x/x.go":     "package x\n\nfunc x() { c.T(\"a dependency's\") }\n",
		".tug/gen.go":             "package gen\n\nfunc x() { c.T(\"tug's own\") }\n",
		"resources/js/app.tsx":    "t('the frontend')",
		"internal/forms/forms.go": "package forms\n\ntype In struct {\n\tUserID int `form:\"userID\"`\n}\n",
	})
	got, err := appTexts(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"title", "starts at", "email address", "password confirmation", "password", "page",
		"photo", "a PNG or JPEG image", "Post created", ":count post deleted|:count posts deleted",
		"Welcome, :name", "Your receipt", "user id",
	}
	for _, w := range want {
		if !slices.Contains(got, w) {
			t.Errorf("appTexts found no %q in %q", w, got)
		}
	}
	for _, not := range []string{"only in a test", "a dependency's", "tug's own", "the frontend", "shown", "secret", "Secret"} {
		if slices.Contains(got, not) {
			t.Errorf("appTexts found %q", not)
		}
	}
}

func TestLangFindsWhatTheAppsPagesSayToAPerson(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{
		"resources/js/pages/Home.tsx": "import { useLang } from '../lib/lang'\n" +
			"const { t, choice } = useLang()\n" +
			"export default function Home({ posts }: Props) {\n" +
			"  const title = t('Say hello')\n" +
			"  const count = choice(':count post|:count posts', posts.length)\n" +
			"  const named = t('Welcome, :name', { name: 'Ann' })\n" +
			"  const quoted = t(\"What's your name?\")\n" +
			"  const escaped = t('Don\\'t go')\n" +
			"  const backticks = t(`Your receipt`)\n" +
			"  const made = t(message)\n" +
			"  const built = t(`a ${kind} of post`)\n" +
			"  return format('a date') + insert('a row') + obj.t('a method of something')\n" +
			"}\n",
		"resources/js/pages/Home.vue":    "<template>{{ t('In a template') }}</template>\n",
		"resources/js/pages/Home.svelte": "<p>{t('In a component')}</p>\n",
		"resources/js/lib/lang.ts":       "export function useLang() { return { t, choice } }\n",
		"resources/js/tug/lang.ts":       "export function t(texts: Texts, text: string) { return t('tug gen wrote this') }\n",
		"resources/js/README.md":         "t('not a page')\n",
		"node_modules/pkg/index.js":      "t('a dependency\\'s')\n",
		"resources/js/.cache/stale.tsx":  "t('left behind')\n",
	})
	got, err := pageTexts(filepath.Join(dir, "resources", "js"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"Say hello", ":count post|:count posts", "Welcome, :name", "What's your name?",
		"Don't go", "Your receipt", "In a template", "In a component",
	}
	for _, w := range want {
		if !slices.Contains(got, w) {
			t.Errorf("pageTexts found no %q in %q", w, got)
		}
	}
	for _, not := range []string{
		"tug gen wrote this", "not a page", "a dependency's", "left behind",
		"a date", "a row", "a method of something",
	} {
		if slices.Contains(got, not) {
			t.Errorf("pageTexts found %q", not)
		}
	}
	// A text made as the page runs isn't one to translate: nothing of
	// t(message) or of a template with a ${} in it.
	for _, text := range got {
		if strings.Contains(text, "${") || text == "" {
			t.Errorf("pageTexts found %q", text)
		}
	}
}

// An app with no frontend of its own has nothing for pageTexts to read.
func TestLangTakesAnAppWithNoPages(t *testing.T) {
	got, err := pageTexts(filepath.Join(t.TempDir(), "resources", "js"))
	if err != nil || len(got) != 0 {
		t.Errorf("pageTexts: %q, %v", got, err)
	}
}

func TestLangAddsTheTextsAFileHasntGotAndKeepsWhatItHas(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lang", "vi.json")
	added, total, err := addTexts(path, []string{":field is required", "Post created", "a <b> & c"})
	if err != nil || added != 3 || total != 3 {
		t.Fatalf("a new file: %d added, %d in all, %v", added, total, err)
	}
	data, _ := os.ReadFile(path)
	if want := "{\n  \":field is required\": \"\",\n  \"Post created\": \"\",\n  \"a <b> & c\": \"\"\n}\n"; string(data) != want {
		t.Errorf("the file is\n%s\nwant\n%s", data, want)
	}

	os.WriteFile(path, []byte(`{"Post created": "Đã tạo bài viết", "An old text": "Một văn bản cũ"}`), 0o644)
	added, total, err = addTexts(path, []string{":field is required", "Post created", "Post deleted"})
	if err != nil || added != 2 || total != 4 {
		t.Fatalf("an old file: %d added, %d in all, %v", added, total, err)
	}
	var texts map[string]string
	data, _ = os.ReadFile(path)
	if err := json.Unmarshal(data, &texts); err != nil {
		t.Fatal(err)
	}
	for text, want := range map[string]string{"Post created": "Đã tạo bài viết", "An old text": "Một văn bản cũ", "Post deleted": "", ":field is required": ""} {
		if got, ok := texts[text]; !ok || got != want {
			t.Errorf("%q: %q, %v; want %q", text, got, ok, want)
		}
	}

	// With nothing to add, the file is left as it was written.
	os.WriteFile(path, []byte(`{"Post created":"Đã tạo bài viết"}`), 0o644)
	if added, _, err := addTexts(path, []string{"Post created"}); err != nil || added != 0 {
		t.Fatalf("nothing new: %d added, %v", added, err)
	}
	if data, _ := os.ReadFile(path); string(data) != `{"Post created":"Đã tạo bài viết"}` {
		t.Errorf("the file was written again: %s", data)
	}

	os.WriteFile(path, []byte(`["not", "texts"]`), 0o644)
	if _, _, err := addTexts(path, []string{"x"}); err == nil {
		t.Error("a file that isn't texts was written over")
	}
}

func TestLangTakesALanguagesTag(t *testing.T) {
	for tag, ok := range map[string]bool{"vi": true, "pt-BR": true, "pt_BR": true, "zh-Hant": true, "e": false, "../vi": false, "vi.json": false, "": false, "english": false} {
		if languageTag(tag) != ok {
			t.Errorf("languageTag(%q) = %v", tag, !ok)
		}
	}
}

// TestTheStartersFrontendsSayTheSameWords keeps the auth starter one app in
// three frontends: a word said in React's pages is said in Vue's and
// Svelte's, so one language file translates any of them. Two exceptions: a
// frontend's own name, which its pages say, and the home page's title,
// which Svelte alone says in its page, as Svelte keeps the document's
// title by its own Head.
func TestTheStartersFrontendsSayTheSameWords(t *testing.T) {
	frontends := []string{"react-auth", "vue-auth", "svelte-auth"}
	names := strings.NewReplacer("React", "<frontend>", "Vue", "<frontend>", "Svelte", "<frontend>")
	said := map[string]map[string]bool{}
	for _, front := range frontends {
		texts, err := pageTexts(filepath.Join(front, "resources", "js"))
		if err != nil {
			t.Fatal(err)
		}
		if len(texts) < 50 {
			t.Fatalf("%s says %d texts, which is too few to be its words", front, len(texts))
		}
		said[front] = map[string]bool{}
		for _, text := range texts {
			said[front][names.Replace(text)] = true
		}
	}
	for _, front := range frontends {
		for _, other := range frontends {
			if front == other {
				continue
			}
			for text := range said[front] {
				if text == "Welcome" {
					continue // the home page's title: see above
				}
				if !said[other][text] {
					t.Errorf("%s says %q and %s doesn't", front, text, other)
				}
			}
		}
	}
}

func TestACheckSaysWhatsLeftToTranslate(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{"lang/vi.json": `{
  "Post created": "Đã tạo bài viết",
  "Log in": "",
  ":count posts": "bài viết",
  "Hello": "Xin chào :name",
  "Welcome, :name": "Chào mừng, :Name",
  "one post|many posts": "{0} không có bài viết|[1,*] :count bài viết",
  ":count comment|:count comments": ":COUNT bình luận",
  "Visit https://example.com at 10:30": "Ghé https://example.com lúc 10:30",
  "An old text": "Một văn bản cũ"
}`})
	texts := []string{
		"Post created", "Log in", "Delete your account", ":count posts", "Hello", "Welcome, :name",
		"one post|many posts", ":count comment|:count comments", "Visit https://example.com at 10:30", "", "Post created",
	}
	c, err := checkTexts(filepath.Join(dir, "lang", "vi.json"), texts)
	if err != nil {
		t.Fatal(err)
	}
	// By the English, in its order: a text not there, one with no words, a
	// placeholder lost, and one the English hasn't; :Name is :name, :COUNT
	// :count, a choice's translation may say :count, which Choice fills, and
	// a URL or a time has none.
	want := []string{
		`":count posts" loses :count`,
		`"Delete your account" isn't there`,
		`"Hello" says :name, which the English doesn't`,
		`"Log in" has no words`,
	}
	if !slices.Equal(c.problems, want) {
		t.Errorf("the problems:\n%s\nwant\n%s", strings.Join(c.problems, "\n"), strings.Join(want, "\n"))
	}
	if want := []string{`"An old text" is said nowhere now, for its translator to delete`}; !slices.Equal(c.notes, want) {
		t.Errorf("the notes: %q", c.notes)
	}
	if c.translate != 2 || c.fix != 2 || c.said != 9 {
		t.Errorf("%d to translate, %d to fix, of %d", c.translate, c.fix, c.said)
	}
	if got := placeholdersOf("Welcome, :name, to :site", "Chào mừng"); got != "loses :name and :site" {
		t.Errorf("two lost: %q", got)
	}
	if got := placeholdersOf("Welcome, :name", "Chào mừng :user"); got != "loses :name, and says :user, which the English doesn't" {
		t.Errorf("one lost and one made up: %q", got)
	}
}

// An English file words only what it would have the app say otherwise: a
// text it hasn't, or leaves empty, is the English, as the app has it.
func TestAnEnglishFileIsCheckedForItsPlaceholdersAlone(t *testing.T) {
	dir := t.TempDir()
	file := `{"Log in": "Sign in", "Post created": "", ":count posts": "Some posts"}`
	write(t, dir, map[string]string{"lang/en.json": file, "lang/en-GB.json": file})
	for _, name := range []string{"en.json", "en-GB.json"} {
		c, err := checkTexts(filepath.Join(dir, "lang", name), []string{"Log in", "Post created", "Delete your account", ":count posts"})
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{`":count posts" loses :count`}; !slices.Equal(c.problems, want) || c.translate != 0 || c.fix != 1 {
			t.Errorf("%s: %q, %d to translate, %d to fix", name, c.problems, c.translate, c.fix)
		}
	}
}

func TestTheCheckReadsOneLanguageOrEvery(t *testing.T) {
	dir := t.TempDir()
	if files, err := langFiles(dir, ""); err != nil || len(files) != 0 {
		t.Errorf("an app with no lang/: %q, %v", files, err)
	}
	write(t, dir, map[string]string{
		"lang/vi.json": "{}", "lang/pt-BR.json": "{}", "lang/.gitkeep": "", "lang/notes.txt": "", "lang/settings.json": "{}",
	})
	every, err := langFiles(dir, "")
	if want := []string{filepath.Join(dir, "lang", "pt-BR.json"), filepath.Join(dir, "lang", "vi.json")}; err != nil || !slices.Equal(every, want) {
		t.Errorf("every language: %q, %v", every, err)
	}
	if one, err := langFiles(dir, "vi"); err != nil || !slices.Equal(one, []string{filepath.Join(dir, "lang", "vi.json")}) {
		t.Errorf("one: %q, %v", one, err)
	}
	if _, err := langFiles(dir, "ja"); err == nil || err.Error() != "lang/ja.json isn't there: tug lang ja makes it" {
		t.Errorf("a language with no file: %v", err)
	}
}

func TestTheCheckSaysWhatsLeftAndFailsForIt(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{
		"lang/ja.json": `{"Post created": "投稿を作成しました", "Hello, :name": "こんにちは"}`,
		"lang/vi.json": `{"Post created": "Đã tạo bài viết", "Log in": "Đăng nhập", "Hello, :name": "Xin chào, :name"}`,
	})
	files := []string{filepath.Join(dir, "lang", "ja.json"), filepath.Join(dir, "lang", "vi.json")}
	texts := []string{"Post created", "Log in", "Hello, :name"}
	var out bytes.Buffer
	err := checkLanguages(&out, files, texts)
	want := `lang/ja.json: "Hello, :name" loses :name
lang/ja.json: "Log in" isn't there
lang/ja.json: 1 of 3 texts to translate, and 1 to fix
lang/vi.json: every text translated, 3 of them
`
	if out.String() != want || err == nil || err.Error() != "1 text to translate, and 1 to fix" {
		t.Errorf("said\n%s\nwant\n%s\nand %v", out.String(), want, err)
	}
	write(t, dir, map[string]string{"lang/ja.json": `{"Post created": "投稿を作成しました", "Hello, :name": "こんにちは、:name", "Log in": "ログイン", "Old": "古い"}`})
	out.Reset()
	if err := checkLanguages(&out, files, texts); err != nil || !strings.Contains(out.String(), `lang/ja.json: "Old" is said nowhere now`) {
		t.Errorf("translated whole, with a note: %v\n%s", err, out.String())
	}
}
