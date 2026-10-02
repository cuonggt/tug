package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
)

func runRoutes(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("routes", flag.ContinueOnError)
	asJSON := flags.Bool("json", false, "print the routes as JSON, for a script")
	flags.Usage = func() {
		fmt.Fprint(flags.Output(), `usage: tug routes [-json] [text]

Lists the app's routes, by path, then method: each one's method, path,
name, the struct it takes, the file and line of the app's that added it,
and its handler, as that line has it. With text, only the routes whose
path or name has it, as tug routes login.

tug routes builds the app and runs it, as tug gen does: the routes are the
ones the app adds as it starts, with its .env.

`)
		flags.PrintDefaults()
	}
	// The flag package stops at the first argument that isn't a flag, and
	// the text can come anywhere among them.
	var texts []string
	for {
		if err := flags.Parse(args); err != nil {
			return err
		}
		if flags.NArg() == 0 {
			break
		}
		texts = append(texts, flags.Arg(0))
		args = flags.Args()[1:]
	}
	if len(texts) > 1 {
		return errors.New("tug routes takes one text to look for, as tug routes login")
	}
	text := strings.Join(texts, "")

	if err := checkProject(); err != nil {
		return err
	}
	env, err := appEnv()
	if err != nil {
		return err
	}
	bin, err := buildApp(env, os.Stderr)
	if err != nil {
		return err
	}
	g, err := runForGen(env, bin)
	if err != nil {
		return err
	}
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	routes := listRoutes(g.RouteList, text, dir)
	if *asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(routes)
	}
	switch {
	case len(routes) > 0:
		return printRoutes(out, routes)
	case text != "":
		return fmt.Errorf("no route's path or name has %q in it", text)
	}
	return errors.New("the app has no routes")
}

