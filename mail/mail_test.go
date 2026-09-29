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

func TestAMessageGoesToCopiesAndItsRepliesElsewhere(t *testing.T) {
	m := Message{
		To:      []string{"Ann <ann@example.com>"},
		Cc:      []string{"bob@example.com", "ANN@example.com"},
		Bcc:     []string{"eve@example.com"},
		ReplyTo: []string{"Cara <cara@example.com>"},
		Subject: "Your question",
	}
	data, _, rcpts, err := m.build("hello@blog.example", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// Each once, and Bcc among them.
	if strings.Join(rcpts, " ") != "ann@example.com bob@example.com eve@example.com" {
		t.Errorf("sent to %v", rcpts)
	}
	msg, _ := read(t, data)
	h := msg.Header
	if h.Get("Cc") != "<bob@example.com>, <ANN@example.com>" || h.Get("Reply-To") != `"Cara" <cara@example.com>` {
		t.Errorf("Cc %q, Reply-To %q", h.Get("Cc"), h.Get("Reply-To"))
	}
	if strings.Contains(string(data), "eve") {
		t.Errorf("Bcc shows in the mail:\n%s", data)
	}
	// Bcc alone is someone to send it to, and no To.
	data, _, rcpts, err = Message{Bcc: []string{"eve@example.com"}, Subject: "News"}.build("hello@blog.example", time.Now())
	if err != nil || len(rcpts) != 1 || strings.Contains(string(data), "\r\nTo:") {
		t.Errorf("to Bcc alone: %v, %v\n%s", rcpts, err, data)
	}
}

func TestAMessageCarriesItsFilesAfterItsText(t *testing.T) {
	pdf := append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte{0, 1, 2, 255}, 100)...)
	m := Message{
		To:      []string{"ann@example.com"},
		Subject: "Your invoice",
		Text:    "It's attached.",
		HTML:    "<p>It's attached.</p>",
		Attachments: []Attachment{
			{Name: "invoice-42.pdf", Content: pdf},
			{Name: "hóa đơn \"tháng 9\".csv", ContentType: "text/csv", Content: []byte("mục,giá\n")},
		},
	}
	data, _, _, err := m.build("hello@blog.example", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(string(data), "\r\n") {
		if len(line) > 998 || strings.Contains(line, "\n") {
			t.Errorf("a line isn't one a server takes: %q", line)
		}
	}
	msg, body := read(t, data)
	mediaType, params, _ := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if mediaType != "multipart/mixed" {
		t.Fatalf("Content-Type %q", msg.Header.Get("Content-Type"))
	}
	parts := multipart.NewReader(strings.NewReader(body), params["boundary"])
	text, err := parts.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	if mediaType, _, _ := mime.ParseMediaType(text.Header.Get("Content-Type")); mediaType != "multipart/alternative" {
		t.Errorf("the first part is %q, not the text and the HTML", text.Header.Get("Content-Type"))
	}
	for _, want := range []struct {
		name, mediaType string
		content         []byte
	}{{"invoice-42.pdf", "application/pdf", pdf}, {"hóa đơn \"tháng 9\".csv", "text/csv", []byte("mục,giá\n")}} {
		p, err := parts.NextPart()
		if err != nil {
			t.Fatal(err)
		}
		disposition, dparams, _ := mime.ParseMediaType(p.Header.Get("Content-Disposition"))
		mediaType, tparams, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
		if disposition != "attachment" || dparams["filename"] != want.name || mediaType != want.mediaType || tparams["name"] != want.name {
			t.Errorf("Content-Disposition %q, Content-Type %q", p.Header.Get("Content-Disposition"), p.Header.Get("Content-Type"))
		}
		encoded, _ := io.ReadAll(p)
		for line := range strings.SplitSeq(strings.TrimRight(string(encoded), "\r\n"), "\r\n") {
			if len(line) > 76 {
				t.Errorf("a base64 line of %d characters", len(line))
			}
		}
		got, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(string(encoded), "\r\n", ""))
		if err != nil || !bytes.Equal(got, want.content) {
			t.Errorf("%s reads back as %q, %v", want.name, got, err)
		}
	}
	if _, err := parts.NextPart(); err != io.EOF {
		t.Errorf("a part more: %v", err)
	}
}

