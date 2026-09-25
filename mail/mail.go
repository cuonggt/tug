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
package mail

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	netmail "net/mail"
	"net/smtp"
	"net/textproto"
	"os"
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

	// Subject is its subject line.
	Subject string

	// Text is its body, in plain text.
	Text string

	// HTML is a body in HTML besides Text, for the mail programs that show
	// it; the others show Text. Optional.
	HTML string
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

// Send writes m out. A message that SMTP wouldn't send is an error here
// too, so that it's found while developing.
func (l *Log) Send(ctx context.Context, m Message) error {
	from := cmp.Or(m.From, l.From)
	if _, _, _, err := m.build(from, time.Now()); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("mail, not sent (MAIL_HOST isn't set):\n")
	fmt.Fprintf(&b, "  From: %s\n  To: %s\n  Subject: %s\n\n", cmp.Or(from, "(nobody)"), strings.Join(m.To, ", "), m.Subject)
	for line := range strings.SplitSeq(strings.TrimRight(m.Text, "\n"), "\n") {
		b.WriteString("  " + line + "\n")
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
	if len(m.To) == 0 {
		return nil, "", nil, errors.New("mail: the message is to nobody")
	}
	var to []string
	for _, s := range m.To {
		a, err := netmail.ParseAddress(s)
		if err != nil {
			return nil, "", nil, fmt.Errorf("mail: To %q isn't an address: %w", s, err)
		}
		rcpts = append(rcpts, a.Address)
		to = append(to, a.String())
	}
	header("To", strings.Join(to, ", "))
	if strings.ContainsAny(m.Subject, "\r\n") {
		return nil, "", nil, errors.New("mail: the subject has a line break in it")
	}
	header("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	header("Date", now.Format(time.RFC1123Z))
	_, domain, _ := strings.Cut(sender, "@")
	header("Message-ID", "<"+rand.Text()+"@"+cmp.Or(domain, "localhost")+">")
	header("MIME-Version", "1.0")

	var body bytes.Buffer
	if m.HTML == "" {
		header("Content-Type", "text/plain; charset=utf-8")
		header("Content-Transfer-Encoding", "quoted-printable")
		writeQP(&body, m.Text)
	} else {
		parts := multipart.NewWriter(&body)
		header("Content-Type", mime.FormatMediaType("multipart/alternative", map[string]string{"boundary": parts.Boundary()}))
		for _, p := range []struct{ contentType, content string }{{"text/plain", m.Text}, {"text/html", m.HTML}} {
			w, err := parts.CreatePart(textproto.MIMEHeader{
				"Content-Type":              {p.contentType + "; charset=utf-8"},
				"Content-Transfer-Encoding": {"quoted-printable"},
			})
			if err != nil {
				return nil, "", nil, err
			}
			writeQP(w, p.content)
		}
		parts.Close()
	}
	h.WriteString("\r\n")
	h.Write(body.Bytes())
	return h.Bytes(), sender, rcpts, nil
}

// writeQP writes s as quoted-printable, which keeps the lines short that
// servers limit, whatever the text, and turns its line breaks into CRLF.
func writeQP(w io.Writer, s string) {
	qp := quotedprintable.NewWriter(w)
	qp.Write([]byte(s))
	qp.Close()
}
