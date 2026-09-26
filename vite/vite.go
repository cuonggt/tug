// Package vite puts a frontend built with Vite into pages that Go renders:
// the tags that load it, from the dev server while that runs and from the
// build's manifest otherwise, and a handler that serves the built files.
//
// The tags go in a root template through Funcs:
//
//	<head>
//	  {{ viteReactRefresh }}
//	  {{ vite "resources/js/app.tsx" }}
//	</head>
//
// See https://vite.dev/guide/backend-integration for the Vite side.
package vite

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"html/template"
	"io/fs"
	"net/http"
	"os"
	"slices"
	"strings"
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
// inputs ("resources/js/app.tsx"). While the dev server runs, they load
// each entry from it, after its client, which does the hot reloading.
// Otherwise they load the built files, with each entry's CSS and that of
// the chunks it imports, and preload those chunks.
func (v *Vite) Tags(entries ...string) (template.HTML, error) {
	if dev := v.DevServer(); dev != "" {
		var b strings.Builder
		fmt.Fprintf(&b, `<script type="module" src="%s"></script>`, html.EscapeString(dev+"/@vite/client"))
		for _, e := range entries {
			src := html.EscapeString(dev + "/" + e)
			if isCSS(e) {
				fmt.Fprintf(&b, `<link rel="stylesheet" href="%s">`, src)
			} else {
				fmt.Fprintf(&b, `<script type="module" src="%s"></script>`, src)
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
			fmt.Fprintf(&b, `<link rel="modulepreload" href="%s">`, html.EscapeString(v.base+f))
		}
	}
	for _, f := range scripts {
		fmt.Fprintf(&b, `<script type="module" src="%s"></script>`, html.EscapeString(v.base+f))
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
// components don't hot reload. For a build it's empty.
func (v *Vite) ReactRefresh() template.HTML {
	dev := v.DevServer()
	if dev == "" {
		return ""
	}
	return template.HTML(`<script type="module">
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
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	http.ServeFileFS(w, r, v.build, name)
}
