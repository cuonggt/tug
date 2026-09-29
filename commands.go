package tug

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// command is one of the app's commands: see Command.
type command struct {
	name, summary string
	run           func(ctx context.Context, args []string) error
}

// Command adds a command to the app's binary, for what it does besides
// serving: a fix to its data, an import, a user made an admin. Run, given
// the command's name as its first argument, runs it in place of serving,
// with the arguments after the name and a context canceled on SIGINT or
// SIGTERM, and returns its error: ./blog users:admin ann@example.com runs
// users:admin with ann@example.com. ./blog help lists the app's commands,
// with their summaries, and a name that isn't one is an error that says
// so.
//
//	app.Command("users:admin", "make a user an admin: users:admin <email>", func(ctx context.Context, args []string) error {
//		if len(args) != 1 {
//			return errors.New("users:admin takes the user's email")
//		}
//		return users.MakeAdmin(ctx, args[0])
//	})
//
// A command runs in the app main made, with its routes, for the links it
// makes, its queue, for the jobs it pushes, and its database, but neither
// the server nor what Go runs: a job it pushes runs on the instances that
// serve. Its arguments are its own, to read with package flag or by hand.
// An app without commands serves whatever its arguments, and under tug
// gen, Run writes the types, whatever they are.
//
// Command panics on a name that's taken, help, which lists them, or one
// that isn't a word of letters, digits and :-_, and once the app is
// serving.
func (a *App) Command(name, summary string, run func(ctx context.Context, args []string) error) {
	a.mustNotServe()
	switch {
	case !commandName(name):
		panic(fmt.Sprintf("tug: a command is named with letters, digits and :-_, as users:admin, not %q", name))
	case name == "help":
		panic("tug: help is the command that lists the others")
	}
	for _, cmd := range a.commands {
		if cmd.name == name {
			panic(fmt.Sprintf("tug: the app has a command named %s already", name))
		}
	}
	a.commands = append(a.commands, command{name, summary, run})
}

// commandName reports whether name can name a command: a word, which a
// flag, starting with a dash, isn't.
func commandName(name string) bool {
	if name == "" || name[0] == '-' {
		return false
	}
	for _, r := range name {
		if !('a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9' || strings.ContainsRune(":-_", r)) {
			return false
		}
	}
	return true
}

// runCommand runs the command args names, with the rest of args, as the
// binary bin, in place of serving, or lists them for help, to out.
func (a *App) runCommand(ctx context.Context, bin string, args []string, out io.Writer) error {
	switch name := args[0]; name {
	case "help", "-h", "-help", "--help":
		a.listCommands(bin, out)
		return nil
	default:
		for _, cmd := range a.commands {
			if cmd.name == name {
				return cmd.run(ctx, args[1:])
			}
		}
		return fmt.Errorf("%s has no command %q: %s help lists the ones it has", bin, name, bin)
	}
}

// listCommands says what the binary bin runs: the server, without a
// command, or one of the app's, each with its summary.
func (a *App) listCommands(bin string, out io.Writer) {
	width := len("help")
	for _, cmd := range a.commands {
		width = max(width, len(cmd.name))
	}
	fmt.Fprintf(out, "Without a command, %s serves. Its commands:\n\n", bin)
	for _, cmd := range a.commands {
		fmt.Fprintf(out, "  %s %-*s  %s\n", bin, width, cmd.name, cmd.summary)
	}
	fmt.Fprintf(out, "  %s %-*s  %s\n", bin, width, "help", "list these commands")
}
