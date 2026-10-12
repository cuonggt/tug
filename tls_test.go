package tug

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cuonggt/tug/session"
)

// writeCert writes a certificate for names, its own CA, and its key, to
// files in dir, as cert.pem and key.pem, and returns their paths and the
// certificate.
func writeCert(t *testing.T, dir string, names ...string) (certFile, keyFile string, cert *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: names[0]},
		DNSNames:              names,
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certFile, keyFile = filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	cert, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile, cert
}

// served is an app serving HTTPS and HTTP, as Run does with Config.TLS,
// on ports of their own.
type served struct {
	https, http string // their addresses
	stop        context.CancelFunc
	done        <-chan error
}

// serveTLS serves app as Run does with Config.TLS, on free ports, with the
// source of certificates Run makes, after change has its way with it.
func serveTLS(t *testing.T, app *App, change func(Certificates)) served {
	t.Helper()
	certs, err := app.certificates()
	if err != nil {
		t.Fatal(err)
	}
	if change != nil {
		change(certs)
	}
	secure, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	plain, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.serveTLS(ctx, certs, secure, plain) }()
	t.Cleanup(func() {
		stop()
		if err := <-done; err != nil {
			t.Errorf("serving returned %v", err)
		}
	})
	return served{https: secure.Addr().String(), http: plain.Addr().String(), stop: stop, done: done}
}

// trusting is a client that trusts cert, and asks for name in the
// handshake, as a browser does for a page of name's.
func trusting(cert *x509.Certificate, name string) *http.Client {
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	return &http.Client{Transport: &http.Transport{
		TLSClientConfig:   &tls.Config{RootCAs: roots, ServerName: name},
		ForceAttemptHTTP2: true,
	}}
}

// noRedirects is a client that shows a redirect rather than follow it.
var noRedirects = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func TestHTTPSAnswersOverHTTP2WithASecureSessionCookie(t *testing.T) {
	captureLog(t)
	certFile, keyFile, cert := writeCert(t, t.TempDir(), "example.com")
	sessions, err := session.New(session.Config{Keys: [][]byte{bytes.Repeat([]byte{7}, 32)}})
	if err != nil {
		t.Fatal(err)
	}
	app := New(Config{Session: sessions, TLS: TLS{CertFile: certFile, KeyFile: keyFile}})
	app.Get("/", func(c *Ctx) error {
		c.Session().Set("seen", true)
		return c.String(200, "over TLS: "+c.Request().TLS.ServerName)
	})
	s := serveTLS(t, app, nil)

	client := trusting(cert, "example.com")
	defer client.CloseIdleConnections() // or the server gives it a second to go, after its GOAWAY
	resp, err := client.Get("https://" + s.https + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "over TLS: example.com" {
		t.Errorf("body %q", body)
	}
	if resp.Proto != "HTTP/2.0" {
		t.Errorf("the request went over %s, want HTTP/2", resp.Proto)
	}
	if cookies := resp.Cookies(); len(cookies) != 1 || !cookies[0].Secure {
		t.Errorf("cookies %v, want the session's, for HTTPS only, though its Config doesn't say", resp.Header["Set-Cookie"])
	}
}

func TestEventsGoOutAsTheyreSentOverHTTP2(t *testing.T) {
	captureLog(t)
	certFile, keyFile, cert := writeCert(t, t.TempDir(), "example.com")
	app := New(Config{TLS: TLS{CertFile: certFile, KeyFile: keyFile}})
	app.Get("/events", func(c *Ctx) error {
		return c.Events(func(ctx context.Context, send func(Event) error) error {
			if err := send(Event{Name: "hello", Data: "world"}); err != nil {
				return err
			}
			<-ctx.Done() // the stream stays open, as a page's does
			return nil
		})
	})
	s := serveTLS(t, app, nil)

	client := trusting(cert, "example.com")
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://"+s.https+"/events", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.Proto != "HTTP/2.0" {
		t.Fatalf("over %s, want HTTP/2", resp.Proto)
	}
	buf := make([]byte, 64)
	n, err := io.ReadAtLeast(resp.Body, buf, len("event: hello\ndata: \"world\"\n\n"))
	if err != nil || !strings.HasPrefix(string(buf[:n]), "event: hello\n") {
		t.Fatalf("read %q, %v, want the event while the stream is open", buf[:n], err)
	}
}

