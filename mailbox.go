package tug

import (
	_ "embed"
	"html/template"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/cuonggt/tug/internal/mailbox"
)

//go:embed mailbox.html
var mailboxHTML string

var mailboxTemplate = template.Must(template.New("mailbox").Parse(mailboxHTML))

// The pages' policies. The pages run no script, have their style in them,
// and frame a mail's HTML from the app; that HTML runs nothing, and its
// links open a tab of their own, which escapes the sandbox to run the
// app's scripts, as the link that verifies an email leads into the app.
const (
	mailboxPolicy = "default-src 'none'; style-src 'unsafe-inline'; frame-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
	mailPolicy    = "sandbox allow-popups allow-popups-to-escape-sandbox; frame-ancestors 'self'"
)

// mailboxPages are the pages of the mail the app kept under tug dev, which
// mail.Mailbox keeps in .tug/mail: the App answers them under
// Config.DevTools, before its own middleware and routes, as it does
// DevTools' endpoints, so they have no session, CSRF or policy of the
// app's, and send their own.
type mailboxPages struct {
	store *mailbox.Store
	mux   *http.ServeMux
}

func newMailboxPages(dir string) *mailboxPages {
	p := &mailboxPages{store: mailbox.New(dir), mux: http.NewServeMux()}
	p.mux.HandleFunc("GET "+mailbox.Path, p.list)
	p.mux.HandleFunc("GET "+mailbox.Path+"/{id}", p.mail)
	p.mux.HandleFunc("GET "+mailbox.Path+"/{id}/html", p.html)
	p.mux.HandleFunc("GET "+mailbox.Path+"/{id}/source", p.source)
	p.mux.HandleFunc("GET "+mailbox.Path+"/{id}/files/{n}", p.file)
	return p
}

// serves reports whether r is for one of p's pages.
func (p *mailboxPages) serves(r *http.Request) bool {
	return r.URL.Path == mailbox.Path || strings.HasPrefix(r.URL.Path, mailbox.Path+"/")
}

func (p *mailboxPages) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
	p.mux.ServeHTTP(w, r)
}

// mailRow is a mail as the list shows it.
type mailRow struct {
	ID, At, To, Subject string
}

func (p *mailboxPages) list(w http.ResponseWriter, r *http.Request) {
	var rows []mailRow
	for _, m := range p.store.List() {
		rows = append(rows, mailRow{ID: m.ID, At: when(m), To: strings.Join(recipients(m), ", "), Subject: m.Subject})
	}
	renderMailbox(w, "list", struct {
		Path string
		Mail []mailRow
	}{mailbox.Path, rows})
}

// mailView is a mail as its page shows it.
type mailView struct {
	Path, ID, At, Subject string
	Header                []mailbox.Field
	Bcc                   string
	HTML                  bool
	Text                  []piece
	Files                 []fileRow
	Unread                string // why the message doesn't read, when it doesn't
}

type fileRow struct {
	N                int
	Name, Type, Size string
}

func (p *mailboxPages) mail(w http.ResponseWriter, r *http.Request) {
	m, message, ok := p.store.Get(r.PathValue("id"))
	if !ok {
		gone(w)
		return
	}
	msg, err := mailbox.Parse(message)
	v := mailView{Path: mailbox.Path, ID: m.ID, At: when(m), Subject: m.Subject, Header: msg.Header,
		Bcc: strings.Join(m.Bcc, ", "), HTML: msg.HTML != "", Text: linked(msg.Text)}
	if err != nil {
		v.Unread = err.Error()
	}
	for n, f := range msg.Files {
		v.Files = append(v.Files, fileRow{N: n, Name: f.Name, Type: f.Type, Size: mailbox.Size(len(f.Content))})
	}
	renderMailbox(w, "mail", v)
}

