package tug

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
)

// TLS has Run serve HTTPS itself, as on a machine of the app's own with no
// proxy in front to end TLS: HTTPS, and HTTP/2, on Config.Addr, ":443"
// unless it says another, and plain HTTP on HTTPAddr, which sends every
// request to HTTPS. The certificates are the files', CertFile and KeyFile,
// or a source's, Certificates, as package letsencrypt's, which gets them
// from Let's Encrypt. The zero value is off: Run serves plain HTTP on
// Config.Addr, for TLS ended in front of the app.
type TLS struct {
	// CertFile and KeyFile are a certificate, with its chain after it, and
	// its key, in PEM files, as Cloudflare's origin certificates or a
	// company's own CA's are. Run reads them as it starts, and again at a
	// handshake, once a minute at most, when either's time has changed, as
	// certbot's renewal replaces them; a pair that doesn't load leaves the
	// last in use, and the log says why. Both or neither, and not with
	// Certificates. ConfigFromEnv reads them from TLS_CERT and TLS_KEY.
	CertFile string
	KeyFile  string

	// Certificates is a source of certificates, in place of the files, as
	// package letsencrypt's Manager. A nil one is none, as the nil Manager
	// letsencrypt.FromEnv returns without TLS_DOMAINS, so what it returns
	// can go straight in.
	Certificates Certificates

	// HTTPAddr is where Run serves plain HTTP beside HTTPS: each request is
	// sent to its path over HTTPS, by a 308, but for what the source of
	// certificates answers there, as Let's Encrypt's challenges. Default
	// ":80". ConfigFromEnv reads it from HTTP_ADDR.
	HTTPAddr string
}

// on reports whether Run serves HTTPS itself.
func (t TLS) on() bool {
	return t.CertFile != "" || t.KeyFile != "" || t.source()
}

// source reports whether there's a source of certificates: an interface
// holding a nil pointer isn't nil, and would be one with no certificates.
func (t TLS) source() bool {
	if t.Certificates == nil {
		return false
	}
	v := reflect.ValueOf(t.Certificates)
	switch v.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Func, reflect.Interface, reflect.Slice, reflect.Chan:
		return !v.IsNil()
	}
	return true
}

// Certificates is a source of certificates for Run's HTTPS, as package
// letsencrypt's Manager, which gets them from Let's Encrypt: GetCertificate
// returns the one for a handshake, as tls.Config's does. It may have more,
// which Run looks for:
//
//   - Hosts() []string, the names it has certificates for: Run's plain HTTP
//     sends a request for one of them to HTTPS at its own host, and any
//     other, as a scanner's by the machine's address, to Config.URL's,
//     whose host has to be one of them, or Run doesn't start. Without it,
//     a request goes to HTTPS at its own host, whatever it is.
//   - HTTPHandler(fallback http.Handler) http.Handler, which Run's plain
//     HTTP answers through, with the redirect as fallback, for what the
//     source answers there, as Let's Encrypt's HTTP-01 challenges.
//   - TLSConfig() *tls.Config, whose NextProtos the handshake offers beside
//     h2 and http/1.1, as acme-tls/1, for Let's Encrypt's TLS-ALPN-01
//     challenges.
//   - Run(ctx context.Context) error, which Run runs beside the server once
//     both ports listen, as Go's work, as letsencrypt's asks for each
//     domain's certificate as the app starts.
//
// HTTPHandler and TLSConfig are autocert.Manager's own.
type Certificates interface {
	GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error)
}

// certificates is the source of Config.TLS, the files' or the one it was
// given, checked as Run starts.
func (a *App) certificates() (Certificates, error) {
	t := a.config.TLS
	if (t.CertFile == "") != (t.KeyFile == "") {
		return nil, errors.New("tug: TLS_CERT and TLS_KEY go together, a certificate and its key: set both, or neither")
	}
	if t.CertFile != "" && t.source() {
		return nil, errors.New("tug: TLS_CERT and TLS_KEY are a certificate of the app's own, in place of Let's Encrypt's for TLS_DOMAINS, or another source of certificates: set one or the other")
	}
	if t.source() {
		hosts, ok := t.Certificates.(interface{ Hosts() []string })
		if !ok || a.config.URL == "" {
			return t.Certificates, nil
		}
		u, _ := url.Parse(a.config.URL) // New checked it
		if !slices.Contains(hosts.Hosts(), strings.ToLower(u.Hostname())) {
			return nil, fmt.Errorf("tug: APP_URL is %s, whose host isn't one of the domains the app has certificates for, %s: the links that leave the app would lead to a certificate for another name",
				a.config.URL, strings.Join(hosts.Hosts(), ", "))
		}
		return t.Certificates, nil
	}
	files := &certFiles{cert: t.CertFile, key: t.KeyFile, now: time.Now}
	if err := files.load(); err != nil {
		return nil, fmt.Errorf("tug: the certificate in TLS_CERT and TLS_KEY doesn't load: %w", err)
	}
	return files, nil
}