func TestPlainHTTPSendsEachRequestToHTTPSWithItsPathQueryAndMethod(t *testing.T) {
	captureLog(t)
	certFile, keyFile, _ := writeCert(t, t.TempDir(), "example.com", "www.example.com")
	app := New(Config{URL: "https://example.com", TLS: TLS{CertFile: certFile, KeyFile: keyFile}})
	s := serveTLS(t, app, nil)
	_, port, _ := net.SplitHostPort(s.https)

	for _, tc := range []struct {
		method, host, want string
	}{
		{"GET", "www.example.com", "https://www.example.com:" + port + "/posts?page=2"},
		{"POST", "example.com:80", "https://example.com:" + port + "/posts?page=2"},
		{"GET", "WWW.Example.com.", "https://www.example.com:" + port + "/posts?page=2"},
		// By the machine's address, or a name the certificate isn't for:
		// to the app's own address.
		{"GET", s.http, "https://example.com/posts?page=2"},
		{"PUT", "other.example", "https://example.com/posts?page=2"},
	} {
		req, _ := http.NewRequest(tc.method, "http://"+s.http+"/posts?page=2", strings.NewReader("title=Hello"))
		req.Host = tc.host
		resp, err := noRedirects.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusPermanentRedirect || resp.Header.Get("Location") != tc.want {
			t.Errorf("%s for %s: %d to %q, want a 308 to %q", tc.method, tc.host, resp.StatusCode, resp.Header.Get("Location"), tc.want)
		}
	}
}

func TestWithoutTheAppsAddressAnotherHostGoesToTheCertificatesName(t *testing.T) {
	captureLog(t)
	certFile, keyFile, _ := writeCert(t, t.TempDir(), "*.example.com", "example.com")
	app := New(Config{TLS: TLS{CertFile: certFile, KeyFile: keyFile}})
	s := serveTLS(t, app, nil)
	_, port, _ := net.SplitHostPort(s.https)

	for host, want := range map[string]string{
		"blog.example.com": "https://blog.example.com:" + port + "/",
		s.http:             "https://example.com/",
	} {
		req, _ := http.NewRequest("GET", "http://"+s.http+"/", nil)
		req.Host = host
		resp, err := noRedirects.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if got := resp.Header.Get("Location"); got != want {
			t.Errorf("for %s: to %q, want %q", host, got, want)
		}
	}
}

// clock is a time a test moves on.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// syncBuffer is a log that a test reads while servers write to it.
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

// syncLog makes slog's default write to a syncBuffer, at Debug too, until
// the test ends.
func syncLog(t *testing.T) *syncBuffer {
	t.Helper()
	var buf syncBuffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &buf
}

// servedSerial is the serial number of the certificate served at addr.
func servedSerial(t *testing.T, addr string) *big.Int {
	t.Helper()
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: "example.com", InsecureSkipVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	return conn.ConnectionState().PeerCertificates[0].SerialNumber
}

