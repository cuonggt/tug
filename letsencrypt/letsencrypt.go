// Package letsencrypt gets an app's certificates from Let's Encrypt, for
// tug's Run to serve HTTPS itself, as on a machine of the app's own with no
// proxy in front to end TLS:
//
//	cfg := tug.ConfigFromEnv()
//	if cfg.TLS.Certificates, err = letsencrypt.FromEnv(); err != nil {
//		log.Fatal(err)
//	}
//
// With TLS_DOMAINS set, as example.com,www.example.com, Run serves HTTPS on
// :443, and HTTP/2, with a certificate for each domain, asked for as the
// app starts, kept in CERTS_PATH, and renewed before it runs out; and HTTP
// on :80, which answers Let's Encrypt's challenges and sends every other
// request to HTTPS. Setting TLS_DOMAINS is accepting Let's Encrypt's terms,
// https://letsencrypt.org/repository/.
//
// It's autocert's, golang.org/x/crypto/acme/autocert, which the Go team
// keeps. It's a package of its own, which package tug doesn't import, so an
// app that ends TLS in front of it doesn't build it.
package letsencrypt

import (
	"cmp"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/net/idna"
)

// StagingURL is Let's Encrypt's staging, for Config.CA or ACME_URL, to try
// a deploy against without spending Let's Encrypt's limits: its
// certificates are the same, but from a CA no browser trusts.
const StagingURL = "https://acme-staging-v02.api.letsencrypt.org/directory"

// Config is which certificates to get, where from, and where to keep them.
type Config struct {
	// Domains are the names to get certificates for, a certificate each, as
	// example.com and www.example.com. A handshake for any other name, as a
	// scanner's by the machine's address, gets none, rather than spend Let's
	// Encrypt's limits on it.
	Domains []string

	// Cache keeps the account's key and the certificates, so a new start
	// doesn't ask for them again: Let's Encrypt gives the same names five
	// certificates a week. Default the directory certs, which autocert
	// writes for the app's user alone. Any autocert.Cache does, as a table
	// of the app's, for instances on more than one machine.
	Cache autocert.Cache

	// CA is the address of the ACME directory the certificates come from:
	// Let's Encrypt's, unless it's another, as StagingURL. A CA that takes
	// an account of its own first, as ZeroSSL's, isn't one.
	CA string
}

// Manager gets the certificates its Config says, keeps them, and renews
// them before they run out, for tug's Config.TLS.Certificates: Run serves
// HTTPS with what GetCertificate returns, answers the challenges on its
// plain HTTP through HTTPHandler, offers TLSConfig's protocol for the
// challenge in the handshake, and runs Run once both ports listen.
type Manager struct {
	domains []string
	m       *autocert.Manager
}

// New returns a Manager for config. It's an error for config to have no
// domain, or a name that isn't one Let's Encrypt can check: an address, a
// wildcard, or a name with no dot in it, as localhost.
func New(config Config) (*Manager, error) {
	var domains []string
	for _, d := range config.Domains {
		name, err := domain(d)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(domains, name) {
			domains = append(domains, name)
		}
	}
	if len(domains) == 0 {
		return nil, errors.New("letsencrypt: no domains to get certificates for, as example.com")
	}
	cache := config.Cache
	if cache == nil {
		cache = autocert.DirCache("certs")
	}
	return &Manager{domains: domains, m: &autocert.Manager{
		// Accepting the CA's terms is the operator's choice, made by
		// setting TLS_DOMAINS. No email: Let's Encrypt sends none of a
		// certificate running out, and an account needs none.
		Prompt:     autocert.AcceptTOS,
		Cache:      cache,
		HostPolicy: autocert.HostWhitelist(domains...),
		Client:     &acme.Client{DirectoryURL: cmp.Or(config.CA, acme.LetsEncryptURL), UserAgent: "tug"},
	}}, nil
}

// domain is d as a certificate names it: in lower case, punycode for a
// name that isn't ASCII, and with no dot at the end.
func domain(d string) (string, error) {
	d = strings.TrimSpace(d)
	switch {
	case net.ParseIP(d) != nil:
		return "", fmt.Errorf("letsencrypt: %s is an address, and a certificate from Let's Encrypt is for a domain, as example.com", d)
	case strings.Contains(d, "*"):
		return "", fmt.Errorf("letsencrypt: %s is a wildcard, which Let's Encrypt checks by DNS alone: name each domain, as www.example.com", d)
	}
	name, err := idna.Lookup.ToASCII(strings.TrimSuffix(d, "."))
	if err != nil {
		return "", fmt.Errorf("letsencrypt: %q isn't a domain, as example.com", d)
	}
	if !strings.Contains(name, ".") {
		return "", fmt.Errorf("letsencrypt: %s isn't a domain Let's Encrypt can reach, as example.com", name)
	}
	return name, nil
}

