// Package mail sends email: through an SMTP server, or, while developing,
// out to the terminal, where a link in it can be clicked.
//
//	mailer, err := mail.FromEnv() // SMTP when MAIL_HOST is set, and Log when it isn't
//	...
//	err = mailer.Send(ctx, mail.Message{
//		To:      []string{user.Email},
//		Subject: "Reset your password",
//		Text:    "Follow this link to choose a new password:\n\n" + link,
//	})
//
// A message can go to copies, Cc and Bcc, have its replies go elsewhere,
// ReplyTo, carry files, and say how to unsubscribe from it in one click:
//
//	err = mailer.Send(ctx, mail.Message{
//		To:          []string{customer.Email},
//		Subject:     "Your invoice",
//		Text:        "It's attached.",
//		Attachments: []mail.Attachment{{Name: "invoice-42.pdf", Content: pdf}},
//	})
//
// Package mailtest has a Mailer for tests, which keeps what it's sent.
package mail

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"maps"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/http"
	netmail "net/mail"
	"net/smtp"
	"net/textproto"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Message is an email.
type Message struct {
	// From is who it's from, as "Ann <ann@example.com>" or the address
	// alone. Default the Mailer's From.
	From string

	// To is who it's for, each written as From is.
	To []string

	// Cc is who else gets it, whom everyone who gets it sees, as To.
	// Optional.
	Cc []string

	// Bcc is who else gets it, whom no one else sees: they're told to the
	// server, and in no header. Optional.
	Bcc []string

	// ReplyTo is where a reply goes, in place of From: whoever filled in a
	// contact form, say. Optional.
	ReplyTo []string

	// Subject is its subject line.
	Subject string

	// Text is its body, in plain text.
	Text string

	// HTML is a body in HTML besides Text, for the mail programs that show
	// it; the others show Text. Optional.
	HTML string

	// Attachments are the files it carries, after Text and HTML, such as
	// an invoice's PDF. Optional.
	Attachments []Attachment

	// Headers are headers of the app's, such as X-Campaign for a
	// provider's reports, beside the ones the fields above set, which they
	// can't be: From, To, Subject, a Content- header and the rest.
	// Optional.
	Headers map[string]string

	// Unsubscribe is an https link that unsubscribes the recipient, for
	// mail sent in bulk, such as a newsletter: the mail carries it as RFC
	// 8058 has it, in List-Unsubscribe, with List-Unsubscribe-Post, and a
	// mail program shows a button that POSTs to it, with the body
	// List-Unsubscribe=One-Click. A link tug's SignedURL makes, to a route
	// tug.Signed wraps, is one no one can forge. Optional.
	Unsubscribe string
}

// Attachment is a file a Message carries.
type Attachment struct {
	// Name is the name the recipient saves it as, such as "invoice-42.pdf".
	Name string

	// ContentType is what it is, such as "application/pdf". Default the
	// type its first bytes say, as http.DetectContentType has it.
	ContentType string

	// Content is the file, whole.
	Content []byte
}

// A Mailer sends email.
type Mailer interface {
	Send(ctx context.Context, m Message) error
}

// FromEnv returns the Mailer the environment sets up, with the variables
// Laravel uses: an SMTP through MAIL_HOST and MAIL_PORT, logging in with
// MAIL_USERNAME and MAIL_PASSWORD when they're set, or, without a
// MAIL_HOST, a Log. Mail is from MAIL_FROM_ADDRESS, by MAIL_FROM_NAME.
func FromEnv() (Mailer, error) {
	from := os.Getenv("MAIL_FROM_ADDRESS")
	if name := os.Getenv("MAIL_FROM_NAME"); name != "" && from != "" {
		from = (&netmail.Address{Name: name, Address: from}).String()
	}
	host := os.Getenv("MAIL_HOST")
	if host == "" {
		return &Log{From: from}, nil
	}
	port := 587
	if p := os.Getenv("MAIL_PORT"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n <= 0 || n > 65535 {
			return nil, fmt.Errorf("mail: MAIL_PORT is %q, which isn't a port", p)
		}
		port = n
	}
	if from == "" {
		return nil, errors.New("mail: MAIL_FROM_ADDRESS isn't set, and mail sent through MAIL_HOST has to say who it's from")
	}
	return &SMTP{Host: host, Port: port, Username: os.Getenv("MAIL_USERNAME"), Password: os.Getenv("MAIL_PASSWORD"), From: from}, nil
}

