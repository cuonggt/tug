package main

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReadResponseReadsPastEachKindOfBody(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		status    int
		keep      bool
	}{
		{"by its length", "HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhello", 200, true},
		{"in chunks", "HTTP/1.1 200 OK\r\ntransfer-encoding: chunked\r\n\r\n5;x=y\r\nhello\r\n1\r\n!\r\n0\r\nTrailer: t\r\n\r\n", 200, true},
		{"with none", "HTTP/1.1 304 Not Modified\r\nETag: x\r\n\r\n", 304, true},
		{"to the connection's end", "HTTP/1.1 200 OK\r\n\r\nhello", 200, false},
		{"and closing", "HTTP/1.1 500 Internal Server Error\r\nConnection: close\r\nContent-Length: 2\r\n\r\nno", 500, false},
		{"of HTTP/1.0", "HTTP/1.0 200 OK\r\nContent-Length: 2\r\n\r\nok", 200, false},
		{"of HTTP/1.0, kept alive", "HTTP/1.0 200 OK\r\nConnection: keep-alive\r\nContent-Length: 2\r\n\r\nok", 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Twice, to see the first was read to its end and no further.
			br := bufio.NewReader(strings.NewReader(tc.raw + tc.raw))
			for range 2 {
				status, size, keep, err := readResponse(br)
				if err != nil {
					t.Fatal(err)
				}
				if status != tc.status || keep != tc.keep {
					t.Errorf("status %d, keep %v; want %d, %v", status, keep, tc.status, tc.keep)
				}
				if tc.keep && size != int64(len(tc.raw)) {
					t.Errorf("size %d, want %d", size, len(tc.raw))
				}
				if !tc.keep {
					break
				}
			}
		})
	}
}

func TestReadResponseRefusesWhatIsntHTTP(t *testing.T) {
	for _, raw := range []string{"SSH-2.0-OpenSSH\r\n", "HTTP/1.1 2x0 OK\r\n\r\n", "HTTP/1.1 200 OK\r\nContent-Length: -1\r\n\r\n"} {
		if _, _, _, err := readResponse(bufio.NewReader(strings.NewReader(raw))); err != errMalformed {
			t.Errorf("%q: %v", raw, err)
		}
	}
}

func TestHistogramPercentilesAreWithinItsBuckets(t *testing.T) {
	var h histogram
	for us := 1; us <= 10000; us++ {
		h.record(time.Duration(us) * time.Microsecond)
	}
	for _, q := range []float64{0.5, 0.9, 0.99} {
		want := q * 10000
		got := float64(h.percentile(q).Microseconds())
		if got < want*0.98 || got > want*1.02 {
			t.Errorf("p%v = %vµs, want about %vµs", q*100, got, want)
		}
	}
	if h.max != 10*time.Millisecond {
		t.Errorf("max %v", h.max)
	}
	for v := range uint64(1 << 20) {
		if i := bucket(v); low(i) > v || low(i+1) <= v {
			t.Fatalf("%d in bucket %d, [%d, %d)", v, i, low(i), low(i+1))
		}
	}
}

func TestALoadCountsWhatAnAppAnswers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Inertia") != "true" {
			http.Error(w, "no", http.StatusBadRequest)
			return
		}
		w.Write([]byte(strings.Repeat("x", 5000))) // more than net/http buffers, so it's chunked
	}))
	defer srv.Close()
	addr := srv.Listener.Addr().String()

	l := load{addr: addr, request: requestFor(addr, "/", "X-Inertia: true"), conns: 4, warmup: 50 * time.Millisecond, duration: 200 * time.Millisecond}
	r := l.run()
	if r.Requests == 0 || r.Errors != 0 || r.Bytes < 5000 || r.P50 <= 0 || r.P99 < r.P50 || r.Max < r.P99 {
		t.Errorf("%+v", r)
	}

	// An app that closes the connection after each answer is answered
	// on a new one each time.
	l.request = requestFor(addr, "/", "X-Inertia: true", "Connection: close")
	if r := l.run(); r.Requests == 0 || r.Errors != 0 {
		t.Errorf("an app that closes each connection: %+v", r)
	}

	l.request = requestFor(addr, "/")
	if r := l.run(); r.Requests != 0 || r.Errors == 0 {
		t.Errorf("an app's 400s counted as answers: %+v", r)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln.Close() // nothing listens there now
	l.addr = ln.Addr().String()
	if r := l.run(); r.Requests != 0 || r.Errors == 0 {
		t.Errorf("an app that isn't there answered: %+v", r)
	}
}
