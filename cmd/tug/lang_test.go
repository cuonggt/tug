package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
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
