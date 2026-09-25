package mail

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	netmail "net/mail"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"
)

// read parses what build wrote, as a mail program would.
func read(t *testing.T, data []byte) (*netmail.Message, string) {
	t.Helper()
	msg, err := netmail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(msg.Body)
	if err != nil {
		t.Fatal(err)
	}
	return msg, string(body)
}

func TestAMessageIsWrittenAsMailProgramsReadIt(t *testing.T) {
	link := "http://127.0.0.1:8080/reset-password/abc.def?email=ann%40example.com&" + strings.Repeat("x", 80)
	m := Message{To: []string{"Ann Lee <ann@example.com>"}, Subject: "Đặt lại mật khẩu", Text: "Hi Ann,\n\n" + link + "\n"}
	data, sender, rcpts, err := m.build(`"Blog" <hello@blog.example>`, time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if sender != "hello@blog.example" || len(rcpts) != 1 || rcpts[0] != "ann@example.com" {
		t.Errorf("envelope %s → %v", sender, rcpts)
	}
	for line := range strings.SplitSeq(strings.TrimSuffix(string(data), "\r\n"), "\r\n") {
		if strings.Contains(line, "\n") || len(line) > 998 {
			t.Errorf("a line isn't one a server takes: %q", line)
		}
	}
	msg, body := read(t, data)
	h := msg.Header
	if subject, _ := new(mime.WordDecoder).DecodeHeader(h.Get("Subject")); subject != "Đặt lại mật khẩu" {
		t.Errorf("Subject %q, decoded %q", h.Get("Subject"), subject)
	}
	if h.Get("From") != `"Blog" <hello@blog.example>` || h.Get("To") != `"Ann Lee" <ann@example.com>` {
		t.Errorf("From %q, To %q", h.Get("From"), h.Get("To"))
	}
	if h.Get("Date") != "Fri, 25 Sep 2026 10:00:00 +0000" || !strings.HasSuffix(h.Get("Message-Id"), "@blog.example>") {
		t.Errorf("Date %q, Message-ID %q", h.Get("Date"), h.Get("Message-Id"))
	}
	text, err := io.ReadAll(qpReader(body))
	if err != nil || string(text) != "Hi Ann,\r\n\r\n"+link+"\r\n" {
		t.Errorf("the body reads back as %q (%v)", text, err)
	}
}

func TestAMessageWithHTMLHasBothBodies(t *testing.T) {
	m := Message{To: []string{"ann@example.com"}, Subject: "Hi", Text: "plain", HTML: "<p>rich</p>"}
	data, _, _, err := m.build("hello@blog.example", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	msg, body := read(t, data)
	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/alternative" {
		t.Fatalf("Content-Type %q", msg.Header.Get("Content-Type"))
	}
	parts := multipart.NewReader(strings.NewReader(body), params["boundary"])
	for _, want := range []struct{ contentType, body string }{{"text/plain; charset=utf-8", "plain"}, {"text/html; charset=utf-8", "<p>rich</p>"}} {
		p, err := parts.NextPart() // decodes quoted-printable itself
		if err != nil {
			t.Fatal(err)
		}
		got, _ := io.ReadAll(p)
		if p.Header.Get("Content-Type") != want.contentType || string(got) != want.body {
			t.Errorf("part %q: %q", p.Header.Get("Content-Type"), got)
		}
	}
}

func TestAMessageCantSmuggleInHeaders(t *testing.T) {
	for _, m := range []Message{
		{To: []string{"ann@example.com"}, Subject: "Hi\r\nBcc: eve@example.com"},
		{To: []string{"ann@example.com\r\nBcc: eve@example.com"}, Subject: "Hi"},
		{To: []string{"not an address"}, Subject: "Hi"},
		{Subject: "to nobody"},
	} {
		if _, _, _, err := m.build("hello@blog.example", time.Now()); err == nil {
			t.Errorf("%+v was written", m)
		}
	}
	if _, _, _, err := (Message{To: []string{"ann@example.com"}}).build("Blog\r\nBcc: eve@example.com", time.Now()); err == nil {
		t.Error("a From with a line break was written")
	}
}

// fakeSMTP is an SMTP server that keeps what it's sent.
type fakeSMTP struct {
	host string
	port int

	mu         sync.Mutex
	auth, from string
	to         []string
	data       []byte
}

func serveSMTP(t *testing.T) *fakeSMTP {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	f := &fakeSMTP{host: "127.0.0.1", port: ln.Addr().(*net.TCPAddr).Port}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		tp := textproto.NewConn(conn)
		tp.PrintfLine("220 fake ESMTP")
		for {
			line, err := tp.ReadLine()
			if err != nil {
				return
			}
			verb, arg, _ := strings.Cut(line, " ")
			f.mu.Lock()
			switch verb {
			case "EHLO":
				tp.PrintfLine("250-fake")
				tp.PrintfLine("250 AUTH PLAIN")
			case "AUTH":
				f.auth = arg
				tp.PrintfLine("235 welcome")
			case "MAIL":
				f.from = arg
				tp.PrintfLine("250 ok")
			case "RCPT":
				f.to = append(f.to, arg)
				tp.PrintfLine("250 ok")
			case "DATA":
				tp.PrintfLine("354 go on")
				f.data, _ = tp.ReadDotBytes()
				tp.PrintfLine("250 queued")
			case "QUIT":
				tp.PrintfLine("221 bye")
				f.mu.Unlock()
				return
			default:
				tp.PrintfLine("502 what?")
			}
			f.mu.Unlock()
		}
	}()
	return f
}