// SMTP sends email through an SMTP server. The connection is encrypted
// when the server offers STARTTLS, and from the start on port 465; a
// password only goes over an encrypted connection, or to this machine.
type SMTP struct {
	Host string
	Port int // default 587

	// Username and Password log in to the server, for one that wants them.
	Username string
	Password string

	// From is who a message is from when it doesn't say.
	From string
}

// Send sends m. A server that's slow to answer has until ctx is done, or
// a minute when ctx has no deadline.
func (s *SMTP) Send(ctx context.Context, m Message) error {
	from := cmp.Or(m.From, s.From)
	if from == "" {
		return errors.New("mail: the message has no From, and neither has the SMTP")
	}
	data, sender, rcpts, err := m.build(from, time.Now())
	if err != nil {
		return err
	}
	port := cmp.Or(s.Port, 587)
	addr := net.JoinHostPort(s.Host, strconv.Itoa(port))
	tlsConfig := &tls.Config{ServerName: s.Host}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	if port == 465 {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: tlsConfig}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("mail: %w", err)
	}
	// net/smtp takes no context: a deadline on the connection, and closing
	// it when ctx ends, keep a server that stops answering from holding on.
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(time.Minute)
	}
	conn.SetDeadline(deadline)
	defer context.AfterFunc(ctx, func() { conn.Close() })()

	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("mail: %w", err)
	}
	defer c.Close()
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("mail: %w", err)
		}
	}
	if s.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", s.Username, s.Password, s.Host)); err != nil {
			return fmt.Errorf("mail: logging in to %s: %w", s.Host, err)
		}
	}
	if err := c.Mail(sender); err != nil {
		return fmt.Errorf("mail: %w", err)
	}
	for _, rcpt := range rcpts {
		if err := c.Rcpt(rcpt); err != nil {
			return fmt.Errorf("mail: to %s: %w", rcpt, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mail: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("mail: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mail: %w", err)
	}
	return c.Quit()
}

// Log writes email out in full rather than sending it, for development:
// to W, or when W is nil to the standard error, which tug dev shows.
type Log struct {
	W    io.Writer
	From string

	mu sync.Mutex
}

// Send writes m out: who it's from and to, Bcc too, which the mail
// itself doesn't show, its subject, its link to unsubscribe, its text, and
// the names and sizes of its files. A message that SMTP wouldn't send is
// an error here too, so that it's found while developing.
func (l *Log) Send(ctx context.Context, m Message) error {
	from := cmp.Or(m.From, l.From)
	if _, _, _, err := m.build(from, time.Now()); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("mail, not sent (MAIL_HOST isn't set):\n")
	fmt.Fprintf(&b, "  From: %s\n", cmp.Or(from, "(nobody)"))
	for _, f := range []struct {
		name string
		list []string
	}{{"To", m.To}, {"Cc", m.Cc}, {"Bcc", m.Bcc}, {"Reply-To", m.ReplyTo}} {
		if len(f.list) > 0 {
			fmt.Fprintf(&b, "  %s: %s\n", f.name, strings.Join(f.list, ", "))
		}
	}
	fmt.Fprintf(&b, "  Subject: %s\n", m.Subject)
	if m.Unsubscribe != "" {
		fmt.Fprintf(&b, "  Unsubscribe: %s\n", m.Unsubscribe)
	}
	b.WriteString("\n")
	for line := range strings.SplitSeq(strings.TrimRight(m.Text, "\n"), "\n") {
		b.WriteString("  " + line + "\n")
	}
	if len(m.Attachments) > 0 {
		b.WriteString("\n")
	}
	for _, a := range m.Attachments {
		fmt.Fprintf(&b, "  Attached: %s, %s\n", a.Name, size(len(a.Content)))
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	w := l.W
	if w == nil {
		w = os.Stderr
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// build writes m as a mail server takes it, and returns the addresses the
// server sends it from and to. from may be empty only for a Log.
func (m Message) build(from string, now time.Time) (data []byte, sender string, rcpts []string, err error) {
	var h bytes.Buffer
	header := func(name, value string) {
		h.WriteString(name + ": " + value + "\r\n")
	}
	if from != "" {
		a, err := netmail.ParseAddress(from)
		if err != nil {
			return nil, "", nil, fmt.Errorf("mail: From %q isn't an address: %w", from, err)
		}
		sender = a.Address
		header("From", a.String())
	}
	// The server sends it to To, Cc and Bcc, each address once, and only
	// Bcc is in no header.
	seen := map[string]bool{}
	for _, f := range []struct {
		name        string
		list        []string
		shown, gets bool
	}{{"To", m.To, true, true}, {"Cc", m.Cc, true, true}, {"Bcc", m.Bcc, false, true}, {"Reply-To", m.ReplyTo, true, false}} {
		var written []string
		for _, s := range f.list {
			a, err := netmail.ParseAddress(s)
			if err != nil {
				return nil, "", nil, fmt.Errorf("mail: %s %q isn't an address: %w", f.name, s, err)
			}
			written = append(written, a.String())
			if k := strings.ToLower(a.Address); f.gets && !seen[k] {
				seen[k] = true
				rcpts = append(rcpts, a.Address)
			}
		}
		if f.shown && len(written) > 0 {
			header(f.name, strings.Join(written, ", "))
		}
	}
	if len(rcpts) == 0 {
		return nil, "", nil, errors.New("mail: the message is to nobody")
	}
	if strings.ContainsAny(m.Subject, "\r\n") {
		return nil, "", nil, errors.New("mail: the subject has a line break in it")
	}
	header("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	header("Date", now.Format(time.RFC1123Z))
	_, domain, _ := strings.Cut(sender, "@")
	header("Message-ID", "<"+rand.Text()+"@"+cmp.Or(domain, "localhost")+">")
	header("MIME-Version", "1.0")
	for _, name := range slices.Sorted(maps.Keys(m.Headers)) {
		value := m.Headers[name]
		if err := checkHeader(name, value); err != nil {
			return nil, "", nil, err
		}
		header(name, mime.QEncoding.Encode("utf-8", value))
	}
	if m.Unsubscribe != "" {
		u, err := url.Parse(m.Unsubscribe)
		if err != nil || u.Scheme != "https" || u.Host == "" || strings.ContainsAny(m.Unsubscribe, "<> \t\r\n") {
			return nil, "", nil, fmt.Errorf("mail: Unsubscribe is an https link, as RFC 8058 has it, not %q", m.Unsubscribe)
		}
		header("List-Unsubscribe", "<"+m.Unsubscribe+">")
		header("List-Unsubscribe-Post", "List-Unsubscribe=One-Click")
	}

	var body bytes.Buffer
	content, err := m.writeBody(&body)
	if err != nil {
		return nil, "", nil, err
	}
	for _, name := range []string{"Content-Type", "Content-Transfer-Encoding"} {
		if v := content.Get(name); v != "" {
			header(name, v)
		}
	}
	h.WriteString("\r\n")
	h.Write(body.Bytes())
	return h.Bytes(), sender, rcpts, nil
}

// ownHeaders are the headers a Message's fields set, which Headers can't,
// with every Content- header, as the body's are the Message's to say.
var ownHeaders = []string{"From", "To", "Cc", "Bcc", "Reply-To", "Subject", "Date", "Message-ID", "MIME-Version", "List-Unsubscribe", "List-Unsubscribe-Post"}

// checkHeader fails for a header of Headers' that isn't one, or that a
// Message's fields set, and for a value with a line break, which would
// end the header and start another, such as a Bcc from a form.
func checkHeader(name, value string) error {
	for _, b := range []byte(name) {
		// RFC 5322's name: printable ASCII, without the colon that ends it.
		if b < '!' || b > '~' || b == ':' {
			return fmt.Errorf("mail: %q isn't a header's name, such as X-Campaign", name)
		}
	}
	if name == "" || strings.HasPrefix(strings.ToLower(name), "content-") || slices.ContainsFunc(ownHeaders, func(own string) bool { return strings.EqualFold(own, name) }) {
		return fmt.Errorf("mail: Headers can't set %q, which the Message's own fields do", name)
	}
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("mail: the header %s has a line break in it", name)
	}
	return nil
}

// writeBody writes m's body to w, and returns the headers that say what it
// is: its text, and its HTML as well, then its files, each a part of the
// whole, multipart/mixed, when it has any.
func (m Message) writeBody(w io.Writer) (textproto.MIMEHeader, error) {
	if len(m.Attachments) == 0 {
		return m.writeText(w)
	}
	mixed := multipart.NewWriter(w)
	var text bytes.Buffer
	h, err := m.writeText(&text)
	if err != nil {
		return nil, err
	}
	part, err := mixed.CreatePart(h)
	if err != nil {
		return nil, err
	}
	part.Write(text.Bytes())
	for _, a := range m.Attachments {
		h, err := a.header()
		if err != nil {
			return nil, err
		}
		part, err := mixed.CreatePart(h)
		if err != nil {
			return nil, err
		}
		writeBase64(part, a.Content)
	}
	if err := mixed.Close(); err != nil {
		return nil, err
	}
	return textproto.MIMEHeader{"Content-Type": {mime.FormatMediaType("multipart/mixed", map[string]string{"boundary": mixed.Boundary()})}}, nil
}

// writeText writes m's text to w as quoted-printable, beside its HTML as
// multipart/alternative when it has some, and returns the headers that say
// which.
func (m Message) writeText(w io.Writer) (textproto.MIMEHeader, error) {
	if m.HTML == "" {
		writeQP(w, m.Text)
		return textproto.MIMEHeader{"Content-Type": {"text/plain; charset=utf-8"}, "Content-Transfer-Encoding": {"quoted-printable"}}, nil
	}
	parts := multipart.NewWriter(w)
	for _, p := range []struct{ contentType, content string }{{"text/plain", m.Text}, {"text/html", m.HTML}} {
		part, err := parts.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {p.contentType + "; charset=utf-8"},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		if err != nil {
			return nil, err
		}
		writeQP(part, p.content)
	}
	if err := parts.Close(); err != nil {
		return nil, err
	}
	return textproto.MIMEHeader{"Content-Type": {mime.FormatMediaType("multipart/alternative", map[string]string{"boundary": parts.Boundary()})}}, nil
}

// header is what the part that carries a's file says of it: its type, and
// its name, both written by mime.FormatMediaType, which encodes a name
// with anything but printable ASCII in it, a line break among them, as
// RFC 2231 has it. The name is in the type as well, for the mail programs
// that read it there.
func (a Attachment) header() (textproto.MIMEHeader, error) {
	if a.Name == "" {
		return nil, errors.New("mail: an attachment has no Name, which it's saved as")
	}
	mediaType, params, err := mime.ParseMediaType(cmp.Or(a.ContentType, http.DetectContentType(a.Content)))
	if err != nil {
		return nil, fmt.Errorf("mail: %s's ContentType, %q, isn't a type, such as application/pdf", a.Name, a.ContentType)
	}
	params["name"] = a.Name
	return textproto.MIMEHeader{
		"Content-Type":              {mime.FormatMediaType(mediaType, params)},
		"Content-Disposition":       {mime.FormatMediaType("attachment", map[string]string{"filename": a.Name})},
		"Content-Transfer-Encoding": {"base64"},
	}, nil
}

// writeBase64 writes b in base64, in lines of 76 characters, as MIME has
// them.
func writeBase64(w io.Writer, b []byte) {
	s := base64.StdEncoding.EncodeToString(b)
	for len(s) > 76 {
		io.WriteString(w, s[:76]+"\r\n")
		s = s[76:]
	}
	io.WriteString(w, s)
}

// size says n bytes as a person reads it: 512 bytes, 48 KB, 1.5 MB, a KB
// being 1,024 bytes, as validate's file_max has it.
func size(n int) string {
	switch {
	case n < 1<<10:
		return fmt.Sprintf("%d bytes", n)
	case n < 1<<20:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	}
}

// writeQP writes s as quoted-printable, which keeps the lines short that
// servers limit, whatever the text, and turns its line breaks into CRLF.
func writeQP(w io.Writer, s string) {
	qp := quotedprintable.NewWriter(w)
	qp.Write([]byte(s))
	qp.Close()
}
