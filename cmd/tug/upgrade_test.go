package main

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// none is a file that isn't there, in the tables below.
const none = "\x00"

func TestAnUpgradeLaysWhatTheStarterChangedOverTheApp(t *testing.T) {
	needsGit(t)
	for _, c := range []struct {
		name          string
		path          string
		was, now, app string // the starter as it was, as it is, and the app's
		want, did     string // the app's after, and what the upgrade says it did
		listed        bool   // whether it's a migration for the app to write
	}{
		{"a file the app left as it was takes the new one", "main.go",
			"1\n", "1 starter\n", "1\n", "1 starter\n", "updated", false},
		{"a file the app changed has the starter's change merged into it", "main.go",
			"1\n2\n3\n4\n", "1\n2\n3\n4 starter\n", "1 app\n2\n3\n4\n", "1 app\n2\n3\n4 starter\n", "merged", false},
		{"where both changed the same lines, they're marked", "main.go",
			"1\n2\n", "1 starter\n2\n", "1 app\n2\n",
			"<<<<<<< app\n1 app\n||||||| v0.1.0\n1\n=======\n1 starter\n>>>>>>> v0.2.0\n2\n", "marked", false},
		{"a file the starter didn't change is the app's", "main.go",
			"1\n", "1\n", "1 app\n", "1 app\n", "", false},
		{"a file the app has as the starter has it now is left", "main.go",
			"1\n", "1 starter\n", "1 starter\n", "1 starter\n", "", false},
		{"a file new in the starter is added", "presences.go",
			none, "1\n", none, "1\n", "added", false},
		{"a file new in the starter, where the app has its own, is merged over nothing", "presences.go",
			none, "1 starter\n", "1 app\n",
			"<<<<<<< app\n1 app\n||||||| v0.1.0\n=======\n1 starter\n>>>>>>> v0.2.0\n", "marked", false},
		{"a file the starter deleted goes, where the app left it as it was", "old.go",
			"1\n", none, "1\n", none, "deleted", false},
		{"a file the starter deleted stays, where the app changed it", "old.go",
			"1\n", none, "1 app\n", "1 app\n", "kept", false},
		{"a file the app deleted stays deleted", "main.go",
			"1\n", "1 starter\n", none, none, "kept", false},
		{"a migration new in the starter is added", "migrations/20261022012800_create_presences.sql",
			none, "CREATE TABLE presences;\n", none, "CREATE TABLE presences;\n", "added", false},
		{"a migration the starter changed is left as the app has it, and listed", "migrations/20260925171438_create_users.sql",
			"CREATE TABLE users (id);\n", "CREATE TABLE users (id, locale);\n", "CREATE TABLE users (id);\n", "CREATE TABLE users (id);\n", "", true},
		{"a migration the starter has none of now stays", "migrations/20260930000000_steps.sql",
			"CREATE TABLE jobs;\n", none, "CREATE TABLE jobs;\n", "CREATE TABLE jobs;\n", "kept", false},
		{"a migration of the app's own by a new one's name stays", "migrations/20261022012800_create_presences.sql",
			none, "CREATE TABLE presences;\n", "CREATE TABLE seen;\n", "CREATE TABLE seen;\n", "kept", false},
		{"the app's .env is its own", ".env",
			"APP_KEY=a\n", "APP_KEY=b\n", "APP_KEY=c\n", "APP_KEY=c\n", "", false},
		{"go.mod is go mod edit's, not the merge's", "go.mod",
			"module blog\n", "module blog\n\ngo 1.26\n", "module blog\n\nrequire x v1.0.0\n", "module blog\n\nrequire x v1.0.0\n", "", false},
		{"tug.json is written after the merge", madeFile,
			"{}\n", `{"starter": "v0.2.0"}` + "\n", "{}\n", "{}\n", "", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			file := filepath.Join(root, filepath.FromSlash(c.path))
			if c.app != none {
				writeFile(t, file, c.app)
			}
			was, now := map[string][]byte{}, map[string][]byte{}
			if c.was != none {
				was[c.path] = []byte(c.was)
			}
			if c.now != none {
				now[c.path] = []byte(c.now)
			}
			u, err := upgradeFiles(root, was, now, [3]string{"app", "v0.1.0", "v0.2.0"})
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(file)
			switch {
			case c.want == none && err == nil:
				t.Errorf("%s is there, %q", c.path, got)
			case c.want != none && string(got) != c.want:
				t.Errorf("%s is %q, not %q", c.path, got, c.want)
			}
			var did []string
			for _, ch := range u.changes {
				did = append(did, ch.did)
			}
			if want := slices.DeleteFunc([]string{c.did}, func(s string) bool { return s == "" }); !slices.Equal(did, want) {
				t.Errorf("the upgrade says %q, not %q", did, want)
			}
			if listed := len(u.migrations) == 1 && strings.Contains(u.migrations[0].diff, "+"+c.now); listed != c.listed || !c.listed && len(u.migrations) > 0 {
				t.Errorf("the migrations to write are %v", u.migrations)
			}
		})
	}
}

