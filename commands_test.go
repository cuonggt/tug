package tug

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// withArgs has the process's arguments be args, as the binary was run
// with them, for the rest of the test.
func withArgs(t *testing.T, args ...string) {
	t.Helper()
	old := os.Args
	os.Args = args
	t.Cleanup(func() { os.Args = old })
}

func TestACommandRunsInPlaceOfServingWithTheArgumentsAfterItsName(t *testing.T) {
	var got []string
	var ctx context.Context
	app := New(Config{Addr: "256.0.0.1:1"}) // an address that Run fails on, were it to listen
	app.Get("/", text("home"))
	started := false
	app.Go(func(context.Context) error {
		started = true
		return nil
	})
	app.Command("users:admin", "make a user an admin", func(c context.Context, args []string) error {
		ctx, got = c, args
		return nil
	})
	withArgs(t, "./blog", "users:admin", "ann@example.com", "--now")
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"ann@example.com", "--now"}) || ctx == nil {
		t.Errorf("the command ran with %q", got)
	}
	if started {
		t.Error("what Go runs started for a command")
	}
}

func TestACommandsErrorIsRuns(t *testing.T) {
	failed := errors.New("no user has that email")
	app := New(Config{})
	app.Command("users:admin", "make a user an admin", func(context.Context, []string) error { return failed })
	withArgs(t, "./blog", "users:admin", "nobody@example.com")
	if err := app.Run(); !errors.Is(err, failed) {
		t.Errorf("got %v", err)
	}
}

func TestHelpListsTheCommandsWithTheirSummaries(t *testing.T) {
	app := New(Config{})
	app.Command("jobs", "the jobs that failed for good, and retry to run them again", func(context.Context, []string) error { return nil })
	app.Command("users:admin", "make a user an admin", func(context.Context, []string) error { return nil })
	for _, arg := range []string{"help", "-h", "--help"} {
		var out bytes.Buffer
		if err := app.runCommand(context.Background(), "./blog", []string{arg}, &out); err != nil {
			t.Fatal(err)
		}
		want := "Without a command, ./blog serves. Its commands:\n\n" +
			"  ./blog jobs         the jobs that failed for good, and retry to run them again\n" +
			"  ./blog users:admin  make a user an admin\n" +
			"  ./blog help         list these commands\n"
		if out.String() != want {
			t.Errorf("%s:\n%s\nwant\n%s", arg, out.String(), want)
		}
	}
}

func TestANameThatIsntACommandSaysSo(t *testing.T) {
	app := New(Config{})
	app.Command("jobs", "the jobs that failed", func(context.Context, []string) error { return nil })
	err := app.runCommand(context.Background(), "./blog", []string{"job"}, &bytes.Buffer{})
	if err == nil || err.Error() != `./blog has no command "job": ./blog help lists the ones it has` {
		t.Errorf("got %v", err)
	}
}

func TestTugGenWritesTheTypesWhateverTheArguments(t *testing.T) {
	ran := false
	app := New(Config{})
	app.Command("jobs", "the jobs that failed", func(context.Context, []string) error {
		ran = true
		return nil
	})
	path := filepath.Join(t.TempDir(), "gen.json")
	t.Setenv("TUG_GEN", path)
	withArgs(t, "./blog", "jobs")
	if err := app.Run(); err != nil || ran {
		t.Fatalf("Run: %v, and the command ran: %v", err, ran)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("no types: %v", err)
	}
}

func TestACommandThatCantBeOnePanics(t *testing.T) {
	run := func(context.Context, []string) error { return nil }
	for name, add := range map[string]func(app *App){
		"a name taken":        func(app *App) { app.Command("jobs", "", run); app.Command("jobs", "", run) },
		"help":                func(app *App) { app.Command("help", "", run) },
		"no name":             func(app *App) { app.Command("", "", run) },
		"a flag":              func(app *App) { app.Command("-v", "", run) },
		"two words":           func(app *App) { app.Command("make admin", "", run) },
		"once it's serving":   func(app *App) { serve(app, "GET", "/", ""); app.Command("jobs", "", run) },
		"a name of the shell": func(app *App) { app.Command("rm;ls", "", run) },
	} {
		if !panics(func() { add(New(Config{})) }) {
			t.Errorf("%s didn't panic", name)
		}
	}
	if !strings.Contains(recovered(func() { New(Config{}).Command("help", "", run) }), "lists the others") {
		t.Error("help's panic doesn't say why")
	}
}

// recovered is what f panics with, as text.
func recovered(f func()) (msg string) {
	defer func() {
		if v := recover(); v != nil {
			msg, _ = v.(string)
		}
	}()
	f()
	return ""
}
