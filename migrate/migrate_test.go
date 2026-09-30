package migrate

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"
)

// memory is a Store in memory, for the tests of what Up, Down and Status
// decide, which run no SQL: each database's Store is tested on its
// database, by migratetest.TestStore.
type memory struct {
	lock   sync.Mutex
	held   bool
	ran    []Ran
	fail   string   // the name of a migration whose Run fails
	runs   []string // the names Run ran, in turn
	undone []string // the names Undo undid
}

func (s *memory) Lock(context.Context) (func(), error) {
	s.lock.Lock()
	s.held = true
	return func() { s.held = false; s.lock.Unlock() }, nil
}

func (s *memory) Ran(context.Context) ([]Ran, error) { return slices.Clone(s.ran), nil }

func (s *memory) Run(_ context.Context, m Migration) error {
	if !s.held {
		return errors.New("run without the lock")
	}
	if m.Name == s.fail {
		return errors.New("no such table: nope")
	}
	s.runs = append(s.runs, m.Name)
	s.ran = append(s.ran, Ran{Name: m.Name, Hash: m.Hash(), At: time.Now()})
	return nil
}

func (s *memory) Undo(_ context.Context, m Migration) error {
	if !s.held {
		return errors.New("undo without the lock")
	}
	s.undone = append(s.undone, m.Name)
	s.ran = slices.DeleteFunc(s.ran, func(r Ran) bool { return r.Name == m.Name })
	return nil
}

// ranAlready is a Store where each of ms has run, as it is now.
func ranAlready(ms ...Migration) *memory {
	s := &memory{}
	for _, m := range ms {
		s.ran = append(s.ran, Ran{Name: m.Name, Hash: m.Hash(), At: time.Date(2026, 10, 1, 9, 30, 0, 0, time.UTC)})
	}
	return s
}

var (
	posts = Migration{Name: "20261001093000_create_posts", Up: "CREATE TABLE posts (id INTEGER PRIMARY KEY);", Down: "DROP TABLE posts;"}
	slugs = Migration{Name: "20261002100000_add_slug_to_posts", Up: "ALTER TABLE posts ADD COLUMN slug TEXT;"}
	index = Migration{Name: "20261003120000_index_posts_slug", Up: "CREATE INDEX posts_slug ON posts (slug);", Down: "DROP INDEX posts_slug;"}
)