func TestAnUpgradeWritesTheOldStarterAndMergesTheNewIntoTheApp(t *testing.T) {
	needsGit(t)
	root := filepath.Join(t.TempDir(), "blog")
	data := starterData{Name: "blog", Module: "example.com/blog", TugVersion: "v0.1.0", Frontend: "react", Auth: true, Database: "sqlite"}
	users := "migrations/20260925171438_create_users.sql"
	// The starter as an older tug wrote it: this one's, with a line of
	// main.go another, the users without their locale, a file it has
	// left out since, and none of what it has added since.
	older := func(dir string) error {
		if err := writeStarter(dir, data); err != nil {
			return err
		}
		edit := func(name string, change func(string) string) error {
			path := filepath.Join(dir, filepath.FromSlash(name))
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(path, []byte(change(string(b))), 0o644)
		}
		if err := edit("main.go", func(s string) string { return "// An older starter's line.\n" + s }); err != nil {
			return err
		}
		if err := edit(users, func(s string) string {
			return strings.Replace(s, "\tlocale            TEXT NOT NULL DEFAULT '',\n", "", 1)
		}); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "old.go"), []byte("package main\n"), 0o644); err != nil {
			return err
		}
		return os.Remove(filepath.Join(dir, "presences_db.go"))
	}
	if err := older(root); err != nil {
		t.Fatal(err)
	}
	ownLine := "\n// The app's own line.\n"
	auth := filepath.Join(root, "auth.go")
	writeFile(t, auth, reader(t, root)("auth.go")+ownLine)
	gitApp(t, root)

	var wrote []string
	stub(t, &oldStarter, func(version, module string, flags []string, dir string) error {
		wrote = append([]string{version, module}, flags...)
		return older(dir)
	})
	settled := false
	stub(t, &settleApp, func() error {
		settled = true
		return nil
	})
	t.Chdir(root)
	var out strings.Builder
	if err := runUpgrade(nil, &out); err == nil || err.Error() != "the upgrade left 1 migration to write" {
		t.Errorf("the upgrade's error: %v", err)
	}
	if want := []string{"v0.1.0", "example.com/blog", "-auth"}; !slices.Equal(wrote, want) {
		t.Errorf("the old starter was asked of %q, not %q", wrote, want)
	}

	now, err := starterFiles(starterData{Name: "blog", Module: "example.com/blog", TugVersion: "v0.0.0", TugDir: checkoutDir(), Frontend: "react", Auth: true, Database: "sqlite"})
	if err != nil {
		t.Fatal(err)
	}
	read := reader(t, root)
	for _, name := range []string{"main.go", "presences_db.go", "AGENTS.md"} {
		if read(name) != string(now[name]) {
			t.Errorf("%s isn't the starter's now", name)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "old.go")); err == nil {
		t.Error("old.go, which the starter has left out since, is there")
	}
	if !strings.HasSuffix(read("auth.go"), ownLine) {
		t.Error("auth.go lost the app's own line")
	}
	if strings.Contains(read(users), "locale") {
		t.Errorf("%s, a migration that may have run, was changed", users)
	}
	for _, want := range []string{"updated  main.go\n", "added    presences_db.go\n", "deleted  old.go\n", "  " + users + "\n", "    +\tlocale"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the upgrade doesn't say %q:\n%s", want, out.String())
		}
	}
	var m made
	if err := json.Unmarshal([]byte(read(madeFile)), &m); err != nil || m.Starter != "v0.0.0" || m.Name != "blog" || !slices.Equal(m.Flags, []string{"-auth"}) {
		t.Errorf("tug.json says %+v, %v", m, err)
	}
	if mod := read("go.mod"); !strings.Contains(mod, "github.com/cuonggt/tug v0.0.0") || !strings.Contains(mod, "github.com/cuonggt/tug => "+checkoutDir()) {
		t.Errorf("go.mod doesn't require this checkout:\n%s", mod)
	}
	if !settled {
		t.Error("the upgrade didn't install the packages and write the types, with nothing marked")
	}
}

