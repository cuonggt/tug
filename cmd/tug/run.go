package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

const runUsage = `usage: tug run <command> [arguments]

Runs one of the app's commands in development, as tug run migrate status:
the app, built as tug gen builds it, run as tug dev runs it, with .env
read, TUG_DEV set, and APP_URL, unless .env has one, at tug dev's address.
The arguments after the command's name are the command's, and what it
says, and its exit status, are the app's. tug run help lists the app's
commands, and tug dev serves it.
`

// runRun is tug run: the app's binary, run with a command, in place of
// serving, and with the environment tug dev gives the app, as the app
// never reads .env itself, and a command needs what it sets, its APP_KEY
// among it. With TUG_DEV, the app takes the argument for a command's, even
// with none, and its own development's, as the auth starter's account, are
// there. Its name to itself, os.Args[0], is tug run, so its help and its
// messages say tug run migrate down, not the binary's path.
func runRun(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		// The arguments are the command's, so tug run takes no flags of
		// its own: a first one that's a flag is a question.
		fmt.Fprint(stderr, runUsage)
		return flag.ErrHelp
	}
	if err := checkProject(); err != nil {
		return err
	}
	env, err := appEnv()
	if err != nil {
		return err
	}
	bin, err := buildApp(env, stderr)
	if err != nil {
		return err
	}
	cmd := exec.Command(bin, args...)
	cmd.Args[0] = "tug run"
	cmd.Env = runEnv(env)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, stdout, stderr

	// Ctrl-C reaches the app itself, which is in the terminal's process
	// group, as a command that asks something reads it: the app stops its
	// command, and tug waits for it, and hands it a SIGTERM sent to tug
	// alone.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case sig := <-signals:
				if sig != os.Interrupt {
					cmd.Process.Signal(sig)
				}
			case <-done:
				return
			}
		}
	}()
	err = cmd.Wait()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		// The app has said why, as its main's log.Fatal does.
		return exitStatus(max(exit.ExitCode(), 1))
	}
	return err
}

// runEnv is the app's environment under tug run: tug dev's, at the
// address tug dev takes, as ADDR or PORT say, or else 127.0.0.1:8080,
// without its look for a free port, as a command listens on none.
func runEnv(env []string) []string {
	addr := envValue(env, "ADDR")
	if addr == "" && envValue(env, "PORT") != "" {
		addr = ":" + envValue(env, "PORT")
	}
	switch {
	case addr == "":
		addr = "127.0.0.1:8080"
	case strings.HasPrefix(addr, ":"):
		addr = "127.0.0.1" + addr
	}
	return devEnv(env, addr)
}

// exitStatus is the status tug exits with, having said nothing more: tug
// run's, the command's, which said why itself.
type exitStatus int

func (s exitStatus) Error() string { return fmt.Sprintf("exit status %d", int(s)) }
