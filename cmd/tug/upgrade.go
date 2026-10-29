package main

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	goversion "go/version"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// runUpgrade is tug upgrade: it writes the starter as the tug that made
// the app wrote it, and as this tug writes it, and lays the difference over
// the app, merged into what the app has changed itself, as git merges a
// branch. out is where it says what it did.
func runUpgrade(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	from := flags.String("from", "", "the tug that made the app's starter, as v0.47.0, for an app with no tug.json to say so")
	tugDir := flags.String("tug-dir", "", "a checkout of tug to upgrade the app to, rather than this tug's release")
	flags.Usage = func() {
		fmt.Fprint(flags.Output(), `usage: tug upgrade [flags]

Brings the app what the starter has gained since the tug that made it,
which tug.json names. It writes the starter as that tug wrote it, and as
this one writes it, and merges the difference into the app, as git
merges a branch: a file the app left as it was takes the new one, one
the app changed has the starter's change merged into it, and where both
changed the same lines, they're marked between <<<<<<< and >>>>>>>. A
migration the app has is never changed, as one that has run stops the
app once it's changed: one the starter changed is listed, for the app to
make the change in a migration of its own.

The app has to be in a git work tree with nothing uncommitted, so what
tug upgrade changes is one diff, which git shows and undoes. Unless a
file is marked, it then runs go mod tidy, installs the packages and
writes the types, as tug new does.

`)
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		flags.Usage()
		return flag.ErrHelp
	}
	if err := checkProject(); err != nil {
		return err
	}
	was, err := madeBefore(*from)
	if err != nil {
		return err
	}
	module, err := modulePath()
	if err != nil {
		return err
	}
	now, err := starterOf(was.Name, module, was.Flags)
	if err != nil {
		return err
	}
	if err := tugModule(&now, *tugDir); err != nil {
		return err
	}
	done, err := upgradable(was.Starter, now)
	if err != nil {
		return err
	}
	after, err := starterFiles(now)
	if err != nil {
		return err
	}
	if done {
		// An app made before tug.json, which -from names as this tug's, says
		// so from now on.
		if _, err := writeIfChanged(madeFile, after[madeFile]); err != nil {
			return err
		}
		fmt.Fprintf(out, "tug upgrade: the app's starter is %s's already\n", was.Starter)
		return nil
	}
	if err := committed(); err != nil {
		return err
	}

	work, err := os.MkdirTemp("", "tug-upgrade-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	fmt.Fprintf(out, "tug upgrade: writing the starter as tug %s wrote it\n", was.Starter)
	dir := filepath.Join(work, was.Name)
	if err := oldStarter(was.Starter, module, was.Flags, dir); err != nil {
		return err
	}
	before, err := readTree(dir)
	if err != nil {
		return err
	}
	to := now.TugVersion
	if now.TugDir != "" {
		to = "the checkout"
	}
	u, err := upgradeFiles(".", before, after, [3]string{"app", was.Starter, to})
	if err != nil {
		return err
	}
	if err := requireTug(now, after["go.mod"]); err != nil {
		return err
	}
	if err := os.WriteFile(madeFile, after[madeFile], 0o644); err != nil {
		return err
	}

	with := ""
	if len(was.Flags) > 0 {
		with = ", with " + strings.Join(was.Flags, " ")
	}
	fmt.Fprintf(out, "tug upgrade: the starter from %s to %s%s\n\n", was.Starter, to, with)
	u.say(out)
	if u.count("marked") > 0 {
		// The Go needn't build until what's marked is put right, and tug gen
		// builds it.
		fmt.Fprintf(out, "\nPut right what's marked, between <<<<<<< and >>>>>>>, then run\ngo mod tidy, %s install and tug gen.\n", packageManager())
	} else {
		fmt.Fprintln(out)
		if err := settleApp(); err != nil {
			return err
		}
	}
	return u.left()
}

// madeBefore is what the app's starter was made from: tug.json's, with
// from's version in its place when it's given, or for an app made before
// tug.json, from, and the flags its files show.
func madeBefore(from string) (made, error) {
	data, err := os.ReadFile(madeFile)
	switch {
	case err == nil:
		var m made
		if err := json.Unmarshal(data, &m); err != nil || m.Starter == "" || m.Name == "" {
			return made{}, errors.New("tug.json isn't as tug new writes it: it says which tug made the starter, the app's name and its flags")
		}
		m.Starter = cmp.Or(from, m.Starter)
		return m, nil
	case !errors.Is(err, fs.ErrNotExist):
		return made{}, err
	case from == "":
		msg := "this app has no tug.json, which says which tug made its starter: name that tug with -from"
		if v, where := likelyFrom(); v != "" {
			msg += fmt.Sprintf(", as tug upgrade -from %s, %s", v, where)
		}
		return made{}, errors.New(msg)
	}
	return madeFromFiles(from)
}

var (
	// guideAt is the link to tug's guide that an app's AGENTS.md has, at the
	// tug its starter came from: a release, or a pseudo-version's commit.
	guideAt = regexp.MustCompile(`github\.com/cuonggt/tug/tree/(v\d+\.\d+\.\d+[^/\s]*|[0-9a-f]{12})/docs`)
	// requiresTug is the line of a go.mod that requires tug.
	requiresTug = regexp.MustCompile(`(?m)^\s*(?:require\s+)?github\.com/cuonggt/tug\s+(v\S+)`)
)

// likelyFrom is the tug an app with no tug.json was most likely made by,
// and why: the one its AGENTS.md links the guide at, which is the
// starter's, or else the one its go.mod requires, which every release's
// notes have said to move with go get first.
func likelyFrom() (string, string) {
	if notes, err := os.ReadFile("AGENTS.md"); err == nil {
		if m := guideAt.FindSubmatch(notes); m != nil {
			return string(m[1]), "the tug its AGENTS.md links the guide at"
		}
	}
	if mod, err := os.ReadFile("go.mod"); err == nil {
		if m := requiresTug.FindSubmatch(mod); m != nil && string(m[1]) != "v0.0.0" {
			return string(m[1]), "the tug its go.mod requires, unless go get has moved it since"
		}
	}
	return "", ""
}

// madeFromFiles is what an app made before tug.json was made from: the tug
// from names, the name its package.json has, and the flags that the files
// their layers wrote show.
func madeFromFiles(from string) (made, error) {
	pkg, err := os.ReadFile("package.json")
	if err != nil {
		return made{}, err
	}
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(pkg, &p); err != nil {
		return made{}, fmt.Errorf("package.json: %v", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		return made{}, err
	}
	m := made{Starter: from, Name: cmp.Or(p.Name, filepath.Base(wd)), Flags: []string{}}
	if isFile("auth.go") {
		m.Flags = append(m.Flags, "-auth")
		db, _ := os.ReadFile("db.go")
		switch {
		case bytes.Contains(db, []byte(`"github.com/jackc/pgx/`)):
			m.Flags = append(m.Flags, "-postgres")
		case bytes.Contains(db, []byte(`"github.com/go-sql-driver/mysql"`)):
			m.Flags = append(m.Flags, "-mysql")
		}
	}
	switch {
	case bytes.Contains(pkg, []byte(`"@inertiajs/vue3"`)):
		m.Flags = append(m.Flags, "-vue")
	case bytes.Contains(pkg, []byte(`"@inertiajs/svelte"`)):
		m.Flags = append(m.Flags, "-svelte")
	}
	if isFile("resources/js/ssr.tsx") || isFile("resources/js/ssr.ts") {
		m.Flags = append(m.Flags, "-ssr")
	}
	return m, nil
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// modulePath is the app's Go module path, from its go.mod, which the
// starter is written with on both sides.
func modulePath() (string, error) {
	mod, err := os.ReadFile("go.mod")
	if err != nil {
		return "", errors.New("there's no go.mod here: tug upgrade runs in the app's directory, where tug new wrote its go.mod")
	}
	for line := range strings.Lines(string(mod)) {
		if path, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.Trim(strings.TrimSpace(path), `"`), nil
		}
	}
	return "", errors.New("go.mod has no module line")
}

// starterOf is the starter an app named name, of module, was made with
// flags for, which tug.json keeps, read as tug new reads its own.
func starterOf(name, module string, flags []string) (starterData, error) {
	fs := flag.NewFlagSet(madeFile, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	pick := starterFlags(fs)
	if err := fs.Parse(flags); err != nil || fs.NArg() > 0 {
		return starterData{}, fmt.Errorf("tug.json's flags, %s, aren't tug new's", strings.Join(flags, " "))
	}
	data := starterData{Name: name, Module: module}
	err := pick(&data)
	return data, err
}

// hexCommit is a commit as -from may name one, which the go command fetches
// as it fetches a release.
var hexCommit = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// upgradable says whether an app whose starter is from's goes to now's
// starter, done when it's there already. from has to be a tug the go
// command can fetch, and no later than now, as tug upgrade never goes back.
func upgradable(from string, now starterData) (done bool, err error) {
	fetchable := release(from) && from != "v0.0.0" ||
		pseudoVersion.MatchString(from) && !strings.Contains(from, "+") ||
		hexCommit.MatchString(from)
	if !fetchable {
		return false, fmt.Errorf("%s is no tug the go command can fetch, as an app made from a checkout of tug says: name the release, or the commit, its starter came from with -from", from)
	}
	if now.TugDir != "" {
		// A checkout is no version to compare with.
		return false, nil
	}
	if from == now.TugVersion {
		return true, nil
	}
	if c, ok := compareReleases(from, now.TugVersion); ok && c > 0 {
		return false, fmt.Errorf("the app's starter is %s's, which is newer than this tug, %s: install that tug, or a later one, to upgrade the app with", from, now.TugVersion)
	}
	return false, nil
}

// compareReleases compares releases a and b by their numbers, as -1, 0 or
// 1, and says whether both are releases it can compare.
func compareReleases(a, b string) (int, bool) {
	x, okA := releaseNumbers(a)
	y, okB := releaseNumbers(b)
	if !okA || !okB {
		return 0, false
	}
	return slices.Compare(x, y), true
}

// releaseNumbers are a release's major, minor and patch numbers.
func releaseNumbers(v string) ([]int, bool) {
	if !release(v) {
		return nil, false
	}
	core, _, _ := strings.Cut(strings.TrimPrefix(v, "v"), "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return nil, false
	}
	numbers := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, false
		}
		numbers[i] = n
	}
	return numbers, true
}

// committed reports what stops an upgrade: no git, an app outside a git
// work tree, or one with changes that aren't committed, as an upgrade has
// to be one diff, which git shows and undoes.
func committed() error {
	if _, err := exec.LookPath("git"); err != nil {
		return errors.New("tug upgrade needs git, which merges the starter's changes, and shows and undoes what it changed")
	}
	out, err := exec.Command("git", "status", "--porcelain", "--", ".").Output()
	if err != nil {
		return errors.New("the app isn't in a git work tree: tug upgrade changes it as one diff, which git shows and undoes, so commit it to a repository first")
	}
	if len(bytes.TrimSpace(out)) > 0 {
		return errors.New("the app has changes that aren't committed: commit them first, so what tug upgrade changes is one diff, which git shows and undoes")
	}
	return nil
}

// oldStarter writes into dir the starter the tug of version wrote, for an
// app of module made with flags: that tug's own tug new, which the go
// command builds once and keeps, as how a starter's layers were laid over
// each other, and filled in, is that tug's code. Tests put a function of
// their own in its place.
var oldStarter = func(version, module string, flags []string, dir string) error {
	args := append([]string{"run", "github.com/cuonggt/tug/cmd/tug@" + version, "new", "-no-install", "-module", module}, flags...)
	cmd := exec.Command("go", append(args, dir)...)
	cmd.Dir = filepath.Dir(dir)
	var log bytes.Buffer
	cmd.Stdout, cmd.Stderr = &log, &log
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tug %s couldn't write its starter (%v):\n%s", version, err, strings.TrimSpace(log.String()))
	}
	return nil
}