func TestAnUpgradeStopsWhereItWouldntBeOneDiff(t *testing.T) {
	needsGit(t)
	root := filepath.Join(t.TempDir(), "blog")
	if err := writeStarter(root, starterData{Name: "blog", Module: "blog", TugVersion: "v0.1.0", Frontend: "react"}); err != nil {
		t.Fatal(err)
	}
	stub(t, &oldStarter, func(string, string, []string, string) error {
		t.Error("an upgrade that stops wrote the old starter")
		return nil
	})
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(root))
	t.Chdir(root)
	if err := runUpgrade(nil, io.Discard); err == nil || !strings.Contains(err.Error(), "isn't in a git work tree") {
		t.Errorf("an upgrade outside a git work tree: %v", err)
	}
	gitApp(t, root)
	writeFile(t, "main.go", reader(t, root)("main.go")+"// A change of the app's.\n")
	if err := runUpgrade(nil, io.Discard); err == nil || !strings.Contains(err.Error(), "aren't committed") {
		t.Errorf("an upgrade with a change not committed: %v", err)
	}
}

func TestAnUpgradeNamesTheTugThatMadeAnAppWithNoTugJSON(t *testing.T) {
	root := filepath.Join(t.TempDir(), "blog")
	if err := writeStarter(root, starterData{Name: "blog", Module: "blog", TugVersion: "v0.47.0", Frontend: "react"}); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	if err := os.Remove(madeFile); err != nil {
		t.Fatal(err)
	}
	if err := runUpgrade(nil, io.Discard); err == nil || !strings.Contains(err.Error(), "-from v0.47.0, the tug its AGENTS.md links the guide at") {
		t.Errorf("an upgrade of an app with no tug.json: %v", err)
	}
	if err := os.Remove("AGENTS.md"); err != nil {
		t.Fatal(err)
	}
	if err := runUpgrade(nil, io.Discard); err == nil || !strings.Contains(err.Error(), "-from v0.47.0, the tug its go.mod requires") {
		t.Errorf("an upgrade of an app with no tug.json or AGENTS.md: %v", err)
	}
}

func TestAnAppWithNoTugJSONIsReadFromItsFiles(t *testing.T) {
	for _, data := range []starterData{
		{Frontend: "react"},
		{Frontend: "vue", SSR: true},
		{Frontend: "svelte", Auth: true, Database: "postgres"},
		{Frontend: "react", Auth: true, Database: "mysql", SSR: true},
		{Frontend: "react", Auth: true, Database: "sqlite"},
	} {
		data.Name, data.Module, data.TugVersion = "my-blog", "example.com/blog", "v0.47.0"
		t.Run(strings.Join(append([]string{data.Frontend}, data.flags()...), " "), func(t *testing.T) {
			// A directory renamed since, which the name in package.json
			// outlives.
			root := filepath.Join(t.TempDir(), "renamed")
			if err := writeStarter(root, data); err != nil {
				t.Fatal(err)
			}
			t.Chdir(root)
			if err := os.Remove(madeFile); err != nil {
				t.Fatal(err)
			}
			m, err := madeBefore("v0.47.0")
			if want := (made{Starter: "v0.47.0", Name: "my-blog", Flags: data.flags()}); err != nil || m.Starter != want.Starter || m.Name != want.Name || !slices.Equal(m.Flags, want.Flags) {
				t.Errorf("read %+v, %v; want %+v", m, err, want)
			}
		})
	}
}

func TestAnUpgradeGoesForwardFromATugTheGoCommandFetches(t *testing.T) {
	release := starterData{TugVersion: "v0.55.0"}
	checkout := starterData{TugVersion: "v0.0.0", TugDir: "/src/tug"}
	for _, c := range []struct {
		from string
		to   starterData
		done bool
		err  string
	}{
		{"v0.47.0", release, false, ""},
		{"v0.55.0", release, true, ""},
		{"v0.56.0", release, false, "newer than this tug, v0.55.0"},
		{"v0.10.0", starterData{TugVersion: "v0.9.0"}, false, "newer than this tug"},
		{"v0.54.1-0.20261029101518-2b8aeb8c316c", release, false, ""},
		{"2b8aeb8c316c", release, false, ""},
		{"v0.56.0", checkout, false, ""},
		{"v0.0.0", release, false, "no tug the go command can fetch"},
		{"v0.54.1-0.20261029101518-2b8aeb8c316c+dirty", release, false, "no tug the go command can fetch"},
		{"(devel)", release, false, "no tug the go command can fetch"},
	} {
		done, err := upgradable(c.from, c.to)
		if done != c.done || (err == nil) != (c.err == "") || err != nil && !strings.Contains(err.Error(), c.err) {
			t.Errorf("upgradable(%q, %s) = %v, %v", c.from, c.to.TugVersion, done, err)
		}
	}
}

