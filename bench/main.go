// Command bench measures tug beside other frameworks, and keeps what it
// measured in results.json, which the website's home page shows, and the
// tables in docs/benchmarks.md and the README are written from. Run it in
// bench/:
//
//	go run . setup             # each app's dependencies and build, by its setup.sh
//	go run . check             # each app started, and its page checked, with no load
//	go run . go                # tug, ServeMux, Gin, Echo, Chi and gonertia, in one process
//	go run . http              # tug, Laravel, Rails, Django and AdonisJS, each under load
//	go run . http tug rails    # only these apps, keeping the others' results
//	go run . docs              # the docs' tables again, from results.json
//
// go measures what each framework adds to a request, with go test's own
// benchmarks; http measures how many requests each app answers a second
// over HTTP, and how long they take, an app at a time, each served as its
// framework's docs say to in production. Close what else runs on the
// machine first: the load and the app share its cores.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"slices"
	"time"
)

func main() {
	var (
		file     = flag.String("results", "results.json", "the results file")
		count    = flag.Int("count", 10, "go: how many times each benchmark runs, for the median")
		conns    = flag.Int("conns", 64, "http: the connections the load keeps open")
		warmup   = flag.Duration("warmup", 10*time.Second, "http: the load before counting, for JITs, caches and pools")
		duration = flag.Duration("duration", 10*time.Second, "http: each round's load")
		rounds   = flag.Int("rounds", 3, "http: rounds of each page, for the median")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: go run . setup|check|go|http|docs [app...]\n\napps: tug, laravel, rails, django, adonis\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	log.SetFlags(0)
	log.SetPrefix("bench: ")
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}

	r, err := readResults(*file)
	if err != nil {
		log.Fatal(err)
	}
	cmd, names := flag.Arg(0), flag.Args()[1:]
	switch cmd {
	case "setup":
		for _, a := range chosen(names) {
			log.Printf("%s: setup.sh", a.Name)
			if err := a.setup(); err != nil {
				log.Fatalf("%s: setup.sh: %v", a.Name, err)
			}
		}
		return
	case "check":
		for _, a := range chosen(names) {
			firstVisit, visit, err := check(a)
			if err != nil {
				log.Fatal(err)
			}
			log.Printf("%s: answers a first visit and a visit with the page; their requests are %d and %d bytes", a.Name, len(firstVisit), len(visit))
		}
		return
	case "go":
		if r.Go, err = runGo(*count); err != nil {
			log.Fatal(err)
		}
	case "http":
		if r.HTTP == nil || r.HTTP.Conns != *conns {
			// Results under another load don't compare with these.
			r.HTTP = &httpResults{}
		}
		h := r.HTTP
		h.Date, h.Machine, h.Conns = time.Now().Format(time.DateOnly), thisMachine(), *conns
		h.Warmup, h.Seconds, h.Rounds = warmup.Seconds(), duration.Seconds(), *rounds
		for _, a := range chosen(names) {
			res, err := measure(a, *conns, *warmup, *duration, *rounds)
			if err != nil {
				log.Fatal(err)
			}
			if i := slices.IndexFunc(h.Apps, func(o appResult) bool { return o.Name == a.Name }); i >= 0 {
				h.Apps[i] = res
			} else {
				h.Apps = append(h.Apps, res)
			}
			// In the apps' order, whichever ran.
			slices.SortStableFunc(h.Apps, func(x, y appResult) int { return order(x.Name) - order(y.Name) })
			// Kept as each app finishes, so a later one's failing loses
			// nothing.
			if err := r.write(*file); err != nil {
				log.Fatal(err)
			}
		}
	case "docs":
	default:
		flag.Usage()
		os.Exit(2)
	}
	if err := r.write(*file); err != nil {
		log.Fatal(err)
	}
	if err := r.writeDocs(); err != nil {
		log.Fatal(err)
	}
}

// chosen is the apps named, or every app.
func chosen(names []string) []app {
	if len(names) == 0 {
		return apps
	}
	var out []app
	for _, n := range names {
		a, ok := appNamed(n)
		if !ok {
			log.Fatalf("no app %q: the apps are tug, laravel, rails, django and adonis", n)
		}
		out = append(out, a)
	}
	return out
}

func order(name string) int {
	return slices.IndexFunc(apps, func(a app) bool { return a.Name == name })
}

// check starts a and checks it answers each request with the page,
// returning the requests the load would send.
func check(a app) (firstVisit, visit []byte, err error) {
	s, err := a.start()
	if err != nil {
		return nil, nil, err
	}
	defer s.stop()
	if err := s.ready(2 * time.Minute); err != nil {
		return nil, nil, err
	}
	return s.requests()
}

// measure starts a, checks its page, and puts each page under load, a
// visit's and a first visit's, rounds times after a warmup.
func measure(a app, conns int, warmup, duration time.Duration, rounds int) (appResult, error) {
	res := appResult{Name: a.Name, Language: a.Language, Server: a.Server}
	var err error
	if res.Versions, err = a.versions(); err != nil {
		return res, err
	}
	log.Printf("%s: starting (%s)", a.Name, a.Server)
	s, err := a.start()
	if err != nil {
		return res, err
	}
	defer s.stop()
	if err := s.ready(2 * time.Minute); err != nil {
		return res, err
	}
	firstVisit, visit, err := s.requests()
	if err != nil {
		return res, err
	}
	for _, p := range []struct {
		name    string
		request []byte
		result  *loadResult
	}{{"a visit", visit, &res.Visit}, {"a first visit", firstVisit, &res.FirstVisit}} {
		l := load{addr: s.addr, request: p.request, conns: conns, warmup: warmup, duration: duration}
		var runs []loadResult
		for i := range rounds {
			if i > 0 {
				l.warmup = time.Second
			}
			r := l.run()
			log.Printf("%s, %s: %s a second, p50 %s, p99 %s, %d errors", a.Name, p.name, thousands(r.PerSecond), millis(r.P50), millis(r.P99), r.Errors)
			runs = append(runs, r)
		}
		slices.SortFunc(runs, func(x, y loadResult) int {
			switch {
			case x.PerSecond < y.PerSecond:
				return -1
			case x.PerSecond > y.PerSecond:
				return 1
			}
			return 0
		})
		*p.result = runs[len(runs)/2]
		if p.result.Errors > 0 {
			return res, fmt.Errorf("%s answered %s with %d errors under load; its log:\n%s", a.Name, p.name, p.result.Errors, s.tail())
		}
	}
	return res, nil
}
