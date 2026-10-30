package main

import (
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cuonggt/tug/migrate"
)

// TestAReleasedMigrationIsNeverChanged holds the starters' migrations to
// migrations.sha256, each one a release has shipped, by the SHA-256 of its
// file, as shasum -a 256 writes it. An app that has run one stops as it
// starts at a file whose SQL has changed, so a later change to a
// starter's table is a migration of its own, which tug upgrade adds to an
// app as it is, as v0.48.0's change to create_users.sql wasn't.
func TestAReleasedMigrationIsNeverChanged(t *testing.T) {
	record, err := os.ReadFile("migrations.sha256")
	if err != nil {
		t.Fatal(err)
	}
	released := map[string]string{} // the hash, by the file's path
	for line := range strings.Lines(string(record)) {
		hash, path, ok := strings.Cut(strings.TrimSuffix(line, "\n"), "  ")
		if !ok {
			t.Fatalf("migrations.sha256: %q isn't a hash and a path, as shasum -a 256 writes them", line)
		}
		released[path] = hash
	}
	files, err := filepath.Glob(filepath.Join("*", "migrations", "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no starter has migrations: %v", err)
	}
	for _, file := range files {
		path := filepath.ToSlash(file)
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(content)
		switch hash, ok := released[path]; {
		case !ok:
			t.Errorf("%s is new: add the line `shasum -a 256 %s` prints to migrations.sha256, as once a release ships it, it never changes", path, path)
		case hash != hex.EncodeToString(sum[:]):
			t.Errorf("%s has changed since a release shipped it, and an app that has run it would stop as it starts: put it back, and make the change in a migration of its own", path)
		}
		delete(released, path)
	}
	for _, path := range slices.Sorted(maps.Keys(released)) {
		t.Errorf("%s is gone, though a release shipped it: an app that has run it keeps it, and a new app's tables are made by it", path)
	}
}

func TestMigrateNewWritesAFileNamedForWhenItsMade(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "migrations")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 1, 16, 30, 0, 0, time.FixedZone("Hanoi", 7*60*60))
	path, err := newMigration(dir, "Create posts!", now)
	if err != nil {
		t.Fatal(err)
	}
	// Named in UTC, whatever the zone of the machine that made it.
	if want := filepath.Join(dir, "20261001093000_create_posts.sql"); path != want {
		t.Errorf("wrote %s, want %s", path, want)
	}
	// Two in one second take the next.
	if again, err := newMigration(dir, "add_slug_to_posts", now); err != nil || filepath.Base(again) != "20261001093001_add_slug_to_posts.sql" {
		t.Errorf("wrote %s, %v", again, err)
	}

	// Its SQL is the app's to write: until it is, the app won't run it.
	_, err = migrate.Load(os.DirFS(dir))
	if err == nil || !strings.Contains(err.Error(), "20261001093000_create_posts.sql has no SQL yet") {
		t.Errorf("load: %v", err)
	}
	file, _ := os.ReadFile(path)
	written := strings.Replace(string(file), "\n\n\n-- down\n", "\nCREATE TABLE posts (id INTEGER PRIMARY KEY);\n\n-- down\n", 1) + "DROP TABLE posts;\n"
	os.WriteFile(path, []byte(written), 0o644)
	os.Remove(filepath.Join(dir, "20261001093001_add_slug_to_posts.sql"))
	ms, err := migrate.Load(os.DirFS(dir))
	if err != nil || len(ms) != 1 || !strings.HasSuffix(ms[0].Up, "CREATE TABLE posts (id INTEGER PRIMARY KEY);") || !strings.HasPrefix(ms[0].Down, "-- What undoes") || !strings.HasSuffix(ms[0].Down, "DROP TABLE posts;") {
		t.Errorf("loaded %+v, %v", ms, err)
	}
}

func TestMigrateNewSaysWhatItNeeds(t *testing.T) {
	root := t.TempDir()
	if _, err := newMigration(filepath.Join(root, "migrations"), "create_posts", time.Now()); err == nil || !strings.Contains(err.Error(), "there's no migrations/ here") {
		t.Errorf("without migrations/: %v", err)
	}
	os.Mkdir(filepath.Join(root, "migrations"), 0o755)
	if _, err := newMigration(filepath.Join(root, "migrations"), "!!", time.Now()); err == nil || !strings.Contains(err.Error(), "says nothing of the change") {
		t.Errorf("a name of no words: %v", err)
	}
	if err := runMigrate([]string{"create_posts"}); err == nil {
		t.Error("tug migrate took a name without new")
	}
}
