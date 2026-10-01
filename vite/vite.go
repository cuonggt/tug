// Package vite puts a frontend built with Vite into pages that Go renders:
// the tags that load it, from the dev server while that runs and from the
// build's manifest otherwise, and a handler that serves the built files.
//
// The tags go in a root template through Funcs, with the page's nonce,
// which its Content-Security-Policy lets its scripts run by, "" without
// one:
//
//	<head>
//	  {{ viteReactRefresh .Nonce }}
//	  {{ vite .Nonce "resources/js/app.tsx" }}
//	</head>
//
// See https://vite.dev/guide/backend-integration for the Vite side.
package vite

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"html/template"
	"io/fs"
	"math"
	"mime"
	"net/http"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Config says where a frontend's build and dev server are.
type Config struct {
	// Build is Vite's build output: the directory with .vite/manifest.json
	// and the files it names, usually an embed.FS through fs.Sub. Before
	// the first build, only the dev server can load the frontend.
	Build fs.FS

	// Base is the URL path the built files are served under, which must be
	// the base vite.config gives `vite build`. Default "/build/".
	Base string

	// HotFile is the file the dev server writes its URL to while it runs.
	// While it's there, pages load their scripts from the dev server, which
	// reloads them as they change, rather than from Build.
	HotFile string
}

// Vite loads a frontend into pages. See New.
type Vite struct {
	build    fs.FS
	base     string
	hotFile  string
	manifest map[string]chunk
	version  string

	// encoded is each file's compressed copies, by its name, made the
	// first time it's asked for.
	mu      sync.Mutex
	encoded map[string]*encoding
}

// chunk is an entry of Vite's manifest: a file of the build, and the files
// it needs.
type chunk struct {
	File    string   `json:"file"`
	CSS     []string `json:"css"`
	Imports []string `json:"imports"`
}

// New reads the build's manifest, when there is one.
func New(cfg Config) (*Vite, error) {
	v := &Vite{build: cfg.Build, base: cfg.Base, hotFile: cfg.HotFile}
	if v.base == "" {
		v.base = "/build/"
	}
	if !strings.HasPrefix(v.base, "/") || !strings.HasSuffix(v.base, "/") {
		return nil, fmt.Errorf("vite: Base %q must start and end with /", v.base)
	}
	if cfg.Build == nil {
		return v, nil
	}
	data, err := fs.ReadFile(cfg.Build, ".vite/manifest.json")
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return v, nil
	case err != nil:
		return nil, fmt.Errorf("vite: %w", err)
	}
	if err := json.Unmarshal(data, &v.manifest); err != nil {
		return nil, fmt.Errorf("vite: .vite/manifest.json: %w", err)
	}
	sum := sha256.Sum256(data)
	v.version = hex.EncodeToString(sum[:16])
	return v, nil
}

// Version names the build, as a hash of its manifest: "" before there is a
// build. It's what inertia.Config.Version wants.
func (v *Vite) Version() string { return v.version }

// DevServer returns the dev server's URL while it runs, and "" otherwise,
// for what else the dev server does, such as package ssr's rendering. It
// reads the hot file each time, so the dev server can start and stop while
// the Go server runs.
func (v *Vite) DevServer() string {
	if v.hotFile == "" {
		return ""
	}
	data, err := os.ReadFile(v.hotFile)
	if err != nil {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(string(data)), "/")
}

