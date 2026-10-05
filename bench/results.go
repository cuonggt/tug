package main

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

// results is results.json: what the last runs measured, and where, which
// the website's home page reads, and the tables in the guide and the
// README are written from.
type results struct {
	Go   *goResults   `json:"go,omitempty"`
	HTTP *httpResults `json:"http,omitempty"`
}

type goResults struct {
	Date    string  `json:"date"`
	Machine machine `json:"machine"`
	Go      string  `json:"go"`
	// Count is how many times each benchmark ran; each number is the
	// median of its runs.
	Count      int       `json:"count"`
	Router     []goBench `json:"router"`
	Visit      []goBench `json:"visit"`
	FirstVisit []goBench `json:"first_visit"`
}

type goBench struct {
	Name   string  `json:"name"`
	Ns     float64 `json:"ns_op"`
	Bytes  int64   `json:"bytes_op"`
	Allocs int64   `json:"allocs_op"`
}

type httpResults struct {
	Date    string  `json:"date"`
	Machine machine `json:"machine"`
	Conns   int     `json:"conns"`
	Warmup  float64 `json:"warmup_s"`
	Seconds float64 `json:"seconds"`
	// Rounds is how many loads of Seconds each app's page had, after its
	// warmup; each result is the round with the median requests a second.
	Rounds int         `json:"rounds"`
	Apps   []appResult `json:"apps"`
}

type appResult struct {
	Name       string     `json:"name"`
	Language   string     `json:"language"`
	Server     string     `json:"server"`
	Versions   []string   `json:"versions"`
	Visit      loadResult `json:"visit"`
	FirstVisit loadResult `json:"first_visit"`
}

type machine struct {
	CPU    string `json:"cpu"`
	Cores  int    `json:"cores"`
	Memory string `json:"memory"`
	OS     string `json:"os"`
}

func readResults(name string) (results, error) {
	var r results
	b, err := os.ReadFile(name)
	if errors.Is(err, fs.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return r, err
	}
	return r, json.Unmarshal(b, &r)
}

func (r results) write(name string) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(name, append(b, '\n'), 0o644)
}

// thisMachine is what the benchmarks run on, as the tables say it.
func thisMachine() machine {
	m := machine{CPU: "unknown", Cores: runtime.NumCPU(), OS: runtime.GOOS}
	switch runtime.GOOS {
	case "darwin":
		m.CPU = output("sysctl", "-n", "machdep.cpu.brand_string")
		if b, err := strconv.ParseInt(output("sysctl", "-n", "hw.memsize"), 10, 64); err == nil {
			m.Memory = fmt.Sprintf("%d GB", b>>30)
		}
		m.OS = strings.TrimSpace(output("sw_vers", "-productName") + " " + output("sw_vers", "-productVersion"))
	case "linux":
		if v := field("/proc/cpuinfo", "model name", ":"); v != "" {
			m.CPU = v
		}
		if kb, err := strconv.ParseInt(strings.TrimSuffix(field("/proc/meminfo", "MemTotal", ":"), " kB"), 10, 64); err == nil {
			m.Memory = fmt.Sprintf("%d GB", (kb+1<<19)>>20)
		}
		if v := field("/etc/os-release", "PRETTY_NAME", "="); v != "" {
			m.OS = strings.Trim(v, `"`)
		}
	}
	return m
}

func output(name string, args ...string) string {
	out, _ := exec.Command(name, args...).Output()
	return strings.TrimSpace(string(out))
}

