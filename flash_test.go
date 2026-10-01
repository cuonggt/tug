package tug

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var (
	flashSaved    = Flash[string]("test-saved")
	flashCodes    = Flash[[]string]("test-codes")
	flashAnything = Flash[any]("test-anything")
)

func TestTugGenWritesTheFlashKeysDeclared(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gen.json")
	if err := New(Config{}).gen(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out struct{ Pages string }
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"  'test-anything'?: unknown\n",
		"  'test-codes'?: string[]\n",
		"  'test-saved'?: string\n",
		"flashDataType: FlashData",
	} {
		if !strings.Contains(out.Pages, want) {
			t.Errorf("pages.ts hasn't\n%s\nin\n%s", want, out.Pages)
		}
	}
}

func TestAFlashKeyDeclaredTwiceOfTwoTypesPanics(t *testing.T) {
	if !panics(func() { Flash[int]("test-saved") }) {
		t.Error("a key declared of string, then of int, didn't panic")
	}
	if panics(func() { Flash[string]("test-saved") }) {
		t.Error("a key declared twice of one type panicked")
	}
	if flashSaved.Key() != "test-saved" {
		t.Errorf("the key is %q", flashSaved.Key())
	}
}

func TestADeclaredKeyFlashesItsValueForTheNextPage(t *testing.T) {
	app, _ := formApp(t)
	app.Post("/saved", func(c *Ctx) error {
		flashSaved.Set(c, "Saved")
		flashCodes.Set(c, []string{"a-b", "c-d"})
		return c.Redirect("/posts/1")
	})
	v := &visitor{t: t, app: app}
	if rec := v.do("POST", "/saved", "{}"); rec.Code != http.StatusSeeOther {
		t.Fatalf("got %d", rec.Code)
	}
	want := map[string]any{"test-saved": "Saved", "test-codes": []any{"a-b", "c-d"}}
	if p := v.page(v.do("GET", "/posts/1", "")); !reflect.DeepEqual(p.Flash, want) {
		t.Errorf("the next page's flash is %v", p.Flash)
	}
}

func TestFlashOfADeclaredKeyTakesAValueOfItsTypeAlone(t *testing.T) {
	logs := captureLog(t)
	app, _ := formApp(t)
	app.Post("/wrong", func(c *Ctx) error {
		c.Flash("test-saved", 42)
		return c.Redirect("/posts/1")
	})
	app.Post("/right", func(c *Ctx) error {
		c.Flash("test-saved", "Saved")
		c.Flash("test-anything", map[string]int{"posts": 2}) // a key declared of any takes all
		c.Flash("test-codes", nil)
		c.Flash("undeclared", 42) // a key not declared flashes as before
		return c.Redirect("/posts/1")
	})
	v := &visitor{t: t, app: app}
	if rec := v.do("POST", "/wrong", "{}"); rec.Code != http.StatusInternalServerError || !strings.Contains(logs.String(), `flash key \"test-saved\" is declared of string, not int`) {
		t.Errorf("a declared key flashed with an int got %d, and the log says\n%s", rec.Code, logs)
	}
	if rec := v.do("POST", "/right", "{}"); rec.Code != http.StatusSeeOther {
		t.Errorf("values of the keys' types got %d", rec.Code)
	}
}
