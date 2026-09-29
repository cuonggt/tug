package tug

import (
	"encoding/json"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"
	"time"

	"github.com/cuonggt/tug/lang"
)

// languages is an app's catalog, English by default, with Vietnamese and
// French.
func languages(t *testing.T) *lang.Catalog {
	t.Helper()
	c, err := lang.Load(fstest.MapFS{
		"vi.json": &fstest.MapFile{Data: []byte(`{
			":field is required": "Vui lòng nhập :field",
			":field must be a whole number": ":Field phải là số nguyên",
			"title": "tiêu đề",
			"age": "tuổi",
			"Not Found": "Không tìm thấy",
			"invalid JSON": "JSON không hợp lệ",
			"this link isn't valid": "Liên kết không hợp lệ",
			"too many requests: wait :count second, and try again|too many requests: wait :count seconds, and try again": "Quá nhiều yêu cầu: hãy đợi :count giây",
			"Post created": "Đã tạo bài viết",
			":count post|:count posts": ":count bài viết"
		}`)},
		"fr.json": &fstest.MapFile{Data: []byte(`{"Post created": "Article créé"}`)},
	}, "en")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestTheRequestsLanguageIsTheAppsChoiceThenTheBrowsersThenTheDefault(t *testing.T) {
	app := New(Config{
		Lang: languages(t),
		// The app's choice, as a session would keep it, in a header here.
		Locale: func(r *http.Request) string { return r.Header.Get("X-Choice") },
	})
	var got string
	app.Get("/", func(c *Ctx) error {
		got = c.Locale()
		return nil
	})
	for _, tc := range []struct {
		choice, accept, want string
	}{
		{"", "fr-FR,fr;q=0.9", "fr"},
		{"vi", "fr-FR,fr;q=0.9", "vi"},
		{"VI-vn", "", "vi"},
		{"de", "fr", "fr"}, // a choice the app has no language for
		{"", "de, ja;q=0.5", "en"},
		{"", "", "en"},
	} {
		serve(app, "GET", "/", "", "X-Choice", tc.choice, "Accept-Language", tc.accept)
		if got != tc.want {
			t.Errorf("choice %q, Accept-Language %q: %q, want %q", tc.choice, tc.accept, got, tc.want)
		}
	}

	english := New(Config{})
	english.Get("/", func(c *Ctx) error {
		got = c.Locale()
		return nil
	})
	serve(english, "GET", "/", "", "Accept-Language", "vi")
	if got != "en" {
		t.Errorf("without Config.Lang: %q", got)
	}
}

func TestTAndChoiceSayTheAppsTextsInTheRequestsLanguage(t *testing.T) {
	app := New(Config{Lang: languages(t)})
	app.Get("/", func(c *Ctx) error {
		return c.String(http.StatusOK, c.T("Post created")+" / "+c.Choice(":count post|:count posts", 3)+" / "+c.T("Welcome, :name", "name", "Ann"))
	})
	for accept, want := range map[string]string{
		"vi": "Đã tạo bài viết / 3 bài viết / Welcome, Ann",
		"fr": "Article créé / 3 posts / Welcome, Ann",
		"":   "Post created / 3 posts / Welcome, Ann",
	} {
		if rec := serve(app, "GET", "/", "", "Accept-Language", accept); rec.Body.String() != want {
			t.Errorf("%q: %q, want %q", accept, rec.Body, want)
		}
	}
}

func TestAFormsErrorsAreInTheRequestsLanguage(t *testing.T) {
	app := New(Config{Lang: languages(t)})
	app.Post("/posts", func(c *Ctx) error {
		var in struct {
			Title string `json:"title" validate:"required"`
			Age   int    `json:"age"`
			Year  int    `json:"year" label:"age"`
		}
		return c.BindValid(&in)
	})
	// A JSON body says the first value that doesn't parse, as
	// encoding/json does: a request for each.
	for body, want := range map[string]map[string]string{
		`{"age": "old"}`: {"title": "Vui lòng nhập tiêu đề", "age": "Tuổi phải là số nguyên"},
		// By its label, which the file translates too.
		`{"title": "Hi", "year": "then"}`: {"year": "Tuổi phải là số nguyên"},
	} {
		rec := serve(app, "POST", "/posts", body,
			"Content-Type", "application/json", "Accept", "application/json", "Accept-Language", "vi")
		var got struct{ Errors map[string]string }
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s: %d %s: %v", body, rec.Code, rec.Body, err)
		}
		if !maps.Equal(got.Errors, want) {
			t.Errorf("%s: %v, want %v", body, got.Errors, want)
		}
	}
}

func TestTugsOwnMessagesAreInTheRequestsLanguage(t *testing.T) {
	app := New(Config{Lang: languages(t), URL: "https://example.com", Keys: [][]byte{linkKey}})
	app.Get("/search", Limit(waits{wait: 30 * time.Second}, byHeader, text("found")))
	app.Get("/invitations/{id}", Signed(text("welcome"))).Name("invitations.accept")
	app.Post("/posts", func(c *Ctx) error {
		var in struct {
			Title string `json:"title"`
		}
		return c.Bind(&in)
	})
	for _, tc := range []struct {
		method, target, body, want string
	}{
		{"GET", "/search", "", "Quá nhiều yêu cầu: hãy đợi 30 giây"},
		{"GET", "/nowhere", "", "Không tìm thấy"},
		{"GET", "/invitations/7", "", "Liên kết không hợp lệ"},
		{"POST", "/posts", `{"title": `, "JSON không hợp lệ"},
	} {
		rec := serve(app, tc.method, tc.target, tc.body, "Accept-Language", "vi", "Content-Type", "application/json")
		if rec.Body.String() != tc.want {
			t.Errorf("%s %s: %d %q, want %q", tc.method, tc.target, rec.Code, rec.Body, tc.want)
		}
	}
	// In English, as they were.
	if rec := serve(app, "GET", "/search", ""); rec.Body.String() != "too many requests: wait 30 seconds, and try again" {
		t.Errorf("in English: %q", rec.Body)
	}
}

func TestTugGensRunWritesTheTextsTugSays(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gen.json")
	if err := New(Config{}).gen(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out struct{ Texts []string }
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		":field is required", ":field must be a whole number", "invalid JSON", tooManyRequests,
		"this link isn't valid", "this link has expired", "Not Found", "Internal Server Error",
	} {
		if !slices.Contains(out.Texts, want) {
			t.Errorf("the texts haven't %q", want)
		}
	}
}