// runTLS is Run with Config.TLS: HTTPS on Config.Addr, and HTTP on
// TLS.HTTPAddr.
func (a *App) runTLS(ctx context.Context) error {
	certs, err := a.certificates()
	if err != nil {
		return err
	}
	secure, err := net.Listen("tcp", a.config.Addr)
	if err != nil {
		return err
	}
	plain, err := net.Listen("tcp", a.config.TLS.HTTPAddr)
	if err != nil {
		secure.Close()
		return err
	}
	return a.serveTLS(ctx, certs, secure, plain)
}

// serveTLS serves HTTPS on secure, with certs, and HTTP on plain, which
// sends each request to HTTPS, until ctx is done, as Serve does. It closes
// both.
func (a *App) serveTLS(ctx context.Context, certs Certificates, secure, plain net.Listener) error {
	// h2 by ALPN, as http.Server's ServeTLS would offer it, which Serve
	// doesn't add to a listener's own, and whatever the source answers in
	// the handshake itself, as Let's Encrypt's challenge.
	protos := []string{"h2", "http/1.1"}
	if c, ok := certs.(interface{ TLSConfig() *tls.Config }); ok {
		for _, p := range c.TLSConfig().NextProtos {
			if !slices.Contains(protos, p) {
				protos = append(protos, p)
			}
		}
	}
	config := &tls.Config{GetCertificate: getCertificate(certs), NextProtos: protos}

	to := toHTTPS{ours: func(string) bool { return true }}
	if _, port, err := net.SplitHostPort(secure.Addr().String()); err == nil && port != "443" {
		to.port = port
	}
	switch c := certs.(type) {
	case *certFiles:
		to.ours, to.home = c.names, c.firstName()
	case interface{ Hosts() []string }:
		hosts := c.Hosts()
		to.ours = func(host string) bool { return slices.Contains(hosts, host) }
		if len(hosts) > 0 {
			to.home = hosts[0]
		}
	}
	if a.config.URL != "" {
		u, _ := url.Parse(a.config.URL) // New checked it
		to.home = u.Host
	}
	var redirect http.Handler = to
	if c, ok := certs.(interface {
		HTTPHandler(fallback http.Handler) http.Handler
	}); ok {
		redirect = c.HTTPHandler(redirect)
	}

	var work []func(context.Context) error
	if c, ok := certs.(interface{ Run(context.Context) error }); ok {
		work = append(work, c.Run)
	}
	return a.serveAll(ctx, work,
		listening{server: a.server(a), ln: tls.NewListener(secure, config), scheme: "https"},
		listening{server: a.server(redirect), ln: plain, scheme: "http"},
	)
}

// getCertificate is certs' GetCertificate, which logs a handshake it
// can't answer for one of the source's own names, as a certificate that
// couldn't be renewed: the server's own log has it at Debug, as most
// handshakes that fail are a client's doing.
func getCertificate(certs Certificates) func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	hosts, ok := certs.(interface{ Hosts() []string })
	if !ok {
		return certs.GetCertificate
	}
	names := hosts.Hosts()
	return func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		cert, err := certs.GetCertificate(hello)
		if err != nil && slices.Contains(names, strings.ToLower(strings.TrimSuffix(hello.ServerName, "."))) {
			slog.Warn("no certificate for a handshake", "domain", hello.ServerName, "err", err)
		}
		return cert, err
	}
}

// toHTTPS is Run's plain HTTP beside HTTPS: each request is sent to its
// path and query over HTTPS by a 308, which keeps its method, as Caddy
// does, at its own host when that's one of the certificates', and
// otherwise, as a request by the machine's address, at home's.
type toHTTPS struct {
	ours func(host string) bool
	home string // the app's host, Config.URL's, or else the certificates' first
	port string // HTTPS's, when it isn't 443
}

