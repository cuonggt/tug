package mailtest_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cuonggt/tug/mail"
	"github.com/cuonggt/tug/mail/mailtest"
)

var ctx = context.Background()

func to(email, subject string) mail.Message {
	return mail.Message{To: []string{email}, Subject: subject, Text: "Hi."}
}

// failures is a testing.TB that keeps what would fail a test, for the
// tests of what fails one.
type failures struct {
	testing.TB
	said []string
}

func (f *failures) Helper()           {}
func (f *failures) Fatal(args ...any) { f.said = append(f.said, fmt.Sprint(args...)) }
func (f *failures) Fatalf(format string, args ...any) {
	f.said = append(f.said, fmt.Sprintf(format, args...))
}
func (f *failures) Errorf(format string, args ...any) {
	f.said = append(f.said, fmt.Sprintf(format, args...))
}

func TestTheMailSentIsNextInOrder(t *testing.T) {
	out := &mailtest.Outbox{}
	out.Send(ctx, to("ann@example.com", "First"))
	out.Send(ctx, to("bob@example.com", "Second"))
	for _, want := range []string{"First", "Second"} {
		if m := out.Next(t); m.Subject != want {
			t.Errorf("next: %s, want %s", m.Subject, want)
		}
	}
	if sent := out.Sent(); len(sent) != 2 {
		t.Errorf("sent %d, want both, read or not", len(sent))
	}
}

func TestNextWaitsForTheJobThatSendsIt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		out := &mailtest.Outbox{}
		go func() {
			time.Sleep(2 * time.Second) // a job, after the request was answered
			out.Send(ctx, to("ann@example.com", "Verify your email"))
		}()
		if m := out.Next(t); m.Subject != "Verify your email" {
			t.Errorf("next: %s", m.Subject)
		}
	})
}

func TestNextFailsTheTestWhenNoMailComes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		out := &mailtest.Outbox{}
		f := &failures{}
		start := time.Now()
		out.Next(f)
		if len(f.said) != 1 || !strings.Contains(f.said[0], "no mail came in 5 seconds") || time.Since(start) != 5*time.Second {
			t.Errorf("after %v, failed with %q", time.Since(start), f.said)
		}
	})
}

func TestNextToTakesTheMailToAnAddressAndLeavesTheRestForNext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		out := &mailtest.Outbox{}
		out.Send(ctx, to("ann@example.com", "For Ann"))
		out.Send(ctx, to("bob@example.com", "For Bob"))
		out.Send(ctx, mail.Message{To: []string{"ann@example.com"}, Cc: []string{"carol@example.com"}, Subject: "For Carol too", Text: "Hi."})
		for address, want := range map[string]string{"bob@example.com": "For Bob", "carol@example.com": "For Carol too"} {
			if m := out.NextTo(t, address); m.Subject != want {
				t.Errorf("next to %s: %s, want %s", address, m.Subject, want)
			}
		}
		if m := out.Next(t); m.Subject != "For Ann" {
			t.Errorf("next: %s, want the one left", m.Subject)
		}
		out.None(t)

		go func() {
			time.Sleep(2 * time.Second) // a job, after the request was answered
			out.Send(ctx, to("dan@example.com", "For Dan"))
		}()
		if m := out.NextTo(t, "dan@example.com"); m.Subject != "For Dan" {
			t.Errorf("next to Dan: %s", m.Subject)
		}
		f := &failures{}
		out.NextTo(f, "eve@example.com")
		if len(f.said) != 1 || !strings.Contains(f.said[0], "no mail to eve@example.com came in 5 seconds") {
			t.Errorf("with none to Eve, failed with %q", f.said)
		}
	})
}

func TestNoneFailsTheTestForAMailThatComes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		out := &mailtest.Outbox{}
		f := &failures{}
		out.None(f)
		if len(f.said) != 0 {
			t.Errorf("failed with no mail: %q", f.said)
		}
		go func() {
			time.Sleep(50 * time.Millisecond)
			out.Send(ctx, to("eve@example.com", "Reset your password"))
		}()
		out.None(f)
		if len(f.said) != 1 || f.said[0] != "mailtest: a mail went to [eve@example.com]: Reset your password" {
			t.Errorf("failed with %q", f.said)
		}
	})
}

func TestADownOutboxFailsItsSendsAsAServerThatsDown(t *testing.T) {
	out := &mailtest.Outbox{}
	out.Down(2)
	for i := range 2 {
		if err := out.Send(ctx, to("ann@example.com", "Verify your email")); !errors.Is(err, mailtest.ErrDown) {
			t.Errorf("send %d: %v", i+1, err)
		}
	}
	if err := out.Send(ctx, to("ann@example.com", "Verify your email")); err != nil {
		t.Fatal(err)
	}
	if sent := out.Sent(); len(sent) != 1 {
		t.Errorf("kept %d, want the one sent once it was up", len(sent))
	}
}

func TestAMessageSMTPWouldntSendIsRefused(t *testing.T) {
	out := &mailtest.Outbox{}
	for _, m := range []mail.Message{
		{Subject: "to nobody"},
		{To: []string{"ann@example.com"}, Subject: "Hi\r\nBcc: eve@example.com"},
		{To: []string{"ann@example.com"}, Headers: map[string]string{"Bcc": "eve@example.com"}},
	} {
		if err := out.Send(ctx, m); err == nil {
			t.Errorf("%+v was kept", m)
		}
	}
	if len(out.Sent()) != 0 {
		t.Error("a message was kept")
	}
}
