package main

import (
	"bytes"
	"errors"
	"flag"
	"strings"
	"testing"
)

func TestRunTakesACommandAndNoFlagsOfItsOwn(t *testing.T) {
	for _, args := range [][]string{nil, {"-h"}, {"--help"}, {"-v", "jobs"}} {
		var stdout, stderr bytes.Buffer
		if err := runRun(args, &stdout, &stderr); !errors.Is(err, flag.ErrHelp) {
			t.Errorf("tug run %q: %v", args, err)
		}
		if !strings.HasPrefix(stderr.String(), "usage: tug run <command> [arguments]") || !strings.Contains(stderr.String(), "tug dev serves it") || stdout.Len() > 0 {
			t.Errorf("tug run %q said:\n%s%s", args, stdout.String(), stderr.String())
		}
	}
}

// A command runs as tug dev runs the app, at the address tug dev would
// take, without a look for a free port, as it listens on none.
func TestRunGivesTheAppTheEnvironmentTugDevGivesIt(t *testing.T) {
	for _, tc := range []struct {
		env       []string
		addr, url string
	}{
		{nil, "127.0.0.1:8080", "http://localhost:8080"},
		{[]string{"PORT=9000"}, "127.0.0.1:9000", "http://localhost:9000"},
		{[]string{"ADDR=:9001", "PORT=9000"}, "127.0.0.1:9001", "http://localhost:9001"},
		{[]string{"APP_URL=https://tunnel.example"}, "127.0.0.1:8080", "https://tunnel.example"},
	} {
		env := runEnv(append(tc.env, "APP_KEY=base64:x", "TLS_DOMAINS=blog.example"))
		if envValue(env, "ADDR") != tc.addr || envValue(env, "APP_URL") != tc.url || envValue(env, "TUG_DEV") != "1" {
			t.Errorf("with %q: ADDR %q, APP_URL %q, TUG_DEV %q", tc.env, envValue(env, "ADDR"), envValue(env, "APP_URL"), envValue(env, "TUG_DEV"))
		}
		if envValue(env, "APP_KEY") != "base64:x" || envValue(env, "TLS_DOMAINS") != "" {
			t.Errorf("with %q: %q", tc.env, env)
		}
	}
}
