// Package mailtest is for testing with package mail: Outbox is a Mailer
// that keeps what it's sent, for a test to read.
package mailtest

import (
	"context"
	"errors"
	"io"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/cuonggt/tug/mail"
)

// Outbox is a mail.Mailer that keeps what it's sent, for a test to read
// mail by mail, as the app's recipients would get it. It refuses a message
// an SMTP server wouldn't be sent, as mail.Log does, and it can be down,
// as a mail server can, for a number of mails. The zero value is ready to
// use; share one by pointer.
//
//	out := &mailtest.Outbox{}
//	app := newApp(env{Mailer: out, ...})
//	// register, which a job mails the link to verify the email
//	m := out.Next(t)
//	if !strings.Contains(m.Text, "/verify-email/") {
//		t.Errorf("the mail has no link:\n%s", m.Text)
//	}
type Outbox struct {
	mu      sync.Mutex
	sent    []mail.Message
	read    []bool        // which of sent Next or NextTo has returned
	down    int           // how many sends to fail
	arrived chan struct{} // closed as a mail comes, for Next and None to wait on
}

// ErrDown is what a send fails with while the Outbox is down.
var ErrDown = errors.New("mailtest: the outbox is down, as a mail server can be")

// Send keeps m, or fails with ErrDown while the Outbox is down, or with
// the error mail.Log has for a message SMTP wouldn't send.
func (o *Outbox) Send(ctx context.Context, m mail.Message) error {
	if err := (&mail.Log{W: io.Discard}).Send(ctx, m); err != nil {
		return err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.down > 0 {
		o.down--
		return ErrDown
	}
	o.sent = append(o.sent, m)
	o.read = append(o.read, false)
	if o.arrived != nil {
		close(o.arrived)
		o.arrived = nil
	}
	return nil
}

// Down has the next n sends fail with ErrDown, as they would while the
// mail server is down: a job that sends one tries again later.
func (o *Outbox) Down(n int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.down = n
}

// waitForMail is how long Next waits for a mail, which a job may send
// after the request that pushed it has been answered, and waitForNoMail
// how long None waits for one that shouldn't come.
const (
	waitForMail   = 5 * time.Second
	waitForNoMail = 100 * time.Millisecond
)

// Next returns the next mail sent that the test hasn't had, in the order
// they were sent, waiting up to five seconds for it, as a job may send it
// after the request that pushed it was answered. It fails the test when
// none comes. Call it from the test's own goroutine, as t.Fatal needs.
func (o *Outbox) Next(t testing.TB) mail.Message {
	t.Helper()
	m, ok := o.next(waitForMail, func(mail.Message) bool { return true })
	if !ok {
		t.Fatal("mailtest: no mail came in 5 seconds")
	}
	return m
}

// NextTo returns the next mail to address that the test hasn't had, to it
// or a copy, as Next does, and leaves the mail to others for Next: a
// change that mails more than one address, whose jobs run in any order,
// or a test's users each mailed in turn.
func (o *Outbox) NextTo(t testing.TB, address string) mail.Message {
	t.Helper()
	m, ok := o.next(waitForMail, func(m mail.Message) bool {
		return slices.Contains(m.To, address) || slices.Contains(m.Cc, address) || slices.Contains(m.Bcc, address)
	})
	if !ok {
		t.Fatalf("mailtest: no mail to %s came in 5 seconds", address)
	}
	return m
}

// None checks that no more mail comes: none the test hasn't had, now or in
// the next 100 milliseconds. It fails the test with the one that comes.
func (o *Outbox) None(t testing.TB) {
	t.Helper()
	if m, ok := o.next(waitForNoMail, func(mail.Message) bool { return true }); ok {
		t.Errorf("mailtest: a mail went to %v: %s", m.To, m.Subject)
	}
}

// next returns the next mail the test hasn't had that it wants, waiting up
// to wait for it, and false when none comes.
func (o *Outbox) next(wait time.Duration, wants func(mail.Message) bool) (mail.Message, bool) {
	deadline := time.After(wait)
	for {
		o.mu.Lock()
		for i, m := range o.sent {
			if !o.read[i] && wants(m) {
				o.read[i] = true
				o.mu.Unlock()
				return m, true
			}
		}
		if o.arrived == nil {
			o.arrived = make(chan struct{})
		}
		arrived := o.arrived
		o.mu.Unlock()
		select {
		case <-arrived:
		case <-deadline:
			return mail.Message{}, false
		}
	}
}

// Sent returns every mail sent so far, in order, the ones Next and NextTo
// have returned among them.
func (o *Outbox) Sent() []mail.Message {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]mail.Message(nil), o.sent...)
}