func TestANewAppSaysWhatItWasMadeFrom(t *testing.T) {
	for _, data := range []starterData{
		{Frontend: "react"},
		{Frontend: "svelte"},
		{Frontend: "vue", Auth: true, Database: "postgres", SSR: true},
		{Frontend: "react", Auth: true, Database: "sqlite"},
	} {
		data.Name, data.Module, data.TugVersion = "blog", "blog", "v0.55.0"
		files, err := starterFiles(data)
		if err != nil {
			t.Fatal(err)
		}
		var m made
		if err := json.Unmarshal(files[madeFile], &m); err != nil || m.Starter != "v0.55.0" || m.Name != "blog" || m.Flags == nil {
			t.Errorf("tug.json is %s, %v", files[madeFile], err)
		}
		// Read back as tug new reads its own flags, they make the same
		// starter, as tug upgrade writes it.
		back, err := starterOf(m.Name, data.Module, m.Flags)
		back.TugVersion = data.TugVersion
		if err != nil || back != data {
			t.Errorf("tug.json's flags, %q, read back as %+v, %v; want %+v", m.Flags, back, err, data)
		}
	}
}

// TestAnAppAnOlderTugMadeUpgradesAndPassesItsOwnTests makes an app with
// accounts as tug v0.47.0 made one, the release before the locale column
// went into create_users.sql, upgrades it to this checkout, writes the
// migration the upgrade lists, as v0.48.0's notes said to, and runs the
// app's checks.
func TestAnAppAnOlderTugMadeUpgradesAndPassesItsOwnTests(t *testing.T) {
	if testing.Short() {
		t.Skip("makes an app with tug v0.47.0, which the go command fetches, and installs its packages")
	}
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("needs npm")
	}
	needsGit(t)
	// tug's own versions straight from GitHub: the go command asks for
	// tug's latest, to say whether it's deprecated, and CI's run of a
	// release's commit starts seconds after its tag, when a module proxy
	// asked for it then keeps it missing for up to half an hour, as v0.56.0
	// was, for this run and for everyone.
	t.Setenv("GOPRIVATE", "github.com/cuonggt/tug")
	dir := filepath.Join(t.TempDir(), "blog")
	older := exec.Command("go", "run", "github.com/cuonggt/tug/cmd/tug@v0.47.0", "new", "-auth", dir)
	older.Dir = filepath.Dir(dir)
	if out, err := older.CombinedOutput(); err != nil {
		t.Fatalf("tug v0.47.0's tug new: %v\n%s", err, out)
	}
	gitApp(t, dir)
	t.Chdir(dir)
	var out strings.Builder
	if err := runUpgrade([]string{"-from", "v0.47.0"}, &out); err == nil || err.Error() != "the upgrade left 1 migration to write" {
		t.Fatalf("the upgrade's error: %v\n%s", err, out.String())
	}
	for _, want := range []string{"  migrations/20260925171438_create_users.sql\n", "    +\tlocale", "added    migrations/20261022012800_create_presences.sql\n", "added    presences_db.go\n"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the upgrade doesn't say %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "marked") {
		t.Errorf("the upgrade of an app as it was made marked a file:\n%s", out.String())
	}
	path, err := newMigration("migrations", "add_locale_to_users", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, "ALTER TABLE users ADD COLUMN locale TEXT NOT NULL DEFAULT '';\n\n-- down\nALTER TABLE users DROP COLUMN locale;\n")
	for _, c := range [][]string{{"go", "vet", "./..."}, {"go", "test", "./..."}, {"npm", "run", "typecheck"}, {"npm", "run", "build"}} {
		cmd := exec.Command(c[0], c[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s: %v\n%s", strings.Join(c, " "), err, out)
		}
	}
	notesHold(t, dir, reader(t, dir)("AGENTS.md"))
}

func needsGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("needs git, which merges what the starter changed")
	}
}

// gitApp makes root a git repository with all of it committed, as an app
// is before it's upgraded, with none of the person's own settings for
// commits in the way.
func gitApp(t *testing.T, root string) {
	t.Helper()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(root))
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "-A"},
		{"-c", "user.name=tug", "-c", "user.email=tug@example.com", "-c", "commit.gpgsign=false", "commit", "-q", "--no-verify", "-m", "Make the app"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
}

// stub puts f in v's place for the test.
func stub[T any](t *testing.T, v *T, f T) {
	t.Helper()
	was := *v
	*v = f
	t.Cleanup(func() { *v = was })
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
