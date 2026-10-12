package letsencrypt

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/pem"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

// A Manager has all that tug's Run looks for in a source of certificates,
// by the same names, as tug.Certificates says them.
var _ interface {
	GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error)
	Hosts() []string
	HTTPHandler(fallback http.Handler) http.Handler
	TLSConfig() *tls.Config
	Run(ctx context.Context) error
} = (*Manager)(nil)

func TestNewTakesOnlyNamesLetsEncryptCanCheck(t *testing.T) {
	m, err := New(Config{Domains: []string{" Example.COM. ", "www.example.com", "example.com", "bücher.example"}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := m.Hosts(), []string{"example.com", "www.example.com", "xn--bcher-kva.example"}; !slices.Equal(got, want) {
		t.Errorf("Hosts %q, want %q: in lower case, once each, as a certificate names them", got, want)
	}

	for _, tc := range []struct {
		domains []string
		want    string
	}{
		{nil, "no domains"},
		{[]string{"203.0.113.7"}, "203.0.113.7 is an address"},
		{[]string{"example.com", "2001:db8::1"}, "2001:db8::1 is an address"},
		{[]string{"*.example.com"}, "*.example.com is a wildcard"},
		{[]string{"localhost"}, "localhost isn't a domain Let's Encrypt can reach"},
		{[]string{"exa mple.com"}, `"exa mple.com" isn't a domain`},
	} {
		if _, err := New(Config{Domains: tc.domains}); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: %v, want an error saying %q", tc.domains, err, tc.want)
		}
	}
}

func TestFromEnvReadsTheDomainsWhereToKeepThemAndTheCA(t *testing.T) {
	t.Setenv("TLS_DOMAINS", "")
	t.Setenv("CERTS_PATH", "")
	t.Setenv("ACME_URL", "")
	if m, err := FromEnv(); m != nil || err != nil {
		t.Fatalf("with no TLS_DOMAINS: %v, %v, want none, for TLS ended in front of the app", m, err)
	}
	t.Setenv("TLS_DOMAINS", " , ")
	if m, err := FromEnv(); m != nil || err != nil {
		t.Fatalf("with TLS_DOMAINS of commas: %v, %v, want none", m, err)
	}

	t.Setenv("TLS_DOMAINS", "example.com, www.example.com,")
	m, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Hosts(); !slices.Equal(got, []string{"example.com", "www.example.com"}) {
		t.Errorf("Hosts %q", got)
	}
	if m.m.Cache != autocert.DirCache("certs") || m.m.Client.DirectoryURL != "https://acme-v02.api.letsencrypt.org/directory" {
		t.Errorf("kept in %v, from %s, want certs/ and Let's Encrypt", m.m.Cache, m.m.Client.DirectoryURL)
	}

	t.Setenv("CERTS_PATH", "/data/certs")
	t.Setenv("ACME_URL", StagingURL)
	if m, err = FromEnv(); err != nil {
		t.Fatal(err)
	}
	if m.m.Cache != autocert.DirCache("/data/certs") || m.m.Client.DirectoryURL != StagingURL {
		t.Errorf("kept in %v, from %s, want CERTS_PATH's and ACME_URL's", m.m.Cache, m.m.Client.DirectoryURL)
	}

	for _, bad := range []string{"acme-staging-v02.api.letsencrypt.org/directory", "http://ca.internal/directory"} {
		t.Setenv("ACME_URL", bad)
		if _, err := FromEnv(); err == nil || !strings.Contains(err.Error(), "ACME_URL is") {
			t.Errorf("ACME_URL %s: %v, want an error that says so", bad, err)
		}
	}
	t.Setenv("ACME_URL", "")
	t.Setenv("TLS_DOMAINS", "example.com,localhost")
	if _, err := FromEnv(); err == nil {
		t.Error("TLS_DOMAINS with localhost made a Manager")
	}
}

func TestAChallengeIsAnsweredOnPort80ForTheDomainsAlone(t *testing.T) {
	cache := autocert.DirCache(t.TempDir())
	m, err := New(Config{Domains: []string{"example.com"}, Cache: cache})
	if err != nil {
		t.Fatal(err)
	}
	// The answer autocert keeps while Let's Encrypt checks, as it does in
	// the cache too, for an instance beside it that gets the check.
	if err := cache.Put(context.Background(), "the-token+http-01", []byte("the-token.the-key")); err != nil {
		t.Fatal(err)
	}
	h := m.HTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "the fallback")
	}))

	for _, tc := range []struct {
		host, path string
		status     int
		body       string
	}{
		{"example.com", "/.well-known/acme-challenge/the-token", 200, "the-token.the-key"},
		{"example.com", "/.well-known/acme-challenge/another", 404, ""},
		{"other.example", "/.well-known/acme-challenge/the-token", 403, ""},
		{"example.com", "/posts", 200, "the fallback"},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "http://"+tc.host+tc.path, nil)
		h.ServeHTTP(w, r)
		if w.Code != tc.status || (tc.body != "" && w.Body.String() != tc.body) {
			t.Errorf("%s%s: %d %q, want %d %q", tc.host, tc.path, w.Code, w.Body, tc.status, tc.body)
		}
	}
}

