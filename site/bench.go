package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"html/template"
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

// A bench is what the home page shows of the benchmarks, in one of the
// site's languages: tug's own numbers, over HTTP and as it runs, and what
// it adds to a request over Go's own router.
type bench struct {
	// Headline is tug's visits a second, and its memory under load, when
	// that was measured.
	Headline string
	Stats    []stat
	Measured template.HTML // when the numbers were measured, and on what
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

// benchOf is what the home page shows of r, in l; without tug's run over
// HTTP, it shows no numbers.
func benchOf(r *benchResults, l *language) *bench {
	if r == nil || r.HTTP == nil {
		return nil
	}
	i := slices.IndexFunc(r.HTTP.Apps, func(a benchApp) bool { return a.Name == "tug" })
	if i < 0 {
		return nil
	}
	tug := r.HTTP.Apps[i]
	visits := l.thousands(tug.Visit.PerSecond)
	v := &bench{}
	v.Stats = append(v.Stats,
		stat{visits, l.t("visits a second"), l.t("Inertia's client asking for the page, answered as JSON, 99 in 100 within :time.", "time", l.millis(tug.Visit.P99))},
		stat{l.thousands(tug.FirstVisit.PerSecond), l.t("first visits a second"), l.t("A browser asking for the page, answered with its HTML, 99 in 100 within :time.", "time", l.millis(tug.FirstVisit.P99))},
	)
	// Both headlines are said, so that a language has words for either.
	v.Headline = l.t(":visits visits a second.", "visits", visits)
	if r.Footprint != nil {
		if i := slices.IndexFunc(r.Footprint.Apps, func(a benchFootprint) bool { return a.Name == "tug" }); i >= 0 {
			f := r.Footprint.Apps[i]
			memory := l.megabytes(f.MemoryPeak)
			v.Headline = l.t(":visits visits a second, in :memory of memory.", "visits", visits, "memory", memory)
			v.Stats = append(v.Stats,
				stat{memory, l.t("of memory under load"), l.t(":memory once it has started, idle.", "memory", l.megabytes(f.MemoryStart))},
				stat{l.startup(f.Startup), l.t("to start"), l.t("From starting the binary to its first answer to the page.")},
				stat{l.megabytes(f.Size), l.t("to deploy"), l.t("One static binary, with nothing to install beside it.")},
			)
		}
	}
	if added, ok := addedToRequest(r); ok {
		v.Stats = append(v.Stats, stat{added, l.t("added to a request"), l.t("What tug's App adds to ServeMux, Go's own router, in one process.")})
	}
	m := r.HTTP.Machine
	date := html.EscapeString(r.HTTP.Date)
	v.Measured = l.tHTML("Measured :date on :machine.",
		"date", template.HTML(`<time datetime="`+date+`">`+date+`</time>`),
		"machine", l.t(":cpu, :cores cores, :os", "cpu", m.CPU, "cores", m.Cores, "os", m.OS))
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

// megabytes is mib, as bench/'s runner says it in the guide, in l's
// numbers.
func (l *language) megabytes(mib float64) string {
	if mib >= 100 {
		return l.thousands(mib) + " MB"
	}
	return l.decimal(mib, 1) + " MB"
}

// startup is ms milliseconds, in seconds from a second up, as bench/'s
// runner says it.
func (l *language) startup(ms float64) string {
	if ms >= 1000 {
		return l.decimal(ms/1000, 1) + " s"
	}
	return l.decimal(ms, 0) + " ms"
}

// millis is ms milliseconds, as bench/'s runner says a percentile.
func (l *language) millis(ms float64) string {
	switch {
	case ms >= 100:
		return l.decimal(ms, 0) + " ms"
	case ms >= 10:
		return l.decimal(ms, 1) + " ms"
	default:
		return l.decimal(ms, 2) + " ms"
	}
}
