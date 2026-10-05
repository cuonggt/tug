package main

import (
	"bufio"
	"bytes"
	"cmp"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// runGo runs the Go benchmarks, routers_test.go's and inertia_test.go's,
// count times each, with go test, as anyone would run them, and keeps the
// median of each.
func runGo(count int) (*goResults, error) {
	cmd := exec.Command("go", "test", "-run", "^$", "-bench", ".", "-benchmem", "-count", strconv.Itoa(count), ".")
	cmd.Stderr = os.Stderr
	var out bytes.Buffer
	cmd.Stdout = &out
	fmt.Fprintf(os.Stderr, "bench: go test -bench, %d times each\n", count)
	if err := cmd.Run(); err != nil {
		os.Stdout.Write(out.Bytes())
		return nil, err
	}
	type benchmark struct{ group, name string }
	runs := map[benchmark][]goBench{}
	var order []benchmark
	s := bufio.NewScanner(&out)
	for s.Scan() {
		group, b, ok := parseBenchLine(s.Text())
		if !ok {
			continue
		}
		key := benchmark{group, b.Name}
		if _, seen := runs[key]; !seen {
			order = append(order, key)
		}
		runs[key] = append(runs[key], b)
	}
	if len(runs) == 0 {
		return nil, fmt.Errorf("go test printed no benchmarks:\n%s", out.Bytes())
	}
	g := &goResults{Date: time.Now().Format(time.DateOnly), Machine: thisMachine(), Go: output("go", "env", "GOVERSION"), Count: count}
	for _, key := range order {
		m := median(runs[key])
		switch key.group {
		case "Router":
			g.Router = append(g.Router, m)
		case "Inertia/visit":
			g.Visit = append(g.Visit, m)
		case "Inertia/first visit":
			g.FirstVisit = append(g.FirstVisit, m)
		}
	}
	return g, nil
}

// A line go test prints for a benchmark:
//
//	BenchmarkInertia/first_visit/tug's_inertia,_ServeMux-10   125550   9555 ns/op   11618 B/op   49 allocs/op
var benchLine = regexp.MustCompile(`^Benchmark(\S+?)(?:-\d+)?\s+\d+\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op`)

// parseBenchLine reads a benchmark's line into its group, as Router or
// Inertia/visit, and its result, by its name, as the benchmark names it:
// go test writes a space in a name as an underscore, and none of the
// names has an underscore of its own.
func parseBenchLine(line string) (string, goBench, bool) {
	m := benchLine.FindStringSubmatch(line)
	if m == nil {
		return "", goBench{}, false
	}
	path := strings.ReplaceAll(m[1], "_", " ")
	i := strings.LastIndexByte(path, '/')
	if i < 0 {
		return "", goBench{}, false
	}
	ns, _ := strconv.ParseFloat(m[2], 64)
	bytes, _ := strconv.ParseInt(m[3], 10, 64)
	allocs, _ := strconv.ParseInt(m[4], 10, 64)
	return path[:i], goBench{Name: path[i+1:], Ns: ns, Bytes: bytes, Allocs: allocs}, true
}

// median is the run with the median time, with its bytes and allocations,
// which hardly vary.
func median(runs []goBench) goBench {
	runs = slices.Clone(runs)
	slices.SortFunc(runs, func(a, b goBench) int { return cmp.Compare(a.Ns, b.Ns) })
	m := runs[len(runs)/2]
	if len(runs)%2 == 0 {
		m.Ns = (runs[len(runs)/2-1].Ns + m.Ns) / 2
	}
	return m
}