func TestTheHandshakeOffersTheChallengesProtocol(t *testing.T) {
	m, err := New(Config{Domains: []string{"example.com"}, Cache: autocert.DirCache(t.TempDir())})
	if err != nil {
		t.Fatal(err)
	}
	if protos := m.TLSConfig().NextProtos; !slices.Contains(protos, "acme-tls/1") || !slices.Contains(protos, "h2") {
		t.Errorf("NextProtos %q, want acme-tls/1, for the TLS-ALPN-01 challenge, beside h2", protos)
	}
}

func TestAHandshakeForAnotherNameAsksTheCAForNothing(t *testing.T) {
	var asked atomic.Bool
	ca := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { asked.Store(true) }))
	defer ca.Close()
	dir := t.TempDir()
	m, err := New(Config{Domains: []string{"example.com"}, Cache: autocert.DirCache(dir), CA: ca.URL})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"other.example", "203.0.113.7", ""} {
		if _, err := m.GetCertificate(&tls.ClientHelloInfo{ServerName: name}); err == nil {
			t.Errorf("a handshake for %q got a certificate", name)
		}
	}
	if entries, _ := os.ReadDir(dir); asked.Load() || len(entries) > 0 {
		t.Errorf("the CA was asked (%v), or an account made (%d files), for a name that isn't the app's", asked.Load(), len(entries))
	}
}

// syncBuffer is a log that a test reads while the Manager writes to it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureLog makes slog's default write to a buffer until the test ends.
func captureLog(t *testing.T) *syncBuffer {
	t.Helper()
	var buf syncBuffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &buf
}

func TestRunLogsADomainItCouldntGetAndReturns(t *testing.T) {
	log := captureLog(t)
	// A 404, which the ACME client doesn't try again, as it does a 5xx.
	ca := httptest.NewTLSServer(http.NotFoundHandler())
	defer ca.Close()
	m, err := New(Config{Domains: []string{"example.com", "www.example.com"}, Cache: autocert.DirCache(t.TempDir()), CA: ca.URL + "/directory"})
	if err != nil {
		t.Fatal(err)
	}
	m.m.Client.HTTPClient = ca.Client()
	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("Run returned %v, want nil: the app serves on, and asks again at a handshake", err)
	}
	for _, d := range []string{"example.com", "www.example.com"} {
		if !strings.Contains(log.String(), `level=ERROR msg="couldn't get a certificate" domain=`+d) {
			t.Errorf("the log doesn't say %s's certificate wasn't got:\n%s", d, log)
		}
	}
}

func TestRunReturnsAsTheAppStopsThoughTheCAHasntAnswered(t *testing.T) {
	captureLog(t)
	release := make(chan struct{})
	ca := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
	defer ca.Close()
	defer close(release)
	m, err := New(Config{Domains: []string{"example.com"}, Cache: autocert.DirCache(t.TempDir()), CA: ca.URL + "/directory"})
	if err != nil {
		t.Fatal(err)
	}
	m.m.Client.HTTPClient = ca.Client()
	ctx, cancel := context.WithCancel(context.Background())
	ran := make(chan error, 1)
	go func() { ran <- m.Run(ctx) }()
	time.Sleep(50 * time.Millisecond) // asking
	cancel()
	select {
	case err := <-ran:
		if err != nil {
			t.Errorf("Run returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run held the app's shutdown up, waiting on the CA")
	}
}

// pebble is a Manager that gets its certificates from the Pebble that
// TUG_TEST_ACME names, as CI runs one, Let's Encrypt's own CA for tests,
// for domain, kept in dir, or skips the test without one. Pebble checks the
// challenges on ports 5001, TLS-ALPN-01's, and 5002, HTTP-01's, as Let's
// Encrypt checks 443 and 80, of 127.0.0.1, where the DNS beside it,
// pebble-challtestsrv's, finds every name:
//
//	pebble-challtestsrv -defaultIPv6 "" -http01 "" -https01 "" -tlsalpn01 "" -doh ""
//	PEBBLE_VA_NOSLEEP=1 PEBBLE_AUTHZREUSE=0 pebble -config test/config/pebble-config.json -dnsserver 127.0.0.1:8053
//	TUG_TEST_ACME=https://127.0.0.1:14000/dir go test ./letsencrypt
func pebble(t *testing.T, domain, dir string) *Manager {
	t.Helper()
	ca := os.Getenv("TUG_TEST_ACME")
	if ca == "" {
		t.Skip("TUG_TEST_ACME names no Pebble to get certificates from")
	}
	m, err := New(Config{Domains: []string{domain}, Cache: autocert.DirCache(dir), CA: ca})
	if err != nil {
		t.Fatal(err)
	}
	// Pebble's API is on a certificate of its own CA's, which no machine
	// trusts.
	m.m.Client.HTTPClient = &http.Client{Transport: asLetsEncrypt{&http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}}
	return m
}

// asLetsEncrypt has Pebble answer an order's finalize as Let's Encrypt
// does, with the order's address in Location, which the ACME client waits
// for the certificate at, and Pebble leaves out.
type asLetsEncrypt struct{ http.RoundTripper }

func (t asLetsEncrypt) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := t.RoundTripper.RoundTrip(r)
	if err == nil && strings.Contains(r.URL.Path, "/finalize-order/") && resp.Header.Get("Location") == "" {
		order := *r.URL
		order.Path = strings.Replace(order.Path, "/finalize-order/", "/my-order/", 1)
		resp.Header.Set("Location", order.String())
	}
	return resp, err
}