func TestTheCertificatesFilesAreReadAgainAsTheyChange(t *testing.T) {
	log := syncLog(t)
	dir := t.TempDir()
	certFile, keyFile, first := writeCert(t, dir, "example.com")
	app := New(Config{TLS: TLS{CertFile: certFile, KeyFile: keyFile}})
	now := &clock{}
	s := serveTLS(t, app, func(c Certificates) {
		files := c.(*certFiles)
		now.now, files.now = files.checked, now.Now
	})

	if got := servedSerial(t, s.https); got.Cmp(first.SerialNumber) != 0 {
		t.Fatalf("served %v, want the files' %v", got, first.SerialNumber)
	}

	// A renewal replaces them: they're looked at again a minute after
	// they last were, and not before.
	_, _, renewed := writeCert(t, dir, "example.com")
	now.add(59 * time.Second)
	if got := servedSerial(t, s.https); got.Cmp(first.SerialNumber) != 0 {
		t.Fatalf("served %v within the minute, want the one read before", got)
	}
	now.add(time.Second)
	if got := servedSerial(t, s.https); got.Cmp(renewed.SerialNumber) != 0 {
		t.Fatalf("served %v, want the renewed %v", got, renewed.SerialNumber)
	}

	// A pair that doesn't load, as a certificate written before its key,
	// leaves the last in use, and the log says why.
	if err := os.WriteFile(certFile, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	now.add(time.Minute)
	if got := servedSerial(t, s.https); got.Cmp(renewed.SerialNumber) != 0 {
		t.Fatalf("served %v, want the renewed one still", got)
	}
	if !strings.Contains(log.String(), "level=WARN msg=\"the certificate's files changed, and don't load") {
		t.Errorf("the log doesn't say the files don't load:\n%s", log)
	}
}

func TestANilSourceOfCertificatesIsNone(t *testing.T) {
	var none *source // as letsencrypt.FromEnv's without TLS_DOMAINS
	app := New(Config{TLS: TLS{Certificates: none}})
	if app.config.TLS.on() || app.config.Addr != ":8080" {
		t.Errorf("a nil source has the app serve HTTPS on %s", app.config.Addr)
	}
	certFile, keyFile, _ := writeCert(t, t.TempDir(), "example.com")
	if _, err := New(Config{TLS: TLS{CertFile: certFile, KeyFile: keyFile, Certificates: none}}).certificates(); err != nil {
		t.Errorf("files beside a nil source: %v", err)
	}
}

func TestTLSIsReadFromTheEnvironment(t *testing.T) {
	t.Setenv("ADDR", "")
	t.Setenv("PORT", "")
	t.Setenv("TLS_CERT", "")
	t.Setenv("TLS_KEY", "")
	t.Setenv("HTTP_ADDR", "")
	if app := New(); app.config.Addr != ":8080" || app.config.TLS.on() {
		t.Fatalf("with no TLS variables: %+v, want plain HTTP on :8080", app.config)
	}

	t.Setenv("TLS_CERT", "/etc/certs/example.com.pem")
	t.Setenv("TLS_KEY", "/etc/certs/example.com.key")
	app := New()
	if tls := app.config.TLS; tls.CertFile != "/etc/certs/example.com.pem" || tls.KeyFile != "/etc/certs/example.com.key" || tls.HTTPAddr != ":80" {
		t.Errorf("TLS %+v", tls)
	}
	if app.config.Addr != ":443" {
		t.Errorf("Addr %q, want HTTPS's own port", app.config.Addr)
	}

	t.Setenv("ADDR", ":8443")
	t.Setenv("HTTP_ADDR", ":8000")
	if app := New(); app.config.Addr != ":8443" || app.config.TLS.HTTPAddr != ":8000" {
		t.Errorf("Addr %q and HTTPAddr %q, want ADDR's and HTTP_ADDR's", app.config.Addr, app.config.TLS.HTTPAddr)
	}
}

// source is a source of certificates with all that Run looks for.
type source struct {
	cert  *tls.Certificate
	hosts []string
	ran   chan struct{}
}

func (s *source) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if hello.ServerName == "down.example.com" {
		return nil, io.ErrUnexpectedEOF
	}
	return s.cert, nil
}

func (s *source) Hosts() []string { return s.hosts }

func (s *source) TLSConfig() *tls.Config {
	return &tls.Config{NextProtos: []string{"h2", "http/1.1", "acme-tls/1"}}
}

func (s *source) HTTPHandler(fallback http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/.well-known/acme-challenge/") {
			io.WriteString(w, "the challenge's answer")
			return
		}
		fallback.ServeHTTP(w, r)
	})
}

func (s *source) Run(ctx context.Context) error {
	close(s.ran)
	return nil
}

func newSource(t *testing.T, hosts ...string) *source {
	t.Helper()
	certFile, keyFile, _ := writeCert(t, t.TempDir(), hosts...)
	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		t.Fatal(err)
	}
	return &source{cert: &pair, hosts: hosts, ran: make(chan struct{})}
}