// field is the value of the first line of file that starts with name and
// then sep.
func field(file, name, sep string) string {
	f, err := os.Open(file)
	if err != nil {
		return ""
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		k, v, ok := strings.Cut(s.Text(), sep)
		if ok && strings.TrimSpace(k) == name {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func (m machine) String() string {
	s := fmt.Sprintf("%s, %d cores", m.CPU, m.Cores)
	if m.Memory != "" {
		s += ", " + m.Memory
	}
	return s + ", " + m.OS
}

// The docs the tables go in, between a <!-- bench:name --> line and a
// <!-- /bench:name --> line, so the numbers in them are always the
// results', never typed in.
var docs = []string{"../docs/benchmarks.md", "../README.md"}

func (r results) writeDocs() error {
	tables := map[string]string{}
	if r.HTTP != nil {
		tables["http"] = r.HTTP.table()
		tables["http-setup"] = r.HTTP.setup()
		tables["http-machine"] = fmt.Sprintf("Measured %s on %s.\n", r.HTTP.Date, r.HTTP.Machine)
	}
	if r.Go != nil {
		tables["router"] = r.Go.routerTable()
		tables["inertia"] = r.Go.inertiaTable()
		tables["go-setup"] = r.Go.setup()
	}
	for _, name := range docs {
		b, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		out := b
		for region, text := range tables {
			out = replaceRegion(out, region, text)
		}
		if !bytes.Equal(out, b) {
			if err := os.WriteFile(name, out, 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

// replaceRegion puts text between doc's lines that start and end region,
// when it has them, with a blank line before and after it, so that
// Markdown reads it as blocks of its own, apart from the comments.
func replaceRegion(doc []byte, region, text string) []byte {
	start, end := []byte("<!-- bench:"+region+" -->\n"), []byte("<!-- /bench:"+region+" -->")
	i := bytes.Index(doc, start)
	if i < 0 {
		return doc
	}
	i += len(start)
	j := bytes.Index(doc[i:], end)
	if j < 0 {
		return doc
	}
	return slices.Concat(doc[:i], []byte("\n"+strings.TrimSpace(text)+"\n\n"), doc[i+j:])
}

// table is the apps, the most requests a second first, with how many
// times as many tug answered.
func (h *httpResults) table() string {
	byVisits := slices.Clone(h.Apps)
	slices.SortStableFunc(byVisits, func(a, b appResult) int { return cmp.Compare(b.Visit.PerSecond, a.Visit.PerSecond) })
	var tug *appResult
	for i := range h.Apps {
		if h.Apps[i].Name == "tug" {
			tug = &h.Apps[i]
		}
	}
	var b strings.Builder
	b.WriteString("| | Language | Visits a second | p99 | First visits a second | p99 |\n")
	b.WriteString("|---|---|--:|--:|--:|--:|\n")
	for _, a := range byVisits {
		name := a.Name
		if a.Name == "tug" {
			name = "**tug**"
		}
		fmt.Fprintf(&b, "| %s | %s | %s%s | %s | %s%s | %s |\n", name, a.Language,
			thousands(a.Visit.PerSecond), times(tug, a, func(r appResult) float64 { return r.Visit.PerSecond }), millis(a.Visit.P99),
			thousands(a.FirstVisit.PerSecond), times(tug, a, func(r appResult) float64 { return r.FirstVisit.PerSecond }), millis(a.FirstVisit.P99))
	}
	return b.String()
}

// times says how many times a's number tug's is, after a's own.
func times(tug *appResult, a appResult, of func(appResult) float64) string {
	if tug == nil || a.Name == "tug" || of(a) == 0 {
		return ""
	}
	return fmt.Sprintf(" (tug %.1f×)", of(*tug)/of(a))
}

func (h *httpResults) setup() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Measured %s on %s: %d connections, %g seconds of warmup, then %d rounds of %g seconds, the median round's. Each app's versions and server:\n\n",
		h.Date, h.Machine, h.Conns, h.Warmup, h.Rounds, h.Seconds)
	for _, a := range h.Apps {
		fmt.Fprintf(&b, "- **%s**: %s; %s.\n", a.Name, strings.Join(a.Versions, ", "), a.Server)
	}
	return b.String()
}

func (g *goResults) routerTable() string {
	byTime := slices.Clone(g.Router)
	slices.SortStableFunc(byTime, func(a, b goBench) int { return cmp.Compare(a.Ns, b.Ns) })
	var b strings.Builder
	b.WriteString("| | ns a request | Bytes | Allocations |\n")
	b.WriteString("|---|--:|--:|--:|\n")
	for _, r := range byTime {
		fmt.Fprintf(&b, "| %s | %s | %d | %d |\n", bold(r.Name), trimFloat(r.Ns), r.Bytes, r.Allocs)
	}
	return b.String()
}

func (g *goResults) inertiaTable() string {
	var b strings.Builder
	b.WriteString("| | A visit, µs | Bytes | Allocations | A first visit, µs | Bytes | Allocations |\n")
	b.WriteString("|---|--:|--:|--:|--:|--:|--:|\n")
	for _, v := range g.Visit {
		i := slices.IndexFunc(g.FirstVisit, func(f goBench) bool { return f.Name == v.Name })
		if i < 0 {
			continue
		}
		f := g.FirstVisit[i]
		fmt.Fprintf(&b, "| %s | %.1f | %s | %d | %.1f | %s | %d |\n", bold(v.Name),
			v.Ns/1000, thousands(float64(v.Bytes)), v.Allocs, f.Ns/1000, thousands(float64(f.Bytes)), f.Allocs)
	}
	return b.String()
}

func (g *goResults) setup() string {
	return fmt.Sprintf("Measured %s on %s, with Go %s: the median of %d runs of each.\n", g.Date, g.Machine, strings.TrimPrefix(g.Go, "go"), g.Count)
}

// bold is name, in bold when it's tug's.
func bold(name string) string {
	if strings.HasPrefix(name, "tug") {
		return "**" + name + "**"
	}
	return name
}

// thousands is n rounded, with commas between its thousands.
func thousands(n float64) string {
	s := strconv.FormatInt(int64(n+0.5), 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func millis(ms float64) string {
	switch {
	case ms >= 100:
		return fmt.Sprintf("%.0f ms", ms)
	case ms >= 10:
		return fmt.Sprintf("%.1f ms", ms)
	default:
		return fmt.Sprintf("%.2f ms", ms)
	}
}

func trimFloat(f float64) string {
	if f >= 100 {
		return strconv.FormatFloat(f, 'f', 0, 64)
	}
	return strconv.FormatFloat(f, 'f', 1, 64)
}