// settleApp gets the app ready after an upgrade, as tug new does once it
// has written the starter: its Go modules, its packages, by the package
// manager its lockfile names, and its types. Tests put a function of their
// own in its place.
var settleApp = func() error {
	env, err := appEnv()
	if err != nil {
		return err
	}
	if err := run(env, "go", "mod", "tidy"); err != nil {
		return err
	}
	if err := run(env, packageManager(), "install"); err != nil {
		return err
	}
	bin, err := buildApp(env, os.Stderr)
	if err != nil {
		return err
	}
	_, err = generate(env, bin)
	return err
}

// readTree reads every file under dir, by its slash-separated path in it.
func readTree(dir string) (map[string][]byte, error) {
	files := map[string][]byte{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		files[filepath.ToSlash(rel)] = content
		return err
	})
	return files, err
}

// change is what an upgrade did to one of the app's files, by its
// slash-separated path: updated, merged, marked, added, deleted or kept,
// and for one it kept, why.
type change struct {
	path, did, why string
}

// migrationChange is a migration the starter changed, which the app keeps
// as it is, and how the starter changed it, for the app to make the change
// in one of its own.
type migrationChange struct {
	path, diff string
}

// upgrade is what an upgrade did to the app's files, and the migrations it
// left for the app to write.
type upgrade struct {
	changes    []change
	migrations []migrationChange
}

