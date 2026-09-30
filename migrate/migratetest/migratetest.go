// Package migratetest has the promises a migrate.Store keeps, as tests,
// which each Store's own tests run on its database.
package migratetest

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cuonggt/tug/migrate"
)

// TestStore tests the Stores that newStores makes, two on one new database,
// as two instances of an app have. Its migrations are SQL every database
// takes, tables made and dropped, whose names start migratetest_.
func TestStore(t *testing.T, newStores func(t *testing.T) (migrate.Store, migrate.Store)) {
	t.Run("a new database has run none", func(t *testing.T) {
		s, _ := newStores(t)
		if ran := ranOn(t, s); len(ran) != 0 {
			t.Errorf("ran %+v", ran)
		}
	})

	t.Run("a migration runs once, and is kept with its hash and when it ran", func(t *testing.T) {
		s, _ := newStores(t)
		one := migrate.Migration{Name: "20260101000000_one", Up: "CREATE TABLE migratetest_one (id INT)"}
		before := time.Now()
		if err := run(t.Context(), s, one); err != nil {
			t.Fatal(err)
		}
		after := time.Now()
		ran := ranOn(t, s)
		if len(ran) != 1 || ran[0].Name != one.Name || ran[0].Hash != one.Hash() {
			t.Fatalf("ran %+v", ran)
		}
		// A Store keeps a time to the millisecond, or the second.
		if at := ran[0].At; at.Before(before.Add(-time.Second)) || at.After(after.Add(time.Second)) {
			t.Errorf("ran at %s, between %s and %s", at, before, after)
		}
		// It has run, as on another instance: its table isn't made again,
		// which would fail.
		if err := run(t.Context(), s, one); err != nil {
			t.Errorf("run again: %v", err)
		}
		if ran := ranOn(t, s); len(ran) != 1 {
			t.Errorf("run again, ran %+v", ran)
		}
	})

	t.Run("a migration that fails does nothing", func(t *testing.T) {
		s, _ := newStores(t)
		bad := migrate.Migration{Name: "20260101000000_bad", Up: "CREATE TABLE migratetest_bad (id INT, id INT)"}
		if err := run(t.Context(), s, bad); err == nil {
			t.Fatal("a table of two columns of one name was made")
		}
		if ran := ranOn(t, s); len(ran) != 0 {
			t.Errorf("ran %+v", ran)
		}
		// Put right, it runs, as nothing of it was left.
		fixed := migrate.Migration{Name: bad.Name, Up: "CREATE TABLE migratetest_bad (id INT)"}
		if err := run(t.Context(), s, fixed); err != nil {
			t.Error(err)
		}
	})

	t.Run("a migration of statements runs each, and its down part undoes it", func(t *testing.T) {
		s, _ := newStores(t)
		two := migrate.Migration{
			Name: "20260101000000_two",
			Up:   "-- Two tables, a statement each.\nCREATE TABLE migratetest_a (id INT);\n\nCREATE TABLE migratetest_b (id INT);",
			Down: "DROP TABLE migratetest_a;\nDROP TABLE migratetest_b;",
		}
		if err := run(t.Context(), s, two); err != nil {
			t.Fatal(err)
		}
		if err := undo(t.Context(), s, two); err != nil {
			t.Fatal(err)
		}
		if ran := ranOn(t, s); len(ran) != 0 {
			t.Errorf("undone, ran %+v", ran)
		}
		// Both tables went, so they're made again.
		if err := run(t.Context(), s, two); err != nil {
			t.Errorf("run again: %v", err)
		}
	})

	t.Run("two instances at once run each migration once", func(t *testing.T) {
		a, b := newStores(t)
		ms := []migrate.Migration{
			{Name: "20260101000001_first", Up: "CREATE TABLE migratetest_first (id INT)"},
			{Name: "20260101000002_second", Up: "CREATE TABLE migratetest_second (id INT)"},
			{Name: "20260101000003_third", Up: "CREATE TABLE migratetest_third (id INT)"},
		}
		errs := make([]error, 2)
		var wg sync.WaitGroup
		for i, s := range []migrate.Store{a, b} {
			wg.Go(func() { _, errs[i] = migrate.Up(t.Context(), s, ms) })
		}
		wg.Wait()
		for _, err := range errs {
			if err != nil {
				t.Error(err)
			}
		}
		ran := ranOn(t, b)
		seen := map[string]int{}
		for _, r := range ran {
			seen[r.Name]++
		}
		if len(ran) != 3 || seen[ms[0].Name] != 1 || seen[ms[1].Name] != 1 || seen[ms[2].Name] != 1 {
			t.Errorf("ran %+v", ran)
		}
	})
}

// ranOn is what has run on s, under its lock.
func ranOn(t *testing.T, s migrate.Store) []migrate.Ran {
	t.Helper()
	unlock, err := s.Lock(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	ran, err := s.Ran(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return ran
}

// run runs m on s, under its lock.
func run(ctx context.Context, s migrate.Store, m migrate.Migration) error {
	unlock, err := s.Lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	return s.Run(ctx, m)
}

// undo undoes m on s, under its lock.
func undo(ctx context.Context, s migrate.Store, m migrate.Migration) error {
	unlock, err := s.Lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	return s.Undo(ctx, m)
}