// html is a mail's HTML, for the frame of its page, sandboxed.
func (p *mailboxPages) html(w http.ResponseWriter, r *http.Request) {
	msg, ok := p.message(r)
	if !ok || msg.HTML == "" {
		gone(w)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", mailPolicy)
	io.WriteString(w, withBase(msg.HTML))
}

// source is a mail as a server takes it.
func (p *mailboxPages) source(w http.ResponseWriter, r *http.Request) {
	_, message, ok := p.store.Get(r.PathValue("id"))
	if !ok {
		gone(w)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(message)
}

// file is a file a mail carries, as a download, never shown as a page.
func (p *mailboxPages) file(w http.ResponseWriter, r *http.Request) {
	msg, ok := p.message(r)
	n, err := strconv.Atoi(r.PathValue("n"))
	if !ok || err != nil || n < 0 || n >= len(msg.Files) {
		gone(w)
		return
	}
	f := msg.Files[n]
	contentType := "application/octet-stream"
	if _, _, err := mime.ParseMediaType(f.Type); err == nil {
		contentType = f.Type
	}
	h := w.Header()
	h.Set("Content-Type", contentType)
	h.Set("Content-Disposition", attachment(baseName(f.Name)))
	w.Write(f.Content)
}

// message is the message of the mail r names, read.
func (p *mailboxPages) message(r *http.Request) (mailbox.Message, bool) {
	_, message, ok := p.store.Get(r.PathValue("id"))
	if !ok {
		return mailbox.Message{}, false
	}
	msg, err := mailbox.Parse(message)
	return msg, err == nil
}

func renderMailbox(w http.ResponseWriter, name string, data any) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", mailboxPolicy)
	if err := mailboxTemplate.ExecuteTemplate(w, name, data); err != nil {
		panic(err) // only for a template that doesn't fit its data, found by the tests
	}
}

// gone answers for a mail that isn't kept, as the mailbox keeps the newest
// 100 alone.
func gone(w http.ResponseWriter) {
	http.Error(w, "That mail isn't in the mailbox, which keeps the newest 100.", http.StatusNotFound)
}

// when is when m came, in the machine's own time, as its clock shows it.
func when(m mailbox.Mail) string { return m.At.Local().Format("Mon 2 Jan, 15:04:05") }

// recipients are whom m went to, those it's To first.
func recipients(m mailbox.Mail) []string {
	return append(append(append([]string(nil), m.To...), m.Cc...), m.Bcc...)
}

// piece is a piece of a mail's text: a link, or what's between them.
type piece struct {
	Text string
	Link bool
}

var linkPattern = regexp.MustCompile(`https?://[^\s<>"]+`)

// linked is text in pieces, each link one, which the page makes a link
// of, as a mail program does, so the link that verifies an email is
// followed from the text too.
func linked(text string) []piece {
	var pieces []piece
	last := 0
	for _, at := range linkPattern.FindAllStringIndex(text, -1) {
		if at[0] > last {
			pieces = append(pieces, piece{Text: text[last:at[0]]})
		}
		pieces = append(pieces, piece{Text: text[at[0]:at[1]], Link: true})
		last = at[1]
	}
	if last < len(text) {
		pieces = append(pieces, piece{Text: text[last:]})
	}
	return pieces
}

// withBase puts <base target="_blank"> in html's head, so a link in the
// mail opens in a tab of its own, rather than in the page's frame: after
// its <head>, or its <html>, from which the browser makes one, or its
// doctype, or else first.
func withBase(html string) string {
	const base = `<base target="_blank">`
	lower := strings.ToLower(html)
	for _, tag := range []string{"<head", "<html", "<!doctype"} {
		at := strings.Index(lower, tag)
		if at < 0 || at+len(tag) >= len(lower) || !strings.ContainsRune("> \t\r\n", rune(lower[at+len(tag)])) {
			continue
		}
		if end := strings.IndexByte(lower[at:], '>'); end >= 0 {
			return html[:at+end+1] + base + html[at+end+1:]
		}
	}
	return base + html
}