// listedRoute is a route as the app writes it in tug gen's run, with its
// handler as the line that added it has it, which tug routes reads, and
// its file as the app's directory has it.
type listedRoute struct {
	Method   string `json:"method"`
	Path     string `json:"path"`
	Name     string `json:"name,omitempty"`
	Handler  string `json:"handler"`
	Function string `json:"function"`
	Takes    string `json:"takes,omitempty"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
}

// listRoutes is the routes whose path or name has text, in any case, or
// all of them for "", by path, then method, with their handlers read from
// the lines that added them, and their files from dir, the app's.
func listRoutes(routes []listedRoute, text, dir string) []listedRoute {
	text = strings.ToLower(text)
	src := sources{}
	var out []listedRoute
	for _, r := range routes {
		if !strings.Contains(strings.ToLower(r.Path), text) && !strings.Contains(strings.ToLower(r.Name), text) {
			continue
		}
		r.Handler = src.handler(r)
		if rel, err := filepath.Rel(dir, r.File); err == nil && r.File != "" && !strings.HasPrefix(rel, "..") {
			r.File = filepath.ToSlash(rel)
		}
		out = append(out, r)
	}
	slices.SortStableFunc(out, func(a, b listedRoute) int {
		return cmp.Or(strings.Compare(a.Path, b.Path), cmp.Compare(methodOrder(a.Method), methodOrder(b.Method)))
	})
	return out
}

// methodOrder is where a route of method comes among a path's: the reads,
// then the writes, as a resource's go, and a route of any method last.
func methodOrder(method string) int {
	if i := slices.Index([]string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}, method); i >= 0 {
		return i
	}
	if method == "ANY" {
		return 9
	}
	return 8
}

// printRoutes writes routes as a table, the handler last, as it's the
// column whose width varies most.
func printRoutes(out io.Writer, routes []listedRoute) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "METHOD\tPATH\tNAME\tTAKES\tADDED\tHANDLER")
	for _, r := range routes {
		added := ""
		if r.File != "" {
			added = r.File + ":" + strconv.Itoa(r.Line)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", r.Method, r.Path, r.Name, r.Takes, added, r.Handler)
	}
	return w.Flush()
}

// sources are the files of the app's that added routes, each parsed once,
// or nil for one that can't be.
type sources map[string]*ast.File

var fset = token.NewFileSet()

// handler is r's handler as the line that added it has it: the
// expression the app gave the router, as a.guestsOnly(a.login), on one
// line, as go/types writes an expression, a function literal without its
// body; or else r's function, as Go names it, where the line can't be
// read, or gives a variable, as a helper of the app's that adds routes
// does.
func (s sources) handler(r listedRoute) string {
	if r.File == "" {
		return r.Function
	}
	f, ok := s[r.File]
	if !ok {
		f, _ = parser.ParseFile(fset, r.File, nil, parser.SkipObjectResolution)
		s[r.File] = f
	}
	if f == nil {
		return r.Function
	}
	switch h := handlerAt(f, r.Line).(type) {
	case nil:
		return r.Function
	case *ast.FuncLit:
		return types.ExprString(h.Type) + " {…}"
	default:
		return types.ExprString(h)
	}
}

// routerMethods are the methods of tug's Router that add a route, and
// where each takes the handler.
var routerMethods = map[string]int{
	"Get": 1, "Post": 1, "Put": 1, "Patch": 1, "Delete": 1, "Options": 1, "Any": 1,
	"Handle": 2,
}

// handlerAt is the handler the call of the router's at line gives it: of
// the calls whose lines take in line, as the runtime gives a call over
// several lines one of them, the innermost; or nil when there's none, or
// it gives a variable of the function it's in.
func handlerAt(f *ast.File, line int) ast.Expr {
	type call struct {
		handler ast.Expr
		span    int  // its lines, less one
		local   bool // the handler is a variable of the function the call is in
	}
	var (
		best  *call
		funcs []ast.Node // the functions Inspect has gone into in the declaration it's in
	)
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncDecl:
			funcs = append(funcs[:0], n)
		case *ast.FuncLit:
			funcs = append(funcs, n)
		case *ast.CallExpr:
			sel, ok := n.Fun.(*ast.SelectorExpr)
			if !ok {
				break
			}
			at, ok := routerMethods[sel.Sel.Name]
			if !ok || len(n.Args) <= at {
				break
			}
			from, to := fset.Position(n.Pos()).Line, fset.Position(n.End()).Line
			if from > line || to < line {
				break
			}
			c := call{handler: n.Args[at], span: to - from}
			if id, ok := c.handler.(*ast.Ident); ok {
				c.local = declares(enclosing(funcs, n), id.Name)
			}
			if best == nil || c.span < best.span {
				best = &c
			}
		}
		return true
	})
	if best == nil || best.local {
		return nil
	}
	return best.handler
}

// enclosing is the innermost of funcs, the functions ast.Inspect went into
// on its way to n, whose body has n in it.
func enclosing(funcs []ast.Node, n ast.Node) ast.Node {
	for _, fn := range slices.Backward(funcs) {
		if fn.Pos() <= n.Pos() && n.End() <= fn.End() {
			return fn
		}
	}
	return nil
}

// declares reports whether fn, a function, has a variable named name: a
// parameter, or one its body declares, as a helper's handler is.
func declares(fn ast.Node, name string) bool {
	var typ *ast.FuncType
	var body *ast.BlockStmt
	switch fn := fn.(type) {
	case *ast.FuncDecl:
		typ, body = fn.Type, fn.Body
	case *ast.FuncLit:
		typ, body = fn.Type, fn.Body
	default:
		return false
	}
	for _, field := range typ.Params.List {
		for _, n := range field.Names {
			if n.Name == name {
				return true
			}
		}
	}
	found := false
	if body != nil {
		ast.Inspect(body, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.AssignStmt:
				if n.Tok == token.DEFINE {
					for _, lhs := range n.Lhs {
						if id, ok := lhs.(*ast.Ident); ok && id.Name == name {
							found = true
						}
					}
				}
			case *ast.ValueSpec:
				for _, n := range n.Names {
					if n.Name == name {
						found = true
					}
				}
			case *ast.RangeStmt:
				for _, x := range []ast.Expr{n.Key, n.Value} {
					if id, ok := x.(*ast.Ident); ok && id.Name == name && n.Tok == token.DEFINE {
						found = true
					}
				}
			}
			return !found
		})
	}
	return found
}
