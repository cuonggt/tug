//go:build unix

package tug

import (
	"context"
	"net"
	"syscall"
	"testing"
	"time"
)

func TestACommandsContextIsCanceledBySIGINT(t *testing.T) {
	started := make(chan struct{})
	app := New(Config{})
	app.Command("import", "import the posts", func(ctx context.Context, args []string) error {
		close(started)
		<-ctx.Done() // a long import, told to stop
		return ctx.Err()
	})
	withArgs(t, "./blog", "import")
	done := make(chan error, 1)
	go func() { done <- app.Run() }()
	<-started
	syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("Run: %v, want the command's context canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the command wasn't told to stop")
	}
}

func TestAnAppWithoutCommandsServesWhateverItsArguments(t *testing.T) {
	captureLog(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	app := New(Config{Addr: addr})
	app.Get("/", text("home"))
	withArgs(t, "./blog", "jobs")
	done := make(chan error, 1)
	go func() { done <- app.Run() }()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if conn, err := net.Dial("tcp", addr); err == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the app didn't serve")
		}
	}
	syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the app didn't stop")
	}
}