// Tags returns the tags that load entries, named as vite.config names its
// inputs ("resources/js/app.tsx"), whose scripts, and the chunks they
// preload, carry nonce, the page's: "" without one. While the dev server
// runs, they load each entry from it, after its client, which does the hot
// reloading. Otherwise they load the built files, with each entry's CSS and
// that of the chunks it imports, and preload those chunks.
func (v *Vite) Tags(nonce string, entries ...string) (template.HTML, error) {
	if !isNonce(nonce) {
		return "", fmt.Errorf(`vite: %q isn't a nonce: the tags take the page's first, as {{ vite .Nonce "resources/js/app.tsx" }}`, nonce)
	}
	n := nonceAttr(nonce)
	if dev := v.DevServer(); dev != "" {
		var b strings.Builder
		fmt.Fprintf(&b, `<script type="module" src="%s"%s></script>`, html.EscapeString(dev+"/@vite/client"), n)
		for _, e := range entries {
			src := html.EscapeString(dev + "/" + e)
			if isCSS(e) {
				fmt.Fprintf(&b, `<link rel="stylesheet" href="%s">`, src)
			} else {
				fmt.Fprintf(&b, `<script type="module" src="%s"%s></script>`, src, n)
			}
		}
		return template.HTML(b.String()), nil
	}

	if v.manifest == nil {
		return "", errors.New("vite: there's no build to load; run `npm run build`, or `npm run dev` for the dev server")
	}
	var styles, preloads, scripts []string
	seen := map[string]bool{}
	for _, e := range entries {
		c, ok := v.manifest[e]
		if !ok {
			return "", fmt.Errorf("vite: %s isn't in the build's manifest; is it an input in vite.config, or a page that doesn't exist?", e)
		}
		if isCSS(c.File) {
			styles = appendNew(styles, c.File)
			continue
		}
		styles = appendNew(styles, c.CSS...)
		v.walkImports(c.Imports, seen, &styles, &preloads)
		scripts = appendNew(scripts, c.File)
	}

	var b strings.Builder
	for _, f := range styles {
		fmt.Fprintf(&b, `<link rel="stylesheet" href="%s">`, html.EscapeString(v.base+f))
	}
	for _, f := range preloads {
		if !slices.Contains(scripts, f) {
			fmt.Fprintf(&b, `<link rel="modulepreload" href="%s"%s>`, html.EscapeString(v.base+f), n)
		}
	}
	for _, f := range scripts {
		fmt.Fprintf(&b, `<script type="module" src="%s"%s></script>`, html.EscapeString(v.base+f), n)
	}
	return template.HTML(b.String()), nil
}

// walkImports collects the CSS and the files of the chunks imports names,
// and of the chunks those import, each once.
func (v *Vite) walkImports(imports []string, seen map[string]bool, styles, preloads *[]string) {
	for _, name := range imports {
		if seen[name] {
			continue
		}
		seen[name] = true
		c := v.manifest[name]
		*styles = appendNew(*styles, c.CSS...)
		*preloads = appendNew(*preloads, c.File)
		v.walkImports(c.Imports, seen, styles, preloads)
	}
}

func appendNew(list []string, items ...string) []string {
	for _, item := range items {
		if item != "" && !slices.Contains(list, item) {
			list = append(list, item)
		}
	}
	return list
}

