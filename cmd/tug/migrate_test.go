package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cuonggt/tug/migrate"
)

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