// neverMerged are the starter's files an upgrade leaves to others: the
// app's .env, with its own key and values, go.mod, which requireTug and go
// mod tidy keep, as tidying moves the starter's one require into a block
// where a merge finds no line of its, and tug.json, which says what the
// upgrade made.
var neverMerged = []string{".env", "go.mod", madeFile}

// upgradeFiles lays over the app in root what the starter changed from was
// to now, each a file by its slash-separated path, as git merges a branch:
// a file the app left as it was takes the new one, one it changed has the
// starter's change merged into it, and where both changed the same lines,
// they're marked, by labels, the app's, was's and now's. What the starter
// never had is the app's alone, and isn't touched.
func upgradeFiles(root string, was, now map[string][]byte, labels [3]string) (upgrade, error) {
	var u upgrade
	paths := slices.Sorted(maps.Keys(was))
	for p := range now {
		if _, ok := was[p]; !ok {
			paths = append(paths, p)
		}
	}
	slices.Sort(paths)
	for _, p := range paths {
		prev, inWas := was[p]
		next, inNow := now[p]
		if slices.Contains(neverMerged, p) || inWas && inNow && bytes.Equal(prev, next) {
			continue
		}
		file := filepath.Join(root, filepath.FromSlash(p))
		app, readErr := os.ReadFile(file)
		inApp := readErr == nil
		if readErr != nil && !errors.Is(readErr, fs.ErrNotExist) {
			return u, readErr
		}
		var err error
		write := func(did string, content []byte) error {
			u.changes = append(u.changes, change{path: p, did: did})
			if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
				return err
			}
			return os.WriteFile(file, content, 0o644)
		}
		keep := func(why string) {
			u.changes = append(u.changes, change{path: p, did: "kept", why: why})
		}

		if strings.HasPrefix(p, "migrations/") && strings.HasSuffix(p, ".sql") {
			// A migration's hash is kept as it runs, and the app stops as it
			// starts at one whose SQL has changed since, so one the app has is
			// never written, and tug can't tell which have run where.
			switch {
			case !inNow:
				if inApp {
					keep("a migration, which may have run, though the starter has none of it now")
				}
			case !inWas && !inApp:
				err = write("added", next)
			case !inWas && !bytes.Equal(app, next):
				keep("a migration of the app's own, by the name of one the starter has added")
			case inWas && !(inApp && bytes.Equal(app, next)):
				var diff string
				if diff, err = starterDiff(prev, next); err == nil {
					u.migrations = append(u.migrations, migrationChange{path: p, diff: diff})
				}
			}
			if err != nil {
				return u, err
			}
			continue
		}

		switch {
		case !inNow:
			switch {
			case !inApp:
			case bytes.Equal(app, prev):
				u.changes = append(u.changes, change{path: p, did: "deleted"})
				err = os.Remove(file)
			default:
				keep("the app changed it, and the starter has left it out since")
			}
		case !inApp && inWas:
			keep("the app deleted it, and the starter has changed it since")
		case !inApp:
			err = write("added", next)
		case bytes.Equal(app, next):
		case inWas && bytes.Equal(app, prev):
			err = write("updated", next)
		default:
			// A file new in the starter, where the app has its own of that
			// name, is merged with it over nothing.
			merged, clashes, mergeErr := mergeFile(app, prev, next, labels)
			if mergeErr != nil {
				return u, fmt.Errorf("%s: %w", p, mergeErr)
			}
			did := "merged"
			if clashes > 0 {
				did = "marked"
			}
			err = write(did, merged)
		}
		if err != nil {
			return u, err
		}
	}
	return u, nil
}