// FromEnv returns the Manager the environment says: TLS_DOMAINS, the
// domains, comma separated, as example.com,www.example.com; CERTS_PATH, the
// directory to keep the certificates in, certs unless it says another; and
// ACME_URL, a CA other than Let's Encrypt, as its staging, StagingURL.
// Without TLS_DOMAINS, it returns nil, which tug's Config.TLS takes as no
// source, and the app serves plain HTTP, for TLS ended in front of it.
func FromEnv() (*Manager, error) {
	var domains []string
	for d := range strings.SplitSeq(os.Getenv("TLS_DOMAINS"), ",") {
		if d = strings.TrimSpace(d); d != "" {
			domains = append(domains, d)
		}
	}
	if len(domains) == 0 {
		return nil, nil
	}
	ca := os.Getenv("ACME_URL")
	if ca != "" {
		if u, err := url.Parse(ca); err != nil || u.Scheme != "https" || u.Host == "" {
			return nil, fmt.Errorf("letsencrypt: ACME_URL is %q, and a CA's directory is an https:// address, as %s", ca, StagingURL)
		}
	}
	return New(Config{
		Domains: domains,
		Cache:   autocert.DirCache(cmp.Or(os.Getenv("CERTS_PATH"), "certs")),
		CA:      ca,
	})
}

// Hosts returns the domains, in lower case, which tug's Run sends its
// plain HTTP's requests for to HTTPS at their own host.
func (m *Manager) Hosts() []string {
	return slices.Clone(m.domains)
}

// GetCertificate returns the certificate for a handshake, as tls.Config's
// does: one kept for its domain, or else one asked for from Let's Encrypt
// then and there, which takes seconds; or the one that answers Let's
// Encrypt's TLS-ALPN-01 challenge; or, for a name that isn't one of the
// domains, none.
func (m *Manager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	return m.m.GetCertificate(hello)
}

// TLSConfig is a tls.Config for an http.Server of the app's own, which
// offers h2 and http/1.1, and acme-tls/1 for Let's Encrypt's TLS-ALPN-01
// challenge, which Let's Encrypt tries first: tug's Run offers the last
// beside its own.
func (m *Manager) TLSConfig() *tls.Config {
	return m.m.TLSConfig()
}

// HTTPHandler answers Let's Encrypt's HTTP-01 challenges, for one of the
// domains, on port 80, and hands every other request to fallback, which
// tug's Run makes the redirect to HTTPS. With it, a certificate is got by
// HTTP-01 where TLS-ALPN-01 doesn't reach the app.
func (m *Manager) HTTPHandler(fallback http.Handler) http.Handler {
	return m.m.HTTPHandler(fallback)
}

// Run asks for each domain's certificate that isn't kept yet, as the app
// starts, rather than at the first visitor's handshake, so no one waits the
// seconds it takes, and says in the log how each went: a domain whose DNS
// doesn't reach the machine shows at once. tug's Run runs it once both
// ports listen, as Let's Encrypt's checks come to them. It returns once
// each has been asked for, or as ctx is done, and nil either way: a domain
// that failed is asked for again at its first handshake.
func (m *Manager) Run(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		// Not stopped by ctx, as autocert has no way to: an order under way
		// is left to finish as the app stops.
		defer close(done)
		var asking sync.WaitGroup
		for _, d := range m.domains {
			asking.Go(func() { m.ask(d) })
		}
		asking.Wait()
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
	return nil
}

// ask asks for the certificate for domain that a browser's handshake
// would. Autocert keeps a domain's ECDSA certificate apart from its RSA
// one, for a client with no ECDSA, and gives a hello with no ECDSA suite in
// it the RSA one, which would leave the browsers' to be asked for at their
// first handshake after all.
func (m *Manager) ask(domain string) {
	cert, err := m.m.GetCertificate(&tls.ClientHelloInfo{
		ServerName:       domain,
		CipherSuites:     []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256},
		SignatureSchemes: []tls.SignatureScheme{tls.ECDSAWithP256AndSHA256},
		SupportedCurves:  []tls.CurveID{tls.CurveP256},
	})
	if err != nil {
		slog.Error("couldn't get a certificate", "domain", domain, "err", err)
		return
	}
	if cert.Leaf != nil {
		slog.Info("a certificate is ready", "domain", domain, "expires", cert.Leaf.NotAfter)
	}
}
