package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// benchResults is the part of bench/results.json the home page shows, as
// bench/'s runner writes it.
type benchResults struct {
	Go *struct {
		Router []benchRender `json:"router"`
		Visit  []benchRender `json:"visit"`
	} `json:"go"`
	HTTP *struct {
		Date    string       `json:"date"`
		Machine benchMachine `json:"machine"`
		Apps    []benchApp   `json:"apps"`
	} `json:"http"`
}

type benchMachine struct {
	CPU   string `json:"cpu"`
	Cores int    `json:"cores"`
	OS    string `json:"os"`
}

type benchApp struct {
	Name       string    `json:"name"`
	Language   string    `json:"language"`
	Visit      benchLoad `json:"visit"`
	FirstVisit benchLoad `json:"first_visit"`
}

type benchLoad struct {
	PerSecond float64 `json:"per_second"`
}

type benchRender struct {
	Name string  `json:"name"`
	Ns   float64 `json:"ns_op"`
}

// A bench is what the home page shows of the benchmarks: each app's
// visits and first visits a second, tug's beside the other frameworks',
// and, in one process, the time the Go adapters take to render the page,
// and the Go routers to route a request.
type bench struct {
	Charts []chart
	// Lead is how many times as many visits tug answered as Next, the
	// fastest of the others, and Most as Fewest, the slowest.
	Lead, Next, Most, Fewest string
	// Others are the other frameworks, as a sentence lists them.
	Others        string
	Machine, Date string
}

// A chart is a table of bars.
type chart struct {
	ID, Title, About string
	Column           string // what its values are, for a screen reader's table
	Bars             []bar
}

// A bar is one row of a chart: its name, its value as it's shown, and
// its length, as a share of the longest.
type bar struct {
	Name  string
	Note  string // what it's written in, or on
	Value string
	Width template.CSS
	Tug   bool
}

// loadBench reads bench/results.json; without one, or without both its
// runs, the home page shows no numbers.
func loadBench(root string) (*bench, error) {
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
	if r.HTTP == nil || r.Go == nil || len(r.HTTP.Apps) < 2 {
		return nil, nil
	}
	v := &bench{Machine: r.HTTP.Machine.String(), Date: r.HTTP.Date}
	var visits, firstVisits []bar

	apps := slices.Clone(r.HTTP.Apps)
	byVisits := func(a, b benchApp) int { return cmp.Compare(b.Visit.PerSecond, a.Visit.PerSecond) }
	slices.SortStableFunc(apps, byVisits)
	var tug float64
	for _, a := range apps {
		if a.Name == "tug" {
			tug = a.Visit.PerSecond
		}
	}
	var others []string
	for _, a := range apps {
		visits = append(visits, bar{Name: a.Name, Note: a.Language, Value: thousands(a.Visit.PerSecond),
			Width: share(a.Visit.PerSecond, apps[0].Visit.PerSecond), Tug: a.Name == "tug"})
		if a.Name != "tug" && a.Visit.PerSecond > 0 {
			others = append(others, a.Name)
			if v.Next == "" {
				v.Next, v.Lead = a.Name, timesOver(tug, a.Visit.PerSecond)
			}
			v.Fewest, v.Most = a.Name, timesOver(tug, a.Visit.PerSecond)
		}
	}
	v.Others = sentence(others)

	slices.SortStableFunc(apps, func(a, b benchApp) int { return cmp.Compare(b.FirstVisit.PerSecond, a.FirstVisit.PerSecond) })
	for _, a := range apps {
		firstVisits = append(firstVisits, bar{Name: a.Name, Note: a.Language, Value: thousands(a.FirstVisit.PerSecond),
			Width: share(a.FirstVisit.PerSecond, apps[0].FirstVisit.PerSecond), Tug: a.Name == "tug"})
	}

	render := timeBars(r.Go.Visit, func(ns float64) string { return strconv.FormatFloat(ns/1000, 'f', 1, 64) + " µs" })
	router := timeBars(r.Go.Router, func(ns float64) string { return strconv.FormatFloat(ns, 'f', 0, 64) + " ns" })

	v.Charts = []chart{
		{ID: "bench-visits", Title: "Visits a second", About: "Inertia's client asking for the page, answered as JSON. More is better.", Column: "Visits a second", Bars: visits},
		{ID: "bench-first-visits", Title: "First visits a second", About: "A browser asking for the page, answered with its HTML. More is better.", Column: "First visits a second", Bars: firstVisits},
		{ID: "bench-render", Title: "A visit, rendered in Go", About: "The page through each Go adapter, in one process. Less is better.", Column: "Time", Bars: render},
		{ID: "bench-router", Title: "What a router adds", About: "One route through ServeMux alone and each Go router, in one process. Less is better.", Column: "Time", Bars: router},
	}
	return v, nil
}

// timeBars are the bars of times, the least first, each as value says it.
// A name such as "gonertia, ServeMux" is the bar's name, then its note,
// what it's on.
func timeBars(times []benchRender, value func(ns float64) string) []bar {
	times = slices.Clone(times)
	slices.SortStableFunc(times, func(a, b benchRender) int { return cmp.Compare(a.Ns, b.Ns) })
	var longest float64
	for _, t := range times {
		longest = max(longest, t.Ns)
	}
	var bars []bar
	for _, t := range times {
		name, note, _ := strings.Cut(t.Name, ", ")
		bars = append(bars, bar{Name: name, Note: note, Value: value(t.Ns), Width: share(t.Ns, longest), Tug: strings.HasPrefix(t.Name, "tug")})
	}
	return bars
}

func (m benchMachine) String() string {
	return fmt.Sprintf("%s, %d cores, %s", m.CPU, m.Cores, m.OS)
}

// share is n's length as a share of the longest, as a bar's CSS.
func share(n, longest float64) template.CSS {
	if longest <= 0 {
		return "--w: 0%"
	}
	return template.CSS(fmt.Sprintf("--w: %.1f%%", 100*n/longest))
}

// timesOver is how many times b a is: "2.4", or "31" from ten up.
func timesOver(a, b float64) string {
	if t := a / b; t < 10 {
		return strconv.FormatFloat(t, 'f', 1, 64)
	} else {
		return strconv.FormatFloat(t, 'f', 0, 64)
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

// sentence is names as a sentence lists them: "a, b and c".
func sentence(names []string) string {
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}