// isNonce reports whether nonce is written as a Content-Security-Policy
// nonce is, in base64, or is "", for none: an entry, as a template that
// hasn't passed the nonce first names, has a dot.
func isNonce(nonce string) bool {
	return strings.Trim(nonce, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/-_=") == ""
}

// nonceAttr is the nonce attribute of a script that carries nonce, or
// nothing for no nonce.
func nonceAttr(nonce string) string {
	if nonce == "" {
		return ""
	}
	return ` nonce="` + html.EscapeString(nonce) + `"`
}

func isCSS(path string) bool {
	for _, ext := range []string{".css", ".scss", ".sass", ".less", ".styl", ".stylus", ".pcss", ".postcss"} {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

// ReactRefresh returns the preamble that @vitejs/plugin-react needs before
// the first script while the dev server runs, without which React
// components don't hot reload, carrying nonce, the page's: "" without one.
// For a build it's empty.
func (v *Vite) ReactRefresh(nonce string) template.HTML {
	dev := v.DevServer()
	if dev == "" {
		return ""
	}
	return template.HTML(`<script type="module"` + nonceAttr(nonce) + `>
import RefreshRuntime from '` + template.JSEscapeString(dev) + `/@react-refresh'
RefreshRuntime.injectIntoGlobalHook(window)
window.$RefreshReg$ = () => {}
window.$RefreshSig$ = () => (type) => type
window.__vite_plugin_react_preamble_installed__ = true
</script>`)
}

// Funcs are template functions for a root template: vite, which is Tags,
// and viteReactRefresh, which is ReactRefresh.
func (v *Vite) Funcs() template.FuncMap {
	return template.FuncMap{
		"vite":             v.Tags,
		"viteReactRefresh": v.ReactRefresh,
	}
}

// ServeHTTP serves the built files under Base. The files Vite writes to
// assets/ are named for a hash of their content, so they're cached for a
// year; the manifest, and anything else starting with a dot, isn't served.
//
// A file of a type that compresses, as JavaScript and CSS do, goes gzipped
// to a browser whose Accept-Encoding takes it, compressed the first time
// it's asked for, and kept: the app is one binary, with no web server in
// front of it to compress what it sends. Its own compressed copies beside
// it in the build, name.br and name.gz, as a compression plugin of Vite's
// writes them, go first, brotli before gzip. A range is of the bytes sent.
func (v *Vite) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name, ok := strings.CutPrefix(r.URL.Path, v.base)
	if !ok || v.build == nil || !fs.ValidPath(name) || name == "." || strings.HasPrefix(name, ".") || strings.Contains(name, "/.") {
		http.NotFound(w, r)
		return
	}
	if fi, err := fs.Stat(v.build, name); err != nil || fi.IsDir() {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	if strings.HasPrefix(name, "assets/") {
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	if e := v.encodings(name); e.varies() {
		// A cache between keeps the copies apart, the one sent as it is
		// among them.
		h.Add("Vary", "Accept-Encoding")
		if coding, body := e.pick(r.Header.Get("Accept-Encoding")); coding != "" {
			h.Set("Content-Encoding", coding)
			h.Set("Content-Type", e.contentType)
			// ServeContent leaves the length out under a Content-Encoding,
			// for a writer that compresses as it goes: these bytes are
			// compressed already, and a range sets its own.
			h.Set("Content-Length", strconv.Itoa(len(body)))
			http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(body))
			return
		}
	}
	http.ServeFileFS(w, r, v.build, name)
}

// encodings returns the compressed copies of the build's file name, made
// the first time it's asked for, once however many ask at once; nil for a
// file of a type that doesn't compress.
func (v *Vite) encodings(name string) *encoding {
	if !compressible(name) {
		return nil
	}
	v.mu.Lock()
	e, ok := v.encoded[name]
	if !ok {
		if v.encoded == nil {
			v.encoded = make(map[string]*encoding)
		}
		e = new(encoding)
		v.encoded[name] = e
	}
	v.mu.Unlock()
	e.once.Do(func() { e.make(v.build, name) })
	return e
}

// compressible reports whether a file of name's type compresses: text, and
// WebAssembly and fonts not compressed already. An image, a WOFF font,
// audio and video are compressed by their formats.
func compressible(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".js", ".mjs", ".cjs", ".css", ".svg", ".json", ".map", ".html", ".htm", ".txt", ".wasm", ".ttf", ".otf":
		return true
	}
	return false
}

// encoding is a file's compressed copies: the build's own, beside it, and
// else tug's gzip, when that's smaller than the file.
type encoding struct {
	once        sync.Once
	contentType string // the file's, as it is
	br, gzip    []byte
}

func (e *encoding) make(build fs.FS, name string) {
	data, err := fs.ReadFile(build, name)
	if err != nil {
		return // sent as it is
	}
	// The type a response of the file as it is has, by its extension, or
	// else its first bytes, as http.ServeFileFS finds it.
	e.contentType = mime.TypeByExtension(path.Ext(name))
	if e.contentType == "" {
		e.contentType = http.DetectContentType(data)
	}
	if br, err := fs.ReadFile(build, name+".br"); err == nil {
		e.br = br
	}
	if gz, err := fs.ReadFile(build, name+".gz"); err == nil {
		e.gzip = gz
	} else if gz := gzipped(data); len(gz) < len(data) {
		e.gzip = gz
	}
}

// varies reports whether the file goes compressed to some browsers.
func (e *encoding) varies() bool {
	return e != nil && (e.br != nil || e.gzip != nil)
}

// pick returns the coding to send, of those the request's Accept-Encoding
// takes, brotli before gzip, and its bytes; "" to send the file as it is,
// which a browser gets even when it refuses that too.
func (e *encoding) pick(accept string) (string, []byte) {
	switch {
	case e.br != nil && takes(accept, "br"):
		return "br", e.br
	case e.gzip != nil && takes(accept, "gzip"):
		return "gzip", e.gzip
	}
	return "", nil
}

// gzipped is data compressed by gzip, at its best, as it's done once.
func gzipped(data []byte) []byte {
	var b bytes.Buffer
	w, _ := gzip.NewWriterLevel(&b, gzip.BestCompression)
	w.Write(data)
	w.Close()
	return b.Bytes()
}

// takes reports whether an Accept-Encoding header takes coding, by its
// name, x-gzip being gzip's, or else by *, with a weight above 0.
func takes(accept, coding string) bool {
	named, star := -1.0, -1.0
	for part := range strings.SplitSeq(accept, ",") {
		name, params, _ := strings.Cut(part, ";")
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "x-gzip" {
			name = "gzip"
		}
		switch name {
		case coding:
			named = weight(params)
		case "*":
			star = weight(params)
		}
	}
	if named >= 0 {
		return named > 0
	}
	return star > 0
}

// weight is the q of a coding's parameters: 1 without one, and 0 for one
// that isn't a number, NaN among them, as ParseFloat reads it.
func weight(params string) float64 {
	for p := range strings.SplitSeq(params, ";") {
		key, value, _ := strings.Cut(p, "=")
		if strings.EqualFold(strings.TrimSpace(key), "q") {
			q, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			if err != nil || q < 0 || math.IsNaN(q) {
				return 0
			}
			return q
		}
	}
	return 1
}
