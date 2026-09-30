// Package migrate runs an app's migrations: files of SQL in its
// migrations/ directory, each a change to its database's tables, run once,
// in the order of their names, which say when they were made:
//
//	migrations/20261001093000_create_posts.sql
//
// A file is the SQL that makes its change, and may end with what undoes
// it, after a "-- down" line, for the last one run to be put right while
// it's new:
//
//	CREATE TABLE posts (id INTEGER PRIMARY KEY, title TEXT NOT NULL);
//
//	-- down
//	DROP TABLE posts;
//
// The package has no SQL of its own, and no driver: a Store, the app's,
// keeps which migrations have run on its database, in the database's own
// SQL, and runs each with its record, where the database can.
package migrate

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"time"
)

// A Migration is a file of migrations/.
type Migration struct {
	// Name is the file's name without .sql: when it was made, to the
	// second, in UTC, and what it does, as 20261001093000_create_posts.
	Name string

	// Up is the SQL that makes the change: the file, to its "-- down" line.
	Up string

	// Down is the SQL after its "-- down" line, which undoes the change: ""
	// for a file without one, which can't be undone.
	Down string
}

// Hash is the SHA-256 of m's Up, in hex, which a Store keeps with its name,
// so that a file changed after it ran is caught.
func (m Migration) Hash() string {
	sum := sha256.Sum256([]byte(m.Up))
	return hex.EncodeToString(sum[:])
}

// Ran is a migration that has run on a database.
type Ran struct {
	Name string
	Hash string // the migration's Hash, as it ran
	At   time.Time
}

// A Store keeps which migrations have run on a database, and runs them, in
// the database's own SQL. Its methods are called with its lock held, but
// for Lock's own.
type Store interface {
	// Lock waits for a lock that one instance of the app holds at a time,
	// for as long as it runs migrations, takes it, and returns what lets it
	// go. A database with no such lock, as SQLite, has each migration's
	// transaction take turns in its place, and Run look again there.
	Lock(ctx context.Context) (unlock func(), err error)

	// Ran returns the migrations that have run.
	Ran(ctx context.Context) ([]Ran, error)

	// Run runs m's Up, and records that it ran, with its Hash, both or
	// neither where the database can, unless it has run already, as on
	// another instance while this one waited: then it does nothing.
	Run(ctx context.Context, m Migration) error

	// Undo runs m's Down, and forgets that m ran, both or neither where
	// the database can.
	Undo(ctx context.Context, m Migration) error
}

// Load reads the migrations in fsys, the files of the app's migrations/, in
// the order of their names. A .sql file named otherwise, or with no SQL
// before its down part, is an error; anything else, as a .gitkeep, is left
// out.
func Load(fsys fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	var ms []Migration
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".sql")
		if !ok || e.IsDir() {
			continue
		}
		if !migrationName(name) {
			return nil, fmt.Errorf("migrate: %s isn't named as a migration is, when it was made, in UTC, and what it does, as 20261001093000_create_posts.sql", e.Name())
		}
		data, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, fmt.Errorf("migrate: %w", err)
		}
		up, down := split(string(data))
		if !hasSQL(up) {
			return nil, fmt.Errorf("migrate: %s has no SQL yet: the change it makes goes before its -- down line", e.Name())
		}
		ms = append(ms, Migration{Name: name, Up: up, Down: down})
	}
	// ReadDir has them in the order of their names already.
	return ms, nil
}

// migrationName reports whether name is written as a migration's is: 14
// digits, 20061001093000 for when it was made, an underscore, and what it
// does, in lower-case letters, digits and underscores.
func migrationName(name string) bool {
	when, what, ok := strings.Cut(name, "_")
	return ok && len(when) == 14 && strings.Trim(when, "0123456789") == "" &&
		what != "" && strings.Trim(what, "abcdefghijklmnopqrstuvwxyz0123456789_") == ""
}

// split parts a file at its "-- down" line, into the SQL that makes its
// change and the SQL that undoes it, "" when there's none. Lines end as
// they do on Unix, so a file checked out on Windows hashes the same.
func split(file string) (up, down string) {
	lines := strings.Split(strings.ReplaceAll(file, "\r\n", "\n"), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "-- down" {
			up, down = strings.Join(lines[:i], "\n"), strings.Join(lines[i+1:], "\n")
			if !hasSQL(down) {
				down = ""
			}
			return strings.TrimSpace(up), strings.TrimSpace(down)
		}
	}
	return strings.TrimSpace(file), ""
}

