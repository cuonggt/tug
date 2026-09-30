package migrate

import (
	"context"
	"fmt"
	"io"
	"strconv"
)

// Command is an app's migrate command, for app.Command to run in place of
// the server: with no arguments, it runs the migrations of ms that haven't
// run; with status, it lists them all, and whether and when each ran; and
// with down, it undoes the last. name is the app's binary, as os.Args[0],
// which what it says to run next names.
func Command(ctx context.Context, s Store, ms []Migration, name string, args []string, w io.Writer) error {
	switch {
	case len(args) == 0:
		ran, err := Up(ctx, s, ms)
		for _, n := range ran {
			fmt.Fprintf(w, "Ran %s.\n", n)
		}
		if err == nil && len(ran) == 0 {
			fmt.Fprintln(w, "Every migration has run: the database is up to date.")
		}
		return err

	case len(args) == 1 && args[0] == "status":
		states, err := Status(ctx, s, ms)
		if err != nil {
			return err
		}
		if len(states) == 0 {
			fmt.Fprintln(w, "There are no migrations: migrations/ has none, and none has run.")
			return nil
		}
		waiting := 0
		for _, st := range states {
			if !st.Ran {
				waiting++
			}
		}
		fmt.Fprintf(w, "%s, %s:\n\n", howMany(len(states), "migration"), notRun(waiting))
		for _, st := range states {
			when := "not run yet"
			if st.Ran {
				when = st.At.UTC().Format("2006-01-02 15:04:05 UTC")
			}
			note := ""
			switch {
			case st.Changed:
				note = ", changed since it ran"
			case st.NoFile:
				note = ", which migrations/ hasn't, as a newer version's"
			}
			fmt.Fprintf(w, "  %-23s  %s%s\n", when, st.Name, note)
		}
		fmt.Fprintf(w, "\n%s migrate runs the ones that haven't run, and %s migrate down undoes the last.\n", name, name)
		return nil

	case len(args) == 1 && args[0] == "down":
		undone, err := Down(ctx, s, ms)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "Undid %s: %s migrate runs it again.\n", undone, name)
		return nil
	}
	return fmt.Errorf("%s migrate runs the migrations that haven't run, %s migrate status lists them, and %s migrate down undoes the last", name, name, name)
}

// notRun says how many of the migrations haven't run.
func notRun(n int) string {
	switch n {
	case 0:
		return "each of them run"
	case 1:
		return "1 not run yet"
	}
	return strconv.Itoa(n) + " not run yet"
}

// howMany is n things, as a sentence says it: 1 migration, 2 migrations.
func howMany(n int, thing string) string {
	if n == 1 {
		return "1 " + thing
	}
	return strconv.Itoa(n) + " " + thing + "s"
}