func TestAFilesNameCantEndItsHeader(t *testing.T) {
	name := "notes\r\nBcc: eve@example.com\r\n.txt"
	m := Message{To: []string{"ann@example.com"}, Attachments: []Attachment{{Name: name, Content: []byte("hi")}}}
	data, _, rcpts, err := m.build("hello@blog.example", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(rcpts) != 1 || strings.Contains(string(data), "\r\nBcc:") {
		t.Errorf("the name added a header: %v\n%s", rcpts, data)
	}
	msg, body := read(t, data)
	_, params, _ := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	parts := multipart.NewReader(strings.NewReader(body), params["boundary"])
	parts.NextPart()
	p, err := parts.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	if _, dparams, _ := mime.ParseMediaType(p.Header.Get("Content-Disposition")); dparams["filename"] != name {
		t.Errorf("the name reads back as %q", dparams["filename"])
	}
}

func TestAnAttachmentHasANameAndAType(t *testing.T) {
	for _, a := range []Attachment{
		{Content: []byte("no name")},
		{Name: "a.txt", ContentType: "text/plain\r\nBcc: eve@example.com", Content: []byte("hi")},
		{Name: "a.txt", ContentType: "not a type", Content: []byte("hi")},
	} {
		m := Message{To: []string{"ann@example.com"}, Attachments: []Attachment{a}}
		if _, _, _, err := m.build("hello@blog.example", time.Now()); err == nil {
			t.Errorf("%+v was written", a)
		}
	}
}

func TestTheAppsHeadersAreWritten(t *testing.T) {
	m := Message{To: []string{"ann@example.com"}, Headers: map[string]string{"X-Campaign": "september", "X-Note": "Tháng chín"}}
	data, _, _, err := m.build("hello@blog.example", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	msg, _ := read(t, data)
	note, _ := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("X-Note"))
	if msg.Header.Get("X-Campaign") != "september" || note != "Tháng chín" {
		t.Errorf("X-Campaign %q, X-Note %q", msg.Header.Get("X-Campaign"), msg.Header.Get("X-Note"))
	}
}

func TestTheAppsHeadersCantSayWhatTheMessageSays(t *testing.T) {
	for _, headers := range []map[string]string{
		{"Bcc": "eve@example.com"},
		{"bcc": "eve@example.com"},
		{"Reply-To": "eve@example.com"},
		{"Content-Type": "text/html"},
		{"List-Unsubscribe": "<https://eve.example>"},
		{"X-Campaign": "september\r\nBcc: eve@example.com"},
		{"X Campaign": "september"},
		{"X-Campaign:": "september"},
		{"": "september"},
	} {
		m := Message{To: []string{"ann@example.com"}, Headers: headers}
		if _, _, _, err := m.build("hello@blog.example", time.Now()); err == nil {
			t.Errorf("%q was written", headers)
		}
	}
}

func TestAMessageSaysHowToUnsubscribeInOneClick(t *testing.T) {
	link := "https://blog.example/unsubscribe/42?expires=1790000000&signature=abc"
	m := Message{To: []string{"ann@example.com"}, Subject: "What's new", Unsubscribe: link}
	data, _, _, err := m.build("hello@blog.example", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	msg, _ := read(t, data)
	if msg.Header.Get("List-Unsubscribe") != "<"+link+">" || msg.Header.Get("List-Unsubscribe-Post") != "List-Unsubscribe=One-Click" {
		t.Errorf("List-Unsubscribe %q, List-Unsubscribe-Post %q", msg.Header.Get("List-Unsubscribe"), msg.Header.Get("List-Unsubscribe-Post"))
	}
	for _, bad := range []string{
		"http://blog.example/unsubscribe/42",
		"mailto:unsubscribe@blog.example",
		"https:///unsubscribe/42",
		"https://blog.example/unsubscribe/42>, <https://eve.example",
		"https://blog.example/unsubscribe/42\r\nBcc: eve@example.com",
		"/unsubscribe/42",
	} {
		m.Unsubscribe = bad
		if _, _, _, err := m.build("hello@blog.example", time.Now()); err == nil {
			t.Errorf("%q was written", bad)
		}
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

func TestSMTPSendsToEveryCopyAndShowsNoBcc(t *testing.T) {
	f := serveSMTP(t)
	s := &SMTP{Host: f.host, Port: f.port, From: "hello@blog.example"}
	err := s.Send(context.Background(), Message{To: []string{"ann@example.com"}, Cc: []string{"bob@example.com"}, Bcc: []string{"eve@example.com"}, Subject: "Hi"})
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.Join(f.to, " ") != "TO:<ann@example.com> TO:<bob@example.com> TO:<eve@example.com>" {
		t.Errorf("RCPT %v", f.to)
	}
	if bytes.Contains(f.data, []byte("eve")) {
		t.Errorf("the mail shows Bcc:\n%s", f.data)
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

func TestLogWritesTheCopiesAndTheFiles(t *testing.T) {
	var b bytes.Buffer
	l := &Log{W: &b, From: "hello@blog.example"}
	err := l.Send(context.Background(), Message{
		To:          []string{"ann@example.com"},
		Cc:          []string{"bob@example.com"},
		Bcc:         []string{"eve@example.com"},
		ReplyTo:     []string{"cara@example.com"},
		Subject:     "Your invoice",
		Text:        "It's attached.",
		Unsubscribe: "https://blog.example/unsubscribe/42",
		Attachments: []Attachment{
			{Name: "notes.txt", Content: make([]byte, 512)},
			{Name: "invoice-42.pdf", Content: make([]byte, 48<<10)},
			{Name: "photos.zip", Content: make([]byte, 3<<19)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"  Cc: bob@example.com\n", "  Bcc: eve@example.com\n", "  Reply-To: cara@example.com\n",
		"  Unsubscribe: https://blog.example/unsubscribe/42\n",
		"  Attached: notes.txt, 512 bytes\n", "  Attached: invoice-42.pdf, 48 KB\n", "  Attached: photos.zip, 1.5 MB\n",
	} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("missing %q in:\n%s", want, b.String())
		}
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
