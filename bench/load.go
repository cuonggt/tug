package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/bits"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// A load is requests sent to an app over HTTP/1.1, as wrk sends them: on
// conns connections kept alive, each sending its next request as the
// answer to its last one comes in, so the app answers as many a second as
// it can. The answers are read by readResponse, which looks at no more of
// them than it has to, so that the clock measures the app and not the
// client, which runs on the same machine.
type load struct {
	addr     string // host:port
	request  []byte // the request, as it's sent
	conns    int
	warmup   time.Duration // sent first and not counted: for JITs, caches and pools
	duration time.Duration
}

// A loadResult is what a load counted after its warmup.
type loadResult struct {
	Requests  int64   `json:"requests"`
	Seconds   float64 `json:"seconds"`
	PerSecond float64 `json:"per_second"`
	// Errors are answers other than 200, and connections that broke.
	Errors int64 `json:"errors"`
	// Bytes is the size of an answer, its headers and body, on average.
	Bytes int64 `json:"bytes"`
	// The time from sending a request to reading the end of its answer,
	// in milliseconds.
	P50 float64 `json:"p50_ms"`
	P99 float64 `json:"p99_ms"`
	Max float64 `json:"max_ms"`
}

const (
	warming = iota
	counting
	done
)

func (l load) run() loadResult {
	var (
		phase   atomic.Int32
		wg      sync.WaitGroup
		workers = make([]*worker, l.conns)
	)
	for i := range workers {
		w := &worker{load: &l, phase: &phase}
		workers[i] = w
		wg.Go(w.run)
	}
	time.Sleep(l.warmup)
	phase.Store(counting)
	start := time.Now()
	time.Sleep(l.duration)
	phase.Store(done)
	took := time.Since(start)
	wg.Wait()

	var (
		r     loadResult
		h     histogram
		bytes int64
	)
	for _, w := range workers {
		r.Requests += w.requests
		r.Errors += w.errors
		bytes += w.bytes
		h.merge(&w.latency)
	}
	r.Seconds = took.Seconds()
	r.PerSecond = float64(r.Requests) / r.Seconds
	if r.Requests > 0 {
		r.Bytes = bytes / r.Requests
	}
	r.P50, r.P99, r.Max = ms(h.percentile(0.50)), ms(h.percentile(0.99)), ms(h.max)
	return r
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// A worker is one connection's requests, one at a time.
type worker struct {
	*load
	phase *atomic.Int32

	requests, errors, bytes int64
	latency                 histogram
}

func (w *worker) run() {
	var (
		conn net.Conn
		br   *bufio.Reader
	)
	defer func() {
		if conn != nil {
			conn.Close()
		}
	}()
	for {
		if w.phase.Load() == done {
			return
		}
		if conn == nil {
			c, err := net.DialTimeout("tcp", w.addr, 5*time.Second)
			if err != nil {
				w.failed()
				time.Sleep(10 * time.Millisecond)
				continue
			}
			conn, br = c, bufio.NewReaderSize(c, 64<<10)
		}
		start := time.Now()
		conn.SetDeadline(start.Add(30 * time.Second))
		status, size, keep, err := w.send(conn, br)
		took := time.Since(start)
		if err != nil {
			w.failed()
			abort(conn)
			conn = nil
			continue
		}
		// An answer counts when it came in while counting, wherever its
		// request started.
		if w.phase.Load() == counting {
			if status == 200 {
				w.requests++
				w.bytes += size
				w.latency.record(took)
			} else {
				w.errors++
			}
		}
		if !keep {
			abort(conn)
			conn = nil
		}
	}
}

// abort closes a connection the app has closed, or that broke, with a
// reset, so that it doesn't wait out TIME_WAIT on the client's side: on
// macOS, that's 30 seconds, and the 16,384 ports a client has run out in
// seconds at thousands of connections a second.
func abort(conn net.Conn) {
	if tcp, ok := conn.(*net.TCPConn); ok {
		tcp.SetLinger(0)
	}
	conn.Close()
}

func (w *worker) send(conn net.Conn, br *bufio.Reader) (status int, size int64, keep bool, err error) {
	if _, err := conn.Write(w.request); err != nil {
		return 0, 0, false, err
	}
	return readResponse(br)
}

func (w *worker) failed() {
	if w.phase.Load() == counting {
		w.errors++
	}
}

var errMalformed = errors.New("a malformed response")

// readResponse reads one response from br: its status, its size, and
// whether its connection stays open for another. Its body is read past,
// by its Content-Length or its chunks, or else to the end of the
// connection.
func readResponse(br *bufio.Reader) (status int, size int64, keep bool, err error) {
	line, err := br.ReadSlice('\n')
	if err != nil {
		return 0, 0, false, err
	}
	size += int64(len(line))
	// HTTP/1.1 200 OK
	if len(line) < 12 || !bytes.HasPrefix(line, []byte("HTTP/1.")) {
		return 0, size, false, errMalformed
	}
	status, err = strconv.Atoi(string(line[9:12]))
	if err != nil {
		return 0, size, false, errMalformed
	}
	keep = line[7] == '1' // HTTP/1.1 keeps a connection open unless it says otherwise; 1.0 doesn't
	length, chunked := int64(-1), false
	for {
		line, err = br.ReadSlice('\n')
		if err != nil {
			return status, size, false, err
		}
		size += int64(len(line))
		if len(bytes.TrimRight(line, "\r\n")) == 0 {
			break
		}
		name, value, ok := bytes.Cut(line, []byte(":"))
		if !ok {
			continue
		}
		value = bytes.TrimSpace(value)
		switch {
		case equalFold(name, "Content-Length"):
			if length, err = strconv.ParseInt(string(value), 10, 64); err != nil || length < 0 {
				return status, size, false, errMalformed
			}
		case equalFold(name, "Transfer-Encoding"):
			chunked = bytes.Contains(bytes.ToLower(value), []byte("chunked"))
		case equalFold(name, "Connection"):
			v := bytes.ToLower(value)
			if bytes.Contains(v, []byte("close")) {
				keep = false
			} else if bytes.Contains(v, []byte("keep-alive")) {
				keep = true
			}
		}
	}
	var n int64
	switch {
	case status/100 == 1 || status == 204 || status == 304:
	case chunked:
		n, err = readChunks(br)
	case length >= 0:
		var d int
		d, err = br.Discard(int(length))
		n = int64(d)
	default:
		n, err = io.Copy(io.Discard, br)
		keep = false
	}
	return status, size + n, keep, err
}

// readChunks reads past a chunked body and its trailer, returning their
// size.
func readChunks(br *bufio.Reader) (int64, error) {
	var size int64
	for {
		line, err := br.ReadSlice('\n')
		if err != nil {
			return size, err
		}
		size += int64(len(line))
		hex, _, _ := bytes.Cut(bytes.TrimRight(line, "\r\n"), []byte(";"))
		n, err := strconv.ParseInt(string(bytes.TrimSpace(hex)), 16, 64)
		if err != nil || n < 0 {
			return size, errMalformed
		}
		if n == 0 {
			for { // the trailer, to the empty line
				line, err := br.ReadSlice('\n')
				if err != nil {
					return size, err
				}
				size += int64(len(line))
				if len(bytes.TrimRight(line, "\r\n")) == 0 {
					return size, nil
				}
			}
		}
		d, err := br.Discard(int(n) + 2) // the chunk and its CRLF
		size += int64(d)
		if err != nil {
			return size, err
		}
	}
}

func equalFold(b []byte, s string) bool {
	return len(b) == len(s) && bytes.EqualFold(b, []byte(s))
}

// A histogram counts durations, in microseconds, in buckets a 64th of a
// power of two wide, so a percentile it gives is within 1.6% of the one
// it stands for, in a fixed amount of memory.
type histogram struct {
	counts [64 * 48]int64
	n      int64
	max    time.Duration
}

// bucket is v's bucket: v itself below 128, and above, v's top seven bits
// and how far they're shifted.
func bucket(v uint64) int {
	if v < 128 {
		return int(v)
	}
	shift := bits.Len64(v) - 7
	return 64*shift + int(v>>shift)
}

// low is the least value in bucket i.
func low(i int) uint64 {
	if i < 128 {
		return uint64(i)
	}
	shift := i/64 - 1
	return uint64(i-64*shift) << shift
}

func (h *histogram) record(d time.Duration) {
	us := uint64(max(d.Microseconds(), 0))
	h.counts[min(bucket(us), len(h.counts)-1)]++
	h.n++
	h.max = max(h.max, d)
}

func (h *histogram) merge(o *histogram) {
	for i, c := range o.counts {
		h.counts[i] += c
	}
	h.n += o.n
	h.max = max(h.max, o.max)
}

// percentile is the duration q of the durations are at or under, the
// middle of its bucket.
func (h *histogram) percentile(q float64) time.Duration {
	if h.n == 0 {
		return 0
	}
	rank := int64(q*float64(h.n) + 0.5)
	var seen int64
	for i, c := range h.counts {
		seen += c
		if seen >= max(rank, 1) {
			mid := (low(i) + low(i+1)) / 2
			return min(time.Duration(mid)*time.Microsecond, h.max)
		}
	}
	return h.max
}

// requestFor is the bytes of a GET of path from addr, with headers, each
// a "Name: value" line.
func requestFor(addr, path string, headers ...string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: tug-bench\r\n", path, addr)
	for _, h := range headers {
		b.WriteString(h + "\r\n")
	}
	b.WriteString("\r\n")
	return b.Bytes()
}