func TestASourceOfCertificatesIsAskedForAllItHas(t *testing.T) {
	log := syncLog(t)
	certs := newSource(t, "example.com", "down.example.com")
	app := New(Config{URL: "https://example.com", TLS: TLS{Certificates: certs}})
	s := serveTLS(t, app, nil)

	select {
	case <-certs.ran:
	case <-time.After(5 * time.Second):
		t.Fatal("the source's Run didn't run once both ports listened")
	}

	// Its challenges on the plain port, and the redirect for the rest.
	resp, err := http.Get("http://" + s.http + "/.well-known/acme-challenge/token")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "the challenge's answer" {
		t.Errorf("the challenge got %d %q", resp.StatusCode, body)
	}
	resp, err = noRedirects.Get("http://" + s.http + "/posts")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got := resp.Header.Get("Location"); got != "https://example.com/posts" {
		t.Errorf("to %q, want the app's address", got)
	}

	// Its protocol in the handshake, as a CA's check asks for it.
	conn, err := tls.Dial("tcp", s.https, &tls.Config{ServerName: "example.com", InsecureSkipVerify: true, NextProtos: []string{"acme-tls/1"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := conn.ConnectionState().NegotiatedProtocol; got != "acme-tls/1" {
		t.Errorf("agreed on %q, want acme-tls/1", got)
	}
	conn.Close()

	// A handshake it can't answer for one of its names is logged, as a
	// certificate it couldn't get; any other failed handshake, at Debug.
	if _, err := tls.Dial("tcp", s.https, &tls.Config{ServerName: "down.example.com", InsecureSkipVerify: true}); err == nil {
		t.Fatal("a handshake with no certificate went through")
	}
	if _, err := tls.Dial("tcp", s.https, &tls.Config{ServerName: "example.com"}); err == nil {
		t.Fatal("a client that doesn't trust the certificate went on")
	}
	for deadline := time.Now().Add(5 * time.Second); strings.Count(log.String(), "TLS handshake error") < 2; {
		if time.Now().After(deadline) {
			t.Fatalf("the failed handshakes weren't logged:\n%s", log)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(log.String(), `level=WARN msg="no certificate for a handshake" domain=down.example.com`) {
		t.Errorf("the certificate the source couldn't give isn't a warning:\n%s", log)
	}
	if strings.Contains(log.String(), "level=WARN msg=\"http: TLS handshake error") {
		t.Errorf("a failed handshake is a warning:\n%s", log)
	}
}

func TestTLSThatDoesntAddUpStopsTheApp(t *testing.T) {
	certFile, keyFile, _ := writeCert(t, t.TempDir(), "example.com")
	for _, tc := range []struct {
		config Config
		want   string
	}{
		{Config{TLS: TLS{CertFile: certFile}}, "TLS_CERT and TLS_KEY go together"},
		{Config{TLS: TLS{KeyFile: keyFile}}, "TLS_CERT and TLS_KEY go together"},
		{Config{TLS: TLS{CertFile: certFile, KeyFile: keyFile, Certificates: newSource(t, "example.com")}}, "set one or the other"},
		{Config{TLS: TLS{CertFile: keyFile, KeyFile: certFile}}, "doesn't load"},
		{Config{TLS: TLS{CertFile: filepath.Join(t.TempDir(), "gone.pem"), KeyFile: keyFile}}, "doesn't load"},
		{Config{URL: "https://example.org", TLS: TLS{Certificates: newSource(t, "example.com", "www.example.com")}},
			"APP_URL is https://example.org, whose host isn't one of the domains the app has certificates for, example.com, www.example.com"},
	} {
		if _, err := New(tc.config).certificates(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%+v: %v, want an error saying %q", tc.config.TLS, err, tc.want)
		}
	}
	if _, err := New(Config{URL: "https://www.example.com", TLS: TLS{Certificates: newSource(t, "example.com", "www.example.com")}}).certificates(); err != nil {
		t.Errorf("an APP_URL of one of the domains: %v", err)
	}
}

func TestPackageTugBuildsNoAutocert(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, dep := range strings.Fields(string(out)) {
		if strings.HasPrefix(dep, "golang.org/x/crypto/acme") || strings.HasPrefix(dep, "golang.org/x/net/") {
			t.Errorf("package tug imports %s, which only an app serving HTTPS with letsencrypt needs", dep)
		}
	}
}
