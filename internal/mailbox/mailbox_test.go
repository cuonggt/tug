package mailbox

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestMailIsKeptAndListedNewestFirst(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mail")
	s := New(dir)
	if list := s.List(); len(list) != 0 {
		t.Fatalf("a mailbox not yet made lists %v", list)
	}
	first, err := s.Keep(Mail{To: []string{"ann@example.com"}, Subject: "First"}, []byte("Subject: First\r\n\r\nHello"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Keep(Mail{To: []string{"bob@example.com"}, Bcc: []string{"eve@example.com"}, Subject: "Second"}, []byte("Subject: Second\r\n\r\nAgain"))
	if err != nil {
		t.Fatal(err)
	}
	list := New(dir).List()
	if len(list) != 2 || list[0].ID != second || list[1].ID != first || list[0].Subject != "Second" || !slices.Equal(list[0].Bcc, []string{"eve@example.com"}) || list[0].At.IsZero() {
		t.Fatalf("listed %+v", list)
	}
	m, message, ok := New(dir).Get(first)
	if !ok || m.Subject != "First" || string(message) != "Subject: First\r\n\r\nHello" {
		t.Errorf("got %+v, %q, %v", m, message, ok)
	}
	for _, id := range []string{"", "../mail", strings.Repeat("Z", 26), first + ".json"} {
		if _, _, ok := s.Get(id); ok {
			t.Errorf("%q got a mail", id)
		}
	}
	// Nothing is left of the files written on the way.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 4 {
		t.Errorf("the mailbox's directory has %v", entries)
	}
}

func TestTheNewest100AreKept(t *testing.T) {
	s := New(t.TempDir())
	var ids []string
	for range 103 {
		id, err := s.Keep(Mail{Subject: "One of many"}, []byte("Subject: One of many\r\n\r\n."))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	list := s.List()
	if len(list) != 100 || list[0].ID != ids[102] || list[99].ID != ids[3] {
		t.Fatalf("kept %d, from %s", len(list), list[len(list)-1].ID)
	}
	if _, _, ok := s.Get(ids[2]); ok {
		t.Error("the fourth newest past 100 is still kept")
	}
}

// mixed is a message as package mail writes one, with its text, HTML and a
// file, its subject an encoded word.
const mixed = "From: <hello@blog.example>\r\n" +
	"To: <ann@example.com>\r\n" +
	"Subject: =?utf-8?q?Caf=C3=A9_tomorrow?=\r\n" +
	"MIME-Version: 1.0\r\n" +
	"Content-Type: multipart/mixed; boundary=outer\r\n" +
	"\r\n" +
	"--outer\r\n" +
	"Content-Type: multipart/alternative; boundary=inner\r\n" +
	"\r\n" +
	"--inner\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"Content-Transfer-Encoding: quoted-printable\r\n" +
	"\r\n" +
	"See you at the caf=C3=A9: https://blog.example/verify-email/1/x=\r\n" +
	"yz\r\n" +
	"--inner\r\n" +
	"Content-Type: text/html; charset=utf-8\r\n" +
	"Content-Transfer-Encoding: quoted-printable\r\n" +
	"\r\n" +
	"<p>See you at the <b>caf=C3=A9</b>.</p>\r\n" +
	"--inner--\r\n" +
	"--outer\r\n" +
	"Content-Type: text/plain; name=\"notes.txt\"\r\n" +
	"Content-Disposition: attachment; filename=\"notes.txt\"\r\n" +
	"Content-Transfer-Encoding: base64\r\n" +
	"\r\n" +
	"aGVsbG8=\r\n" +
	"--outer--\r\n"

func TestAMessageIsReadAsAMailProgramReadsIt(t *testing.T) {
	msg, err := Parse([]byte(mixed))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range msg.Header {
		names = append(names, f.Name)
	}
	if !slices.Equal(names, []string{"From", "To", "Subject", "MIME-Version", "Content-Type"}) || msg.Header[2].Value != "Café tomorrow" {
		t.Errorf("headers %+v", msg.Header)
	}
	if msg.Text != "See you at the café: https://blog.example/verify-email/1/xyz" || msg.HTML != "<p>See you at the <b>café</b>.</p>" {
		t.Errorf("text %q, HTML %q", msg.Text, msg.HTML)
	}
	if len(msg.Files) != 1 || msg.Files[0].Name != "notes.txt" || msg.Files[0].Type != "text/plain" || string(msg.Files[0].Content) != "hello" {
		t.Errorf("files %+v", msg.Files)
	}

	// A text alone, quoted-printable, as a mail without HTML or files is.
	msg, err = Parse([]byte("Subject: Hi\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\nA line=\r\n, whole"))
	if err != nil || msg.Text != "A line, whole" || msg.HTML != "" || len(msg.Files) != 0 {
		t.Errorf("got %+v, %v", msg, err)
	}
}