func (h toHTTPS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	host = strings.ToLower(strings.TrimSuffix(strings.Trim(host, "[]"), "."))
	switch {
	case host != "" && h.ours(host):
		if h.port != "" {
			host = net.JoinHostPort(host, h.port)
		} else if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
	case h.home != "":
		host = h.home
	default:
		http.Error(w, "this site isn't served here", http.StatusMisdirectedRequest)
		return
	}
	http.Redirect(w, r, "https://"+host+r.URL.RequestURI(), http.StatusPermanentRedirect)
}

// certFiles is a certificate of the app's own, from its files, read again
// as they change, as a renewal replaces them.
type certFiles struct {
	cert, key string
	now       func() time.Time

	mu                sync.Mutex
	current           *tls.Certificate
	certTime, keyTime time.Time // the files' when they were read
	checked           time.Time
}

// load reads the files as they are.
func (f *certFiles) load() error {
	certTime, keyTime, err := f.times()
	if err != nil {
		return err
	}
	pair, err := tls.LoadX509KeyPair(f.cert, f.key)
	if err != nil {
		return err
	}
	f.current, f.certTime, f.keyTime, f.checked = &pair, certTime, keyTime, f.now()
	return nil
}

// times are the files' modification times.
func (f *certFiles) times() (cert, key time.Time, err error) {
	ci, err := os.Stat(f.cert)
	if err != nil {
		return cert, key, err
	}
	ki, err := os.Stat(f.key)
	if err != nil {
		return cert, key, err
	}
	return ci.ModTime(), ki.ModTime(), nil
}

func (f *certFiles) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if now := f.now(); now.Sub(f.checked) >= time.Minute {
		f.checked = now
		f.reload()
	}
	return f.current, nil
}

// reload reads the files again when either has changed since they were
// read, and keeps what was read before when they don't load, as when
// they're read between the new certificate and its key.
func (f *certFiles) reload() {
	certTime, keyTime, err := f.times()
	if err != nil {
		slog.Warn("the certificate's files aren't there: the one read before is still in use", "err", err)
		return
	}
	if certTime.Equal(f.certTime) && keyTime.Equal(f.keyTime) {
		return
	}
	// The pair that didn't load isn't read again until a file changes.
	f.certTime, f.keyTime = certTime, keyTime
	pair, err := tls.LoadX509KeyPair(f.cert, f.key)
	if err != nil {
		slog.Warn("the certificate's files changed, and don't load: the one read before is still in use", "cert", f.cert, "key", f.key, "err", err)
		return
	}
	f.current = &pair
	slog.Info("the certificate's files changed, and the new one is in use", "cert", f.cert, "expires", pair.Leaf.NotAfter)
}

// names reports whether the certificate is for host.
func (f *certFiles) names(host string) bool {
	f.mu.Lock()
	leaf := f.current.Leaf
	f.mu.Unlock()
	return leaf.VerifyHostname(host) == nil
}

// firstName is the first of the certificate's names that isn't a wildcard,
// or "".
func (f *certFiles) firstName() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, name := range f.current.Leaf.DNSNames {
		if !strings.HasPrefix(name, "*.") {
			return name
		}
	}
	return ""
}

// serverLog is an http.Server's own log, into the handler that's slog's
// default as it starts, at Warn, but for a handshake that failed, at
// Debug: most are a client's doing, a scanner's by the machine's address,
// or one that gave up, and a certificate the app couldn't get is logged as
// getCertificate fails.
type serverLog struct{ h slog.Handler }

func (l serverLog) Write(p []byte) (int, error) {
	msg := strings.TrimSuffix(string(p), "\n")
	level := slog.LevelWarn
	if strings.HasPrefix(msg, "http: TLS handshake error") {
		level = slog.LevelDebug
	}
	ctx := context.Background()
	if !l.h.Enabled(ctx, level) {
		return len(p), nil
	}
	var pcs [1]uintptr
	runtime.Callers(4, pcs[:]) // past Callers, Write, and log.Logger's Output and Printf, to net/http's
	return len(p), l.h.Handle(ctx, slog.NewRecord(time.Now(), level, msg, pcs[0]))
}
