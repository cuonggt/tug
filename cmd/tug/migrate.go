package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// runMigrate is tug migrate: new writes a new migration's file, for the
// app to run. Running one needs the app's own driver and database, which
// its binary has: ./blog migrate.
func runMigrate(args []string) error {
	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	flags.Usage = func() {
		fmt.Fprint(flags.Output(), `usage: tug migrate new <name>

Writes a new migration, migrations/<when>_<name>.sql, named for when it's
made, in UTC, and for what it does, as create_posts, for the change to
the database's tables in its own SQL, and after a "-- down" line, what
undoes it. The app runs the migrations that haven't run as it starts, and
under tug dev, as it's built again when one is written; its binary's
migrate command runs them too, lists them, and undoes the last:
./blog migrate, ./blog migrate status and ./blog migrate down.
`)
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 2 || flags.Arg(0) != "new" {
		flags.Usage()
		return flag.ErrHelp
	}
	path, err := newMigration("migrations", flags.Arg(1), time.Now())
	if err != nil {
		return err
	}
	fmt.Println("tug migrate: wrote", path)
	return nil
}

// newMigration writes a new migration's file in dir, named for when it's
// made, now, in UTC, and for name, in lower-case letters, digits and
// underscores, and returns its path. Two made in one second take the next
// seconds, as each migration's time is its own.
func newMigration(dir, name string, now time.Time) (string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return "", errors.New("there's no migrations/ here: tug migrate runs in the app's directory, where an app tug new -auth made has its migrations")
	}
	if err != nil {
		return "", err
	}
	what := migrationWords(name)
	if what == "" {
		return "", fmt.Errorf("%q says nothing of the change: a migration's name says what it does, as create_posts", name)
	}
	taken := map[string]bool{}
	for _, e := range entries {
		when, _, _ := strings.Cut(e.Name(), "_")
		taken[when] = true
	}
	when := now.UTC()
	for taken[when.Format("20060102150405")] {
		when = when.Add(time.Second)
	}
	path := filepath.Join(dir, when.Format("20060102150405")+"_"+what+".sql")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	_, err = f.WriteString(`-- The change this migration makes to the database's tables, in its own
-- SQL. A line that ends in ; ends a statement.


-- down
-- What undoes the change, which migrate down runs, while it's new. A
-- migration without it can't be undone.
`)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return path, err
}

// migrationWords is name as a migration's file is named: in lower-case
// letters and digits, with an underscore for what's between them, as
// "Create posts" is create_posts.
func migrationWords(name string) string {
	var b strings.Builder
	gap := false
	for _, r := range strings.ToLower(name) {
		if 'a' <= r && r <= 'z' || '0' <= r && r <= '9' {
			if gap && b.Len() > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r)
			gap = false
		} else {
			gap = true
		}
	}
	return b.String()
}