func TestLoadReadsTheFilesInTheOrderOfTheirNames(t *testing.T) {
	files := fstest.MapFS{
		"20261002100000_add_slug_to_posts.sql": {Data: []byte("-- The posts' slugs, for their links.\nALTER TABLE posts ADD COLUMN slug TEXT;\n\n-- down\n-- SQLite drops a column since 3.35.\n")},
		"20261001093000_create_posts.sql":      {Data: []byte("CREATE TABLE posts (id INTEGER PRIMARY KEY);\r\n\r\n-- down\r\nDROP TABLE posts;\r\n")},
		".gitkeep":                             {},
		"README.md":                            {Data: []byte("# Migrations")},
	}
	ms, err := Load(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 2 || ms[0].Name != "20261001093000_create_posts" || ms[1].Name != "20261002100000_add_slug_to_posts" {
		t.Fatalf("loaded %+v", ms)
	}
	// Written on Windows, it hashes as it would anywhere else.
	if ms[0] != posts || ms[0].Hash() != posts.Hash() {
		t.Errorf("got %+v, want %+v", ms[0], posts)
	}
	// A down part of comments alone can't undo anything.
	if ms[1].Up != "-- The posts' slugs, for their links.\nALTER TABLE posts ADD COLUMN slug TEXT;" || ms[1].Down != "" {
		t.Errorf("got %+v", ms[1])
	}
}

func TestLoadRefusesAFileNamedAsNoMigrationIs(t *testing.T) {
	for _, name := range []string{"create_posts.sql", "2026_create_posts.sql", "20261001093000_Create_Posts.sql", "20261001093000_.sql", "20261001093000.sql", "2026100109300a_create_posts.sql", "20261001093000-create-posts.sql"} {
		_, err := Load(fstest.MapFS{name: {Data: []byte("CREATE TABLE posts (id INT);")}})
		if err == nil || !strings.Contains(err.Error(), name+" isn't named as a migration is") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestLoadRefusesAMigrationWithNoSQLYet(t *testing.T) {
	_, err := Load(fstest.MapFS{"20261001093000_create_posts.sql": {Data: []byte("-- The posts.\n\n-- down\nDROP TABLE posts;\n")}})
	if err == nil || !strings.Contains(err.Error(), "20261001093000_create_posts.sql has no SQL yet") {
		t.Errorf("err = %v", err)
	}
}

func TestUpRunsWhatHasntRunInOrderUnderTheLock(t *testing.T) {
	s := ranAlready(posts)
	ran, err := Up(t.Context(), s, []Migration{posts, slugs, index})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{slugs.Name, index.Name}; !slices.Equal(ran, want) || !slices.Equal(s.runs, want) {
		t.Errorf("said it ran %v, and ran %v, want %v", ran, s.runs, want)
	}
	if s.held {
		t.Error("the lock is still held")
	}
	if ran, err := Up(t.Context(), s, []Migration{posts, slugs, index}); err != nil || len(ran) != 0 {
		t.Errorf("again: ran %v, %v", ran, err)
	}
}

func TestUpLeavesANewerVersionsMigrationAlone(t *testing.T) {
	// The version after this one has run index, which this one hasn't.
	s := ranAlready(posts, index)
	if ran, err := Up(t.Context(), s, []Migration{posts, slugs}); err != nil || !slices.Equal(ran, []string{slugs.Name}) {
		t.Errorf("ran %v, %v", ran, err)
	}
}

func TestAFileChangedSinceItRanStopsTheRunBeforeAnyRuns(t *testing.T) {
	s := ranAlready(posts)
	edited := posts
	edited.Up = "CREATE TABLE posts (id INTEGER PRIMARY KEY, title TEXT);"
	_, err := Up(t.Context(), s, []Migration{edited, slugs})
	if err == nil || !strings.Contains(err.Error(), "migrations/20261001093000_create_posts.sql has changed since it ran") {
		t.Errorf("err = %v", err)
	}
	if len(s.runs) != 0 {
		t.Errorf("ran %v", s.runs)
	}
}

func TestAMigrationThatFailsStopsTheRest(t *testing.T) {
	s := &memory{fail: slugs.Name}
	ran, err := Up(t.Context(), s, []Migration{posts, slugs, index})
	if err == nil || err.Error() != "migrations/20261002100000_add_slug_to_posts.sql: no such table: nope" {
		t.Errorf("err = %v", err)
	}
	if !slices.Equal(ran, []string{posts.Name}) || !slices.Equal(s.runs, []string{posts.Name}) {
		t.Errorf("said it ran %v, and ran %v", ran, s.runs)
	}
}

func TestDownUndoesTheMigrationWhoseNameIsLast(t *testing.T) {
	// index ran first, on a branch, and posts after, once it merged.
	s := ranAlready(index, posts)
	undone, err := Down(t.Context(), s, []Migration{posts, slugs, index})
	if err != nil || undone != index.Name || !slices.Equal(s.undone, []string{index.Name}) {
		t.Fatalf("undid %q, %v, and %v", undone, s.undone, err)
	}
	if s.held {
		t.Error("the lock is still held")
	}
	if ran, _ := s.Ran(t.Context()); len(ran) != 1 || ran[0].Name != posts.Name {
		t.Errorf("ran %+v", ran)
	}
}

func TestDownUndoesNothingItCantUndoWell(t *testing.T) {
	edited := index
	edited.Down = "DROP INDEX IF EXISTS posts_slug;"
	edited.Up = "CREATE INDEX posts_slug ON posts (slug, id);"
	for _, tc := range []struct {
		name string
		s    *memory
		ms   []Migration
		want string
	}{
		{"with nothing run", &memory{}, []Migration{posts}, "no migration has run, to undo"},
		{"without a down part", ranAlready(posts, slugs), []Migration{posts, slugs}, "migrations/20261002100000_add_slug_to_posts.sql has no down part"},
		{"changed since it ran", ranAlready(posts, index), []Migration{posts, edited}, "migrations/20261003120000_index_posts_slug.sql has changed since it ran"},
		{"a newer version's", ranAlready(posts, index), []Migration{posts}, "the last migration that ran, 20261003120000_index_posts_slug, has no file in migrations/"},
	} {
		if _, err := Down(t.Context(), tc.s, tc.ms); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v", tc.name, err)
		}
		if len(tc.s.undone) != 0 {
			t.Errorf("%s: undid %v", tc.name, tc.s.undone)
		}
	}
}

func TestStatusSaysWhereEachMigrationStands(t *testing.T) {
	newer := Migration{Name: "20261004080000_newer", Up: "SELECT 1;"}
	s := ranAlready(posts, slugs, newer)
	edited := slugs
	edited.Up = "ALTER TABLE posts ADD COLUMN slug TEXT NOT NULL DEFAULT '';"
	states, err := Status(t.Context(), s, []Migration{index, edited, posts})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 1, 9, 30, 0, 0, time.UTC)
	want := []State{
		{Name: posts.Name, Ran: true, At: at},
		{Name: slugs.Name, Ran: true, At: at, Changed: true},
		{Name: index.Name},
		{Name: newer.Name, Ran: true, At: at, NoFile: true},
	}
	if !slices.Equal(states, want) {
		t.Errorf("got %+v\nwant %+v", states, want)
	}
}

func TestTheCommandRunsListsAndUndoesTheMigrations(t *testing.T) {
	s := ranAlready(posts)
	ms := []Migration{posts, slugs, index}
	command := func(args ...string) (string, error) {
		var out strings.Builder
		err := Command(t.Context(), s, ms, "./blog", args, &out)
		return out.String(), err
	}

	if out, err := command("status"); err != nil || out != `3 migrations, 2 not run yet:

  2026-10-01 09:30:00 UTC  20261001093000_create_posts
  not run yet              20261002100000_add_slug_to_posts
  not run yet              20261003120000_index_posts_slug

./blog migrate runs the ones that haven't run, and ./blog migrate down undoes the last.
` {
		t.Errorf("status: %v\n%s", err, out)
	}
	if out, err := command(); err != nil || out != "Ran 20261002100000_add_slug_to_posts.\nRan 20261003120000_index_posts_slug.\n" {
		t.Errorf("migrate: %v\n%s", err, out)
	}
	if out, err := command(); err != nil || out != "Every migration has run: the database is up to date.\n" {
		t.Errorf("migrate again: %v\n%s", err, out)
	}
	if out, err := command("down"); err != nil || out != "Undid 20261003120000_index_posts_slug: ./blog migrate runs it again.\n" {
		t.Errorf("down: %v\n%s", err, out)
	}
	if out, _ := command("status"); !strings.HasPrefix(out, "3 migrations, 1 not run yet:\n") {
		t.Errorf("status after down:\n%s", out)
	}
	for _, args := range [][]string{{"up"}, {"status", "all"}, {"rollback"}} {
		if _, err := command(args...); err == nil || !strings.Contains(err.Error(), "./blog migrate status lists them") {
			t.Errorf("%v: %v", args, err)
		}
	}
}