// serve serves on addr until the test ends.
func serve(t *testing.T, addr string, srv *http.Server, tls bool) {
	t.Helper()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("Pebble checks the challenges on %s: %v", addr, err)
	}
	go func() {
		if tls {
			srv.ServeTLS(ln, "", "")
		} else {
			srv.Serve(ln)
		}
	}()
	t.Cleanup(func() { srv.Close() })
}

// unique is a name of its own for each run's certificate, as a Pebble that
// runs on would otherwise have checked it in a run before.
func unique(label string) string {
	return label + "-" + strconv.FormatInt(time.Now().UnixNano(), 36) + ".tug.test"
}

// keptLeaf is the certificate a file of autocert's keeps, after its key.
func keptLeaf(t *testing.T, kept []byte) []byte {
	t.Helper()
	for rest := kept; ; {
		var block *pem.Block
		if block, rest = pem.Decode(rest); block == nil {
			t.Fatal("the file keeps no certificate")
		}
		if block.Type == "CERTIFICATE" {
			return block.Bytes
		}
	}
}

func TestACertificateIsGotAsTheAppStartsAndServedWithoutAnotherOrder(t *testing.T) {
	log := captureLog(t)
	domain, dir := unique("starts"), t.TempDir()
	m := pebble(t, domain, dir)
	serve(t, "127.0.0.1:5001", &http.Server{TLSConfig: m.TLSConfig()}, true)
	serve(t, "127.0.0.1:5002", &http.Server{Handler: m.HTTPHandler(nil)}, false)

	if err := m.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(log.String(), `msg="a certificate is ready" domain=`+domain) {
		t.Fatalf("the log doesn't say the certificate was got:\n%s", log)
	}
	kept, err := os.ReadFile(filepath.Join(dir, domain))
	if err != nil {
		t.Fatalf("the certificate isn't kept in its directory: %v", err)
	}

	// A browser's handshake gets that one, ECDSA's, as autocert has it
	// from Run.
	conn, err := tls.Dial("tcp", "127.0.0.1:5001", &tls.Config{ServerName: domain, InsecureSkipVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	leaf := conn.ConnectionState().PeerCertificates[0]
	if leaf.PublicKeyAlgorithm.String() != "ECDSA" || !slices.Equal(leaf.DNSNames, []string{domain}) {
		t.Errorf("served a %s certificate for %q, want ECDSA's for %s", leaf.PublicKeyAlgorithm, leaf.DNSNames, domain)
	}
	if !bytes.Equal(keptLeaf(t, kept), leaf.Raw) {
		t.Error("the handshake got a certificate other than the one kept as the app started")
	}
}

func TestACertificateIsGotByTLSALPN01WithNothingOnPort80(t *testing.T) {
	captureLog(t)
	domain := unique("alpn")
	m := pebble(t, domain, t.TempDir())
	// No HTTPHandler: autocert tries TLS-ALPN-01 alone, and nothing answers
	// on 5002.
	serve(t, "127.0.0.1:5001", &http.Server{TLSConfig: m.TLSConfig()}, true)

	if err := m.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	cert, err := m.GetCertificate(&tls.ClientHelloInfo{ServerName: domain, CipherSuites: []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256}})
	if err != nil || cert.Leaf == nil || !slices.Equal(cert.Leaf.DNSNames, []string{domain}) {
		t.Fatalf("got %v, %v, want the certificate for %s", cert, err, domain)
	}
}
