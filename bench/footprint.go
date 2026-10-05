package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

// An appFootprint is what an app takes besides time: memory, as it starts
// and as it serves, how long it takes to start, and its size.
type appFootprint struct {
	Name string `json:"name"`
	// Processes is how many the app runs, as its process group has them.
	Processes int `json:"processes"`
	// Startup is the time from starting the app to its first answer to the
	// page, the median of a few starts, in milliseconds.
	Startup float64 `json:"startup_ms"`
	// The memory of all its processes, in MiB: once it has answered its
	// first requests, at the most while under load, and idle again after.
	MemoryStart float64 `json:"memory_start_mib"`
	MemoryPeak  float64 `json:"memory_peak_mib"`
	MemoryAfter float64 `json:"memory_after_mib"`
	// Size is what's deployed, in MiB: tug's one binary, which needs
	// nothing installed beside it.
	Size float64 `json:"size_mib"`
}

type footprintResults struct {
	Date    string  `json:"date"`
	Machine machine `json:"machine"`
	// Memory is how memory was measured: macOS's footprint, or Linux's PSS.
	Memory string `json:"memory"`
	// Load is the load the peak and after were measured around, in seconds.
	Load float64        `json:"load_s"`
	Apps []appFootprint `json:"apps"`
}

// footprint measures a: its size, its startup over starts cold starts, and
// its memory around a load of the page's visits for as long as under.
func footprint(a app, starts int, under time.Duration) (appFootprint, error) {
	f := appFootprint{Name: a.Name}
	size, err := a.size()
	if err != nil {
		return f, err
	}
	f.Size = mib(size)

	var times []time.Duration
	for range starts {
		took, err := a.startup()
		if err != nil {
			return f, err
		}
		times = append(times, took)
	}
	slices.Sort(times)
	f.Startup = float64(times[len(times)/2].Microseconds()) / 1000

	s, err := a.start()
	if err != nil {
		return f, err
	}
	defer s.stop()
	if err := s.ready(2 * time.Minute); err != nil {
		return f, err
	}
	_, visit, err := s.requests()
	if err != nil {
		return f, err
	}
	// Settled, its first requests answered, as what's set up at the first
	// request has been.
	time.Sleep(2 * time.Second)
	pids := s.processes()
	f.Processes = len(pids)
	start, err := memoryOf(pids)
	if err != nil {
		return f, err
	}
	f.MemoryStart = mib(start)

	done := make(chan loadResult)
	go func() {
		done <- load{addr: s.addr, request: visit, conns: 64, duration: under}.run()
	}()
	var peak int64
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for running := true; running; {
		select {
		case r := <-done:
			if r.Errors > 0 {
				return f, fmt.Errorf("%s answered a visit with %d errors under load; its log:\n%s", a.Name, r.Errors, s.tail())
			}
			running = false
		case <-tick.C:
			// Each sample is of the group's processes there then.
			if m, err := memoryOf(s.processes()); err == nil {
				peak = max(peak, m)
			}
		}
	}
	f.MemoryPeak = mib(peak)
	time.Sleep(2 * time.Second)
	after, err := memoryOf(s.processes())
	if err != nil {
		return f, err
	}
	f.MemoryAfter = mib(after)
	return f, nil
}

func mib(bytes int64) float64 { return float64(bytes) / (1 << 20) }

// startup starts a, and returns how long it took to answer the page, which
// it asks for as often as a process can be started and listen, every 5 ms.
func (a app) startup() (time.Duration, error) {
	began := time.Now()
	s, err := a.start()
	if err != nil {
		return 0, err
	}
	defer s.stop()
	client := &http.Client{Timeout: 5 * time.Second}
	url := "http://" + s.addr + "/posts/42"
	for time.Since(began) < 2*time.Minute {
		select {
		case err := <-s.exit:
			return 0, fmt.Errorf("%s stopped as it started (%v); its log:\n%s", a.Name, err, s.tail())
		default:
		}
		if resp, err := client.Get(url); err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return time.Since(began), nil
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	return 0, fmt.Errorf("%s didn't answer in 2 minutes; its log:\n%s", a.Name, s.tail())
}

// processes are the server's, those in its process group.
func (s *server) processes() []int {
	out, err := exec.Command("ps", "-A", "-o", "pid=,pgid=").Output()
	if err != nil {
		return nil
	}
	var pids []int
	group := s.cmd.Process.Pid
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) != 2 {
			continue
		}
		pid, _ := strconv.Atoi(f[0])
		pgid, _ := strconv.Atoi(f[1])
		if pgid == group {
			pids = append(pids, pid)
		}
	}
	return pids
}

// memoryOf is the memory pids take together, counting what they share once:
// on macOS, footprint's, the memory Activity Monitor shows; on Linux, the
// sum of their proportional set sizes.
func memoryOf(pids []int) (int64, error) {
	if len(pids) == 0 {
		return 0, errors.New("no processes to measure")
	}
	switch runtime.GOOS {
	case "darwin":
		return footprintOf(pids)
	case "linux":
		var total int64
		for _, pid := range pids {
			kb, err := strconv.ParseInt(strings.TrimSuffix(field(fmt.Sprintf("/proc/%d/smaps_rollup", pid), "Pss", ":"), " kB"), 10, 64)
			if err != nil {
				continue // gone since it was listed
			}
			total += kb << 10
		}
		return total, nil
	}
	return 0, fmt.Errorf("measuring memory on %s isn't done", runtime.GOOS)
}

// memoryMethod says how memoryOf measures, for the tables.
func memoryMethod() string {
	if runtime.GOOS == "linux" {
		return "PSS"
	}
	return "footprint"
}

func footprintOf(pids []int) (int64, error) {
	out, err := os.CreateTemp("", "bench-footprint-*.json")
	if err != nil {
		return 0, err
	}
	out.Close()
	defer os.Remove(out.Name())
	args := []string{"-j", out.Name(), "--noCategories"}
	for _, pid := range pids {
		args = append(args, "-p", strconv.Itoa(pid))
	}
	if b, err := exec.Command("footprint", args...).CombinedOutput(); err != nil {
		return 0, fmt.Errorf("footprint: %v: %s", err, b)
	}
	b, err := os.ReadFile(out.Name())
	if err != nil {
		return 0, err
	}
	var r struct {
		Total int64 `json:"total footprint"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return 0, fmt.Errorf("footprint's JSON: %w", err)
	}
	return r.Total, nil
}

// size is the bytes of what's deployed of a: the files of its Deployed
// paths.
func (a app) size() (int64, error) {
	var total int64
	for _, p := range a.Deployed {
		err := filepath.WalkDir(filepath.Join(a.dir(), p), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Type().IsRegular() {
				info, err := d.Info()
				if err != nil {
					return err
				}
				total += info.Size()
			}
			return nil
		})
		if err != nil {
			return 0, fmt.Errorf("%s's size: %w", a.Name, err)
		}
	}
	return total, nil
}