// mergeFile merges what the starter changed from base to theirs into ours,
// as git merge-file does, and returns how many places both changed, which
// it marks, with what base had between them, by labels.
func mergeFile(ours, base, theirs []byte, labels [3]string) ([]byte, int, error) {
	dir, err := os.MkdirTemp("", "tug-merge-")
	if err != nil {
		return nil, 0, err
	}
	defer os.RemoveAll(dir)
	names := []string{"ours", "base", "theirs"}
	for i, content := range [][]byte{ours, base, theirs} {
		if err := os.WriteFile(filepath.Join(dir, names[i]), content, 0o600); err != nil {
			return nil, 0, err
		}
	}
	// --diff3 whatever the person's merge.conflictStyle, as what the
	// starter had before shows what it changed.
	cmd := exec.Command("git", "merge-file", "-p", "--diff3", "-L", labels[0], "-L", labels[1], "-L", labels[2], names[0], names[1], names[2])
	cmd.Dir = dir
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	err = cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return out.Bytes(), 0, nil
	case errors.As(err, &exit) && exit.ExitCode() > 0 && exit.ExitCode() < 128:
		// git merge-file's status is how many places it marked.
		return out.Bytes(), exit.ExitCode(), nil
	}
	return nil, 0, fmt.Errorf("git merge-file: %v: %s", err, strings.TrimSpace(stderr.String()))
}

