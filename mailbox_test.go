package tug

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/cuonggt/tug/mail"
)

// keep sends m to a mailbox, as an app under tug dev does, and returns its
// ID, from the line in the log.
func keep(t *testing.T, m mail.Message) string {
	t.Helper()
	var log bytes.Buffer
	box := &mail.Mailbox{URL: "http://localhost:8080", W: &log, From: "hello@blog.example"}
	if err := box.Send(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(log.String())
	return line[strings.LastIndex(line, "/")+1:]
}

func TestTheMailKeptUnderTugDevIsShownAtItsOwnPages(t *testing.T) {
	t.Chdir(t.TempDir())
	app := New(Config{DevTools: true})
	app.Get("/", func(c *Ctx) error { return c.String(http.StatusOK, "home") })
	link := "http://localhost:8080/verify-email/1/abc"
	id := keep(t, mail.Message{
		To:          []string{"ann@example.com"},
		Bcc:         []string{"eve@example.com"},
		Subject:     "Verify your email",
		Text:        "Follow this link:\n\n" + link + "\n",
		HTML:        `<!doctype html><html><body><a href="` + link + `">Verify my email</a><script>alert(1)</script></body></html>`,
		Attachments: []mail.Attachment{{Name: "notes.txt", Content: []byte("hello")}},
	})

	rec := serve(app, "GET", "/_tug/mail", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `<a href="/_tug/mail/`+id+`">Verify your email</a>`) || !strings.Contains(rec.Body.String(), "ann@example.com, eve@example.com") {
		t.Fatalf("the list: %d\n%s", rec.Code, rec.Body)
	}
	if rec.Header().Get("Content-Security-Policy") != mailboxPolicy || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("the list's headers %v", rec.Header())
	}

	rec = serve(app, "GET", "/_tug/mail/"+id, "")
	page := rec.Body.String()
	for _, want := range []string{
		"<tr><th>Subject</th><td>Verify your email</td></tr>",
		"<tr><th>To</th><td>&lt;ann@example.com&gt;</td></tr>",
		"<tr><th>Bcc</th><td>eve@example.com",
		`<iframe src="/_tug/mail/` + id + `/html"`,
		`<a href="` + link + `" target="_blank" rel="noreferrer">` + link + `</a>`,
		`<a href="/_tug/mail/` + id + `/files/0">notes.txt</a>`,
		`<a href="/_tug/mail/` + id + `/source">`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("no %s in the mail's page:\n%s", want, page)
		}
	}
	if strings.Contains(page, "<script") {
		t.Errorf("the mail's page runs a script:\n%s", page)
	}

	// Its HTML, which runs nothing, its links opening a tab of their own.
	rec = serve(app, "GET", "/_tug/mail/"+id+"/html", "")
	if rec.Header().Get("Content-Security-Policy") != mailPolicy || !strings.HasPrefix(rec.Body.String(), `<!doctype html><html><base target="_blank"><body><a href="`+link+`">`) {
		t.Errorf("the mail's HTML: %v\n%s", rec.Header(), rec.Body)
	}

	// Its file, as a download.
	rec = serve(app, "GET", "/_tug/mail/"+id+"/files/0", "")
	if rec.Body.String() != "hello" || rec.Header().Get("Content-Disposition") != "attachment; filename=notes.txt" || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("the file: %v\n%s", rec.Header(), rec.Body)
	}

	// Its source, as a server takes it, with no Bcc.
	rec = serve(app, "GET", "/_tug/mail/"+id+"/source", "")
	if src := rec.Body.String(); !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") || !strings.Contains(src, "Subject: Verify your email\r\n") || strings.Contains(src, "eve@example.com") {
		t.Errorf("the source:\n%s", src)
	}

	for _, path := range []string{"/_tug/mail/" + strings.Repeat("0", 26), "/_tug/mail/" + id + "/files/1", "/_tug/mail/notes.txt"} {
		if rec := serve(app, "GET", path, ""); rec.Code != http.StatusNotFound {
			t.Errorf("%s: %d", path, rec.Code)
		}
	}
	if rec := serve(app, "POST", "/_tug/mail", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("a POST got %d", rec.Code)
	}
	if rec := serve(app, "GET", "/", ""); rec.Body.String() != "home" {
		t.Errorf("the app's own route: %d %s", rec.Code, rec.Body)
	}
}

func TestWithoutDevToolsThereIsNoMailbox(t *testing.T) {
	t.Chdir(t.TempDir())
	keep(t, mail.Message{To: []string{"ann@example.com"}, Subject: "Hello", Text: "Hi."})
	app := New(Config{})
	if rec := serve(app, "GET", "/_tug/mail", ""); rec.Code != http.StatusNotFound {
		t.Errorf("without DevTools, the mailbox got %d:\n%s", rec.Code, rec.Body)
	}
}

func TestAMailsLinksOpenATabOfTheirOwn(t *testing.T) {
	for html, want := range map[string]string{
		`<!doctype html><html><head><title>Hi</title></head><body>Hi</body></html>`: `<!doctype html><html><head><base target="_blank"><title>Hi</title>`,
		`<!DOCTYPE html><HTML lang="en"><BODY>Hi</BODY></HTML>`:                     `<!DOCTYPE html><HTML lang="en"><base target="_blank"><BODY>`,
		`<!doctype html><body><header>Hi</header></body>`:                           `<!doctype html><base target="_blank"><body><header>`,
		`<p>Hi</p>`: `<base target="_blank"><p>Hi</p>`,
	} {
		if got := withBase(html); !strings.HasPrefix(got, want) {
			t.Errorf("%s:\ngot  %s\nwant %s…", html, got, want)
		}
	}
}

func TestAMailsTextLinksItsLinks(t *testing.T) {
	got := linked("Follow this link:\n\nhttps://blog.example/verify-email/1/abc\n\nor http://x.example.")
	want := []piece{
		{Text: "Follow this link:\n\n"},
		{Text: "https://blog.example/verify-email/1/abc", Link: true},
		{Text: "\n\nor "},
		{Text: "http://x.example.", Link: true},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for n := range want {
		if got[n] != want[n] {
			t.Errorf("piece %d is %+v, want %+v", n, got[n], want[n])
		}
	}
	if pieces := linked(""); len(pieces) != 0 {
		t.Errorf("no text is %+v", pieces)
	}
}
