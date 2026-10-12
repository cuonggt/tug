package main

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestEveryAppsNotesForAgentsAreTheApps(t *testing.T) {
	var kinds []starterData
	for _, frontend := range frontends {
		kinds = append(kinds,
			starterData{Frontend: frontend},
			starterData{Frontend: frontend, SSR: true},
			starterData{Frontend: frontend, Auth: true, Database: "sqlite"},
			starterData{Frontend: frontend, Auth: true, Database: "sqlite", SSR: true},
			starterData{Frontend: frontend, Auth: true, Database: "postgres"},
			starterData{Frontend: frontend, Auth: true, Database: "mysql"},
		)
	}
	for _, data := range kinds {
		name := data.Frontend
		if data.Auth {
			name += "-auth-" + data.Database
		}
		if data.SSR {
			name += "-ssr"
		}
		t.Run(name, func(t *testing.T) {
			data.Name, data.Module, data.TugVersion = "blog", "blog", "v0.43.0"
			root := filepath.Join(t.TempDir(), "blog")
			if err := writeStarter(root, data); err != nil {
				t.Fatal(err)
			}
			read := reader(t, root)
			notes := read("AGENTS.md")
			if strings.Contains(notes, "[[") {
				t.Error("AGENTS.md has a placeholder left")
			}
			if !strings.Contains(notes, "https://github.com/cuonggt/tug/tree/v0.43.0/docs") {
				t.Error("AGENTS.md doesn't link the guide at the version the app requires")
			}
			if data.Auth && !strings.Contains(notes, data.DatabaseName()) {
				t.Errorf("AGENTS.md doesn't say the app's database is %s", data.DatabaseName())
			}
			if strings.Contains(notes, "createForm") == data.React() {
				t.Errorf("AGENTS.md's forms aren't %s's", data.Framework())
			}
			if strings.Contains(notes, "docker compose up -d") != (data.Postgres() || data.MySQL()) {
				t.Error("AGENTS.md's checks start the database where the app has none to start, or don't where it has")
			}
			if strings.Contains(notes, "renders on the server too") != data.SSR {
				t.Error("AGENTS.md's rule for a page rendered on the server is in an app without SSR, or not in one with it")
			}
			// Claude Code reads AGENTS.md itself only where there's no
			// CLAUDE.md: this one keeps it read once the app has one.
			if got := read("CLAUDE.md"); got != "@AGENTS.md\n" {
				t.Errorf("CLAUDE.md is %q, not AGENTS.md's import", got)
			}
			notesHold(t, root, notes)
		})
	}
}

// codeSpan is a span of code in Markdown, and fence a block of it.
var (
	codeSpan = regexp.MustCompile("`([^`\n]+)`")
	fence    = regexp.MustCompile("(?s)```.*?```")
	npmRun   = regexp.MustCompile(`npm run ([\w:-]+)`)
	ownName  = regexp.MustCompile(`^(?:a\.)?([a-z][a-z0-9]*[A-Z][A-Za-z0-9]*)$`)
	pathExts = []string{".go", ".ts", ".tsx", ".vue", ".svelte", ".json", ".sql", ".md", ".yaml", ".html", ".css", ".mod"}
)

// notesHold fails the test for what the AGENTS.md of the app in root gives
// that the app hasn't, notesProblems'.
func notesHold(t *testing.T, root, notes string) {
	t.Helper()
	for _, p := range notesProblems(t, root, notes) {
		t.Error(p)
	}
}

// notesProblems are what notes, an AGENTS.md, gives that the app in root
// hasn't, as a note an agent trusts that has gone out of date is worse
// than none: a path in backticks that's no file or directory of the
// app's, a script npm runs that its package.json hasn't, and a name of its
// own, in backticks by itself, as usersOnly or a.inTx, that its Go doesn't
// declare and its frontend doesn't have. A path with a part to fill in, as
// <Name>, is an example, as a block of code is, and resources/js/tug is
// tug gen's, which install writes.
func notesProblems(t *testing.T, root, notes string) []string {
	t.Helper()
	var problems []string
	var pkg struct{ Scripts map[string]string }
	if err := json.Unmarshal([]byte(reader(t, root)("package.json")), &pkg); err != nil {
		t.Fatal(err)
	}
	for _, m := range npmRun.FindAllStringSubmatch(notes, -1) {
		if _, ok := pkg.Scripts[m[1]]; !ok {
			problems = append(problems, "AGENTS.md runs npm run "+m[1]+", which package.json hasn't")
		}
	}
	declared := mainNames(t, root)
	frontend := frontendText(t, root)
	for _, m := range codeSpan.FindAllStringSubmatch(fence.ReplaceAllString(notes, ""), -1) {
		span := m[1]
		if path, ok := appPath(span); ok {
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil && !strings.HasPrefix(path, genDir) {
				problems = append(problems, "AGENTS.md gives "+span+", which isn't the app's")
			}
		}
		if n := ownName.FindStringSubmatch(span); n != nil && !declared[n[1]] && !regexp.MustCompile(`\b`+n[1]+`\b`).MatchString(frontend) {
			problems = append(problems, "AGENTS.md gives "+span+", which neither the app's Go nor its frontend has")
		}
	}
	return problems
}