// starterDiff is how the starter changed a file, from was to now, as git
// diff writes it, from its first hunk.
func starterDiff(was, now []byte) (string, error) {
	dir, err := os.MkdirTemp("", "tug-diff-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, "was"), was, 0o600); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "now"), now, 0o600); err != nil {
		return "", err
	}
	cmd := exec.Command("git", "diff", "--no-index", "--no-color", "--no-ext-diff", "-U2", "was", "now")
	cmd.Dir = dir
	out, err := cmd.Output()
	var exit *exec.ExitError
	if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 1) {
		// 1 says the files differ, which they do.
		return "", fmt.Errorf("git diff: %v", err)
	}
	if i := bytes.Index(out, []byte("\n@@")); i >= 0 {
		out = out[i+1:]
	}
	return string(out), nil
}

// requireTug points the app's go.mod at the tug data is, as tug new writes
// it: the version, the checkout that replaces it, or none, and the go line
// of the starter's go.mod, mod, when it's later than the app's own. The
// rest of go.mod is the app's, and go mod tidy's.
func requireTug(data starterData, mod []byte) error {
	args := []string{"mod", "edit", "-require=github.com/cuonggt/tug@" + data.TugVersion}
	if data.TugDir != "" {
		args = append(args, "-replace=github.com/cuonggt/tug="+data.TugDir)
	} else {
		args = append(args, "-dropreplace=github.com/cuonggt/tug")
	}
	have, err := os.ReadFile("go.mod")
	if err != nil {
		return err
	}
	if want := goLine(mod); want != "" && goversion.Compare("go"+want, "go"+goLine(have)) > 0 {
		args = append(args, "-go="+want)
	}
	if out, err := exec.Command("go", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("go %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return nil
}

var goDirective = regexp.MustCompile(`(?m)^go\s+(\S+)`)

// goLine is the Go version a go.mod's go line says.
func goLine(mod []byte) string {
	if m := goDirective.FindSubmatch(mod); m != nil {
		return string(m[1])
	}
	return ""
}

// say writes what the upgrade did to each file, and the migrations it left
// to write, each with how the starter changed it.
func (u upgrade) say(out io.Writer) {
	if len(u.changes) == 0 && len(u.migrations) == 0 {
		fmt.Fprintln(out, "The starter has changed none of the app's files since.")
	}
	for _, c := range u.changes {
		if c.why == "" {
			fmt.Fprintf(out, "  %-7s  %s\n", c.did, c.path)
		} else {
			fmt.Fprintf(out, "  %-7s  %s: %s\n", c.did, c.path, c.why)
		}
	}
	if len(u.migrations) == 0 {
		return
	}
	fmt.Fprintln(out, "\nThe starter changed these migrations, which the app keeps as they are,\nas one that has run stops the app once it's changed. Make each change in\na migration of the app's own, with tug migrate new:")
	for _, m := range u.migrations {
		fmt.Fprintf(out, "\n  %s\n", m.path)
		for line := range strings.Lines(m.diff) {
			fmt.Fprint(out, "    ", strings.TrimSuffix(line, "\n"), "\n")
		}
	}
}

// count is how many of the app's files the upgrade did what to.
func (u upgrade) count(did string) int {
	n := 0
	for _, c := range u.changes {
		if c.did == did {
			n++
		}
	}
	return n
}

// left is the error of an upgrade that has left the app something to do,
// so that a script running it knows.
func (u upgrade) left() error {
	var parts []string
	if n := u.count("marked"); n > 0 {
		parts = append(parts, several(n, "file", "files")+" marked")
	}
	if n := len(u.migrations); n > 0 {
		parts = append(parts, several(n, "migration", "migrations")+" to write")
	}
	if len(parts) == 0 {
		return nil
	}
	return fmt.Errorf("the upgrade left %s", strings.Join(parts, " and "))
}

// several is n of a thing, by its word for one or for more.
func several(n int, one, more string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + more
}
