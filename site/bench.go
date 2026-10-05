package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
)

// benchResults is the part of bench/results.json the home page shows, as
// bench/'s runner writes it: tug's own numbers.
type benchResults struct {
	Go *struct {
		Router []benchTime `json:"router"`
	} `json:"go"`
	HTTP *struct {
		Date    string       `json:"date"`
		Machine benchMachine `json:"machine"`
		Apps    []benchApp   `json:"apps"`
	} `json:"http"`
	Footprint *struct {
		Apps []benchFootprint `json:"apps"`
	} `json:"footprint"`
}

// benchFootprint is an app's memory, startup and size.
type benchFootprint struct {
	Name        string  `json:"name"`
	Startup     float64 `json:"startup_ms"`
	MemoryStart float64 `json:"memory_start_mib"`
	MemoryPeak  float64 `json:"memory_peak_mib"`
	Size        float64 `json:"size_mib"`
}

type benchMachine struct {
	CPU   string `json:"cpu"`
	Cores int    `json:"cores"`
	OS    string `json:"os"`
}

type benchApp struct {
	Name       string    `json:"name"`
	Visit      benchLoad `json:"visit"`
	FirstVisit benchLoad `json:"first_visit"`
}

type benchLoad struct {
	PerSecond float64 `json:"per_second"`
	P99       float64 `json:"p99_ms"`
}

type benchTime struct {
	Name string  `json:"name"`
	Ns   float64 `json:"ns_op"`
}

// A bench is what the home page shows of the benchmarks: tug's own
// numbers, over HTTP and as it runs, and what it adds to a request over
// Go's own router.
type bench struct {
	// Visits and Memory are the headline's: tug's visits a second, and its
	// memory under load, when that was measured.
	Visits, Memory string
	Stats          []stat
	Machine, Date  string
}

// A stat is one of tug's numbers: its value, what it counts, and how it
// was measured.
type stat struct {
	Value, Title, About string
}

// readBenchResults reads bench/results.json, or returns nil without one.
func readBenchResults(root string) (*benchResults, error) {
	b, err := os.ReadFile(filepath.Join(root, "bench", "results.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var r benchResults
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("bench/results.json: %w", err)
	}
	return &r, nil
}

// benchOf is what the home page shows of r; without tug's run over HTTP,
// it shows no numbers.
func benchOf(r *benchResults) *bench {
	if r == nil || r.HTTP == nil {
		return nil
	}
	i := slices.IndexFunc(r.HTTP.Apps, func(a benchApp) bool { return a.Name == "tug" })
	if i < 0 {
		return nil
	}
	tug := r.HTTP.Apps[i]
	v := &bench{Machine: r.HTTP.Machine.String(), Date: r.HTTP.Date, Visits: thousands(tug.Visit.PerSecond)}
	v.Stats = append(v.Stats,
		stat{thousands(tug.Visit.PerSecond), "visits a second", "Inertia's client asking for the page, answered as JSON, 99 in 100 within " + millis(tug.Visit.P99) + "."},
		stat{thousands(tug.FirstVisit.PerSecond), "first visits a second", "A browser asking for the page, answered with its HTML, 99 in 100 within " + millis(tug.FirstVisit.P99) + "."},
	)
	if r.Footprint != nil {
		if i := slices.IndexFunc(r.Footprint.Apps, func(a benchFootprint) bool { return a.Name == "tug" }); i >= 0 {
			f := r.Footprint.Apps[i]
			v.Memory = megabytes(f.MemoryPeak)
			v.Stats = append(v.Stats,
				stat{megabytes(f.MemoryPeak), "of memory under load", megabytes(f.MemoryStart) + " once it has started, idle."},
				stat{startup(f.Startup), "to start", "From starting the binary to its first answer to the page."},
				stat{megabytes(f.Size), "to deploy", "One static binary, with nothing to install beside it."},
			)
		}
	}
	if added, ok := addedToRequest(r); ok {
		v.Stats = append(v.Stats, stat{added, "added to a request", "What tug's App adds to ServeMux, Go's own router, in one process."})
	}
	return v
}

// addedToRequest is how much longer tug's App took to route a request than
// ServeMux alone, when r has both and it took longer.
func addedToRequest(r *benchResults) (string, bool) {
	if r.Go == nil {
		return "", false
	}
	var mux, tug float64
	for _, t := range r.Go.Router {
		switch t.Name {
		case "ServeMux":
			mux = t.Ns
		case "tug":
			tug = t.Ns
		}
	}
	if mux <= 0 || tug <= mux {
		return "", false
	}
	return strconv.FormatFloat(tug-mux, 'f', 0, 64) + " ns", true
}

func (m benchMachine) String() string {
	return fmt.Sprintf("%s, %d cores, %s", m.CPU, m.Cores, m.OS)
}

// megabytes is mib, as bench/'s runner says it in the guide.
func megabytes(mib float64) string {
	if mib >= 100 {
		return thousands(mib) + " MB"
	}
	return strconv.FormatFloat(mib, 'f', 1, 64) + " MB"
}

// startup is ms milliseconds, in seconds from a second up, as bench/'s
// runner says it.
func startup(ms float64) string {
	if ms >= 1000 {
		return strconv.FormatFloat(ms/1000, 'f', 1, 64) + " s"
	}
	return strconv.FormatFloat(ms, 'f', 0, 64) + " ms"
}

// millis is ms milliseconds, as bench/'s runner says a percentile.
func millis(ms float64) string {
	switch {
	case ms >= 100:
		return strconv.FormatFloat(ms, 'f', 0, 64) + " ms"
	case ms >= 10:
		return strconv.FormatFloat(ms, 'f', 1, 64) + " ms"
	default:
		return strconv.FormatFloat(ms, 'f', 2, 64) + " ms"
	}
}

// thousands is n rounded, with commas between its thousands.
func thousands(n float64) string {
	s := strconv.FormatInt(int64(n+0.5), 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
