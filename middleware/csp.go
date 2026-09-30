package middleware

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/json"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/cuonggt/tug/internal/nonce"
)

// CSPConfig is the Content-Security-Policy CSP sends.
type CSPConfig struct {
	// ReportOnly sends the policy as Content-Security-Policy-Report-Only:
	// the browser blocks nothing, and reports what the policy would have
	// blocked, for an app to see before it enforces the policy, as one
	// whose pages have scripts that don't carry the nonce yet.
	ReportOnly bool

	// ReportPath is where the browser reports what the policy blocks, or
	// would have, as "/csp-reports": CSP answers the reports itself, before
	// any route, as CORS answers a preflight, and logs each one. Without
	// it, the browser tells its own console alone.
	ReportPath string

	// Sources adds to a directive's sources, as a bucket's origin to
	// img-src, for the photos kept there that the pages show:
	// {"img-src": {"https://photos.s3.us-east-1.amazonaws.com"}}. A
	// directive the policy hasn't is added, and a blank source skipped.
	Sources map[string][]string

	// DevServer is vite.Vite's DevServer, which says the dev server's URL
	// while it runs: the policy lets in its styles, images and fonts then,
	// and its connection for hot reloading. Its scripts load as the app's
	// own do, by the nonce.
	DevServer func() string
}

// A directive of a policy: its name, and the sources it allows.
type directive struct {
	name    string
	sources []string
}

// policy is what the pages may run and load, before a CSPConfig adds to
// it, with the nonce added to script-src for each response.
var policy = []directive{
	{"default-src", []string{"'self'"}},
	// A script with the nonce runs, and what it loads, as Vite's entry
	// loads the modules of the app, which can't carry the nonce.
	{"script-src", []string{"'strict-dynamic'"}},
	// shadcn's components and Inertia's progress bar set styles inline,
	// and a style that gets into a page does little a script can't.
	{"style-src", []string{"'self'", "'unsafe-inline'"}},
	// A favicon written into the page, and a photo shown before it's
	// uploaded, from the file chosen.
	{"img-src", []string{"'self'", "data:", "blob:"}},
	{"font-src", []string{"'self'", "data:"}},
	{"connect-src", []string{"'self'"}},
	{"object-src", []string{"'none'"}},
	// A <base> that got into a page would send the app's own scripts, which
	// carry the nonce, to another site for their code.
	{"base-uri", []string{"'none'"}},
	{"form-action", []string{"'self'"}},
	{"frame-ancestors", []string{"'self'"}},
}

// maxReport is the most of a report CSP reads.
const maxReport = 64 << 10

// CSP sets a Content-Security-Policy on every response, which says what
// its page may run and load, so a script that got into it by some way but
// the app's own isn't run: one in a person's text that an escape missed,
// say.
//
// A script runs when it carries the response's nonce, a new random one
// each response, which NonceFrom reads, and package inertia hands the root
// template as .Nonce, for Vite's tags and the template's own scripts to
// carry, as <script nonce="{{ .Nonce }}">; or when a script that carries
// it loads it ('strict-dynamic'), as Vite's entry loads the app's modules.
// Styles may be inline too, and images and fonts data: URLs. Otherwise
// what a page loads and connects to is the app's own; a form goes to the
// app alone; nothing may be embedded as a plugin, or be a <base> for the
// page's links; and only the app's own pages may put one in a frame.
//
// CSP panics on a directive or a source it can't put in the header, and on
// a ReportPath that isn't a path: a mistyped setting should stop the app
// at startup.
func CSP(cfg CSPConfig) func(http.Handler) http.Handler {
	directives := slices.Clone(policy)
	// In order, so the header comes out the same whatever order the map has.
	for _, name := range slices.Sorted(maps.Keys(cfg.Sources)) {
		if !directiveName(name) {
			panic(`middleware: CSP: "` + name + `" isn't a directive, as img-src`)
		}
		i := slices.IndexFunc(directives, func(d directive) bool { return d.name == name })
		if i < 0 {
			directives = append(directives, directive{name: name})
			i = len(directives) - 1
		}
		d := &directives[i]
		d.sources = slices.Clone(d.sources)
		for _, s := range cfg.Sources[name] {
			switch s = strings.TrimSpace(s); {
			case s == "":
			case strings.ContainsAny(s, " \t\r\n;,"):
				panic(`middleware: CSP: "` + s + `" isn't a source, as https://photos.example.com or 'self'`)
			case !slices.Contains(d.sources, s):
				d.sources = append(d.sources, s)
			}
		}
	}
	if p := cfg.ReportPath; p != "" && (!strings.HasPrefix(p, "/") || strings.ContainsAny(p, " \t\r\n;,")) {
		panic(`middleware: CSP: "` + p + `" isn't a path for the reports, as /csp-reports`)
	}
	header := "Content-Security-Policy"
	if cfg.ReportOnly {
		header = "Content-Security-Policy-Report-Only"
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.ReportPath != "" && r.URL.Path == cfg.ReportPath && r.Method == http.MethodPost {
				logReport(w, r, cfg.ReportOnly)
				return
			}
			n := rand.Text()
			var dev string
			if cfg.DevServer != nil {
				dev = cfg.DevServer()
			}
			w.Header().Set(header, policyHeader(directives, n, devSources(dev), cfg.ReportPath))
			next.ServeHTTP(w, r.WithContext(nonce.With(r.Context(), n)))
		})
	}
}