// hasSQL reports whether sql has a line that isn't blank or a comment.
func hasSQL(sql string) bool {
	for line := range strings.Lines(sql) {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "--") {
			return true
		}
	}
	return false
}

// Up runs the migrations of ms that haven't run, in order, under the
// Store's lock, and returns the names of those it ran. One that ran but has
// changed since stops it, before any runs; one that ran but that ms hasn't,
// as a newer version's, is left alone, as an instance of the version before
// runs beside the new ones through a deploy. A migration that fails stops
// it, and those before it stay run.
func Up(ctx context.Context, s Store, ms []Migration) ([]string, error) {
	unlock, err := s.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()
	ran, err := ranByName(ctx, s)
	if err != nil {
		return nil, err
	}
	for _, m := range ms {
		if r, ok := ran[m.Name]; ok && r.Hash != m.Hash() {
			return nil, changed(m)
		}
	}
	var done []string
	for _, m := range ms {
		if _, ok := ran[m.Name]; ok {
			continue
		}
		if err := s.Run(ctx, m); err != nil {
			return done, fmt.Errorf("migrations/%s.sql: %w", m.Name, err)
		}
		done = append(done, m.Name)
	}
	return done, nil
}

// Down undoes the last migration that ran, the one whose name comes last,
// by its file's down part, and returns its name. It's for a migration just
// written, to put right and run again: one whose file has no down part, or
// has changed since it ran, or isn't in ms, isn't undone.
func Down(ctx context.Context, s Store, ms []Migration) (string, error) {
	unlock, err := s.Lock(ctx)
	if err != nil {
		return "", err
	}
	defer unlock()
	ran, err := s.Ran(ctx)
	if err != nil {
		return "", err
	}
	if len(ran) == 0 {
		return "", errors.New("no migration has run, to undo")
	}
	last := slices.MaxFunc(ran, func(a, b Ran) int { return cmp.Compare(a.Name, b.Name) })
	i := slices.IndexFunc(ms, func(m Migration) bool { return m.Name == last.Name })
	switch {
	case i < 0:
		return "", fmt.Errorf("the last migration that ran, %s, has no file in migrations/, as a newer version's: it's that version's to undo", last.Name)
	case ms[i].Hash() != last.Hash:
		return "", changed(ms[i])
	case ms[i].Down == "":
		return "", fmt.Errorf("migrations/%s.sql has no down part, after a -- down line, to undo it with", last.Name)
	}
	if err := s.Undo(ctx, ms[i]); err != nil {
		return "", fmt.Errorf("migrations/%s.sql: %w", last.Name, err)
	}
	return last.Name, nil
}

// A State is where a migration stands on a database.
type State struct {
	Name    string
	Ran     bool      // it has run
	At      time.Time // when it ran
	Changed bool      // it ran, and its file has changed since
	NoFile  bool      // it ran, and has no file, as a newer version's
}

// Status is where each migration of ms stands, and each that ran that ms
// hasn't, in the order of their names.
func Status(ctx context.Context, s Store, ms []Migration) ([]State, error) {
	unlock, err := s.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()
	ran, err := ranByName(ctx, s)
	if err != nil {
		return nil, err
	}
	var states []State
	for _, m := range ms {
		r, ok := ran[m.Name]
		states = append(states, State{Name: m.Name, Ran: ok, At: r.At, Changed: ok && r.Hash != m.Hash()})
		delete(ran, m.Name)
	}
	for _, r := range ran {
		states = append(states, State{Name: r.Name, Ran: true, At: r.At, NoFile: true})
	}
	slices.SortFunc(states, func(a, b State) int { return cmp.Compare(a.Name, b.Name) })
	return states, nil
}

func ranByName(ctx context.Context, s Store) (map[string]Ran, error) {
	ran, err := s.Ran(ctx)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]Ran, len(ran))
	for _, r := range ran {
		byName[r.Name] = r
	}
	return byName, nil
}

// changed is the error of a migration whose file has changed since it ran,
// which the database is unlike now.
func changed(m Migration) error {
	return fmt.Errorf("migrations/%s.sql has changed since it ran: a migration that has run stays as it is, and the next one changes what it did", m.Name)
}
