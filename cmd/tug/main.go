// Command tug makes and runs tug apps:
//
//	tug new blog     a new app, in ./blog, ready to run
//	tug dev          run the app, rebuilt and reloaded as it changes
//	tug gen          write the TypeScript of the app's pages and routes
//	tug build        build the app into one binary, frontend and all
//
// Every command but new runs in the app's directory, where its go.mod,
// main package and package.json are.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime/debug"
)

const usage = `tug makes and runs tug apps.

  tug new <dir>    make a new app in dir, ready to run
  tug dev          run the app, rebuilding and reloading it as it changes
  tug gen          write the TypeScript of the app's pages and routes
  tug build        build the app into one binary, with its frontend in it
  tug version      print tug's version

Run "tug <command> -h" for a command's flags.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch cmd, args := os.Args[1], os.Args[2:]; cmd {
	case "new":
		err = runNew(args)
	case "dev":
		err = runDev(args)
	case "gen":
		err = runGen(args)
	case "build":
		err = runBuild(args)
	case "version":
		fmt.Println("tug", version())
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "tug: there's no command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if errors.Is(err, flag.ErrHelp) {
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "tug:", err)
		os.Exit(1)
	}
}

// version is tug's module version, or "(devel)" when built from a
// checkout rather than installed from a release.
func version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}