// NonceFrom returns the nonce CSP made for the response to the request
// whose context ctx is, for the response's own scripts to carry, as
// <script nonce="...">: "" without CSP.
func NonceFrom(ctx context.Context) string {
	return nonce.From(ctx)
}

// policyHeader is the policy, the directives with n in script-src, and
// with dev's sources, the dev server's, in the directives they're for.
func policyHeader(directives []directive, n string, dev map[string][]string, reportPath string) string {
	var b strings.Builder
	for i, d := range directives {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(d.name)
		if d.name == "script-src" {
			b.WriteString(" 'nonce-" + n + "'")
		}
		for _, s := range d.sources {
			b.WriteString(" " + s)
		}
		for _, s := range dev[d.name] {
			b.WriteString(" " + s)
		}
	}
	if reportPath != "" {
		b.WriteString("; report-uri " + reportPath)
	}
	return b.String()
}

// devSources are what the dev server at dev, a URL, serves beyond scripts:
// styles, images, fonts, and the connection it hot reloads the pages by,
// a WebSocket.
func devSources(dev string) map[string][]string {
	u, err := url.Parse(dev)
	if dev == "" || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil
	}
	origin, ws := u.Scheme+"://"+u.Host, "ws://"+u.Host
	if u.Scheme == "https" {
		ws = "wss://" + u.Host
	}
	return map[string][]string{
		"style-src":   {origin},
		"img-src":     {origin},
		"font-src":    {origin},
		"connect-src": {origin, ws},
	}
}

// logReport logs what the browser reports a page's policy blocked, or
// would have, as report-uri has a browser send it, and answers 204.
func logReport(w http.ResponseWriter, r *http.Request, reportOnly bool) {
	var sent struct {
		Report *struct {
			Page      string `json:"document-uri"`
			Directive string `json:"effective-directive"`
			Violated  string `json:"violated-directive"`
			Blocked   string `json:"blocked-uri"`
			Source    string `json:"source-file"`
			Line      int    `json:"line-number"`
			Sample    string `json:"script-sample"`
		} `json:"csp-report"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxReport)).Decode(&sent); err != nil || sent.Report == nil {
		http.Error(w, "this is where a browser reports what a page's Content-Security-Policy blocked", http.StatusBadRequest)
		return
	}
	report := sent.Report
	source := report.Source
	if source != "" && report.Line > 0 {
		source += ":" + strconv.Itoa(report.Line)
	}
	msg := "a page's Content-Security-Policy blocked what it asked for"
	if reportOnly {
		msg = "a page's Content-Security-Policy would have blocked what it asked for"
	}
	attrs := []any{"page", report.Page, "directive", cmp.Or(report.Directive, report.Violated), "blocked", report.Blocked}
	// What a browser doesn't say, as where an inline handler was, is left
	// out, rather than logged empty.
	if source != "" {
		attrs = append(attrs, "source", source)
	}
	if report.Sample != "" {
		attrs = append(attrs, "sample", report.Sample)
	}
	slog.WarnContext(r.Context(), msg, attrs...)
	w.WriteHeader(http.StatusNoContent)
}

// directiveName reports whether name is written as a directive's is:
// lower-case letters, with hyphens between.
func directiveName(name string) bool {
	return name != "" && strings.Trim(name, "abcdefghijklmnopqrstuvwxyz-") == "" &&
		!strings.HasPrefix(name, "-") && !strings.HasSuffix(name, "-")
}