func TestSMTPSendsThroughTheServer(t *testing.T) {
	f := serveSMTP(t)
	s := &SMTP{Host: f.host, Port: f.port, Username: "blog", Password: "s3cret", From: "Blog <hello@blog.example>"}
	err := s.Send(context.Background(), Message{To: []string{"Ann <ann@example.com>", "bob@example.com"}, Subject: "Hi", Text: "Hello.\n.\nA line that's a dot."})
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if want := "PLAIN " + base64.StdEncoding.EncodeToString([]byte("\x00blog\x00s3cret")); f.auth != want {
		t.Errorf("AUTH %q, want %q", f.auth, want)
	}
	if f.from != "FROM:<hello@blog.example>" || strings.Join(f.to, " ") != "TO:<ann@example.com> TO:<bob@example.com>" {
		t.Errorf("MAIL %s, RCPT %v", f.from, f.to)
	}
	// The server reads LFs for CRLFs, and the end of DATA puts one after the text.
	msg, body := read(t, f.data)
	text, _ := io.ReadAll(qpReader(body))
	if msg.Header.Get("Subject") != "Hi" || string(text) != "Hello.\n.\nA line that's a dot.\n" {
		t.Errorf("sent %q", f.data)
	}
}

func TestSMTPGivesUpWhenTheContextEnds(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() { // a server that takes the connection and never says hello
		if conn, err := ln.Accept(); err == nil {
			defer conn.Close()
			time.Sleep(5 * time.Second)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	s := &SMTP{Host: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port, From: "hello@blog.example"}
	if err := s.Send(ctx, Message{To: []string{"ann@example.com"}, Subject: "Hi"}); err == nil {
		t.Fatal("sent to a server that never answered")
	}
	if waited := time.Since(start); waited > 2*time.Second {
		t.Errorf("waited %v for a context of 100ms", waited)
	}
}

func TestLogWritesTheMessageOutWithItsLinksWhole(t *testing.T) {
	var b bytes.Buffer
	link := "http://127.0.0.1:8080/reset-password/" + strings.Repeat("x", 100)
	l := &Log{W: &b, From: "hello@blog.example"}
	if err := l.Send(context.Background(), Message{To: []string{"ann@example.com"}, Subject: "Reset your password", Text: "Here:\n\n" + link + "\n"}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"To: ann@example.com", "Subject: Reset your password", "\n  " + link + "\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if err := l.Send(context.Background(), Message{Subject: "to nobody"}); err == nil {
		t.Error("a message to nobody was written out")
	}
}

func TestTheEnvironmentPicksTheMailer(t *testing.T) {
	t.Setenv("MAIL_HOST", "")
	t.Setenv("MAIL_FROM_ADDRESS", "hello@blog.example")
	t.Setenv("MAIL_FROM_NAME", "The Blog")
	m, err := FromEnv()
	if l, ok := m.(*Log); err != nil || !ok || l.From != `"The Blog" <hello@blog.example>` {
		t.Fatalf("without MAIL_HOST: %#v, %v", m, err)
	}

	t.Setenv("MAIL_HOST", "smtp.example.com")
	t.Setenv("MAIL_USERNAME", "blog")
	t.Setenv("MAIL_PASSWORD", "s3cret")
	m, err = FromEnv()
	if s, ok := m.(*SMTP); err != nil || !ok || s.Host != "smtp.example.com" || s.Port != 587 || s.Username != "blog" || s.Password != "s3cret" {
		t.Fatalf("with MAIL_HOST: %#v, %v", m, err)
	}

	t.Setenv("MAIL_PORT", "smtp")
	if _, err := FromEnv(); err == nil {
		t.Error("a MAIL_PORT that isn't a number was taken")
	}
	t.Setenv("MAIL_PORT", "465")
	t.Setenv("MAIL_FROM_ADDRESS", "")
	if _, err := FromEnv(); err == nil {
		t.Error("SMTP without a From was taken")
	}
}

func qpReader(s string) io.Reader {
	return quotedprintable.NewReader(strings.NewReader(s))
}