func TestANoteForAgentsThatIsntTheAppsIsCaught(t *testing.T) {
	root := filepath.Join(t.TempDir(), "blog")
	if err := writeStarter(root, starterData{Name: "blog", Module: "blog", TugVersion: "v0.43.0", Frontend: "react"}); err != nil {
		t.Fatal(err)
	}
	notes := "Run `npm run typecheck`, then npm run lint. See `main.go`, `newApp`, `resources/js/tug/pages.ts`, " +
		"`resources/js/pages/<Name>.tsx` and `Posts/Index`; not `pages.go`, `resources/js/nope.ts`, `migrations/` or `a.noSuchThing`.\n" +
		"```go\nfunc postsIndex(c *tug.Ctx) error // `examples.go` in a block\n```\n"
	want := []string{
		"AGENTS.md runs npm run lint, which package.json hasn't",
		"AGENTS.md gives pages.go, which isn't the app's",
		"AGENTS.md gives resources/js/nope.ts, which isn't the app's",
		"AGENTS.md gives migrations/, which isn't the app's",
		"AGENTS.md gives a.noSuchThing, which neither the app's Go nor its frontend has",
	}
	if got := notesProblems(t, root, notes); !slices.Equal(got, want) {
		t.Errorf("problems:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// appPath reports whether a span of code is a path in the app, and which:
// a file by its extension, a directory by its slash, or anything in
// resources/; not an import, as @/tug/pages, or one with a part to fill
// in, as resources/js/pages/<Name>.tsx.
func appPath(span string) (string, bool) {
	if strings.ContainsAny(span, " <>*{}()'\"=@:") || strings.HasPrefix(span, ".") {
		return "", false
	}
	dir := strings.HasSuffix(span, "/")
	path := strings.TrimSuffix(span, "/")
	if dir || strings.HasPrefix(path, "resources/") {
		return path, true
	}
	for _, ext := range pathExts {
		if strings.HasSuffix(path, ext) {
			return path, true
		}
	}
	return "", false
}

// mainNames are the names the Go of the app in root declares at its top:
// its functions, methods, types, variables and constants.
func mainNames(t *testing.T, root string) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	files, _ := filepath.Glob(filepath.Join(root, "*.go"))
	for _, path := range files {
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				names[d.Name.Name] = true
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						names[s.Name.Name] = true
					case *ast.ValueSpec:
						for _, n := range s.Names {
							names[n.Name] = true
						}
					}
				}
			}
		}
	}
	return names
}

// frontendText is all of the app's own frontend, in resources/js.
func frontendText(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(filepath.Join(root, "resources", "js"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		content, err := os.ReadFile(path)
		b.Write(content)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestTheGuideIsLinkedAtTheTugTheAppRequires(t *testing.T) {
	for _, c := range []struct {
		data starterData
		want string
	}{
		{starterData{TugVersion: "v0.43.0"}, "v0.43.0"},
		// A pseudo-version go install fetched names a commit that's
		// published.
		{starterData{TugVersion: "v0.43.1-0.20261012090000-0123456789ab"}, "0123456789ab"},
		{starterData{TugVersion: "v0.0.0", TugDir: "/src/tug"}, "main"},
	} {
		if got, want := c.data.Guide(), "https://github.com/cuonggt/tug/tree/"+c.want+"/docs"; got != want {
			t.Errorf("tug %s, from %q: %s, want %s", c.data.TugVersion, c.data.TugDir, got, want)
		}
	}
}
